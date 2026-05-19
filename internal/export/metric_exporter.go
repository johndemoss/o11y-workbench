package export

import (
	"context"

	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

type MetricExporter interface {
	Export(ctx context.Context, metrics []*metricspb.ResourceMetrics) error
	Shutdown(ctx context.Context) error
}
