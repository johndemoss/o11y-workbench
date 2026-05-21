package server

import (
	"context"
	"encoding/json"
	"fmt"
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
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusStopped   RunStatus = "stopped"
	RunStatusStreaming RunStatus = "streaming"
)

type Run struct {
	mu                   sync.Mutex
	ID                   string             `json:"id"`
	Status               RunStatus          `json:"status"`
	Config               gen.Config         `json:"config"`
	StartedAt            time.Time          `json:"started_at"`
	CompletedAt          *time.Time         `json:"completed_at,omitempty"`
	DurationNs           int64              `json:"duration_ns"`
	SpanCount            int                `json:"span_count"`
	TraceCount           int                `json:"trace_count"`
	TraceCountCompleted  int                `json:"trace_count_completed"`
	LogCount             int                `json:"log_count,omitempty"`
	LogCountCompleted    int                `json:"log_count_completed,omitempty"`
	MetricCount          int                `json:"metric_count,omitempty"`
	MetricCountCompleted int                `json:"metric_count_completed,omitempty"`
	MetricData           *json.RawMessage   `json:"metric_data,omitempty"`
	Scenario             string             `json:"scenario,omitempty"`
	Source               string             `json:"source,omitempty"`
	Error                string             `json:"error,omitempty"`
	TraceData            *json.RawMessage   `json:"trace_data,omitempty"`
	LogData              *json.RawMessage   `json:"log_data,omitempty"`
	Endpoints            []string           `json:"endpoints"`
	Insecure             bool               `json:"insecure"`
	Output               string             `json:"output"`
	cancel               context.CancelFunc // not exported, not json
	pauseCh              chan struct{}      // nil when running; non-nil live channel when paused; not exported
}

func (r *Run) Pause() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Status != RunStatusRunning {
		return
	}
	r.Status = RunStatusPaused
	r.pauseCh = make(chan struct{})
}

func (r *Run) Resume() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Status != RunStatusPaused {
		return
	}
	r.Status = RunStatusRunning
	close(r.pauseCh)
	r.pauseCh = nil
}

func (r *Run) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	if r.Status == RunStatusPending || r.Status == RunStatusRunning || r.Status == RunStatusPaused {
		r.Status = RunStatusStopped
		r.Error = "stopped"
		now := time.Now()
		r.CompletedAt = &now
	}
}

func (r *Run) snapshot() *Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := &Run{
		ID: r.ID, Status: r.Status, Config: r.Config,
		StartedAt: r.StartedAt, CompletedAt: r.CompletedAt,
		DurationNs: r.DurationNs, SpanCount: r.SpanCount,
		TraceCount: r.TraceCount, TraceCountCompleted: r.TraceCountCompleted,
		LogCount: r.LogCount, LogCountCompleted: r.LogCountCompleted,
		MetricCount: r.MetricCount, MetricCountCompleted: r.MetricCountCompleted,
		Scenario: r.Scenario, Error: r.Error,
		Endpoints: append([]string(nil), r.Endpoints...),
		Insecure:  r.Insecure, Output: r.Output,
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
	mu    sync.Mutex
	spans []*tracev1.ResourceSpans
}

func (e *bufferExporter) Export(_ context.Context, spans []*tracev1.ResourceSpans) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, spans...)
	return nil
}

func (e *bufferExporter) Shutdown(_ context.Context) error {
	return nil
}

type bufferLogExporter struct {
	mu   sync.Mutex
	logs []*logsv1.ResourceLogs
}

func (e *bufferLogExporter) Export(_ context.Context, logs []*logsv1.ResourceLogs) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.logs = append(e.logs, logs...)
	return nil
}

func (e *bufferLogExporter) Shutdown(_ context.Context) error {
	return nil
}

type bufferMetricExporter struct {
	mu      sync.Mutex
	metrics []*metricspb.ResourceMetrics
}

func (e *bufferMetricExporter) Export(_ context.Context, metrics []*metricspb.ResourceMetrics) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.metrics = append(e.metrics, metrics...)
	return nil
}

func (e *bufferMetricExporter) Shutdown(_ context.Context) error { return nil }

type multiExporter []export.TraceExporter

func (e multiExporter) Export(ctx context.Context, spans []*tracev1.ResourceSpans) error {
	for _, exp := range e {
		if err := exp.Export(ctx, spans); err != nil {
			return err
		}
	}
	return nil
}

