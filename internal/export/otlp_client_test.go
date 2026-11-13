package export

import (
	"context"
	"net"
	"testing"
	"time"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
)

type acceptingLogsServer struct {
	collectorlogsv1.UnimplementedLogsServiceServer
}

func (acceptingLogsServer) Export(context.Context, *collectorlogsv1.ExportLogsServiceRequest) (*collectorlogsv1.ExportLogsServiceResponse, error) {
	return &collectorlogsv1.ExportLogsServiceResponse{}, nil
}

type acceptingMetricsServer struct {
	collectormetricsv1.UnimplementedMetricsServiceServer
}

func (acceptingMetricsServer) Export(context.Context, *collectormetricsv1.ExportMetricsServiceRequest) (*collectormetricsv1.ExportMetricsServiceResponse, error) {
	return &collectormetricsv1.ExportMetricsServiceResponse{}, nil
}

func TestOTLPClientsExportToLocalCollector(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer()
	collectortracev1.RegisterTraceServiceServer(gs, &partialTraceServer{})
	collectorlogsv1.RegisterLogsServiceServer(gs, acceptingLogsServer{})
	collectormetricsv1.RegisterMetricsServiceServer(gs, acceptingMetricsServer{})
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	_, port, err := net.SplitHostPort(lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{lis.Addr().String(), "localhost:" + port, "dns:///localhost:" + port} {
		t.Run(endpoint, func(t *testing.T) {
			traces, err := NewOTLPExporter(endpoint, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { traces.Shutdown(context.Background()) })
			logs, err := NewOTLPLogExporter(endpoint, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { logs.Shutdown(context.Background()) })
			metrics, err := NewOTLPMetricExporter(endpoint, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { metrics.Shutdown(context.Background()) })
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := traces.Export(ctx, tenSpans()); err != nil {
				t.Errorf("traces: %v", err)
			}
			if err := logs.Export(ctx, nil); err != nil {
				t.Errorf("logs: %v", err)
			}
			if err := metrics.Export(ctx, nil); err != nil {
				t.Errorf("metrics: %v", err)
			}
		})
	}
}
