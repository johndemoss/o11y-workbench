package server

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/johndemoss/o11y-workbench/internal/gen"
)

func TestBufferExporter_SpanCountIncrements(t *testing.T) {
	buf := &bufferExporter{}
	rs := &tracev1.ResourceSpans{
		ScopeSpans: []*tracev1.ScopeSpans{
			{Spans: []*tracev1.Span{{Name: "a"}, {Name: "b"}}},
			{Spans: []*tracev1.Span{{Name: "c"}}},
		},
	}
	if err := buf.Export(context.Background(), []*tracev1.ResourceSpans{rs}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if got := buf.SpanCount(); got != 3 {
		t.Fatalf("SpanCount = %d, want 3", got)
	}
	if err := buf.Export(context.Background(), []*tracev1.ResourceSpans{
		{ScopeSpans: []*tracev1.ScopeSpans{{Spans: []*tracev1.Span{{Name: "d"}}}}},
	}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if got := buf.SpanCount(); got != 4 {
		t.Fatalf("SpanCount = %d, want 4", got)
	}
}

func TestBuildRunPayloads_ProducesOTLPJSON(t *testing.T) {
	spans := []*tracev1.ResourceSpans{{
		ScopeSpans: []*tracev1.ScopeSpans{{
			Scope: &commonv1.InstrumentationScope{Name: "test"},
			Spans: []*tracev1.Span{{
				TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				SpanId:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
				Name:    "root",
			}},
		}},
	}}
	traceData, logData, metricData := buildRunPayloads(spans, nil, nil)
	if traceData == nil {
		t.Fatal("expected trace payload")
	}
	if logData != nil || metricData != nil {
		t.Fatal("expected nil log/metric payloads")
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(*traceData, &probe); err != nil {
		t.Fatalf("trace JSON: %v", err)
	}
}

func TestPublishCompletedRun_AtomicSnapshot(t *testing.T) {
	raw := json.RawMessage(`{"resourceSpans":[]}`)
	run := &Run{ID: "pub-1", Status: RunStatusRunning}
	run.mu.Lock()
	run.SpanCount = 2
	run.ResourceSpanCount = 1
	run.TraceData = &raw
	run.Status = RunStatusCompleted
	now := time.Now()
	run.CompletedAt = &now
	run.mu.Unlock()

	snap := run.snapshot()
	if snap.Status != RunStatusCompleted {
		t.Fatalf("status = %s", snap.Status)
	}
	if snap.CompletedAt == nil {
		t.Fatal("CompletedAt nil")
	}
	if snap.TraceData == nil {
		t.Fatal("TraceData nil")
	}
	if !snap.HasTraceData {
		t.Fatal("HasTraceData false")
	}
}

func TestExecuteRun_FinalizeMarshalOutsideLock(t *testing.T) {
	store := NewRunStore()
	cfg := gen.Config{
		Seed: 7, TraceCount: 1, Topology: gen.TopologyFlat,
		SpanCount: 3, ServiceName: "lock-test",
		BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: time.Second,
		Signals:       gen.Signals{Traces: true},
	}
	run, err := StartRun(context.Background(), store, cfg, nil, true, "file")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		snap := store.Get(run.ID)
		if snap != nil && (snap.Status == RunStatusCompleted || snap.Status == RunStatusFailed || snap.Status == RunStatusStopped) {
			if snap.Status != RunStatusCompleted {
				t.Fatalf("status = %s err=%s", snap.Status, snap.Error)
			}
			if snap.TraceData == nil {
				t.Fatal("missing TraceData after finalize")
			}
			if snap.SpanCount <= 0 {
				t.Fatalf("SpanCount = %d", snap.SpanCount)
			}
			return
		}
		// Prove readers can acquire the lock during/after finalize without deadlock.
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = store.Get(run.ID)
		}()
		wg.Wait()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for run completion")
}

func TestOnProgress_UsesBufferCount(t *testing.T) {
	store := NewRunStore()
	cfg := gen.Config{
		Seed: 11, TraceCount: 2, Topology: gen.TopologyFlat,
		SpanCount: 3, ServiceName: "progress-count",
		BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: time.Second,
		Signals:       gen.Signals{Traces: true},
	}
	run, err := StartRun(context.Background(), store, cfg, nil, true, "file")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	var snap *Run
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		snap = store.Get(run.ID)
		if snap != nil && (snap.Status == RunStatusCompleted || snap.Status == RunStatusFailed || snap.Status == RunStatusStopped) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if snap == nil {
		t.Fatal("run missing from store")
	}
	if snap.Status != RunStatusCompleted {
		t.Fatalf("status = %s err=%s", snap.Status, snap.Error)
	}
	if snap.TraceData == nil {
		t.Fatal("missing TraceData after finalize")
	}
	if snap.TraceCountCompleted < 2 {
		t.Fatalf("TraceCountCompleted = %d, want >= 2", snap.TraceCountCompleted)
	}

	spans, err := parseOTLPJSON(*snap.TraceData)
	if err != nil {
		t.Fatalf("parseOTLPJSON: %v", err)
	}
	want, _ := countSpansIn(spans)
	if want <= 0 {
		t.Fatal("TraceData recount produced 0 spans")
	}
	if snap.SpanCount != want {
		t.Fatalf("SpanCount = %d, TraceData recount = %d", snap.SpanCount, want)
	}
}

func TestBufferCaptureLimitAndSnapshotSource(t *testing.T) {
	buf := &bufferExporter{}
	large := &tracev1.ResourceSpans{ScopeSpans: []*tracev1.ScopeSpans{{Spans: []*tracev1.Span{{Name: string(make([]byte, captureSignalLimit))}}}}}
	if err := buf.Export(context.Background(), []*tracev1.ResourceSpans{large}); err != nil {
		t.Fatal(err)
	}
	if buf.dropped != 1 || len(buf.Snapshot()) != 0 {
		t.Fatalf("dropped=%d retained=%d", buf.dropped, len(buf.Snapshot()))
	}
	run := &Run{Source: "remote-tap", CaptureTruncated: true, CaptureDropped: 1, CaptureLimitBytes: 3 * captureSignalLimit}
	snap := run.snapshotSummary()
	if snap.Source != run.Source || !snap.CaptureTruncated || snap.CaptureDropped != 1 || snap.CaptureLimitBytes != run.CaptureLimitBytes {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestStopIsNonterminalUntilFinalization(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	run := &Run{Status: RunStatusRunning, cancel: cancel}
	run.Stop()
	if run.Status != RunStatusStopping || isTerminalStatus(run.Status) || ctx.Err() != context.Canceled {
		t.Fatalf("status=%s context=%v", run.Status, ctx.Err())
	}
}

func TestPausedGateObservesDeadline(t *testing.T) {
	gate := newRunPauseGate()

	// Hold an export in flight so the pause genuinely parks instead of
	// completing instantly.
	if err := gate.beginExport(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := gate.pause(ctx); done <- err }()

	// Cancel before releasing the export: whether the parked pause wakes on
	// the context or on the release broadcast, ctx.Err() is already set, so
	// it must report Canceled and reopen admission.
	cancel()
	gate.endExport()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("pause error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pause did not observe cancellation")
	}
	if err := gate.beginExport(context.Background()); err != nil {
		t.Fatalf("admission did not reopen after cancelled pause: %v", err)
	}
	gate.endExport()
	gate.stop()
}
