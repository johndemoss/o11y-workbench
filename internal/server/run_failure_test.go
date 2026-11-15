package server

import (
	"context"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func waitForTerminal(t *testing.T, store *RunStore, id string, timeout time.Duration) *Run {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		snap := store.Get(id)
		if snap == nil {
			t.Fatal("run disappeared")
		}
		if isTerminalStatus(snap.Status) {
			return snap
		}
		if time.Now().After(deadline) {
			t.Fatalf("run %s did not finish: status=%s error=%s", id, snap.Status, snap.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestFiniteLogCorrelationStreamsReferences proves finite correlated runs use
// the streaming path and still retain every generated log record.
func TestFiniteLogCorrelationStreamsReferences(t *testing.T) {
	store := NewRunStore()
	defer store.Close()

	cfg := gen.Config{
		Seed: 3,
		Signals: gen.Signals{
			Traces: true,
			Logs:   true,
		},
		TraceCount: 5,
		Topology:   gen.TopologyFlat,
		SpanCount:  2,
		Rate:       0,
		LogConfig: gen.LogConfig{
			Enabled:     true,
			LogsPerSpan: 1,
			LogCount:    10,
		},
	}

	run, err := StartRun(context.Background(), store, cfg, nil, true, "file")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	snap := waitForTerminal(t, store, run.ID, 15*time.Second)
	if snap.Status != RunStatusCompleted {
		t.Fatalf("status = %s, error = %s", snap.Status, snap.Error)
	}
	if snap.LogRecordCount != 10 {
		t.Fatalf("log record count = %d, want 10", snap.LogRecordCount)
	}
	if snap.LogData == nil {
		t.Fatal("correlated logs were not retained")
	}
}

// TestUnlimitedRunFailsOnCorrelatedLogError proves a failing signal stops an
// indefinite run promptly and the originating error is published.
func TestUnlimitedRunFailsOnCorrelatedLogError(t *testing.T) {
	store := NewRunStore()
	defer store.Close()

	cfg := gen.Config{
		Seed: 11,
		Signals: gen.Signals{
			Traces: true,
			Logs:   true,
		},
		TraceCount: 0,
		Topology:   gen.TopologyFlat,
		SpanCount:  2,
		Rate:       50,
		LogConfig: gen.LogConfig{
			Enabled:      true,
			LogsPerSpan:  1,
			LogCount:     10,
			BodyTemplate: "{{",
		},
	}

	run, err := StartRun(context.Background(), store, cfg, nil, true, "file")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	snap := waitForTerminal(t, store, run.ID, 10*time.Second)
	if snap.Status != RunStatusFailed {
		t.Fatalf("status = %s, want failed", snap.Status)
	}
	if !strings.Contains(snap.Error, "parse body template") {
		t.Fatalf("error = %q, want the log template failure", snap.Error)
	}
}

type lifecycleCollector struct {
	collectortracev1.UnimplementedTraceServiceServer
	traces           atomic.Int64
	logRecords       atomic.Int64
	traceReached     chan struct{}
	logEntered       chan struct{}
	releaseLog       chan struct{}
	traceOnce        sync.Once
	logOnce          sync.Once
	failTrace        bool
	metricExports    atomic.Int64
	metricBeforeLogs atomic.Bool
	logFinished      atomic.Bool
}

func (c *lifecycleCollector) Export(ctx context.Context, _ *collectortracev1.ExportTraceServiceRequest) (*collectortracev1.ExportTraceServiceResponse, error) {
	n := c.traces.Add(1)
	if c.failTrace && n == 2 {
		select {
		case <-c.logEntered:
			return nil, status.Error(codes.InvalidArgument, "trace failure")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if n == 66 {
		c.traceOnce.Do(func() { close(c.traceReached) })
	}
	return &collectortracev1.ExportTraceServiceResponse{}, nil
}

type lifecycleLogs struct {
	collectorlogsv1.UnimplementedLogsServiceServer
	c *lifecycleCollector
}

func (l lifecycleLogs) Export(ctx context.Context, req *collectorlogsv1.ExportLogsServiceRequest) (*collectorlogsv1.ExportLogsServiceResponse, error) {
	l.c.logOnce.Do(func() { close(l.c.logEntered) })
	select {
	case <-l.c.releaseLog:
		for _, resource := range req.ResourceLogs {
			for _, scope := range resource.ScopeLogs {
				l.c.logRecords.Add(int64(len(scope.LogRecords)))
			}
		}
		l.c.logFinished.Store(true)
		return &collectorlogsv1.ExportLogsServiceResponse{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type lifecycleMetrics struct {
	collectormetricsv1.UnimplementedMetricsServiceServer
	c *lifecycleCollector
}

func (m lifecycleMetrics) Export(context.Context, *collectormetricsv1.ExportMetricsServiceRequest) (*collectormetricsv1.ExportMetricsServiceResponse, error) {
	m.c.metricExports.Add(1)
	if m.c.logRecords.Load() != 7000 {
		m.c.metricBeforeLogs.Store(true)
	}
	return &collectormetricsv1.ExportMetricsServiceResponse{}, nil
}

func startLifecycleCollector(t *testing.T, c *lifecycleCollector) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	collectortracev1.RegisterTraceServiceServer(s, c)
	collectorlogsv1.RegisterLogsServiceServer(s, lifecycleLogs{c: c})
	collectormetricsv1.RegisterMetricsServiceServer(s, lifecycleMetrics{c: c})
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	return lis.Addr().String()
}

func awaitLifecycleEvent(t *testing.T, event <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-event:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func TestFiniteCorrelationBackpressureAndMetricOrder(t *testing.T) {
	c := &lifecycleCollector{traceReached: make(chan struct{}), logEntered: make(chan struct{}), releaseLog: make(chan struct{})}
	endpoint := startLifecycleCollector(t, c)
	store := NewRunStore()
	defer store.Close()
	var release sync.Once
	defer release.Do(func() { close(c.releaseLog) })
	cfg := gen.Config{
		Seed: 3, Signals: gen.Signals{Traces: true, Logs: true, Metrics: true},
		TraceCount: 70, Topology: gen.TopologyFlat, SpanCount: 1,
		LogConfig:    gen.LogConfig{Enabled: true, LogsPerSpan: 100},
		MetricConfig: gen.MetricConfig{Enabled: true, MetricCount: 1, GaugeRatio: 1},
	}
	run, err := StartRun(context.Background(), store, cfg, []string{endpoint}, true, "otlp")
	if err != nil {
		t.Fatal(err)
	}
	awaitLifecycleEvent(t, c.logEntered, "blocked log export")
	awaitLifecycleEvent(t, c.traceReached, "66th trace export")
	if c.metricBeforeLogs.Load() {
		t.Fatal("metrics started before correlated logs completed")
	}
	release.Do(func() { close(c.releaseLog) })
	snap := waitForTerminal(t, store, run.ID, 15*time.Second)
	if snap.Status != RunStatusCompleted || snap.SpanCount != 70 || snap.LogRecordCount != 7000 {
		t.Fatalf("status=%s error=%s spans=%d logs=%d", snap.Status, snap.Error, snap.SpanCount, snap.LogRecordCount)
	}
	if c.metricBeforeLogs.Load() || !c.logFinished.Load() || c.metricExports.Load() != 1 || snap.MetricRecordCount != 1 {
		t.Fatalf("metric ordering: early=%v logs_finished=%v exported_logs=%d metric_exports=%d retained_metrics=%d",
			c.metricBeforeLogs.Load(), c.logFinished.Load(), c.logRecords.Load(), c.metricExports.Load(), snap.MetricRecordCount)
	}
}

func TestUnlimitedTraceFailureCancelsIndependentLogs(t *testing.T) {
	c := &lifecycleCollector{traceReached: make(chan struct{}), logEntered: make(chan struct{}), releaseLog: make(chan struct{}), failTrace: true}
	endpoint := startLifecycleCollector(t, c)
	store := NewRunStore()
	defer store.Close()
	cfg := gen.Config{
		Seed: 3, Signals: gen.Signals{Traces: true, Logs: true},
		Topology: gen.TopologyFlat, SpanCount: 1,
		LogConfig: gen.LogConfig{Enabled: true, LogCount: 1000},
	}
	run, err := StartRun(context.Background(), store, cfg, []string{endpoint}, true, "otlp")
	if err != nil {
		t.Fatal(err)
	}
	awaitLifecycleEvent(t, c.logEntered, "independent log export")
	snap := waitForTerminal(t, store, run.ID, 10*time.Second)
	if snap.Status != RunStatusFailed || !strings.Contains(snap.Error, "trace failure") {
		t.Fatalf("status=%s error=%q, want trace failure", snap.Status, snap.Error)
	}
	if snap.TraceData == nil || snap.LogData == nil {
		t.Fatal("failed run did not retain exported captures")
	}
}

func TestFiniteCorrelatedRunDeadlineDuringExportCompletes(t *testing.T) {
	store := NewRunStore()
	defer store.Close()
	cfg := gen.Config{
		Seed: 3, Signals: gen.Signals{Traces: true, Logs: true},
		TraceCount: 5, Topology: gen.TopologyFlat, SpanCount: 1,
		Duration:  300 * time.Millisecond,
		LogConfig: gen.LogConfig{Enabled: true, LogsPerSpan: 1},
	}
	run, err := StartRun(context.Background(), store, cfg, []string{"10.255.255.255:4317"}, true, "otlp")
	if err != nil {
		t.Fatal(err)
	}
	snap := waitForTerminal(t, store, run.ID, 10*time.Second)
	if snap.Status != RunStatusCompleted || snap.Error != "" {
		t.Fatalf("status=%s error=%q, want completed without error", snap.Status, snap.Error)
	}
}
