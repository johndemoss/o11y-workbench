package ottl

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
// OTLP JSON, suitable for re-storing or displaying.
//
// The input traceData must be a JSON-serialized ExportTraceServiceRequest
// (protojson format). Each OTTL statement is executed against every span in
// the trace data.
func TransformRunData(traceData json.RawMessage, statements []string) (json.RawMessage, error) {
	if len(statements) == 0 {
		return nil, fmt.Errorf("no statements provided")
	}

	// Step 1: unmarshal JSON trace data into protobuf types.
	var srcReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(traceData, &srcReq); err != nil {
		return nil, fmt.Errorf("unmarshal trace data: %w", err)
	}

	// Step 2: marshal protobuf to binary wire format.
	protoBytes, err := proto.Marshal(&srcReq)
	if err != nil {
		return nil, fmt.Errorf("marshal protobuf: %w", err)
	}

	// Step 3: unmarshal binary protobuf into pdata/ptrace types for OTTL.
	var unmarshaler ptrace.ProtoUnmarshaler
	traces, err := unmarshaler.UnmarshalTraces(protoBytes)
	if err != nil {
		return nil, fmt.Errorf("unmarshal to pdata: %w", err)
	}

	// Step 4: set up OTTL span-context parser and statement sequence.
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
		return nil, fmt.Errorf("create OTTL parser: %w", err)
	}

	parsedStmts, err := parser.ParseStatements(statements)
	if err != nil {
		return nil, fmt.Errorf("parse OTTL statements: %w", err)
	}

	seq := ottlspan.NewStatementSequence(parsedStmts, settings)

	// Step 5: execute statements against every span in the trace data.
	ctx := context.Background()
	rss := traces.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		sss := rs.ScopeSpans()
		for j := 0; j < sss.Len(); j++ {
			ss := sss.At(j)
			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				tCtx := ottlspan.NewTransformContextPtr(rs, ss, span)
				if err := seq.Execute(ctx, tCtx); err != nil {
					tCtx.Close()
					return nil, fmt.Errorf("execute on span %d/%d/%d: %w", i, j, k, err)
				}
				tCtx.Close()
			}
		}
	}

	// Step 6: convert modified pdata Traces back to OTLP protobuf JSON.
	var marshaler ptrace.ProtoMarshaler
	outBytes, err := marshaler.MarshalTraces(traces)
	if err != nil {
		return nil, fmt.Errorf("marshal pdata: %w", err)
	}

	var dstReq collectortracev1.ExportTraceServiceRequest
	if err := proto.Unmarshal(outBytes, &dstReq); err != nil {
		return nil, fmt.Errorf("unmarshal pdata to protobuf: %w", err)
	}

	outJSON, err := protojson.MarshalOptions{
		UseProtoNames: true,
		Indent:        "  ",
	}.Marshal(&dstReq)
	if err != nil {
		return nil, fmt.Errorf("marshal result JSON: %w", err)
	}

	return outJSON, nil
}

// TransformResult holds the outcome of a transform request.
type TransformResult struct {
	Valid            bool             `json:"valid"`
	Errors           []string         `json:"errors,omitempty"`
	TransformedData  *json.RawMessage `json:"transformed_data,omitempty"`
	StatementsParsed int              `json:"statements_parsed,omitempty"`
}
