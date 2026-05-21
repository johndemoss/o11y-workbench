package export

import (
	"context"
	"fmt"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	insecurecreds "google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/johndemoss/o11y-workbench/internal/logging"
)

type TracesExporter struct {
	conn     *grpc.ClientConn
	client   collectortracev1.TraceServiceClient
	endpoint string
	headers  map[string]string
}

type LogsExporter struct {
	conn     *grpc.ClientConn
	client   collectorlogsv1.LogsServiceClient
	endpoint string
	headers  map[string]string
}

type MetricsExporter struct {
	conn     *grpc.ClientConn
	client   collectormetricsv1.MetricsServiceClient
	endpoint string
	headers  map[string]string
}

type OTLPExporter = TracesExporter
type OTLPLogExporter = LogsExporter
type OTLPMetricExporter = MetricsExporter

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

func NewOTLPLogExporter(endpoint string, insecure bool, headers map[string]string) (*OTLPLogExporter, error) {
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

	return &OTLPLogExporter{
		conn:     conn,
		client:   collectorlogsv1.NewLogsServiceClient(conn),
		endpoint: endpoint,
		headers:  headers,
	}, nil
}

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

func (e *TracesExporter) Export(ctx context.Context, spans []*tracev1.ResourceSpans) error {
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

func (e *TracesExporter) Shutdown(ctx context.Context) error {
	if e.conn == nil {
		return nil
	}
	return e.conn.Close()
}

func (e *LogsExporter) Export(ctx context.Context, logs []*logsv1.ResourceLogs) error {
	// Count total log records
	totalLogs := 0
	for _, rl := range logs {
		for _, sl := range rl.ScopeLogs {
			totalLogs += len(sl.LogRecords)
		}
	}
	logging.Debugf("OTLP LogsExporter: Exporting %d ResourceLogs (%d total log records) to %s", len(logs), totalLogs, e.endpoint)

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
		logging.Warnf("OTLP LogsExporter: Export FAILED to %s: %v", e.endpoint, err)
		return fmt.Errorf("export logs to %s: %w", e.endpoint, err)
	}

	logging.Debugf("OTLP LogsExporter: Export SUCCESS to %s", e.endpoint)
	return nil
}

func (e *LogsExporter) Shutdown(ctx context.Context) error {
	if e.conn == nil {
		return nil
	}
	return e.conn.Close()
}

func (e *MetricsExporter) Export(ctx context.Context, metrics []*metricspb.ResourceMetrics) error {
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

func (e *MetricsExporter) Shutdown(ctx context.Context) error {
	if e.conn == nil {
		return nil
	}
	return e.conn.Close()
}