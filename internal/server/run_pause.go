package server

import (
	"context"
	"sync"
	"sync/atomic"

	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/johndemoss/o11y-workbench/internal/export"
)

// runPauseGate blocks new exports while a run is paused and tracks in-flight
// exports so a pause request only completes once nothing is being exported.
// It never holds a lock across an exporter call.
type runPauseGate struct {
	mu           sync.Mutex
	paused       bool
	acknowledged bool
	exportFailed bool
	stopped      bool
	sequence     uint64
	inflight     int
	changed      chan struct{}
	done         chan struct{}
}

func newRunPauseGate() *runPauseGate {
	return &runPauseGate{changed: make(chan struct{}), done: make(chan struct{})}
}

func (g *runPauseGate) broadcastLocked() {
	close(g.changed)
	g.changed = make(chan struct{})
}

// beginExport waits for admission and counts the export as in flight.
func (g *runPauseGate) beginExport(ctx context.Context) error {
	for {
		g.mu.Lock()
		if !g.paused {
			g.inflight++
			g.mu.Unlock()
			return nil
		}
		changed := g.changed
		g.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// beginFailedExport admits a retry after an exporter has failed, including
// cancellation flushes that use a fresh context.
func (g *runPauseGate) beginFailedExport() {
	g.mu.Lock()
	g.inflight++
	g.mu.Unlock()
}

// failExport prevents a pending pause from acknowledging while a failed
// exporter may still retry its buffered data.
func (g *runPauseGate) failExport() {
	g.mu.Lock()
	g.exportFailed = true
	g.broadcastLocked()
	g.mu.Unlock()
}

func (g *runPauseGate) endExport() {
	g.mu.Lock()
	if g.inflight > 0 {
		g.inflight--
	}
	g.broadcastLocked()
	g.mu.Unlock()
}

// pause closes admission, then waits for in-flight exports to finish. Only the
// most recent pause request may reopen admission if its context is cancelled.
func (g *runPauseGate) pause(ctx context.Context) (uint64, error) {
	g.mu.Lock()
	g.sequence++
	sequence := g.sequence
	if !g.paused && !g.stopped && !g.exportFailed {
		g.paused = true
		g.broadcastLocked()
	}
	for g.inflight > 0 && !g.stopped && !g.exportFailed && g.paused {
		changed := g.changed
		g.mu.Unlock()
		select {
		case <-changed:
		case <-g.done:
			return 0, context.Canceled
		case <-ctx.Done():
			g.mu.Lock()
			if !g.stopped && !g.acknowledged && g.sequence == sequence {
				g.paused = false
				g.broadcastLocked()
			}
			g.mu.Unlock()
			return 0, ctx.Err()
		}
		g.mu.Lock()
	}
	stopped := g.stopped || g.exportFailed || !g.paused
	g.mu.Unlock()
	if stopped {
		return 0, context.Canceled
	}
	if err := ctx.Err(); err != nil {
		g.mu.Lock()
		if !g.stopped && !g.acknowledged && g.sequence == sequence {
			g.paused = false
			g.broadcastLocked()
		}
		g.mu.Unlock()
		return 0, err
	}
	return sequence, nil
}

func (g *runPauseGate) cancelPause(sequence uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.stopped && !g.acknowledged && g.sequence == sequence && g.paused {
		g.paused = false
		g.broadcastLocked()
	}
}

func (g *runPauseGate) acknowledge() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.paused || g.stopped || g.exportFailed || g.inflight != 0 {
		return false
	}
	g.acknowledged = true
	return true
}

func (g *runPauseGate) pauseAcknowledged() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.paused && g.acknowledged && !g.stopped && !g.exportFailed
}

func (g *runPauseGate) resume() {
	g.mu.Lock()
	if g.paused {
		g.acknowledged = false
		g.paused = false
		g.broadcastLocked()
	}
	g.mu.Unlock()
}

// stop unblocks every waiter for the lifetime of the run.
func (g *runPauseGate) stop() {
	g.mu.Lock()
	if !g.stopped {
		g.stopped = true
		close(g.done)
		g.acknowledged = false
		g.paused = false
		g.broadcastLocked()
	}
	g.mu.Unlock()
}

