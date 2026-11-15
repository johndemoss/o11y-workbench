package server

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/export"
	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/region"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

func pauseResult(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for pause")
		return nil
	}
}

func TestOverlappingPauseCancellationPreservesBarrier(t *testing.T) {
	gate := newRunPauseGate()
	run := &Run{Status: RunStatusRunning, gate: gate}
	if err := gate.beginExport(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := make(chan error, 1), make(chan error, 1)
	go func() { a <- run.Pause(ctx) }()
	waitShutdownCondition(t, func() bool {
		gate.mu.Lock()
		defer gate.mu.Unlock()
		return gate.sequence == 1
	}, "first pause did not close admission")
	go func() { b <- run.Pause(context.Background()) }()
	waitShutdownCondition(t, func() bool {
		gate.mu.Lock()
		defer gate.mu.Unlock()
		return gate.sequence == 2
	}, "second pause did not close admission")
	cancel()
	if err := pauseResult(t, a); !errors.Is(err, context.Canceled) {
		t.Fatalf("first pause: %v", err)
	}
	select {
	case err := <-b:
		t.Fatalf("pause returned before in-flight export completed: %v", err)
	default:
	}
	gate.endExport()
	if err := pauseResult(t, b); err != nil {
		t.Fatal(err)
	}
	blocked, stop := context.WithCancel(context.Background())
	stop()
	if err := gate.beginExport(blocked); !errors.Is(err, context.Canceled) {
		t.Fatalf("export admitted while paused: %v", err)
	}
	if err := run.Resume(); err != nil {
		t.Fatal(err)
	}
	if err := gate.beginExport(context.Background()); err != nil {
		t.Fatal(err)
	}
	gate.endExport()
}

func TestPauseRequiresAcknowledgedLiveGate(t *testing.T) {
	tests := []struct {
		name string
		gate func(t *testing.T) *runPauseGate
	}{
		{name: "missing", gate: func(*testing.T) *runPauseGate { return nil }},
		{name: "unacknowledged", gate: func(*testing.T) *runPauseGate { return newRunPauseGate() }},
		{name: "stopped", gate: func(t *testing.T) *runPauseGate {
			gate := newRunPauseGate()
			run := &Run{Status: RunStatusRunning, gate: gate}
			if err := run.Pause(context.Background()); err != nil {
				t.Fatal(err)
			}
			gate.stop()
			return gate
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := &Run{Status: RunStatusPaused, gate: tt.gate(t)}
			if err := run.Pause(context.Background()); err == nil {
				t.Fatal("pause succeeded without an acknowledged live gate")
			}
		})
	}
	run := &Run{Status: RunStatusRunning, gate: newRunPauseGate()}
	if err := run.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := run.Pause(context.Background()); err != nil {
		t.Fatalf("repeated acknowledged pause: %v", err)
	}
	run.Stop()
}

type failingPauseLogExporter struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
	err     error
}

func (e *failingPauseLogExporter) Export(context.Context, []*logsv1.ResourceLogs) error {
	call := e.calls.Add(1)
	e.entered <- struct{}{}
	if call == 1 {
		<-e.release
	}
	return e.err
}

func (*failingPauseLogExporter) Shutdown(context.Context) error { return nil }

type receiptFailingPauseLogExporter struct{ *failingPauseLogExporter }

func (e receiptFailingPauseLogExporter) ExportWithReceipt(ctx context.Context, logs []*logsv1.ResourceLogs) (export.ExportReceipt, error) {
	err := e.Export(ctx, logs)
	return export.ReceiptRPCFailure(export.CountLogRecords(logs), err), err
}

type partialPauseLogExporter struct{}

func (partialPauseLogExporter) Export(context.Context, []*logsv1.ResourceLogs) error { return nil }
func (partialPauseLogExporter) Shutdown(context.Context) error                       { return nil }
func (partialPauseLogExporter) ExportWithReceipt(context.Context, []*logsv1.ResourceLogs) (export.ExportReceipt, error) {
	return export.ReceiptFromRejected(5, 2, "partial"), nil
}

