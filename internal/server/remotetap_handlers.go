package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/export"
	"github.com/johndemoss/o11y-workbench/internal/remotetap"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	DefaultMaxSpans = 10000
	DefaultMaxBytes = 10 * 1024 * 1024 // 10MB
)

// sampler implements sampling strategies for remotetap data
type sampler struct {
	mu           sync.Mutex
	samplingType string  // "head", "tail", "random", "none"
	samplingRate float64 // For head: count limit; tail: every Nth; random: probability
	count        int     // For head sampling: how many kept so far
	offset       int     // For tail sampling: current position in sequence
}

// newSampler creates a new sampler
func newSampler(samplingType string, samplingRate float64) *sampler {
	return &sampler{
		samplingType: samplingType,
		samplingRate: samplingRate,
		count:        0,
		offset:       0,
	}
}

// ShouldSample returns true if the item should be kept based on sampling strategy
func (s *sampler) ShouldSample() bool {
	switch s.samplingType {
	case "none":
		return true
	case "head":
		// Keep first N items (samplingRate = count limit)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.count < int(s.samplingRate) {
			s.count++
			return true
		}
		return false
	case "tail":
		// Keep every Nth item (samplingRate = interval)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.offset++
		if s.offset%int(s.samplingRate) == 0 {
			return true
		}
		return false
	case "random":
		// Keep with probability = samplingRate (0.0-1.0)
		return rand.Float64() < s.samplingRate
	default:
		return true
	}
}

// ringBuffer retains complete single-record resource envelopes under shared limits.
type ringBuffer struct {
	mu           sync.Mutex
	items        []proto.Message
	maxSpans     int
	maxBytes     int
	spanCount    int
	currentBytes int
	evictedCount int
}

func newRingBuffer(maxSpans, maxBytes int) *ringBuffer {
	return &ringBuffer{maxSpans: maxSpans, maxBytes: maxBytes}
}

func (rb *ringBuffer) add(item proto.Message) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	size := proto.Size(item) + 64
	if size > rb.maxBytes {
		rb.evictedCount++
		return false
	}
	_, isSpan := item.(*tracev1.ResourceSpans)
	for (isSpan && rb.spanCount >= rb.maxSpans) || rb.currentBytes+size > rb.maxBytes {
		if len(rb.items) == 0 {
			break
		}
		rb.currentBytes -= proto.Size(rb.items[0]) + 64
		if _, ok := rb.items[0].(*tracev1.ResourceSpans); ok {
			rb.spanCount--
		}
		rb.items[0] = nil
		rb.items = rb.items[1:]
		rb.evictedCount++
	}
	rb.items = append(rb.items, item)
	if isSpan {
		rb.spanCount++
	}
	rb.currentBytes += size
	return true
}

func (rb *ringBuffer) Add(item *tracev1.ResourceSpans) bool {
	return rb.add(item)
}

func (rb *ringBuffer) GetAll() []*tracev1.ResourceSpans {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	var result []*tracev1.ResourceSpans
	for _, item := range rb.items {
		if traces, ok := item.(*tracev1.ResourceSpans); ok {
			result = append(result, traces)
		}
	}
	return result
}

func (rb *ringBuffer) payloads() ([]*tracev1.ResourceSpans, []*logsv1.ResourceLogs, []*metricsv1.ResourceMetrics) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	var traces []*tracev1.ResourceSpans
	var logs []*logsv1.ResourceLogs
	var metrics []*metricsv1.ResourceMetrics
	for _, item := range rb.items {
		switch item := item.(type) {
		case *tracev1.ResourceSpans:
			traces = append(traces, item)
		case *logsv1.ResourceLogs:
			logs = append(logs, item)
		case *metricsv1.ResourceMetrics:
			metrics = append(metrics, item)
		}
	}
	return traces, logs, metrics
}

func (rb *ringBuffer) EvictedCount() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.evictedCount
}

func (rb *ringBuffer) CurrentBytes() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.currentBytes
}