type gatedTraceExporter struct {
	gate  *runPauseGate
	inner export.TraceExporter
}

func (g gatedTraceExporter) Export(ctx context.Context, spans []*tracev1.ResourceSpans) error {
	_, err := g.ExportWithReceipt(ctx, spans)
	return err
}

func (g gatedTraceExporter) ExportWithReceipt(ctx context.Context, spans []*tracev1.ResourceSpans) (export.ExportReceipt, error) {
	if err := g.gate.beginExport(ctx); err != nil {
		return export.ExportReceipt{}, err
	}
	defer g.gate.endExport()
	if inner, ok := g.inner.(interface {
		ExportWithReceipt(context.Context, []*tracev1.ResourceSpans) (export.ExportReceipt, error)
	}); ok {
		return inner.ExportWithReceipt(ctx, spans)
	}
	if err := g.inner.Export(ctx, spans); err != nil {
		return export.ReceiptRPCFailure(export.CountSpans(spans), err), err
	}
	return export.ReceiptFullSuccess(export.CountSpans(spans)), nil
}

func (g gatedTraceExporter) Shutdown(ctx context.Context) error {
	return g.inner.Shutdown(ctx)
}

type gatedLogExporter struct {
	gate   *runPauseGate
	inner  export.LogExporter
	failed atomic.Bool
}

func (g *gatedLogExporter) Export(ctx context.Context, logs []*logsv1.ResourceLogs) error {
	_, err := g.ExportWithReceipt(ctx, logs)
	return err
}

func (g *gatedLogExporter) ExportWithReceipt(ctx context.Context, logs []*logsv1.ResourceLogs) (export.ExportReceipt, error) {
	if g.failed.Load() {
		g.gate.beginFailedExport()
	} else if err := g.gate.beginExport(ctx); err != nil {
		return export.ExportReceipt{}, err
	}
	defer g.gate.endExport()
	if inner, ok := g.inner.(interface {
		ExportWithReceipt(context.Context, []*logsv1.ResourceLogs) (export.ExportReceipt, error)
	}); ok {
		receipt, err := inner.ExportWithReceipt(ctx, logs)
		if err != nil {
			g.failed.Store(true)
			g.gate.failExport()
		}
		return receipt, err
	}
	if err := g.inner.Export(ctx, logs); err != nil {
		g.failed.Store(true)
		g.gate.failExport()
		return export.ReceiptRPCFailure(export.CountLogRecords(logs), err), err
	}
	return export.ReceiptFullSuccess(export.CountLogRecords(logs)), nil
}

func (g *gatedLogExporter) Shutdown(ctx context.Context) error {
	return g.inner.Shutdown(ctx)
}

type gatedMetricExporter struct {
	gate  *runPauseGate
	inner export.MetricExporter
}

func (g gatedMetricExporter) Export(ctx context.Context, metrics []*metricspb.ResourceMetrics) error {
	_, err := g.ExportWithReceipt(ctx, metrics)
	return err
}

func (g gatedMetricExporter) ExportWithReceipt(ctx context.Context, metrics []*metricspb.ResourceMetrics) (export.ExportReceipt, error) {
	if err := g.gate.beginExport(ctx); err != nil {
		return export.ExportReceipt{}, err
	}
	defer g.gate.endExport()
	if inner, ok := g.inner.(interface {
		ExportWithReceipt(context.Context, []*metricspb.ResourceMetrics) (export.ExportReceipt, error)
	}); ok {
		return inner.ExportWithReceipt(ctx, metrics)
	}
	if err := g.inner.Export(ctx, metrics); err != nil {
		return export.ReceiptRPCFailure(export.CountDataPoints(metrics), err), err
	}
	return export.ReceiptFullSuccess(export.CountDataPoints(metrics)), nil
}

func (g gatedMetricExporter) Shutdown(ctx context.Context) error {
	return g.inner.Shutdown(ctx)
}
