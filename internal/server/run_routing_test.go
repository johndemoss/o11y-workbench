package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/region"
	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

type routingCollector struct {
	traces, logs, metrics atomic.Int64
	connections           atomic.Int64
}

func (c *routingCollector) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	return ctx
}
func (c *routingCollector) HandleRPC(context.Context, stats.RPCStats) {}
func (c *routingCollector) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}
func (c *routingCollector) HandleConn(_ context.Context, s stats.ConnStats) {
	switch s.(type) {
	case *stats.ConnBegin:
		c.connections.Add(1)
	case *stats.ConnEnd:
		c.connections.Add(-1)
	}
}

type routingTraces struct {
	collectortracev1.UnimplementedTraceServiceServer
	c *routingCollector
}

func (r routingTraces) Export(_ context.Context, req *collectortracev1.ExportTraceServiceRequest) (*collectortracev1.ExportTraceServiceResponse, error) {
	for _, rs := range req.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			r.c.traces.Add(int64(len(ss.Spans)))
		}
	}
	return &collectortracev1.ExportTraceServiceResponse{}, nil
}

type routingLogs struct {
	collectorlogsv1.UnimplementedLogsServiceServer
	c *routingCollector
}

func (r routingLogs) Export(_ context.Context, req *collectorlogsv1.ExportLogsServiceRequest) (*collectorlogsv1.ExportLogsServiceResponse, error) {
	for _, rs := range req.ResourceLogs {
		for _, ss := range rs.ScopeLogs {
			r.c.logs.Add(int64(len(ss.LogRecords)))
		}
	}
	return &collectorlogsv1.ExportLogsServiceResponse{}, nil
}

type routingMetrics struct {
	collectormetricsv1.UnimplementedMetricsServiceServer
	c *routingCollector
}

func (r routingMetrics) Export(_ context.Context, req *collectormetricsv1.ExportMetricsServiceRequest) (*collectormetricsv1.ExportMetricsServiceResponse, error) {
	for _, rs := range req.ResourceMetrics {
		for _, ss := range rs.ScopeMetrics {
			r.c.metrics.Add(int64(len(ss.Metrics)))
		}
	}
	return &collectormetricsv1.ExportMetricsServiceResponse{}, nil
}

func startRoutingCollector(t *testing.T) (*routingCollector, string) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c := &routingCollector{}
	gs := grpc.NewServer(grpc.StatsHandler(c))
	collectortracev1.RegisterTraceServiceServer(gs, routingTraces{c: c})
	collectorlogsv1.RegisterLogsServiceServer(gs, routingLogs{c: c})
	collectormetricsv1.RegisterMetricsServiceServer(gs, routingMetrics{c: c})
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	return c, lis.Addr().String()
}

func TestParallelRunsKeepRoutingAndCloseConnections(t *testing.T) {
	before := append([]region.Name(nil), region.All()...)
	// Restore legacy process state even when proving the pre-fix failure.
	t.Cleanup(func() { region.SetEndpoints(nil) })
	store := NewRunStore()
	s := &Server{store: store}
	collectors := make([]*routingCollector, 2)
	responses := make([]*httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for i := range collectors {
		c, endpoint := startRoutingCollector(t)
		collectors[i] = c
		body, err := json.Marshal(map[string]any{
			"seed": 42, "trace_count": 20, "rate": 100, "topology": "flat", "span_count": 1,
			"output": "otlp", "endpoints": []string{endpoint},
			"signals":       map[string]bool{"traces": true, "logs": true, "metrics": true},
			"log_config":    map[string]int{"logs_per_span": 1},
			"metric_config": map[string]any{"metric_count": 1, "gauge_ratio": 1},
		})
		if err != nil {
			t.Fatal(err)
		}
		responses[i] = httptest.NewRecorder()
		wg.Go(func() {
			s.handleStartRun(responses[i], httptest.NewRequest("POST", "/api/runs", bytes.NewReader(body)))
		})
	}
	wg.Wait()
	for _, response := range responses {
		if response.Code != 202 {
			t.Fatalf("start status=%d", response.Code)
		}
		var run Run
		if err := json.Unmarshal(response.Body.Bytes(), &run); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { store.StopRun(run.ID) })
	}
	if !reflect.DeepEqual(before, region.All()) {
		t.Error("HTTP requests changed process-global routing")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		done := true
		for _, c := range collectors {
			if c.traces.Load() != 20 || c.logs.Load() != 20 || c.metrics.Load() != 1 || c.connections.Load() != 0 {
				done = false
			}
		}
		if done {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	for i, c := range collectors {
		t.Errorf("collector %d: traces=%d logs=%d metrics=%d connections=%d; want 20/20/1/0",
			i, c.traces.Load(), c.logs.Load(), c.metrics.Load(), c.connections.Load())
	}
}

func TestRunRoutesMultipleDestinationsAndFlatFallback(t *testing.T) {
	for _, flat := range []bool{false, true} {
		name := "topology"
		if flat {
			name = "flat workload"
		}
		t.Run(name, func(t *testing.T) {
			a, epA := startRoutingCollector(t)
			b, epB := startRoutingCollector(t)
			store := NewRunStore()
			cfg := gen.Config{Seed: 42, TraceCount: 20, Rate: 1000, SpanCount: 1, Topology: gen.TopologyFlat, Signals: gen.Signals{Traces: true}}
			if flat {
				cfg.Workload = gen.WorkloadProfile{Shape: "flat", ItemsPerRequest: 1}
			}
			run, err := StartRun(context.Background(), store, cfg, []string{epA, epB}, true, "otlp")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { store.StopRun(run.ID) })
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if store.Get(run.ID).Status == RunStatusCompleted && a.connections.Load() == 0 && b.connections.Load() == 0 {
					if got := a.traces.Load() + b.traces.Load(); got != 20 {
						t.Fatalf("delivered=%d, want 20", got)
					}
					if flat {
						if a.traces.Load() != 20 || b.traces.Load() != 0 {
							t.Fatal("flat payload without cloud.region did not use first destination")
						}
					} else if a.traces.Load() == 0 || b.traces.Load() == 0 {
						t.Fatal("topology did not fan out to both run-local destinations")
					}
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatalf("run did not finish and close connections: status=%s", store.Get(run.ID).Status)
		})
	}
}

func TestCanceledRunClosesOTLPConnections(t *testing.T) {
	c, endpoint := startRoutingCollector(t)
	store := NewRunStore()
	cfg := gen.Config{Seed: 42, Rate: 100, SpanCount: 1, Topology: gen.TopologyFlat, Signals: gen.Signals{Traces: true}}
	run, err := StartRun(context.Background(), store, cfg, []string{endpoint}, true, "otlp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.StopRun(run.ID) })
	deadline := time.Now().Add(5 * time.Second)
	for c.traces.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no trace reached collector")
		}
		time.Sleep(10 * time.Millisecond)
	}
	store.StopRun(run.ID)
	for c.connections.Load() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("canceled run retained OTLP connections")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