type RemotetapFilters struct {
	// Attribute filters
	Services       []string          // Empty = all
	SpanKinds      []string          // Empty = all (SERVER, CLIENT, PRODUCER, CONSUMER, INTERNAL)
	ResourceAttrs  map[string]string // Empty = all (key=value pairs)
	MinLogSeverity string            // Empty = all (DEBUG, INFO, WARN, ERROR)

	// Sampling
	SamplingType string  // "head", "tail", "random", "none"
	SamplingRate float64 // For head: count limit; tail: every Nth; random: 0.0-1.0

	// Limits
	MaxSpans int
	MaxBytes int
}

type RemotetapManager struct {
	mu         sync.Mutex
	closed     bool
	ctx        context.Context
	cancel     context.CancelFunc
	streamDone chan struct{}
	endpoint   string
	client     *remotetap.Client
	run        *Run
	store      *RunStore
	filters    RemotetapFilters
	buffer     *ringBuffer
	sampler    *sampler
}

func NewRemotetapManager(store *RunStore) *RemotetapManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &RemotetapManager{
		store:  store,
		ctx:    ctx,
		cancel: cancel,
		filters: RemotetapFilters{
			MaxSpans:     10000,
			MaxBytes:     10 * 1024 * 1024,
			SamplingType: "none",
			SamplingRate: 0,
		},
		sampler: newSampler("none", 0),
	}
}

func (m *RemotetapManager) Connect(ctx context.Context, endpoint string, filters RemotetapFilters) (*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, fmt.Errorf("remotetap manager is closed")
	}
	if m.client != nil {
		return nil, fmt.Errorf("already connected to %s", m.endpoint)
	}
	if filters.MaxSpans == 0 {
		filters.MaxSpans = DefaultMaxSpans
	}
	if filters.MaxBytes == 0 {
		filters.MaxBytes = DefaultMaxBytes
	}
	if filters.SamplingType == "" {
		filters.SamplingType = "none"
	}
	if err := validateTAPFilters(filters); err != nil {
		return nil, err
	}

	// The request controls admission/dial only; the manager owns the live stream.
	dialCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(m.ctx, cancel)
	defer stop()
	client := remotetap.NewClient(endpoint)
	if err := client.Connect(dialCtx); err != nil {
		return nil, fmt.Errorf("remotetap connect: %w", err)
	}

	id := fmt.Sprintf("run-%d", runIDCounter.Add(1))
	run := &Run{
		ID:                id,
		Status:            RunStatusStreaming,
		StartedAt:         time.Now(),
		Source:            "remotetap",
		CaptureLimitBytes: int64(filters.MaxBytes),
	}
	m.store.Add(run)

	m.endpoint = endpoint
	m.client = client
	m.run = run
	m.filters = filters
	m.buffer = newRingBuffer(m.filters.MaxSpans, m.filters.MaxBytes)
	m.sampler = newSampler(filters.SamplingType, filters.SamplingRate)

	done := make(chan struct{})
	m.streamDone = done
	response := run.snapshot()
	go m.streamLoop(client, done)

	return response, nil
}

func (m *RemotetapManager) streamLoop(client *remotetap.Client, done chan struct{}) {
	defer close(done)
	defer m.onDisconnect(client)
	defer client.Disconnect()
	errs := client.Err()
	for {
		select {
		case data, ok := <-client.Data():
			if !ok {
				return
			}
			m.processData(data)
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			run := m.getRun()
			if run != nil {
				run.mu.Lock()
				run.Error = err.Error()
				run.mu.Unlock()
			}
		}
	}
}

