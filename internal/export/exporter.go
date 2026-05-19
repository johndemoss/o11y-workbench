package export

import (
	"context"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// Format represents the encoding format for exported traces.
type Format int

const (
	// FormatJSON exports traces as human-readable JSON.
	FormatJSON Format = iota
	// FormatProtobuf exports traces as binary protobuf.
	FormatProtobuf
)

func (f Format) String() string {
	switch f {
	case FormatJSON:
		return "json"
	case FormatProtobuf:
		return "protobuf"
	default:
		return "unknown"
	}
}

// ParseFormat parses a format string and returns the corresponding Format.
func ParseFormat(s string) (Format, error) {
	switch s {
	case "json":
		return FormatJSON, nil
	case "protobuf":
		return FormatProtobuf, nil
	default:
		return 0, &FormatError{Input: s}
	}
}

// FormatError is returned when a format string cannot be parsed.
type FormatError struct {
	Input string
}

func (e *FormatError) Error() string {
	return "invalid format: " + e.Input
}

type Exporter[T any] interface {
	Export(ctx context.Context, data []T) error
	Shutdown(ctx context.Context) error
}

type TraceExporter interface {
	Export(ctx context.Context, spans []*tracev1.ResourceSpans) error
	Shutdown(ctx context.Context) error
}
