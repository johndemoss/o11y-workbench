package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/johndemoss/o11y-workbench/internal/export"
	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/logging"
	"github.com/johndemoss/o11y-workbench/internal/region"
)

type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusPaused    RunStatus = "paused"
	RunStatusStopping  RunStatus = "stopping"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusStopped   RunStatus = "stopped"
	RunStatusStreaming RunStatus = "streaming"

	// maxCompletedRuns caps how many terminal runs stay in memory.
	maxCompletedRuns = 50
	// Each signal retains at most 4 MiB of protobuf payload per run.
	captureSignalLimit = 4 << 20
)

func isTerminalStatus(s RunStatus) bool {
	switch s {
	case RunStatusCompleted, RunStatusFailed, RunStatusStopped:
		return true
	default:
		return false
	}
}

type Run struct {
	mu                   sync.Mutex
	ID                   string     `json:"id"`
	Status               RunStatus  `json:"status"`
	Config               gen.Config `json:"config"`
	StartedAt            time.Time  `json:"started_at"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	DurationNs           int64      `json:"duration_ns"`
	SpanCount            int        `json:"span_count"`
	TraceCount           int        `json:"trace_count"`
	TraceCountCompleted  int        `json:"trace_count_completed"`
	LogCount             int        `json:"log_count"`
	LogCountCompleted    int        `json:"log_count_completed"`
	MetricCount          int        `json:"metric_count,omitempty"`
	MetricCountCompleted int        `json:"metric_count_completed,omitempty"`
	// Computed breakdowns — populated during finalization so the backend is the single source of truth.
	// These replace client-side aggregation previously done in scripts.js.
	ResourceSpanCount int              `json:"resource_span_count,omitempty"`
	LogRecordCount    int              `json:"log_record_count,omitempty"`
	LogSeverityCounts map[string]int   `json:"log_severity_counts,omitempty"`
	MetricRecordCount int              `json:"metric_record_count,omitempty"`
	MetricSeriesCount int              `json:"metric_series_count,omitempty"`
	MetricTypes       []string         `json:"metric_types,omitempty"`
	MetricData        *json.RawMessage `json:"metric_data,omitempty"`
	Scenario          string           `json:"scenario,omitempty"`
	Source            string           `json:"source,omitempty"`
	Error             string           `json:"error,omitempty"`
	TraceData         *json.RawMessage `json:"trace_data,omitempty"`
	LogData           *json.RawMessage `json:"log_data,omitempty"`
	// Has* flags are set from payload pointers so List can omit TraceData/LogData/MetricData
	// while Jobs List signal filters and detail fetch still know payloads exist.
	HasTraceData      bool               `json:"has_trace_data,omitempty"`
	HasLogData        bool               `json:"has_log_data,omitempty"`
	HasMetricData     bool               `json:"has_metric_data,omitempty"`
	Endpoints         []string           `json:"endpoints"`
	Insecure          bool               `json:"insecure"`
	Output            string             `json:"output"`
	Format            string             `json:"format,omitempty"`
	CaptureTruncated  bool               `json:"capture_truncated"`
	CaptureDropped    int64              `json:"capture_dropped"`
	CaptureLimitBytes int64              `json:"capture_limit_bytes"`
	GeneratorStats    *gen.Stats         `json:"generator_stats,omitempty"`
	cancel            context.CancelFunc // not exported, not json
	gate              *runPauseGate      // nil when the run is not executing; not exported
	routes            []region.Region    // immutable per-run destinations, never process-global
}

// Pause waits until no export is in flight before reporting the paused state.
func (r *Run) Pause(ctx context.Context) error {
	r.mu.Lock()
	if r.Status == RunStatusPaused {
		if r.gate == nil || !r.gate.pauseAcknowledged() {
			r.mu.Unlock()
			return fmt.Errorf("run is not executing")
		}
		r.mu.Unlock()
		return nil
	}
	if r.Status != RunStatusRunning {
		r.mu.Unlock()
		return fmt.Errorf("run is not running")
	}
	gate := r.gate
	r.mu.Unlock()
	if gate == nil {
		return fmt.Errorf("run is not executing")
	}
	sequence, err := gate.pause(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	if err := ctx.Err(); err != nil {
		gate.cancelPause(sequence)
		r.mu.Unlock()
		return err
	}
	if (r.Status != RunStatusRunning && r.Status != RunStatusPaused) || !gate.acknowledge() {
		r.mu.Unlock()
		return context.Canceled
	}
	r.Status = RunStatusPaused
	r.mu.Unlock()
	return nil
}

func (r *Run) Resume() error {
	r.mu.Lock()
	if r.Status != RunStatusPaused {
		r.mu.Unlock()
		return fmt.Errorf("run is not paused")
	}
	if r.gate != nil {
		r.gate.resume()
	}
	r.Status = RunStatusRunning
	r.mu.Unlock()
	return nil
}

func (r *Run) Stop() {
	r.mu.Lock()
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	if r.Status == RunStatusPending || r.Status == RunStatusRunning || r.Status == RunStatusPaused {
		r.Status = RunStatusStopping
		r.Error = "stopped"
	}
	if r.gate != nil {
		r.gate.stop()
	}
	r.mu.Unlock()
}

func (r *Run) snapshot() *Run {
	return r.snapshotWithPayloads(true)
}

func (r *Run) snapshotSummary() *Run {
	return r.snapshotWithPayloads(false)
}

func (r *Run) snapshotWithPayloads(includePayloads bool) *Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := &Run{
		ID: r.ID, Status: r.Status, Config: r.Config,
		StartedAt: r.StartedAt, CompletedAt: r.CompletedAt,
		DurationNs: r.DurationNs, SpanCount: r.SpanCount,
		TraceCount: r.TraceCount, TraceCountCompleted: r.TraceCountCompleted,
		LogCount: r.LogCount, LogCountCompleted: r.LogCountCompleted,
		MetricCount: r.MetricCount, MetricCountCompleted: r.MetricCountCompleted,
		ResourceSpanCount: r.ResourceSpanCount,
		MetricSeriesCount: r.MetricSeriesCount,
		LogRecordCount:    r.LogRecordCount,
		LogSeverityCounts: func() map[string]int {
			if r.LogSeverityCounts == nil {
				return nil
			}
			out := make(map[string]int, len(r.LogSeverityCounts))
			for k, v := range r.LogSeverityCounts {
				out[k] = v
			}
			return out
		}(),
		MetricRecordCount: r.MetricRecordCount,
		MetricTypes:       append([]string(nil), r.MetricTypes...),
		Scenario:          r.Scenario, Source: r.Source, Error: r.Error,
		Format: r.Format, CaptureTruncated: r.CaptureTruncated,
		CaptureDropped: r.CaptureDropped, CaptureLimitBytes: r.CaptureLimitBytes,
		HasTraceData:  r.TraceData != nil,
		HasLogData:    r.LogData != nil,
		HasMetricData: r.MetricData != nil,
		Endpoints:     append([]string(nil), r.Endpoints...),
		Insecure:      r.Insecure, Output: r.Output,
		GeneratorStats: func() *gen.Stats {
			if r.GeneratorStats == nil {
				return nil
			}
			c := *r.GeneratorStats
			return &c
		}(),
	}
	if !includePayloads {
		return c
	}
	if r.TraceData != nil {
		data := make(json.RawMessage, len(*r.TraceData))
		copy(data, *r.TraceData)
		c.TraceData = &data
	}
	if r.LogData != nil {
		data := make(json.RawMessage, len(*r.LogData))
		copy(data, *r.LogData)
		c.LogData = &data
	}
	if r.MetricData != nil {
		data := make(json.RawMessage, len(*r.MetricData))
		copy(data, *r.MetricData)
		c.MetricData = &data
	}
	return c
}

var runIDCounter atomic.Int64

type bufferExporter struct {
	mu        sync.Mutex
	spans     []*tracev1.ResourceSpans
	spanCount int
	bytes     int
	dropped   int64
}

func (e *bufferExporter) Export(_ context.Context, spans []*tracev1.ResourceSpans) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, rs := range spans {
		size := proto.Size(rs) + 256
		if size > captureSignalLimit-e.bytes {
			e.dropped += int64(export.CountSpans([]*tracev1.ResourceSpans{rs}))
			continue
		}
		e.bytes += size
		e.spanCount += export.CountSpans([]*tracev1.ResourceSpans{rs})
		e.spans = append(e.spans, rs)
	}
	return nil
}

func (e *bufferExporter) SpanCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.spanCount
}

func (e *bufferExporter) Snapshot() []*tracev1.ResourceSpans {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]*tracev1.ResourceSpans, len(e.spans))
	copy(out, e.spans)
	return out
}

func (e *bufferExporter) Shutdown(_ context.Context) error {
	return nil
}

type bufferLogExporter struct {
	mu      sync.Mutex
	logs    []*logsv1.ResourceLogs
	bytes   int
	dropped int64
}

func (e *bufferLogExporter) Export(_ context.Context, logs []*logsv1.ResourceLogs) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, rl := range logs {
		size := proto.Size(rl) + 256
		if size > captureSignalLimit-e.bytes {
			e.dropped += int64(export.CountLogRecords([]*logsv1.ResourceLogs{rl}))
			continue
		}
		e.bytes += size
		e.logs = append(e.logs, rl)
	}
	return nil
}

func (e *bufferLogExporter) Shutdown(_ context.Context) error {
	return nil
}

type bufferMetricExporter struct {
	mu      sync.Mutex
	metrics []*metricspb.ResourceMetrics
	bytes   int
	dropped int64
}

func (e *bufferMetricExporter) Export(_ context.Context, metrics []*metricspb.ResourceMetrics) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, rm := range metrics {
		size := proto.Size(rm) + 256
		if size > captureSignalLimit-e.bytes {
			e.dropped += int64(export.CountDataPoints([]*metricspb.ResourceMetrics{rm}))
			continue
		}
		e.bytes += size
		e.metrics = append(e.metrics, rm)
	}
	return nil
}

func (e *bufferMetricExporter) Shutdown(_ context.Context) error { return nil }

type multiExporter []export.TraceExporter

func (e multiExporter) Export(ctx context.Context, spans []*tracev1.ResourceSpans) error {
	_, err := e.ExportWithReceipt(ctx, spans)
	return err
}

func (e multiExporter) ExportWithReceipt(ctx context.Context, spans []*tracev1.ResourceSpans) (export.ExportReceipt, error) {
	count := export.CountSpans(spans)
	receipt := export.ReceiptFullSuccess(count)
	for _, exp := range e {
		if re, ok := exp.(interface {
			ExportWithReceipt(context.Context, []*tracev1.ResourceSpans) (export.ExportReceipt, error)
		}); ok {
			var err error
			receipt, err = re.ExportWithReceipt(ctx, spans)
			if err != nil {
				return receipt, err
			}
		} else if err := exp.Export(ctx, spans); err != nil {
			return export.ReceiptRPCFailure(count, err), err
		}
	}
	return receipt, nil
}

func (e multiExporter) Shutdown(ctx context.Context) error {
	for _, exp := range e {
		if err := exp.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

var ErrRunStoreClosed = errors.New("run store is closed")

// ErrRunNotFound reports an unknown run identifier.
var ErrRunNotFound = errors.New("run not found")

type RunStore struct {
	mu      sync.RWMutex
	runs    map[string]*Run
	active  map[string]*Run // Jobs remain owned even if UI history is deleted.
	closed  bool
	workers sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewRunStore() *RunStore {
	ctx, cancel := context.WithCancel(context.Background())
	return &RunStore{runs: make(map[string]*Run), active: make(map[string]*Run), ctx: ctx, cancel: cancel}
}

// Close rejects new work, cancels owned jobs and joins their exporter cleanup.
// Concurrent callers all wait; worker admission and Close are serialized.
func (s *RunStore) Close() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		for _, run := range s.active {
			run.Stop()
		}
		// Mark explicit stops before canceling export owners, so an RPC failure
		// cannot finalize a shutdown cancellation as an ordinary failed job.
		s.cancel()
	}
	s.mu.Unlock()
	s.workers.Wait()
}

func (s *RunStore) Add(r *Run) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[r.ID] = r
}

func (s *RunStore) Get(id string) *Run {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok := s.runs[id]
	if !ok {
		return nil
	}
	return raw.snapshot()
}

func (s *RunStore) List() []*Run {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Run, 0, len(s.runs))
	for _, r := range s.runs {
		result = append(result, r.snapshotSummary())
	}
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].StartedAt.After(result[i].StartedAt) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

func marshalTraceJSON(spans []*tracev1.ResourceSpans) ([]byte, error) {
	req := &collectortracev1.ExportTraceServiceRequest{ResourceSpans: spans}
	opts := protojson.MarshalOptions{UseProtoNames: false, Indent: ""}
	data, err := opts.Marshal(req)
	if err != nil {
		return nil, err
	}
	return export.FixTraceSpanIDsForMarshal(data)
}

func marshalLogJSON(logs []*logsv1.ResourceLogs) ([]byte, error) {
	req := &collectorlogsv1.ExportLogsServiceRequest{ResourceLogs: logs}
	opts := protojson.MarshalOptions{UseProtoNames: false, Indent: ""}
	data, err := opts.Marshal(req)
	if err != nil {
		return nil, err
	}
	return export.FixTraceSpanIDsForMarshal(data)
}

func marshalMetricJSON(metrics []*metricspb.ResourceMetrics) ([]byte, error) {
	req := &collectormetricsv1.ExportMetricsServiceRequest{ResourceMetrics: metrics}
	opts := protojson.MarshalOptions{UseProtoNames: false, Indent: ""}
	data, err := opts.Marshal(req)
	if err != nil {
		return nil, err
	}
	return export.FixTraceSpanIDsForMarshal(data)
}

type multiLogExporter []export.LogExporter

func (e multiLogExporter) Export(ctx context.Context, logs []*logsv1.ResourceLogs) error {
	_, err := e.ExportWithReceipt(ctx, logs)
	return err
}

func (e multiLogExporter) ExportWithReceipt(ctx context.Context, logs []*logsv1.ResourceLogs) (export.ExportReceipt, error) {
	count := export.CountLogRecords(logs)
	receipt := export.ReceiptFullSuccess(count)
	for _, exp := range e {
		if re, ok := exp.(interface {
			ExportWithReceipt(context.Context, []*logsv1.ResourceLogs) (export.ExportReceipt, error)
		}); ok {
			var err error
			receipt, err = re.ExportWithReceipt(ctx, logs)
			if err != nil {
				return receipt, err
			}
		} else if err := exp.Export(ctx, logs); err != nil {
			return export.ReceiptRPCFailure(count, err), err
		}
	}
	return receipt, nil
}

func (e multiLogExporter) Shutdown(ctx context.Context) error {
	for _, exp := range e {
		if err := exp.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

type multiMetricExporter []export.MetricExporter

func (e multiMetricExporter) Export(ctx context.Context, metrics []*metricspb.ResourceMetrics) error {
	_, err := e.ExportWithReceipt(ctx, metrics)
	return err
}

func (e multiMetricExporter) ExportWithReceipt(ctx context.Context, metrics []*metricspb.ResourceMetrics) (export.ExportReceipt, error) {
	count := export.CountDataPoints(metrics)
	receipt := export.ReceiptFullSuccess(count)
	for _, exp := range e {
		if re, ok := exp.(interface {
			ExportWithReceipt(context.Context, []*metricspb.ResourceMetrics) (export.ExportReceipt, error)
		}); ok {
			var err error
			receipt, err = re.ExportWithReceipt(ctx, metrics)
			if err != nil {
				return receipt, err
			}
		} else if err := exp.Export(ctx, metrics); err != nil {
			return export.ReceiptRPCFailure(count, err), err
		}
	}
	return receipt, nil
}
func (e multiMetricExporter) Shutdown(ctx context.Context) error {
	for _, exp := range e {
		if err := exp.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

// StartRun detaches accepted jobs from HTTP request cancellation, not from their store.
func StartRun(_ context.Context, store *RunStore, cfg gen.Config, endpoints []string, insecure bool, output string) (*Run, error) {
	if err := gen.ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid run configuration: %w", err)
	}
	if (cfg.Workload.Active() || gen.IsWorkloadScenario(cfg.Scenario)) && (cfg.IncludesLogs() || cfg.IncludesMetrics()) {
		return nil, errors.New("workload profiles support traces only")
	}
	id := fmt.Sprintf("run-%d", runIDCounter.Add(1))
	scenario := ""
	if cfg.Scenario != "" {
		scenario = cfg.Scenario
	}
	routes := routesForEndpoints(endpoints)
	epCopy := make([]string, len(routes))
	for i, route := range routes {
		epCopy[i] = route.Endpoint
	}
	run := &Run{
		ID: id, Status: RunStatusPending, Config: cfg,
		Scenario: scenario, StartedAt: time.Now(), TraceCount: cfg.TraceCount,
		Endpoints: epCopy, Insecure: insecure, Output: output, routes: routes,
		Format: "json", CaptureLimitBytes: 3 * captureSignalLimit,
	}
	if cfg.IncludesLogs() {
		run.LogCount = cfg.LogConfig.LogCount
		if cfg.LogConfig.LogsPerSpan > 0 && (cfg.IncludesTraces() && cfg.IncludesLogs()) {
			// LogCount will be computed after trace generation.
			run.LogCount = 0
		}
	}
	if cfg.IncludesMetrics() {
		run.MetricCount = cfg.MetricConfig.MetricCount
	}
	// Generators start duration budgets after setup. This parent controls only
	// the store lifetime, never an HTTP request or generation duration deadline.
	genCtx, cancel := context.WithCancel(store.ctx)
	run.cancel = cancel
	store.mu.Lock()
	if store.closed {
		store.mu.Unlock()
		cancel()
		return nil, ErrRunStoreClosed
	}
	store.runs[id] = run
	store.active[id] = run
	store.workers.Add(1)
	store.mu.Unlock()

	// HTTP response encoding must not race with progress updates.
	response := run.snapshot()
	go func() {
		defer store.workers.Done()
		defer cancel()
		defer func() {
			store.mu.Lock()
			delete(store.active, id)
			store.mu.Unlock()
		}()
		executeRun(run, store, cfg, genCtx)
	}()
	return response, nil
}

func (s *RunStore) StopRun(id string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r, ok := s.runs[id]; ok {
		r.Stop()
	}
}

// PauseRun waits for the run's in-flight exports before returning.
func (s *RunStore) PauseRun(ctx context.Context, id string) error {
	s.mu.RLock()
	r := s.runs[id]
	s.mu.RUnlock()
	if r == nil {
		return ErrRunNotFound
	}
	return r.Pause(ctx)
}

func (s *RunStore) ResumeRun(id string) error {
	s.mu.RLock()
	r := s.runs[id]
	s.mu.RUnlock()
	if r == nil {
		return ErrRunNotFound
	}
	return r.Resume()
}

func (s *RunStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.runs, id)
}

// PruneCompleted deletes oldest terminal runs until at most max remain.
// Never deletes pending, running, paused, or streaming runs.
func (s *RunStore) PruneCompleted(max int) {
	if max < 0 {
		max = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	type timed struct {
		id        string
		startedAt time.Time
	}
	var terminal []timed
	for id, r := range s.runs {
		r.mu.Lock()
		st, started := r.Status, r.StartedAt
		r.mu.Unlock()
		if isTerminalStatus(st) {
			terminal = append(terminal, timed{id: id, startedAt: started})
		}
	}
	if len(terminal) <= max {
		return
	}
	sort.Slice(terminal, func(i, j int) bool {
		return terminal[i].startedAt.Before(terminal[j].startedAt)
	})
	for _, t := range terminal[:len(terminal)-max] {
		delete(s.runs, t.id)
	}
}

func countSpansIn(spans []*tracev1.ResourceSpans) (spanCount, resourceSpanCount int) {
	resourceSpanCount = len(spans)
	for _, rs := range spans {
		for _, ss := range rs.ScopeSpans {
			spanCount += len(ss.Spans)
		}
	}
	return
}

func summarizeLogs(logs []*logsv1.ResourceLogs) (total int, sevCounts map[string]int) {
	for _, rl := range logs {
		for _, sl := range rl.ScopeLogs {
			total += len(sl.LogRecords)
		}
	}
	if total == 0 {
		return 0, nil
	}
	sevCounts = make(map[string]int)
	for _, rl := range logs {
		for _, sl := range rl.ScopeLogs {
			for _, lr := range sl.LogRecords {
				sev := lr.SeverityText
				if sev == "" {
					sev = "INFO"
				}
				sevCounts[strings.ToLower(sev)]++
			}
		}
	}
	return total, sevCounts
}

func summarizeMetrics(metrics []*metricspb.ResourceMetrics) (total, series int, types []string) {
	typeSet := make(map[string]struct{})
	for _, rm := range metrics {
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				series++
				switch m.Data.(type) {
				case *metricspb.Metric_Gauge:
					total += len(m.GetGauge().DataPoints)
					typeSet["gauge"] = struct{}{}
				case *metricspb.Metric_Sum:
					total += len(m.GetSum().DataPoints)
					typeSet["sum"] = struct{}{}
				case *metricspb.Metric_Histogram:
					total += len(m.GetHistogram().DataPoints)
					typeSet["histogram"] = struct{}{}
				case *metricspb.Metric_ExponentialHistogram:
					typeSet["exponential_histogram"] = struct{}{}
				case *metricspb.Metric_Summary:
					typeSet["summary"] = struct{}{}
				}
			}
		}
	}
	if len(typeSet) > 0 {
		types = make([]string, 0, len(typeSet))
		for t := range typeSet {
			types = append(types, t)
		}
		sort.Strings(types)
	}
	return total, series, types
}

// buildRunPayloads marshals buffered OTLP signals to JSON without holding run.mu.
func buildRunPayloads(spans []*tracev1.ResourceSpans, logs []*logsv1.ResourceLogs, metrics []*metricspb.ResourceMetrics) (traceData, logData, metricData *json.RawMessage) {
	if len(spans) > 0 {
		if data, err := marshalTraceJSON(spans); err == nil {
			raw := json.RawMessage(data)
			traceData = &raw
		}
	}
	if len(logs) > 0 {
		if data, err := marshalLogJSON(logs); err == nil {
			raw := json.RawMessage(data)
			logData = &raw
		}
	}
	if len(metrics) > 0 {
		if data, err := marshalMetricJSON(metrics); err == nil {
			raw := json.RawMessage(data)
			metricData = &raw
		}
	}
	return
}

// routesForEndpoints snapshots defaults or constructs run-local routing. The first
// custom route is local so flat evidence payloads without cloud.region also reach
// the first destination. No request changes CLI process defaults.
func routesForEndpoints(endpoints []string) []region.Region {
	var routes []region.Region
	if len(endpoints) == 0 {
		for _, name := range region.All() {
			routes = append(routes, region.Region{Name: name, Endpoint: region.GetEndpoint(name)})
		}
		return routes
	}
	for i, endpoint := range endpoints {
		name := region.Name(fmt.Sprintf("endpoint-%d", i))
		if i == 0 {
			name = region.RegionLocal
		}
		routes = append(routes, region.Region{Name: name, Endpoint: endpoint})
	}
	return routes
}

func executeRun(run *Run, store *RunStore, cfg gen.Config, genCtx context.Context) {
	defer func() {
		run.mu.Lock()
		st := run.Status
		run.mu.Unlock()
		if isTerminalStatus(st) {
			store.PruneCompleted(maxCompletedRuns)
		}
	}()

	gate := newRunPauseGate()
	run.mu.Lock()
	run.gate = gate
	if run.Status != RunStatusStopping {
		run.Status = RunStatusRunning
	}
	run.mu.Unlock()

	start := time.Now()
	genCtx, stopGeneration := context.WithCancel(genCtx)
	defer stopGeneration()
	defer func() {
		gate.stop()
		run.mu.Lock()
		run.gate = nil
		run.mu.Unlock()
	}()
	if cfg.Duration > 0 {
		var cancel context.CancelFunc
		genCtx, cancel = context.WithTimeout(genCtx, cfg.Duration)
		defer cancel()
	}
	// Cancellation must release pause admission before joining generators: a
	// correlated log generator may flush with a fresh context on cancellation.
	stopGateWatcher := context.AfterFunc(genCtx, gate.stop)
	defer stopGateWatcher()
	buf := &bufferExporter{}
	logBuf := &bufferLogExporter{}
	metricBuf := &bufferMetricExporter{}
	var traceErr error
	routes := run.routes
	if len(routes) == 0 { // Direct engine/test call rather than StartRun.
		routes = routesForEndpoints(run.Endpoints)
	}
	names := make([]region.Name, len(routes))
	for i, route := range routes {
		names[i] = route.Name
	}

	// Determine which signals to generate.
	doTraces := cfg.IncludesTraces()
	doLogs := cfg.IncludesLogs()
	doMetrics := cfg.IncludesMetrics()

	// Correlated logs consume span references while traces run, for both finite
	// and indefinite generation, so references are never retained in a slice.
	correlate := doTraces && doLogs && cfg.LogConfig.LogsPerSpan > 0
	var (
		refs       chan []gen.SpanRef
		logDone    chan struct{}
		metricDone chan struct{}
	)
	if correlate {
		refs = make(chan []gen.SpanRef, 64)
	}

	// The first non-cancellation failure is the run's root cause and cancels
	// every sibling generator; later cancellation errors are not reported.
	var failureMu sync.Mutex
	var failureErr error
	recordFailure := func(err error) {
		if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		failureMu.Lock()
		if failureErr == nil {
			failureErr = err
		}
		failureMu.Unlock()
		stopGeneration()
	}
	runFailure := func() error {
		failureMu.Lock()
		defer failureMu.Unlock()
		return failureErr
	}

	// Indefinite traces run concurrently with the other signals.
	streaming := doTraces && cfg.TraceCount == 0 && (doLogs || doMetrics)
	var traceDone chan error
	firstTrace := make(chan struct{})
	var firstTraceOnce sync.Once
	generateTraces := func() error {
		if correlate {
			defer close(refs)
		}
		exp := export.TraceExporter(buf)
		if run.Output == "otlp" {
			otlpExporters := make(map[region.Name]export.TraceExporter)
			for _, route := range routes {
				reg, ep := route.Name, route.Endpoint
				otlpExp, err := export.NewOTLPExporter(ep, run.Insecure, nil)
				if err != nil {
					return fmt.Errorf("create OTLP exporter for %s: %w", reg, err)
				}
				// Close every successfully constructed connection, including partial setup failures.
				defer func() { _ = otlpExp.Shutdown(context.Background()) }()
				// Wrap exporter to use fresh context with timeout for each export
				otlpExporters[reg] = export.WithExportTimeoutContext(genCtx, otlpExp, 30*time.Second)
			}
			router := export.NewEndpointRouter(otlpExporters)
			exp = multiExporter{buf, router}
		}
		exp = gatedTraceExporter{gate: gate, inner: exp}

		g := gen.NewGenerator(cfg)
		g.Regions = names
		g.OnProgress = func(completed, total int) {
			run.mu.Lock()
			run.TraceCountCompleted = completed
			run.SpanCount = buf.SpanCount()
			run.mu.Unlock()
			firstTraceOnce.Do(func() { close(firstTrace) })
		}
		if correlate {
			g.OnTraceSpans = func(batch []gen.SpanRef) {
				select {
				case refs <- batch:
				case <-genCtx.Done():
				}
			}
		}
		err := g.Run(genCtx, exp)
		st := g.Stats()
		run.mu.Lock()
		run.GeneratorStats = &st
		run.mu.Unlock()
		return err
	}
	if doTraces {
		if correlate {
			logExp, closers, err := buildRunLogExporter(run, routes, logBuf, genCtx)
			if err != nil {
				recordFailure(err)
			} else {
				for _, closer := range closers {
					defer closer()
				}
				logExp = &gatedLogExporter{gate: gate, inner: logExp}
				lg := gen.NewLogGenerator(cfg.LogConfig)
				lg.Regions = names
				lg.OnProgress = func(completed, total int) {
					run.mu.Lock()
					run.LogCountCompleted = completed
					run.LogCount = total
					run.mu.Unlock()
				}
				logDone = make(chan struct{})
				go func() {
					defer close(logDone)
					err := lg.RunCorrelated(genCtx, refs, logExp)
					recordFailure(err)
				}()
			}
		}
		if streaming {
			if doMetrics {
				if metricExp, closers, err := buildRunMetricExporter(run, routes, metricBuf, genCtx); err != nil {
					recordFailure(err)
				} else {
					for _, closer := range closers {
						defer closer()
					}
					metricExp = gatedMetricExporter{gate: gate, inner: metricExp}
					mg := gen.NewMetricGenerator(cfg.MetricConfig)
					mg.Regions = names
					mg.OnProgress = func(completed, total int) {
						run.mu.Lock()
						run.MetricCountCompleted = completed
						run.MetricCount = total
						run.mu.Unlock()
					}
					metricDone = make(chan struct{})
					go func() {
						defer close(metricDone)
						recordFailure(mg.Run(genCtx, metricExp))
					}()
				}
			}
			traceDone = make(chan error, 1)
			go func() {
				err := generateTraces()
				recordFailure(err)
				traceDone <- err
			}()
			select {
			case <-firstTrace:
			case traceErr = <-traceDone:
				traceDone = nil
			case <-genCtx.Done():
			}
			recordFailure(traceErr)
		} else {
			traceErr = generateTraces()
			recordFailure(traceErr)
		}
	}

	// --- LOG GENERATION ---
	// Streaming correlation already consumed references while traces were running.
	if doLogs && !correlate && runFailure() == nil {
		logging.Debugf("Starting log generation. doLogs=%v, correlate=%v", doLogs, correlate)

		logExp, closers, err := buildRunLogExporter(run, routes, logBuf, genCtx)
		for _, closer := range closers {
			defer closer()
		}
		if err != nil {
			recordFailure(err)
		} else {
			logExp = &gatedLogExporter{gate: gate, inner: logExp}
			lg := gen.NewLogGenerator(cfg.LogConfig)
			lg.Regions = names
			lg.OnProgress = func(completed, total int) {
				run.mu.Lock()
				run.LogCountCompleted = completed
				if run.LogCount == 0 {
					run.LogCount = total
				}
				run.mu.Unlock()
			}
			recordFailure(lg.Run(genCtx, logExp))
			logging.Debugf("Log generation completed. failure=%v", runFailure())
		}
	}
	// Finite metrics follow both trace generation and correlated logs.
	if !streaming && logDone != nil {
		<-logDone
		logDone = nil
	}

	// --- METRIC GENERATION ---
	if doMetrics && !streaming && runFailure() == nil {
		logging.Debugf("Starting metric generation. doMetrics=%v", doMetrics)
		logging.Debugf("cfg.MetricConfig = %+v", cfg.MetricConfig)

		metricExp := export.MetricExporter(metricBuf)
		if run.Output == "otlp" {
			otlpMetricExporters := make(map[region.Name]export.MetricExporter)
			for _, route := range routes {
				reg, ep := route.Name, route.Endpoint
				otlpMetricExp, err := export.NewOTLPMetricExporter(ep, run.Insecure, nil)
				if err != nil {
					recordFailure(fmt.Errorf("create OTLP metric exporter for %s: %w", reg, err))
					goto finalize
				}
				defer func() { _ = otlpMetricExp.Shutdown(context.Background()) }() // Best-effort connection cleanup.
				// Wrap metric exporter to use fresh context with timeout for each export
				otlpMetricExporters[reg] = export.WithMetricExportTimeoutContext(genCtx, otlpMetricExp, 30*time.Second)
			}
			router := export.NewMetricEndpointRouter(otlpMetricExporters)
			metricExp = multiMetricExporter{metricBuf, router}
		}
		metricExp = gatedMetricExporter{gate: gate, inner: metricExp}

		mg := gen.NewMetricGenerator(cfg.MetricConfig)
		mg.Regions = names
		mg.OnProgress = func(completed, total int) {
			run.mu.Lock()
			run.MetricCountCompleted = completed
			if run.MetricCount == 0 {
				run.MetricCount = total
			}
			run.mu.Unlock()
		}
		recordFailure(mg.Run(genCtx, metricExp))
	}

finalize:
	if runFailure() != nil {
		stopGeneration()
	}
	if traceDone != nil {
		traceErr = <-traceDone
		recordFailure(traceErr)
		traceDone = nil
	}
	if logDone != nil {
		<-logDone
	}
	if metricDone != nil {
		<-metricDone
	}
	elapsed := time.Since(start).Nanoseconds()

	// Snapshot buffers once; generation is finished so exporters are idle.
	spans := buf.Snapshot()
	logs := append([]*logsv1.ResourceLogs(nil), logBuf.logs...)
	metrics := append([]*metricspb.ResourceMetrics(nil), metricBuf.metrics...)

	logTotal, sevCounts := summarizeLogs(logs)
	metricTotal, metricSeries, metricTypes := summarizeMetrics(metrics)
	spanTotal := buf.SpanCount()
	if spanTotal == 0 {
		spanTotal, _ = countSpansIn(spans)
	}

	run.mu.Lock()
	run.DurationNs = elapsed
	run.LogRecordCount = logTotal
	if sevCounts != nil {
		run.LogSeverityCounts = sevCounts
	}
	run.MetricSeriesCount = metricSeries
	run.MetricRecordCount = metricTotal
	if metricTypes != nil {
		run.MetricTypes = metricTypes
	}
	run.CaptureDropped = buf.dropped + logBuf.dropped + metricBuf.dropped
	run.CaptureTruncated = run.CaptureDropped > 0
	run.mu.Unlock()

	// A cancelled sibling generator never replaces the originating failure.
	err := runFailure()

	traceData, logData, metricData := buildRunPayloads(spans, logs, metrics)

	// Publish payloads and the terminal transition together.

	run.mu.Lock()
	run.SpanCount = spanTotal
	run.ResourceSpanCount = len(spans)
	run.TraceData = traceData
	run.LogData = logData
	run.MetricData = metricData
	switch {
	case run.Status == RunStatusStopping:
		run.Status = RunStatusStopped
		run.Error = "stopped"
	case err != nil:
		run.Status = RunStatusFailed
		run.Error = err.Error()
	default:
		run.Status = RunStatusCompleted
	}
	now := time.Now()
	run.CompletedAt = &now
	run.mu.Unlock()
}

