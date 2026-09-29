package export

import (
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

type MetricFileExporter = GenericFileExporter[*metricspb.ResourceMetrics, *collectormetricsv1.ExportMetricsServiceRequest]

func NewMetricFileExporter(outputDir string, format Format) (*MetricFileExporter, error) {
	return NewGenericFileExporter(
		outputDir,
		format,
		"metric",
		func(metrics []*metricspb.ResourceMetrics) *collectormetricsv1.ExportMetricsServiceRequest {
			return &collectormetricsv1.ExportMetricsServiceRequest{ResourceMetrics: metrics}
		},
		func(req *collectormetricsv1.ExportMetricsServiceRequest) ([]byte, error) {
			opts := protojson.MarshalOptions{
				UseProtoNames: false,
				Indent:        "",
			}
			return opts.Marshal(req)
		},
		func(req *collectormetricsv1.ExportMetricsServiceRequest) ([]byte, error) {
			return proto.Marshal(req)
		},
	)
}