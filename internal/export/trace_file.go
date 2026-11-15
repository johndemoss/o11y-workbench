package export

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"

	"github.com/johndemoss/o11y-workbench/internal/region"
)

// TraceFileExporter writes each trace to its own file organized by
// <outputDir>/<traceID>/<endpoint>/spans-<unique>.{json,pb}.
// Each file is a self-contained ExportTraceServiceRequest that can be
// sent directly to a collector (e.g. via curl).
type TraceFileExporter struct {
	outputDir string
	format    Format
}

func NewTraceFileExporter(outputDir string, format Format) *TraceFileExporter {
	return &TraceFileExporter{
		outputDir: outputDir,
		format:    format,
	}
}

// Export writes one self-contained, uniquely named request per batch.
func (e *TraceFileExporter) Export(ctx context.Context, spans []*tracev1.ResourceSpans) error {
	if len(spans) == 0 {
		return nil
	}

	traceIDHex := traceIDFromSpans(spans[0])
	if traceIDHex == "" {
		return fmt.Errorf("no valid trace ID found in spans")
	}

	reg := regionFromResource(spans[0])
	endpointSlug := sanitizeEndpoint(region.GetEndpoint(reg))

	dir := filepath.Join(e.outputDir, traceIDHex, endpointSlug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create trace directory: %w", err)
	}

	req := &collectortracev1.ExportTraceServiceRequest{
		ResourceSpans: spans,
	}

	var data []byte
	var ext string

	switch e.format {
	case FormatJSON:
		var err error
		data, err = marshalOTLPJSON(req)
		if err != nil {
			return fmt.Errorf("marshal OTLP JSON: %w", err)
		}
		ext = "json"

	case FormatProtobuf:
		var err error
		data, err = proto.Marshal(req)
		if err != nil {
			return fmt.Errorf("marshal protobuf: %w", err)
		}
		ext = "pb"
	}

	f, err := os.CreateTemp(dir, "spans-*."+ext)
	if err != nil {
		return fmt.Errorf("create spans file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return fmt.Errorf("write spans file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return fmt.Errorf("close spans file: %w", err)
	}

	return nil
}

func (e *TraceFileExporter) Shutdown(ctx context.Context) error {
	return nil
}

// traceIDFromSpans returns the hex-encoded trace ID from the first span found.
func traceIDFromSpans(rs *tracev1.ResourceSpans) string {
	for _, ss := range rs.ScopeSpans {
		for _, span := range ss.Spans {
			if len(span.TraceId) > 0 {
				return fmt.Sprintf("%x", span.TraceId)
			}
		}
	}
	return ""
}

// sanitizeEndpoint replaces characters unsafe for directory names.
func sanitizeEndpoint(ep string) string {
	replacer := strings.NewReplacer(":", "_", "/", "_", " ", "_")
	return replacer.Replace(ep)
}
