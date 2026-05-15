package export

import (
	"context"
	"fmt"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/grpc"
	insecurecreds "google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// OTLPLogExporter sends log records to an OTLP gRPC endpoint.
type OTLPLogExporter struct {
	conn     *grpc.ClientConn
	client   collectorlogsv1.LogsServiceClient
	endpoint string
	headers  map[string]string
}

// NewOTLPLogExporter creates a gRPC connection and returns a log exporter
// that streams log records using that connection.
func NewOTLPLogExporter(endpoint string, insecure bool, headers map[string]string) (*OTLPLogExporter, error) {
	creds := insecurecreds.NewCredentials()

	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", endpoint, err)
	}

	return &OTLPLogExporter{
		conn:     conn,
		client:   collectorlogsv1.NewLogsServiceClient(conn),
		endpoint: endpoint,
		headers:  headers,
	}, nil
}

func (e *OTLPLogExporter) Export(ctx context.Context, logs []*logsv1.ResourceLogs) error {
	req := &collectorlogsv1.ExportLogsServiceRequest{
		ResourceLogs: logs,
	}

	if len(e.headers) > 0 {
		md := make(metadata.MD, len(e.headers))
		for k, v := range e.headers {
			md[k] = []string{v}
		}
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	_, err := e.client.Export(ctx, req)
	if err != nil {
		return fmt.Errorf("export logs to %s: %w", e.endpoint, err)
	}

	return nil
}

func (e *OTLPLogExporter) Shutdown(ctx context.Context) error {
	if e.conn == nil {
		return nil
	}
	return e.conn.Close()
}
