package ottl

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottllog"
)

// TransformLogRunData parses and executes OTTL transform statements against log
// data stored in a completed run. It returns the transformed log data as
// OTLP JSON. Each OTTL statement is executed against every log record.
func TransformLogRunData(logData json.RawMessage, statements []string) (json.RawMessage, error) {
	if len(statements) == 0 {
		return nil, fmt.Errorf("no statements provided")
	}

	// Step 1: unmarshal JSON log data into protobuf types.
	var srcReq collectorlogsv1.ExportLogsServiceRequest
	if err := protojson.Unmarshal(logData, &srcReq); err != nil {
		return nil, fmt.Errorf("unmarshal log data: %w", err)
	}

	// Step 2: marshal protobuf to binary wire format.
	protoBytes, err := proto.Marshal(&srcReq)
	if err != nil {
		return nil, fmt.Errorf("marshal protobuf: %w", err)
	}

	// Step 3: unmarshal binary protobuf into pdata/plog types for OTTL.
	var unmarshaler plog.ProtoUnmarshaler
	logs, err := unmarshaler.UnmarshalLogs(protoBytes)
	if err != nil {
		return nil, fmt.Errorf("unmarshal to pdata: %w", err)
	}

	// Step 4: set up OTTL log-context parser and statement sequence.
	settings := component.TelemetrySettings{
		Logger:         zap.NewNop(),
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  noop.NewMeterProvider(),
		Resource:       pcommon.NewResource(),
	}

	parser, err := ottllog.NewParser(
		parserFuncs[*ottllog.TransformContext](),
		settings,
	)
	if err != nil {
		return nil, fmt.Errorf("create OTTL log parser: %w", err)
	}

	parsedStmts, err := parser.ParseStatements(statements)
	if err != nil {
		return nil, fmt.Errorf("parse OTTL statements: %w", err)
	}

	seq := ottllog.NewStatementSequence(parsedStmts, settings)

	// Step 5: execute statements against all log records.
	ctx := context.Background()
	rls := logs.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		rl := rls.At(i)
		sls := rl.ScopeLogs()
		for j := 0; j < sls.Len(); j++ {
			sl := sls.At(j)
			lrs := sl.LogRecords()
			for k := 0; k < lrs.Len(); k++ {
				lr := lrs.At(k)
				tCtx := ottllog.NewTransformContext(rl, sl, lr)
				if err := seq.Execute(ctx, tCtx); err != nil {
					tCtx.Close()
					return nil, fmt.Errorf("execute on log %d/%d/%d: %w", i, j, k, err)
				}
				tCtx.Close()
			}
		}
	}

	// Step 6: convert modified pdata Logs back to OTLP protobuf JSON.
	var marshaler plog.ProtoMarshaler
	outBytes, err := marshaler.MarshalLogs(logs)
	if err != nil {
		return nil, fmt.Errorf("marshal pdata: %w", err)
	}

	var dstReq collectorlogsv1.ExportLogsServiceRequest
	if err := proto.Unmarshal(outBytes, &dstReq); err != nil {
		return nil, fmt.Errorf("unmarshal pdata to protobuf: %w", err)
	}

	outJSON, err := protojson.MarshalOptions{
		UseProtoNames: false,
		Indent:        "",
	}.Marshal(&dstReq)
	if err != nil {
		return nil, fmt.Errorf("marshal result JSON: %w", err)
	}

	return outJSON, nil
}