func TestGatedLogExporterPreservesReceipt(t *testing.T) {
	exp := &gatedLogExporter{gate: newRunPauseGate(), inner: partialPauseLogExporter{}}
	receipt, err := exp.ExportWithReceipt(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Outcome != export.OutcomePartialRejection || receipt.AcceptedItems != 3 || receipt.RejectedItems != 2 {
		t.Fatalf("partial receipt lost: %+v", receipt)
	}
}

func TestFailedLogExportRetryBypassesPendingPause(t *testing.T) {
	for _, withReceipt := range []bool{false, true} {
		name := "export"
		if withReceipt {
			name = "receipt"
		}
		t.Run(name, func(t *testing.T) {
			gate := newRunPauseGate()
			defer gate.stop()
			exportErr := errors.New("log export failed")
			inner := &failingPauseLogExporter{
				entered: make(chan struct{}, 2),
				release: make(chan struct{}, 1),
				err:     exportErr,
			}
			defer func() {
				select {
				case inner.release <- struct{}{}:
				default:
				}
			}()
			var wrapped export.LogExporter = inner
			if withReceipt {
				wrapped = receiptFailingPauseLogExporter{inner}
			}
			exp := &gatedLogExporter{gate: gate, inner: wrapped}
			first := make(chan error, 1)
			go func() { first <- exp.Export(context.Background(), nil) }()
			select {
			case <-inner.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("first export did not start")
			}
			run := &Run{Status: RunStatusRunning, gate: gate}
			paused := make(chan error, 1)
			go func() { paused <- run.Pause(context.Background()) }()
			waitShutdownCondition(t, func() bool {
				gate.mu.Lock()
				defer gate.mu.Unlock()
				return gate.paused
			}, "pause did not close admission")
			inner.release <- struct{}{}
			if err := pauseResult(t, first); !errors.Is(err, exportErr) {
				t.Fatalf("first export: %v", err)
			}
			if err := pauseResult(t, paused); !errors.Is(err, context.Canceled) {
				t.Fatalf("pause acknowledged a failed export: %v", err)
			}
			retry := make(chan error, 1)
			go func() { retry <- exp.Export(context.Background(), nil) }()
			select {
			case <-inner.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("retry blocked at paused gate")
			}
			if err := pauseResult(t, retry); !errors.Is(err, exportErr) {
				t.Fatalf("retry: %v", err)
			}
		})
	}
}

func TestPauseResumeTransitionsAndStop(t *testing.T) {
	gate := newRunPauseGate()
	run := &Run{Status: RunStatusRunning, gate: gate}
	for range 100 {
		if err := run.Pause(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := run.Resume(); err != nil {
			t.Fatal(err)
		}
	}
	if err := gate.beginExport(context.Background()); err != nil {
		t.Fatal(err)
	}
	paused := make(chan error, 1)
	go func() { paused <- run.Pause(context.Background()) }()
	waitShutdownCondition(t, func() bool {
		gate.mu.Lock()
		defer gate.mu.Unlock()
		return gate.paused
	}, "pause did not close admission")
	run.Stop()
	if err := pauseResult(t, paused); !errors.Is(err, context.Canceled) {
		t.Fatalf("pause after stop: %v", err)
	}
	gate.endExport()
}

func TestResumeCannotReopenLaterAcknowledgedPause(t *testing.T) {
	gate := newRunPauseGate()
	run := &Run{Status: RunStatusRunning, gate: gate}
	if err := run.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
	gate.mu.Lock()
	resumed := make(chan error, 1)
	go func() { resumed <- run.Resume() }()
	waitShutdownCondition(t, func() bool {
		if run.mu.TryLock() {
			run.mu.Unlock()
			return false
		}
		return true
	}, "resume did not start")
	paused := make(chan error, 1)
	go func() { paused <- run.Pause(context.Background()) }()
	gate.mu.Unlock()
	if err := pauseResult(t, resumed); err != nil {
		t.Fatal(err)
	}
	if err := pauseResult(t, paused); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := gate.beginExport(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("export admitted after later pause: %v", err)
	}
}

type partialPauseTraceExporter struct{}

func (partialPauseTraceExporter) Export(context.Context, []*tracev1.ResourceSpans) error { return nil }
func (partialPauseTraceExporter) Shutdown(context.Context) error                         { return nil }
func (partialPauseTraceExporter) ExportWithReceipt(_ context.Context, spans []*tracev1.ResourceSpans) (export.ExportReceipt, error) {
	return export.ReceiptFromRejected(export.CountSpans(spans), 2, "partial"), nil
}

func TestGatedTraceExporterPreservesGeneratorReceipts(t *testing.T) {
	cfg := gen.Config{Seed: 1, TraceCount: 1, Rate: 1000, Workload: gen.WorkloadProfile{
		Shape: "flat", ExperimentID: "pause-receipt", ItemsPerRequest: 10,
	}, Signals: gen.Signals{Traces: true}, Regions: []region.Name{region.RegionLocal}}
	g := gen.NewGenerator(cfg)
	if err := g.Run(context.Background(), gatedTraceExporter{gate: newRunPauseGate(), inner: partialPauseTraceExporter{}}); err != nil {
		t.Fatal(err)
	}
	stats := g.Stats()
	if stats.AttemptedItems != 10 || stats.AckedItems != 8 || stats.RejectedItems != 2 {
		t.Fatalf("partial receipt lost: %+v", stats)
	}
}

func TestPausedDurationFlushesCorrelatedLogs(t *testing.T) {
	store := NewRunStore()
	defer store.Close()
	cfg := gen.Config{Seed: 3, Signals: gen.Signals{Traces: true, Logs: true},
		Topology: gen.TopologyFlat, SpanCount: 1, Rate: 1, Duration: 3 * time.Second,
		LogConfig: gen.LogConfig{Enabled: true, LogsPerSpan: 1},
	}
	run, err := StartRun(context.Background(), store, cfg, nil, true, "file")
	if err != nil {
		t.Fatal(err)
	}
	waitShutdownCondition(t, func() bool { return store.Get(run.ID).LogCountCompleted > 0 }, "correlated log was not buffered")
	if err := store.PauseRun(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	snap := waitForTerminal(t, store, run.ID, 8*time.Second)
	if snap.Status != RunStatusCompleted || snap.LogRecordCount < 1 || snap.LogData == nil {
		t.Fatalf("duration flush: status=%s error=%q logs=%d", snap.Status, snap.Error, snap.LogRecordCount)
	}
}
