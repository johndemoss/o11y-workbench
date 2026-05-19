package export

import (
	"google.golang.org/protobuf/proto"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

type FileExporter = GenericFileExporter[*tracev1.ResourceSpans, *collectortracev1.ExportTraceServiceRequest]

func NewFileExporter(outputDir string, format Format) (*FileExporter, error) {
	return NewFileExporterTrace(outputDir, format)
}

func NewFileExporterTrace(outputDir string, format Format) (*FileExporter, error) {
	return NewGenericFileExporter(
		outputDir,
		format,
		"trace",
		func(spans []*tracev1.ResourceSpans) *collectortracev1.ExportTraceServiceRequest {
			return &collectortracev1.ExportTraceServiceRequest{ResourceSpans: spans}
		},
		func(req *collectortracev1.ExportTraceServiceRequest) ([]byte, error) {
			return marshalOTLPJSON(req)
		},
		func(req *collectortracev1.ExportTraceServiceRequest) ([]byte, error) {
			return proto.Marshal(req)
		},
	)
}