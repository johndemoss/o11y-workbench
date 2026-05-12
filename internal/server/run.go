package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"

	"github.com/johndemoss/mocktracegen/internal/export"
	"github.com/johndemoss/mocktracegen/internal/gen"
	"github.com/johndemoss/mocktracegen/internal/region"
)

type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
)

type Run struct {
	mu          sync.Mutex
	ID          string           `json:"id"`
	Status      RunStatus        `json:"status"`
	Config      gen.Config       `json:"config"`
	StartedAt   time.Time        `json:"started_at"`
	CompletedAt *time.Time       `json:"completed_at,omitempty"`
	DurationNs  int64            `json:"duration_ns"`
	SpanCount   int              `json:"span_count"`
	TraceCount  int              `json:"trace_count"`
	Scenario    string           `json:"scenario,omitempty"`
	Error       string           `json:"error,omitempty"`
	TraceData   *json.RawMessage `json:"trace_data,omitempty"`
	Endpoints   []string         `json:"endpoints"`
	Insecure    bool             `json:"insecure"`
	Output      string           `json:"output"`
}

func (r *Run) snapshot() *Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := &Run{
		ID: r.ID, Status: r.Status, Config: r.Config,
		StartedAt: r.StartedAt, CompletedAt: r.CompletedAt,
		DurationNs: r.DurationNs, SpanCount: r.SpanCount,
		TraceCount: r.TraceCount, Scenario: r.Scenario, Error: r.Error,
		Endpoints: append([]string(nil), r.Endpoints...),
		Insecure:  r.Insecure, Output: r.Output,
	}
	if r.TraceData != nil {
		data := make(json.RawMessage, len(*r.TraceData))
		copy(data, *r.TraceData)
		c.TraceData = &data
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

type multiExporter []export.Exporter

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
	r := s.runs[id]
	if r == nil {
		return nil
	}
	return r.snapshot()
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
	opts := protojson.MarshalOptions{UseProtoNames: true, Indent: "  "}
	return opts.Marshal(req)
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
	store.Add(run)
	go executeRun(run, store, cfg)
	return run, nil
}

func executeRun(run *Run, store *RunStore, cfg gen.Config) {
	run.mu.Lock()
	run.Status = RunStatusRunning
	run.mu.Unlock()

	start := time.Now()

	buf := &bufferExporter{}
	genCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp := export.Exporter(buf)
	if run.Output == "otlp" {
		otlpExporters := make(map[region.Name]export.Exporter)
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
			otlpExporters[reg] = otlpExp
		}
		router := export.NewEndpointRouter(otlpExporters)
		exp = multiExporter{buf, router}
	}

	err := gen.NewGenerator(cfg).Run(genCtx, exp)
	elapsed := time.Since(start).Nanoseconds()

	run.mu.Lock()
	run.DurationNs = elapsed

	if err != nil {
		run.Status = RunStatusFailed
		run.Error = err.Error()
		now := time.Now()
		run.CompletedAt = &now
		run.mu.Unlock()
		return
	}

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

	run.Status = RunStatusCompleted
	now := time.Now()
	run.CompletedAt = &now
	run.mu.Unlock()
}
