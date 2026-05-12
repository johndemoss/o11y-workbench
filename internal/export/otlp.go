package export

import (
	"context"
	"fmt"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	insecurecreds "google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type OTLPExporter struct {
	conn     *grpc.ClientConn
	client   collectortracev1.TraceServiceClient
	endpoint string
	headers  map[string]string
}

// NewOTLPExporter creates a gRPC connection to the endpoint and returns
// an exporter that streams traces using that connection.
func NewOTLPExporter(endpoint string, insecure bool, headers map[string]string) (*OTLPExporter, error) {
	var creds credentials.TransportCredentials
	if insecure {
		creds = insecurecreds.NewCredentials()
	} else {
		creds = credentials.NewTLS(nil)
	}

	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", endpoint, err)
	}

	return &OTLPExporter{
		conn:     conn,
		client:   collectortracev1.NewTraceServiceClient(conn),
		endpoint: endpoint,
		headers:  headers,
	}, nil
}

func (e *OTLPExporter) Export(ctx context.Context, spans []*tracev1.ResourceSpans) error {
	req := &collectortracev1.ExportTraceServiceRequest{
		ResourceSpans: spans,
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
		return fmt.Errorf("export to %s: %w", e.endpoint, err)
	}

	return nil
}

func (e *OTLPExporter) Shutdown(ctx context.Context) error {
	if e.conn == nil {
		return nil
	}
	return e.conn.Close()
}