func (m *RemotetapManager) processData(data *remotetap.RemotetapData) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run := m.run
	if run == nil {
		return
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	for _, rs := range data.Traces {
		if rs == nil || !m.keepResource(rs.Resource) {
			continue
		}
		for _, scope := range rs.ScopeSpans {
			if scope == nil {
				continue
			}
			for _, span := range scope.Spans {
				if span == nil || !m.keepSpanKind(span.Kind) || !m.sampler.ShouldSample() {
					continue
				}
				m.buffer.add(&tracev1.ResourceSpans{Resource: rs.Resource, SchemaUrl: rs.SchemaUrl,
					ScopeSpans: []*tracev1.ScopeSpans{{Scope: scope.Scope, SchemaUrl: scope.SchemaUrl, Spans: []*tracev1.Span{span}}}})
				run.SpanCount++
			}
		}
	}
	for _, rl := range data.Logs {
		if rl == nil || !m.keepResource(rl.Resource) {
			continue
		}
		for _, scope := range rl.ScopeLogs {
			if scope == nil {
				continue
			}
			for _, record := range scope.LogRecords {
				if record == nil || !m.shouldKeepLog(record) || !m.sampler.ShouldSample() {
					continue
				}
				m.buffer.add(&logsv1.ResourceLogs{Resource: rl.Resource, SchemaUrl: rl.SchemaUrl,
					ScopeLogs: []*logsv1.ScopeLogs{{Scope: scope.Scope, SchemaUrl: scope.SchemaUrl, LogRecords: []*logsv1.LogRecord{record}}}})
				run.LogCount++
			}
		}
	}
	for _, rm := range data.Metrics {
		if rm == nil || !m.keepResource(rm.Resource) {
			continue
		}
		for _, scope := range rm.ScopeMetrics {
			if scope == nil {
				continue
			}
			for _, metric := range scope.Metrics {
				if metric == nil || !m.sampler.ShouldSample() {
					continue
				}
				m.buffer.add(&metricsv1.ResourceMetrics{Resource: rm.Resource, SchemaUrl: rm.SchemaUrl,
					ScopeMetrics: []*metricsv1.ScopeMetrics{{Scope: scope.Scope, SchemaUrl: scope.SchemaUrl, Metrics: []*metricsv1.Metric{metric}}}})
				run.MetricCount++
			}
		}
	}
	traces, _, _ := m.buffer.payloads()
	traceIDs := make(map[string]struct{}, len(traces))
	for _, rs := range traces {
		for _, scope := range rs.ScopeSpans {
			for _, span := range scope.Spans {
				traceIDs[string(span.TraceId)] = struct{}{}
			}
		}
	}
	run.TraceCount = len(traceIDs)
}

func (m *RemotetapManager) Disconnect() error {
	m.mu.Lock()
	client, done := m.client, m.streamDone
	m.mu.Unlock()
	return disconnectTAP(client, done)
}

// Close also rejects reconnects and interrupts any in-progress dial.
func (m *RemotetapManager) Close() error {
	m.cancel()
	m.mu.Lock()
	m.closed = true
	client, done := m.client, m.streamDone
	m.mu.Unlock()
	return disconnectTAP(client, done)
}

func disconnectTAP(client *remotetap.Client, done <-chan struct{}) error {
	if client != nil {
		_ = client.Disconnect()
	}
	if done != nil {
		<-done
	}
	return nil
}

func (m *RemotetapManager) onDisconnect(client *remotetap.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client != client {
		return
	}
	if m.run != nil {
		m.run.mu.Lock()
		if m.buffer != nil {
			traces, logs, metrics := m.buffer.payloads()
			m.run.CaptureDropped = int64(m.buffer.EvictedCount())
			m.run.CaptureTruncated = m.run.CaptureDropped > 0
			marshal := func(message proto.Message) *json.RawMessage {
				data, err := protojson.Marshal(message)
				if err == nil {
					data, err = export.FixTraceSpanIDsForMarshal(data)
				}
				if err != nil {
					m.run.Error = "serialize TAP capture: " + err.Error()
					return nil
				}
				raw := json.RawMessage(data)
				return &raw
			}
			if len(traces) > 0 {
				m.run.TraceData = marshal(&collectortracev1.ExportTraceServiceRequest{ResourceSpans: traces})
			}
			if len(logs) > 0 {
				m.run.LogData = marshal(&collectorlogsv1.ExportLogsServiceRequest{ResourceLogs: logs})
			}
			if len(metrics) > 0 {
				m.run.MetricData = marshal(&collectormetricsv1.ExportMetricsServiceRequest{ResourceMetrics: metrics})
			}
		}
		now := time.Now()
		m.run.Status = RunStatusCompleted
		m.run.CompletedAt = &now
		m.run.mu.Unlock()
		if m.store != nil {
			m.store.PruneCompleted(maxCompletedRuns)
		}
	}
	m.client = nil
	m.run = nil
	m.endpoint = ""
}

