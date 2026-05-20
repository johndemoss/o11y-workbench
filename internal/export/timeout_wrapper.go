package export

import (
	"context"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

// WithExportTimeout wraps a TraceExporter to use a fresh context with timeout for each export,
// preventing race conditions when the parent generation context expires.
func WithExportTimeout(exp TraceExporter, timeout time.Duration) TraceExporter {
	return &traceExporterWithTimeout{
		inner:   exp,
		timeout: timeout,
	}
}

type traceExporterWithTimeout struct {
	inner   TraceExporter
	timeout time.Duration
}

func (w *traceExporterWithTimeout) Export(ctx context.Context, spans []*tracev1.ResourceSpans) error {
	// Create a fresh context with timeout for this export, independent of parent context
	exportCtx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()

	// Use the fresh context for the export
	return w.inner.Export(exportCtx, spans)
}

func (w *traceExporterWithTimeout) Shutdown(ctx context.Context) error {
	return w.inner.Shutdown(ctx)
}

// WithLogExportTimeout wraps a LogExporter to use a fresh context with timeout for each export.
func WithLogExportTimeout(exp LogExporter, timeout time.Duration) LogExporter {
	return &logExporterWithTimeout{
		inner:   exp,
		timeout: timeout,
	}
}

type logExporterWithTimeout struct {
	inner   LogExporter
	timeout time.Duration
}

func (w *logExporterWithTimeout) Export(ctx context.Context, logs []*logsv1.ResourceLogs) error {
	exportCtx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()
	return w.inner.Export(exportCtx, logs)
}

func (w *logExporterWithTimeout) Shutdown(ctx context.Context) error {
	return w.inner.Shutdown(ctx)
}

// WithMetricExportTimeout wraps a MetricExporter to use a fresh context with timeout for each export.
func WithMetricExportTimeout(exp MetricExporter, timeout time.Duration) MetricExporter {
	return &metricExporterWithTimeout{
		inner:   exp,
		timeout: timeout,
	}
}

type metricExporterWithTimeout struct {
	inner   MetricExporter
	timeout time.Duration
}

func (w *metricExporterWithTimeout) Export(ctx context.Context, metrics []*metricspb.ResourceMetrics) error {
	exportCtx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()
	return w.inner.Export(exportCtx, metrics)
}

func (w *metricExporterWithTimeout) Shutdown(ctx context.Context) error {
	return w.inner.Shutdown(ctx)
}
