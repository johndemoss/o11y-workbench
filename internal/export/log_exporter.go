package export

import (
	"context"

	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
)

// LogExporter sends log records to an output destination.
type LogExporter interface {
	// Export sends the given resource logs to the configured output.
	Export(ctx context.Context, logs []*logsv1.ResourceLogs) error
	// Shutdown flushes any remaining data and releases resources.
	Shutdown(ctx context.Context) error
}
