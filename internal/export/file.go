package export

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// FileExporter exports traces to files on disk as JSON or protobuf binary.
type FileExporter struct {
	outputDir string
	format    Format
	mu        sync.Mutex
	index     int
}

// NewFileExporter creates a new FileExporter that writes to outputDir in the given format.
func NewFileExporter(outputDir string, format Format) (*FileExporter, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	return &FileExporter{
		outputDir: outputDir,
		format:    format,
		index:     0,
	}, nil
}

func (e *FileExporter) Export(ctx context.Context, spans []*tracev1.ResourceSpans) error {
	e.mu.Lock()
	defer e.mu.Unlock()

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

	timestamp := time.Now().UnixNano()
	index := e.index
	e.index++

	filename := filepath.Join(e.outputDir, fmt.Sprintf("trace_%d_%d.%s", timestamp, index, ext))

	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("output file already exists: %s", filename)
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}

	return nil
}

func (e *FileExporter) Shutdown(ctx context.Context) error {
	// no-op for file exporter
	return nil
}
