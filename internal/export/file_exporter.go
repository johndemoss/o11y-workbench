package export

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type GenericFileExporter[T any, R any] struct {
	outputDir    string
	format       Format
	prefix       string
	toRequest    func([]T) R
	marshalJSON  func(R) ([]byte, error)
	marshalProto func(R) ([]byte, error)
	mu           sync.Mutex
	index        int
}

func NewGenericFileExporter[T any, R any](
	outputDir string,
	format Format,
	prefix string,
	toRequest func([]T) R,
	marshalJSON func(R) ([]byte, error),
	marshalProto func(R) ([]byte, error),
) (*GenericFileExporter[T, R], error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	return &GenericFileExporter[T, R]{
		outputDir:    outputDir,
		format:       format,
		prefix:       prefix,
		toRequest:     toRequest,
		marshalJSON:  marshalJSON,
		marshalProto: marshalProto,
		index:        0,
	}, nil
}

func (e *GenericFileExporter[T, R]) Export(ctx context.Context, resources []T) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	req := e.toRequest(resources)

	var data []byte
	var ext string

	switch e.format {
	case FormatJSON:
		var err error
		data, err = e.marshalJSON(req)
		if err != nil {
			return fmt.Errorf("marshal JSON: %w", err)
		}
		ext = "json"

	case FormatProtobuf:
		var err error
		data, err = e.marshalProto(req)
		if err != nil {
			return fmt.Errorf("marshal protobuf: %w", err)
		}
		ext = "pb"
	}

	timestamp := time.Now().UnixNano()
	index := e.index
	e.index++

	filename := filepath.Join(e.outputDir, fmt.Sprintf("%s_%d_%d.%s", e.prefix, timestamp, index, ext))

	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("output file already exists: %s", filename)
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer func() { if err := file.Close(); err != nil { log.Printf("error closing file: %v", err) } }()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}

	return nil
}

func (e *GenericFileExporter[T, R]) Shutdown(ctx context.Context) error {
	return nil
}