func (e multiExporter) Shutdown(ctx context.Context) error {
	for _, exp := range e {
		if err := exp.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

type RunStore struct {
	mu   sync.RWMutex
	runs map[string]*Run
}

func NewRunStore() *RunStore {
	return &RunStore{runs: make(map[string]*Run)}
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
		result = append(result, r.snapshot())
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
	opts := protojson.MarshalOptions{UseProtoNames: false, Indent: "  "}
	data, err := opts.Marshal(req)
	if err != nil {
		return nil, err
	}
	return export.FixTraceSpanIDsForMarshal(data)
}

func marshalLogJSON(logs []*logsv1.ResourceLogs) ([]byte, error) {
	req := &collectorlogsv1.ExportLogsServiceRequest{ResourceLogs: logs}
	opts := protojson.MarshalOptions{UseProtoNames: false, Indent: "  "}
	return opts.Marshal(req)
}

func marshalMetricJSON(metrics []*metricspb.ResourceMetrics) ([]byte, error) {
	req := &collectormetricsv1.ExportMetricsServiceRequest{ResourceMetrics: metrics}
	opts := protojson.MarshalOptions{UseProtoNames: false, Indent: "  "}
	return opts.Marshal(req)
}

type multiLogExporter []export.LogExporter

func (e multiLogExporter) Export(ctx context.Context, logs []*logsv1.ResourceLogs) error {
	for _, exp := range e {
		if err := exp.Export(ctx, logs); err != nil {
			return err
		}
	}
	return nil
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
	for _, exp := range e {
		if err := exp.Export(ctx, metrics); err != nil {
			return err
		}
	}
	return nil
}
func (e multiMetricExporter) Shutdown(ctx context.Context) error {
	for _, exp := range e {
		if err := exp.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

func StartRun(ctx context.Context, store *RunStore, cfg gen.Config, endpoints []string, insecure bool, output string) (*Run, error) {
	id := fmt.Sprintf("run-%d", runIDCounter.Add(1))
	scenario := ""
	if cfg.Scenario != "" {
		scenario = cfg.Scenario
	}
	epCopy := append([]string(nil), endpoints...)
	run := &Run{
		ID: id, Status: RunStatusPending, Config: cfg,
		Scenario: scenario, StartedAt: time.Now(), TraceCount: cfg.TraceCount,
		Endpoints: epCopy, Insecure: insecure, Output: output,
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
	store.Add(run)

	var genCtx context.Context
	var cancel context.CancelFunc

	// When multiple signals are enabled, don't use a deadline context
	// because signals generate sequentially and would time out.
	// The Duration in cfg controls the trace generator's rate limiting internally.
	multiSignal := (cfg.IncludesTraces() && cfg.IncludesLogs()) ||
		(cfg.IncludesTraces() && cfg.IncludesMetrics()) ||
		(cfg.IncludesLogs() && cfg.IncludesMetrics())

	if cfg.Duration > 0 && !multiSignal {
		genCtx, cancel = context.WithTimeout(context.Background(), cfg.Duration)
	} else {
		genCtx, cancel = context.WithCancel(context.Background())
	}

	run.mu.Lock()
	run.cancel = cancel
	run.mu.Unlock()
	go executeRun(run, store, cfg, genCtx)
	return run, nil
}

func (s *RunStore) StopRun(id string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r, ok := s.runs[id]; ok {
		r.Stop()
	}
}

func (s *RunStore) PauseRun(id string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r, ok := s.runs[id]; ok {
		r.Pause()
	}
}

func (s *RunStore) ResumeRun(id string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r, ok := s.runs[id]; ok {
		r.Resume()
	}
}

func (s *RunStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.runs, id)
}

func executeRun(run *Run, store *RunStore, cfg gen.Config, genCtx context.Context) {
	run.mu.Lock()
	run.Status = RunStatusRunning
	run.mu.Unlock()

	start := time.Now()
	buf := &bufferExporter{}
	logBuf := &bufferLogExporter{}
	metricBuf := &bufferMetricExporter{}
	var traceErr, logErr, metricErr error

	// Determine which signals to generate.
	doTraces := cfg.IncludesTraces()
	doLogs := cfg.IncludesLogs()
	doMetrics := cfg.IncludesMetrics()

	// --- TRACE GENERATION ---
	if doTraces {
		exp := export.TraceExporter(buf)
		if run.Output == "otlp" {
			otlpExporters := make(map[region.Name]export.TraceExporter)
			for _, reg := range region.All() {
				ep := region.GetEndpoint(reg)
				otlpExp, err := export.NewOTLPExporter(ep, run.Insecure, nil)
				if err != nil {
					run.mu.Lock()
					run.Status = RunStatusFailed
					run.Error = fmt.Sprintf("create OTLP exporter for %s: %v", reg, err)
					run.DurationNs = time.Since(start).Nanoseconds()
					now := time.Now()
					run.CompletedAt = &now
					run.mu.Unlock()
					return
				}
				// Wrap exporter to use fresh context with timeout for each export
				otlpExporters[reg] = export.WithExportTimeout(otlpExp, 30*time.Second)
			}
			router := export.NewEndpointRouter(otlpExporters)
			exp = multiExporter{buf, router}
		}

		g := gen.NewGenerator(cfg)
		g.OnProgress = func(completed, total int) {
			run.mu.Lock()
			run.TraceCountCompleted = completed
			buf.mu.Lock()
			spanTotal := 0
			for _, rs := range buf.spans {
				for _, ss := range rs.ScopeSpans {
					spanTotal += len(ss.Spans)
				}
			}
			run.SpanCount = spanTotal
			buf.mu.Unlock()
			run.mu.Unlock()

			run.mu.Lock()
			ch := run.pauseCh
			run.mu.Unlock()
			if ch != nil {
				select {
				case <-ch:
				case <-genCtx.Done():
				}
			}
		}
		traceErr = g.Run(genCtx, exp)
	}

	// --- LOG GENERATION ---
	if doLogs && traceErr == nil {
		logging.Debugf("Starting log generation. doLogs=%v, traceErr=%v", doLogs, traceErr)
		logging.Debugf("cfg.LogConfig = %+v", cfg.LogConfig)
		logging.Debugf("len(buf.spans) = %d", len(buf.spans))

		logExp := export.LogExporter(logBuf)
		if run.Output == "otlp" {
			otlpLogExporters := make(map[region.Name]export.LogExporter)
			for _, reg := range region.All() {
				ep := region.GetEndpoint(reg)
				otlpLogExp, err := export.NewOTLPLogExporter(ep, run.Insecure, nil)
				if err != nil {
					run.mu.Lock()
					run.Status = RunStatusFailed
					run.Error = fmt.Sprintf("create OTLP log exporter for %s: %v", reg, err)
					run.DurationNs = time.Since(start).Nanoseconds()
					now := time.Now()
					run.CompletedAt = &now
					run.mu.Unlock()
					return
				}
				// Wrap log exporter to use fresh context with timeout for each export
				otlpLogExporters[reg] = export.WithLogExportTimeout(otlpLogExp, 30*time.Second)
			}
			router := export.NewLogEndpointRouter(otlpLogExporters)
			logExp = multiLogExporter{logBuf, router}
		}

		lg := gen.NewLogGenerator(cfg.LogConfig)
		lg.OnProgress = func(completed, total int) {
			run.mu.Lock()
			run.LogCountCompleted = completed
			if run.LogCount == 0 {
				run.LogCount = total
			}
			run.mu.Unlock()
		}

		var traceSpans []*tracev1.ResourceSpans
		if cfg.IncludesTraces() && cfg.IncludesLogs() {
			traceSpans = buf.spans
			logging.Debugf("Passing %d ResourceSpans to log generator for correlation", len(traceSpans))
		} else {
			logging.Debugf("No trace correlation - generating standalone logs")
		}
		logErr = lg.Run(genCtx, logExp, traceSpans)
		logging.Debugf("Log generation completed. logErr=%v", logErr)
	}

	// --- METRIC GENERATION ---
	if doMetrics && traceErr == nil && logErr == nil {
		logging.Debugf("Starting metric generation. doMetrics=%v, traceErr=%v, logErr=%v", doMetrics, traceErr, logErr)
		logging.Debugf("cfg.MetricConfig = %+v", cfg.MetricConfig)

		metricExp := export.MetricExporter(metricBuf)
		if run.Output == "otlp" {
			otlpMetricExporters := make(map[region.Name]export.MetricExporter)
			for _, reg := range region.All() {
				ep := region.GetEndpoint(reg)
				otlpMetricExp, err := export.NewOTLPMetricExporter(ep, run.Insecure, nil)
				if err != nil {
					run.mu.Lock()
					run.Status = RunStatusFailed
					run.Error = fmt.Sprintf("create OTLP metric exporter for %s: %v", reg, err)
					run.DurationNs = time.Since(start).Nanoseconds()
					now := time.Now()
					run.CompletedAt = &now
					run.mu.Unlock()
					return
				}
				// Wrap metric exporter to use fresh context with timeout for each export
				otlpMetricExporters[reg] = export.WithMetricExportTimeout(otlpMetricExp, 30*time.Second)
			}
			router := export.NewMetricEndpointRouter(otlpMetricExporters)
			metricExp = multiMetricExporter{metricBuf, router}
		}

		mg := gen.NewMetricGenerator(cfg.MetricConfig)
		mg.OnProgress = func(completed, total int) {
			run.mu.Lock()
			run.MetricCountCompleted = completed
			if run.MetricCount == 0 {
				run.MetricCount = total
			}
			run.mu.Unlock()
		}
		metricErr = mg.Run(genCtx, metricExp)
	}

	elapsed := time.Since(start).Nanoseconds()
	run.mu.Lock()
	run.DurationNs = elapsed

	// Count log records from log buffer.
	logTotal := 0
	for _, rl := range logBuf.logs {
		for _, sl := range rl.ScopeLogs {
			logTotal += len(sl.LogRecords)
		}
	}
	if logTotal > 0 {
		run.LogCountCompleted = logTotal
		if run.LogCount == 0 {
			run.LogCount = logTotal
		}
	}

	// Count metric data points.
	metricTotal := 0
	if metricBuf != nil {
		for _, rm := range metricBuf.metrics {
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					switch m.Data.(type) {
					case *metricspb.Metric_Gauge:
						metricTotal += len(m.GetGauge().DataPoints)
					case *metricspb.Metric_Sum:
						metricTotal += len(m.GetSum().DataPoints)
					case *metricspb.Metric_Histogram:
						metricTotal += len(m.GetHistogram().DataPoints)
					}
				}
			}
		}
	}
	if metricTotal > 0 {
		run.MetricCountCompleted = metricTotal
		if run.MetricCount == 0 {
			run.MetricCount = metricTotal
		}
	}

	// Check for cancellation / stop first.
	if traceErr == context.Canceled || logErr == context.Canceled || metricErr == context.Canceled {
		if run.Status == RunStatusStopped {
			total := 0
			for _, rs := range buf.spans {
				for _, ss := range rs.ScopeSpans {
					total += len(ss.Spans)
				}
			}
			run.SpanCount = total
			if len(buf.spans) > 0 {
				data, err := marshalTraceJSON(buf.spans)
				if err == nil {
					raw := json.RawMessage(data)
					run.TraceData = &raw
				}
			}
			if len(logBuf.logs) > 0 {
				data, err := marshalLogJSON(logBuf.logs)
				if err == nil {
					raw := json.RawMessage(data)
					run.LogData = &raw
				}
			}
			if len(metricBuf.metrics) > 0 {
				data, err := marshalMetricJSON(metricBuf.metrics)
				if err == nil {
					raw := json.RawMessage(data)
					run.MetricData = &raw
				}
			}
			run.mu.Unlock()
			return
		}
	}

	// Duration expired — clean completion, not failure.
	if (traceErr == context.DeadlineExceeded || logErr == context.DeadlineExceeded || metricErr == context.DeadlineExceeded) && cfg.Duration > 0 {
		traceErr = nil
		logErr = nil
		metricErr = nil
	}

	// Determine the primary error.
	err := traceErr
	if err == nil {
		err = logErr
	}
	if err == nil {
		err = metricErr
	}

	if err != nil {
		if err == context.Canceled {
			run.Error = "cancelled"
		} else {
			run.Error = err.Error()
		}
		run.Status = RunStatusFailed
		now := time.Now()
		run.CompletedAt = &now
		run.mu.Unlock()
		return
	}

	// Finalise trace data.
	totalSpans := 0
	for _, rs := range buf.spans {
		for _, ss := range rs.ScopeSpans {
			totalSpans += len(ss.Spans)
		}
	}
	run.SpanCount = totalSpans

	if len(buf.spans) > 0 {
		data, err := marshalTraceJSON(buf.spans)
		if err == nil {
			raw := json.RawMessage(data)
			run.TraceData = &raw
		}
	}

	// Finalise log data.
	if len(logBuf.logs) > 0 {
		data, err := marshalLogJSON(logBuf.logs)
		if err == nil {
			raw := json.RawMessage(data)
			run.LogData = &raw
		}
	}

	// Finalise metric data.
	if len(metricBuf.metrics) > 0 {
		data, err := marshalMetricJSON(metricBuf.metrics)
		if err == nil {
			raw := json.RawMessage(data)
			run.MetricData = &raw
		}
	}

	run.Status = RunStatusCompleted
	now := time.Now()
	run.CompletedAt = &now
	run.mu.Unlock()
}