// StopByRunID stops the remotetap connection for the given runID.
// Returns nil if the run was found and stopped, error otherwise.
func (m *RemotetapManager) StopByRunID(runID string) error {
	m.mu.Lock()
	if m.run == nil || m.run.ID != runID {
		m.mu.Unlock()
		return fmt.Errorf("run not found: %s", runID)
	}
	client, done := m.client, m.streamDone
	m.mu.Unlock()
	return disconnectTAP(client, done)
}

func (m *RemotetapManager) Status() (connected bool, runID string, spanCount, logCount, metricCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client == nil {
		return false, "", 0, 0, 0
	}

	if m.run != nil {
		m.run.mu.Lock()
		defer m.run.mu.Unlock()
		runID = m.run.ID
		spanCount = m.run.SpanCount
		logCount = m.run.LogCount
		metricCount = m.run.MetricCount
	}
	return true, runID, spanCount, logCount, metricCount
}

// GetUsageStats returns current buffer usage statistics.
func (m *RemotetapManager) GetUsageStats() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.usageStatsLocked()
}

func (m *RemotetapManager) usageStatsForRun(runID string) (map[string]interface{}, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.run == nil || m.run.ID != runID || m.client == nil {
		return nil, false
	}
	return m.usageStatsLocked(), true
}

func (m *RemotetapManager) usageStatsLocked() map[string]interface{} {

	spanCount := int64(0)
	traceCount := int64(0)
	var currentBytes int64 = 0
	var evictedCount int64 = 0

	if m.run != nil {
		m.run.mu.Lock()
		spanCount = int64(m.run.SpanCount)
		traceCount = int64(m.run.TraceCount)
		m.run.mu.Unlock()
	}
	if m.buffer != nil {
		currentBytes = int64(m.buffer.CurrentBytes())
		evictedCount = int64(m.buffer.EvictedCount())
	}

	return map[string]interface{}{
		"spanCount":    spanCount,
		"traceCount":   traceCount,
		"currentBytes": currentBytes,
		"evictedCount": evictedCount,
		"maxSpans":     m.filters.MaxSpans,
		"maxBytes":     m.filters.MaxBytes,
	}
}

func (m *RemotetapManager) getRun() *Run {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.run
}

// shouldKeepTrace returns true if the trace passes attribute filters.
// An empty filter means "accept all".
func (m *RemotetapManager) shouldKeepTrace(rs *tracev1.ResourceSpans) bool {
	if rs == nil || !m.keepResource(rs.Resource) {
		return false
	}
	if len(m.filters.SpanKinds) == 0 {
		return true
	}
	for _, scope := range rs.ScopeSpans {
		if scope != nil {
			for _, span := range scope.Spans {
				if span != nil && m.keepSpanKind(span.Kind) {
					return true
				}
			}
		}
	}
	return false
}

func (m *RemotetapManager) keepSpanKind(kind tracev1.Span_SpanKind) bool {
	if len(m.filters.SpanKinds) == 0 {
		return true
	}
	name := strings.TrimPrefix(kind.String(), "SPAN_KIND_")
	for _, allowed := range m.filters.SpanKinds {
		if name == strings.TrimPrefix(strings.ToUpper(allowed), "SPAN_KIND_") {
			return true
		}
	}
	return false
}

