package export

import (
	"context"

	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

// MetricExporter sends metric data to an output destination.
type MetricExporter interface {
	// Export sends the given resource metrics to the configured output.
	Export(ctx context.Context, metrics []*metricspb.ResourceMetrics) error
	// Shutdown flushes any remaining data and releases resources.
	Shutdown(ctx context.Context) error
}
