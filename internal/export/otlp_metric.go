package export

import (
	"context"
	"fmt"

	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	insecurecreds "google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// OTLPMetricExporter sends metric records to an OTLP gRPC endpoint.
type OTLPMetricExporter struct {
	conn     *grpc.ClientConn
	client   collectormetricsv1.MetricsServiceClient
	endpoint string
	headers  map[string]string
}

// NewOTLPMetricExporter creates a gRPC connection and returns a metric exporter
// that streams metric records using that connection.
func NewOTLPMetricExporter(endpoint string, insecure bool, headers map[string]string) (*OTLPMetricExporter, error) {
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

	return &OTLPMetricExporter{
		conn:     conn,
		client:   collectormetricsv1.NewMetricsServiceClient(conn),
		endpoint: endpoint,
		headers:  headers,
	}, nil
}

// Export sends a batch of metric records to the OTLP endpoint.
func (e *OTLPMetricExporter) Export(ctx context.Context, metrics []*metricspb.ResourceMetrics) error {
	req := &collectormetricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: metrics,
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
		return fmt.Errorf("export metrics to %s: %w", e.endpoint, err)
	}

	return nil
}

// Shutdown closes the gRPC connection and releases resources.
func (e *OTLPMetricExporter) Shutdown(ctx context.Context) error {
	if e.conn == nil {
		return nil
	}
	return e.conn.Close()
}
