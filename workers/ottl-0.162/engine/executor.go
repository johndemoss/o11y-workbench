package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlspan"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"
)

// TransformRunData parses and executes OTTL transform statements against trace
// data stored in a completed run. It returns the transformed trace data as
// OTLP JSON, plus the total number of spans processed. Each OTTL statement is
// executed against every span.
func TransformRunData(traceData json.RawMessage, statements []string) (json.RawMessage, int, error) {
	return TransformRunDataSelected(traceData, statements, nil)
}

// TransformRunDataSelected is like TransformRunData but only counts spans
// at the given flat indexes for the returned span count. spanIndexes nil or
// empty means all spans are counted. The transform is applied to all spans.
// The flat index walks ResourceSpans(i) → ScopeSpans(j) → Spans(k) depth-first.
func TransformRunDataSelected(traceData json.RawMessage, statements []string, spanIndexes []int) (json.RawMessage, int, error) {
	if len(statements) == 0 {
		return nil, 0, fmt.Errorf("no statements provided")
	}

	// Step 1: convert hex trace IDs to base64, then unmarshal JSON trace data into protobuf types.
	fixedData, err := FixTraceSpanIDsForUnmarshal(traceData)
	if err != nil {
		return nil, 0, fmt.Errorf("fix trace IDs: %w", err)
	}
	var srcReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(fixedData, &srcReq); err != nil {
		return nil, 0, fmt.Errorf("unmarshal trace data: %w", err)
	}

	// Step 2: marshal protobuf to binary wire format.
	protoBytes, err := proto.Marshal(&srcReq)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal protobuf: %w", err)
	}

	// Step 3: unmarshal binary protobuf into pdata/ptrace types for OTTL.
	var unmarshaler ptrace.ProtoUnmarshaler
	traces, err := unmarshaler.UnmarshalTraces(protoBytes)
	if err != nil {
		return nil, 0, fmt.Errorf("unmarshal to pdata: %w", err)
	}

	// Step 4-6: transform all spans via OTTL and count selected spans.
	traces, flatIdx, err := TransformSpans(traces, statements, ErrorModePropagate)
	if err != nil {
		return nil, 0, err
	}

	// Step 7: count only selected spans if spanIndexes is specified.
	selectedCount := flatIdx
	if len(spanIndexes) > 0 {
		selected := make(map[int]bool)
		for _, idx := range spanIndexes {
			if idx >= 0 {
				selected[idx] = true
			}
		}
		selectedCount = 0
		for idx := 0; idx < flatIdx; idx++ {
			if selected[idx] {
				selectedCount++
			}
		}
	}

	// Step 8: convert modified pdata Traces back to OTLP protobuf JSON.
	var marshaler ptrace.ProtoMarshaler
	outBytes, err := marshaler.MarshalTraces(traces)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal pdata: %w", err)
	}

	var dstReq collectortracev1.ExportTraceServiceRequest
	if err := proto.Unmarshal(outBytes, &dstReq); err != nil {
		return nil, 0, fmt.Errorf("unmarshal pdata to protobuf: %w", err)
	}

	data, err := protojson.MarshalOptions{
		UseProtoNames: false,
		Indent:        "",
	}.Marshal(&dstReq)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal result JSON: %w", err)
	}

	outJSON, err := FixTraceSpanIDsForMarshal(data)
	if err != nil {
		return nil, 0, fmt.Errorf("fix trace IDs: %w", err)
	}
	return outJSON, selectedCount, nil
}

// TransformSpans executes OTTL transform statements against all spans in the
// provided ptrace.Traces. It returns the transformed traces and the total number
// of spans processed. The errorMode parameter controls how execution errors are
// handled.
func TransformSpans(traces ptrace.Traces, statements []string, errorMode ErrorMode) (ptrace.Traces, int, error) {
	if len(statements) == 0 {
		return traces, 0, fmt.Errorf("no statements provided")
	}

	settings := component.TelemetrySettings{
		Logger:         zap.NewNop(),
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  noop.NewMeterProvider(),
		Resource:       pcommon.NewResource(),
	}

	parser, err := ottlspan.NewParser(
		ottlfuncs.StandardFuncs[*ottlspan.TransformContext](),
		settings,
	)
	if err != nil {
		return traces, 0, fmt.Errorf("create OTTL parser: %w", err)
	}

	parsedStmts, err := parser.ParseStatements(statements)
	if err != nil {
		return traces, 0, fmt.Errorf("parse OTTL statements: %w", err)
	}

	seq := ottlspan.NewStatementSequence(parsedStmts, settings)

	ctx := context.Background()
	rss := traces.ResourceSpans()
	flatIdx := 0
	logger := zap.NewExample()

	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		sss := rs.ScopeSpans()
		for j := 0; j < sss.Len(); j++ {
			ss := sss.At(j)
			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				tCtx := ottlspan.NewTransformContext(rs, ss, span)
				execErr := seq.Execute(ctx, tCtx)
				tCtx.Close()
				if execErr != nil {
					switch errorMode {
					case ErrorModeIgnore:
						logger.Warn("OTTL execution error", zap.Error(execErr))
					case ErrorModeSilent:
					default:
						return traces, 0, fmt.Errorf("execute on span %d/%d/%d: %w", i, j, k, execErr)
					}
				}
				flatIdx++
			}
		}
	}

	return traces, flatIdx, nil
}

// TransformResult holds the outcome of a transform request.
type TransformResult struct {
	Valid                  bool             `json:"valid"`
	Errors                 []string         `json:"errors,omitempty"`
	OriginalData           *json.RawMessage `json:"original_data,omitempty"`
	TransformedData        *json.RawMessage `json:"transformed_data,omitempty"`
	StatementsParsed       int              `json:"statements_parsed,omitempty"`
	SpanCount              int              `json:"span_count,omitempty"`
	TransformedCount       int              `json:"transformed_count,omitempty"`
	LogOriginalData        *json.RawMessage `json:"log_original_data,omitempty"`
	LogTransformedData     *json.RawMessage `json:"log_transformed_data,omitempty"`
	LogStatementsParsed    int              `json:"log_statements_parsed,omitempty"`
	LogTransformedCount    int              `json:"log_transformed_count,omitempty"`
	MetricOriginalData     *json.RawMessage `json:"metric_original_data,omitempty"`
	MetricTransformedData  *json.RawMessage `json:"metric_transformed_data,omitempty"`
	MetricStatementsParsed int              `json:"metric_statements_parsed,omitempty"`
	MetricTransformedCount int              `json:"metric_transformed_count,omitempty"`
}
