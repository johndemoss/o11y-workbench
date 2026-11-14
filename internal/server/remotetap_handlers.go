package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/export"
	"github.com/johndemoss/o11y-workbench/internal/remotetap"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
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

// ringBuffer implements bounded storage with FIFO eviction
type ringBuffer struct {
	mu           sync.Mutex
	items        []*tracev1.ResourceSpans
	maxSpans     int
	maxBytes     int
	currentBytes int
	evictedCount int
}

func newRingBuffer(maxSpans, maxBytes int) *ringBuffer {
	return &ringBuffer{
		maxSpans:     maxSpans,
		maxBytes:     maxBytes,
		currentBytes: 0,
		evictedCount: 0,
	}
}

// Add adds an item to the buffer, evicting oldest items if at capacity.
// Returns true if added, false if item too large to store.
func (rb *ringBuffer) Add(item *tracev1.ResourceSpans) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	itemSize := estimateSize(item)
	if itemSize > rb.maxBytes {
		return false // item too large to store
	}

	// Evict oldest items until there's room
	for len(rb.items) >= rb.maxSpans || rb.currentBytes+itemSize > rb.maxBytes {
		if len(rb.items) == 0 {
			break
		}
		evicted := rb.items[0]
		rb.items = rb.items[1:]
		rb.currentBytes -= estimateSize(evicted)
		rb.evictedCount++
	}

	rb.items = append(rb.items, item)
	rb.currentBytes += itemSize
	return true
}

// GetAll returns all items in the buffer.
func (rb *ringBuffer) GetAll() []*tracev1.ResourceSpans {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	result := make([]*tracev1.ResourceSpans, len(rb.items))
	copy(result, rb.items)
	return result
}

// EvictedCount returns the total number of evicted items.
func (rb *ringBuffer) EvictedCount() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.evictedCount
}

// CurrentBytes returns the current total byte size.
func (rb *ringBuffer) CurrentBytes() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.currentBytes
}

// estimateSize returns an approximate byte size of a ResourceSpans.
func estimateSize(rs *tracev1.ResourceSpans) int {
	// Rough estimation using JSON size
	data, _ := json.Marshal(rs)
	return len(data)
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
		ID:        id,
		Status:    RunStatusStreaming,
		StartedAt: time.Now(),
		Source:    "remotetap",
	}
	m.store.Add(run)

	m.endpoint = endpoint
	m.client = client
	m.run = run
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
	for {
		select {
		case data, ok := <-client.Data():
			if !ok {
				return
			}
			spanCount := 0
			for _, rs := range data.Traces {
				for _, ss := range rs.ScopeSpans {
					spanCount += len(ss.Spans)
				}
			}
			m.processData(data, spanCount)
		case err, ok := <-client.Err():
			if !ok {
				return
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

func (m *RemotetapManager) processData(data *remotetap.RemotetapData, spanCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run := m.run
	if run == nil {
		return
	}

	run.mu.Lock()
	defer run.mu.Unlock()

	if len(data.Traces) > 0 {
		run.SpanCount += spanCount
		for _, rs := range data.Traces {
			m.buffer.Add(rs)
		}
		run.TraceCount += len(data.Traces)
	}

	if len(data.Logs) > 0 {
		for range data.Logs {
			run.LogCount++
		}
	}

	if len(data.Metrics) > 0 {
		for range data.Metrics {
			run.MetricCount++
		}
	}
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
		// Serialize ring buffer to TraceData so Pipeline Prototyper can access it
		if m.buffer != nil {
			items := m.buffer.GetAll()
			if len(items) > 0 {
				req := &collectortracev1.ExportTraceServiceRequest{ResourceSpans: items}
				opts := protojson.MarshalOptions{UseProtoNames: false, Indent: ""}
				if data, err := opts.Marshal(req); err == nil {
					if fixed, err := export.FixTraceSpanIDsForMarshal(data); err == nil {
						raw := json.RawMessage(fixed)
						m.run.TraceData = &raw
					}
				}
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
	// Check service.name filter
	if len(m.filters.Services) > 0 {
		svcName := getResourceAttr(rs, "service.name")
		found := false
		for _, svc := range m.filters.Services {
			if svcName == svc {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check resource attributes filter
	if len(m.filters.ResourceAttrs) > 0 {
		for k, v := range m.filters.ResourceAttrs {
			if getResourceAttr(rs, k) != v {
				return false
			}
		}
	}

	// Check span kind filter
	if len(m.filters.SpanKinds) > 0 {
		found := false
		for _, ss := range rs.ScopeSpans {
			for _, span := range ss.Spans {
				kindStr := span.Kind.String()
				for _, allowed := range m.filters.SpanKinds {
					if kindStr == allowed {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
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

	validSamplingTypes := map[string]bool{"head": true, "tail": true, "random": true, "none": true}
	if body.Filters.SamplingType != "" && !validSamplingTypes[body.Filters.SamplingType] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "samplingType must be one of: head, tail, random, none"})
		return
	}
	if body.Filters.SamplingRate < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "samplingRate must be >= 0"})
		return
	}
	if body.Filters.MaxSpans <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "maxSpans must be > 0"})
		return
	}
	if body.Filters.MaxBytes <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "maxBytes must be > 0"})
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
	stats := s.remotetap.GetUsageStats()
	writeJSON(w, http.StatusOK, stats)
}
