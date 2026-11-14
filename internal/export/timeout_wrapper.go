package export

import (
	"context"
	"time"

	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// WithExportTimeout wraps a TraceExporter to use a fresh context with timeout for each export,
// preventing race conditions when the parent generation context expires.
func WithExportTimeout(exp TraceExporter, timeout time.Duration) TraceExporter {
	return WithExportTimeoutContext(context.Background(), exp, timeout)
}

// WithExportTimeoutContext gives exports a separate owner lifetime. Generation
// deadlines may expire before final flush, but owner shutdown must cancel RPCs.
func WithExportTimeoutContext(owner context.Context, exp TraceExporter, timeout time.Duration) TraceExporter {
	return &traceExporterWithTimeout{inner: exp, timeout: timeout, owner: owner}
}

type traceExporterWithTimeout struct {
	owner   context.Context
	inner   TraceExporter
	timeout time.Duration
}

type traceReceiptExporter interface {
	ExportWithReceipt(ctx context.Context, spans []*tracev1.ResourceSpans) (ExportReceipt, error)
}

func (w *traceExporterWithTimeout) Export(ctx context.Context, spans []*tracev1.ResourceSpans) error {
	_, err := w.ExportWithReceipt(ctx, spans)
	return err
}

// ExportWithReceipt preserves optional receipt accounting across the timeout wrapper.
func (w *traceExporterWithTimeout) ExportWithReceipt(ctx context.Context, spans []*tracev1.ResourceSpans) (ExportReceipt, error) {
	exportCtx, cancel := context.WithTimeout(w.owner, w.timeout)
	defer cancel()
	if re, ok := w.inner.(traceReceiptExporter); ok {
		return re.ExportWithReceipt(exportCtx, spans)
	}
	err := w.inner.Export(exportCtx, spans)
	attempted := CountSpans(spans)
	if err != nil {
		return ReceiptRPCFailure(attempted, err), err
	}
	// Non-receipt exporters only prove response success, not destination delivery.
	return ReceiptFullSuccess(attempted), nil
}

func (w *traceExporterWithTimeout) Shutdown(ctx context.Context) error {
	return w.inner.Shutdown(ctx)
}

// WithLogExportTimeout wraps a LogExporter to use a fresh context with timeout for each export.
func WithLogExportTimeout(exp LogExporter, timeout time.Duration) LogExporter {
	return WithLogExportTimeoutContext(context.Background(), exp, timeout)
}

// WithLogExportTimeoutContext preserves final flush while honoring owner shutdown.
func WithLogExportTimeoutContext(owner context.Context, exp LogExporter, timeout time.Duration) LogExporter {
	return &logExporterWithTimeout{inner: exp, timeout: timeout, owner: owner}
}

type logExporterWithTimeout struct {
	owner   context.Context
	inner   LogExporter
	timeout time.Duration
}

type logReceiptExporter interface {
	ExportWithReceipt(ctx context.Context, logs []*logsv1.ResourceLogs) (ExportReceipt, error)
}

func (w *logExporterWithTimeout) Export(ctx context.Context, logs []*logsv1.ResourceLogs) error {
	_, err := w.ExportWithReceipt(ctx, logs)
	return err
}

func (w *logExporterWithTimeout) ExportWithReceipt(ctx context.Context, logs []*logsv1.ResourceLogs) (ExportReceipt, error) {
	exportCtx, cancel := context.WithTimeout(w.owner, w.timeout)
	defer cancel()
	if re, ok := w.inner.(logReceiptExporter); ok {
		return re.ExportWithReceipt(exportCtx, logs)
	}
	err := w.inner.Export(exportCtx, logs)
	attempted := CountLogRecords(logs)
	if err != nil {
		return ReceiptRPCFailure(attempted, err), err
	}
	return ReceiptFullSuccess(attempted), nil
}

func (w *logExporterWithTimeout) Shutdown(ctx context.Context) error {
	return w.inner.Shutdown(ctx)
}

// WithMetricExportTimeout wraps a MetricExporter to use a fresh context with timeout for each export.
func WithMetricExportTimeout(exp MetricExporter, timeout time.Duration) MetricExporter {
	return WithMetricExportTimeoutContext(context.Background(), exp, timeout)
}

// WithMetricExportTimeoutContext preserves final flush while honoring owner shutdown.
func WithMetricExportTimeoutContext(owner context.Context, exp MetricExporter, timeout time.Duration) MetricExporter {
	return &metricExporterWithTimeout{inner: exp, timeout: timeout, owner: owner}
}

type metricExporterWithTimeout struct {
	owner   context.Context
	inner   MetricExporter
	timeout time.Duration
}

type metricReceiptExporter interface {
	ExportWithReceipt(ctx context.Context, metrics []*metricspb.ResourceMetrics) (ExportReceipt, error)
}

func (w *metricExporterWithTimeout) Export(ctx context.Context, metrics []*metricspb.ResourceMetrics) error {
	_, err := w.ExportWithReceipt(ctx, metrics)
	return err
}

func (w *metricExporterWithTimeout) ExportWithReceipt(ctx context.Context, metrics []*metricspb.ResourceMetrics) (ExportReceipt, error) {
	exportCtx, cancel := context.WithTimeout(w.owner, w.timeout)
	defer cancel()
	if re, ok := w.inner.(metricReceiptExporter); ok {
		return re.ExportWithReceipt(exportCtx, metrics)
	}
	err := w.inner.Export(exportCtx, metrics)
	attempted := CountDataPoints(metrics)
	if err != nil {
		return ReceiptRPCFailure(attempted, err), err
	}
	return ReceiptFullSuccess(attempted), nil
}

func (w *metricExporterWithTimeout) Shutdown(ctx context.Context) error {
	return w.inner.Shutdown(ctx)
}
