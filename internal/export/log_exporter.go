package export

import (
	"context"

	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
)

type LogExporter interface {
	Export(ctx context.Context, logs []*logsv1.ResourceLogs) error
	Shutdown(ctx context.Context) error
}