func (m *RemotetapManager) keepResource(resource *resourcev1.Resource) bool {
	get := func(key string) string {
		return getResourceAttr(&tracev1.ResourceSpans{Resource: resource}, key)
	}
	if len(m.filters.Services) > 0 {
		found := false
		for _, service := range m.filters.Services {
			if get("service.name") == service {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for key, value := range m.filters.ResourceAttrs {
		if get(key) != value {
			return false
		}
	}
	return true
}

// shouldKeepLog returns true if the log passes attribute filters.
// An empty filter means "accept all".
func (m *RemotetapManager) shouldKeepLog(lr *logsv1.LogRecord) bool {
	// Check min log severity filter
	if m.filters.MinLogSeverity != "" {
		threshold := severityFromString(m.filters.MinLogSeverity)
		if lr.SeverityNumber < threshold {
			return false
		}
	}
	return true
}

// getResourceAttr retrieves an attribute value from a ResourceSpans by key.
func getResourceAttr(rs *tracev1.ResourceSpans, key string) string {
	if rs == nil || rs.Resource == nil {
		return ""
	}
	for _, attr := range rs.Resource.Attributes {
		if attr.Key == key {
			if attr.Value != nil {
				if strVal, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue); ok {
					return strVal.StringValue
				}
			}
			break
		}
	}
	return ""
}

// severityFromString converts a severity string to its numeric value.
// DEBUG=4, INFO=6, WARN=10, ERROR=14
func severityFromString(s string) logsv1.SeverityNumber {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return logsv1.SeverityNumber_SEVERITY_NUMBER_DEBUG
	case "INFO":
		return logsv1.SeverityNumber_SEVERITY_NUMBER_INFO
	case "WARN":
		return logsv1.SeverityNumber_SEVERITY_NUMBER_WARN
	case "ERROR":
		return logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR
	default:
		return 0
	}
}

func validateTAPFilters(filters RemotetapFilters) error {
	if filters.MaxSpans <= 0 || filters.MaxBytes <= 0 {
		return fmt.Errorf("maxSpans and maxBytes must be positive")
	}
	rate := filters.SamplingRate
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 {
		return fmt.Errorf("invalid samplingRate")
	}
	switch filters.SamplingType {
	case "", "none":
	case "head", "tail":
		if rate < 1 || math.Trunc(rate) != rate || rate > float64(math.MaxInt32) {
			return fmt.Errorf("%s samplingRate must be a positive integer", filters.SamplingType)
		}
	case "random":
		if rate > 1 {
			return fmt.Errorf("random samplingRate must be between 0 and 1")
		}
	default:
		return fmt.Errorf("samplingType must be one of: head, tail, random, none")
	}
	if filters.MinLogSeverity != "" && severityFromString(filters.MinLogSeverity) == 0 {
		return fmt.Errorf("invalid minLogSeverity")
	}
	return nil
}

func (s *Server) handleRemotetapConnect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string           `json:"endpoint"`
		Filters  RemotetapFilters `json:"filters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if body.Endpoint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint is required"})
		return
	}

	if err := validateTAPFilters(body.Filters); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	run, err := s.remotetap.Connect(r.Context(), body.Endpoint, body.Filters)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"run_id": run.ID,
		"status": "streaming",
	})
}

func (s *Server) handleRemotetapDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := s.remotetap.Disconnect(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

func (s *Server) handleRemotetapStatus(w http.ResponseWriter, r *http.Request) {
	connected, runID, spanCount, logCount, metricCount := s.remotetap.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"connected":    connected,
		"run_id":       runID,
		"span_count":   spanCount,
		"log_count":    logCount,
		"metric_count": metricCount,
	})
}

func (s *Server) handleRemotetapStop(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runID is required"})
		return
	}

	if err := s.remotetap.StopByRunID(runID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) handleRemotetapUsage(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runID is required"})
		return
	}
	stats, ok := s.remotetap.usageStatsForRun(runID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "active TAP run not found"})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
