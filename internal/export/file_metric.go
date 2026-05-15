package export

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// MetricFileExporter exports metrics to files on disk as JSON or protobuf binary.
type MetricFileExporter struct {
	outputDir string
	format    Format
	mu        sync.Mutex
	index     int
}

// NewMetricFileExporter creates a new MetricFileExporter that writes to outputDir in the given format.
func NewMetricFileExporter(outputDir string, format Format) (*MetricFileExporter, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	return &MetricFileExporter{
		outputDir: outputDir,
		format:    format,
		index:     0,
	}, nil
}

func (e *MetricFileExporter) Export(ctx context.Context, metrics []*metricspb.ResourceMetrics) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	req := &collectormetricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: metrics,
	}

	var data []byte
	var ext string

	switch e.format {
	case FormatJSON:
		opts := protojson.MarshalOptions{
			UseProtoNames: true,
			Indent:        "  ",
		}
		var err error
		data, err = opts.Marshal(req)
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

	filename := filepath.Join(e.outputDir, fmt.Sprintf("metric_%d_%d.%s", timestamp, index, ext))

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

func (e *MetricFileExporter) Shutdown(ctx context.Context) error {
	// no-op for file exporter
	return nil
}
