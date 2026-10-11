package engine

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
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottllog"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"
)

// FilterLogRunData evaluates OTTL conditions against each log record in the log data
// and returns a filtered result containing only the logs that match (include mode)
// or only the logs that do NOT match (exclude mode).
//
// Conditions are ORed together — if ANY condition evaluates to true for a log,
// the log is considered a match.
func FilterLogRunData(logData json.RawMessage, conditions []string, mode FilterMode) (json.RawMessage, int, int, error) {
	if len(conditions) == 0 {
		return nil, 0, 0, fmt.Errorf("no conditions provided")
	}

	// Step 1: unmarshal JSON log data into protobuf types.
	var srcReq collectorlogsv1.ExportLogsServiceRequest
	if err := protojson.Unmarshal(logData, &srcReq); err != nil {
		return nil, 0, 0, fmt.Errorf("unmarshal log data: %w", err)
	}

	// Step 2: marshal protobuf to binary wire format, then to pdata for OTTL.
	protoBytes, err := proto.Marshal(&srcReq)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("marshal protobuf: %w", err)
	}

	var unmarshaler plog.ProtoUnmarshaler
	logs, err := unmarshaler.UnmarshalLogs(protoBytes)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("unmarshal to pdata: %w", err)
	}

	// Step 3: set up OTTL log-context parser and condition sequence.
	settings := component.TelemetrySettings{
		Logger:         zap.NewNop(),
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  noop.NewMeterProvider(),
		Resource:       pcommon.NewResource(),
	}

	parser, err := ottllog.NewParser(
		ottlfuncs.StandardFuncs[*ottllog.TransformContext](),
		settings,
	)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("create OTTL log parser: %w", err)
	}

	parsedConditions, err := parser.ParseConditions(conditions)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("parse OTTL conditions: %w", err)
	}

	condSeq := ottllog.NewConditionSequence(parsedConditions, settings)

	// Step 4: evaluate conditions against every log record, recording matches.
	ctx := context.Background()
	rls := logs.ResourceLogs()

	type logKey struct {
		ri, si, pi int
	}
	matched := make(map[logKey]bool)

	totalLogs := 0
	for i := 0; i < rls.Len(); i++ {
		rl := rls.At(i)
		sls := rl.ScopeLogs()
		for j := 0; j < sls.Len(); j++ {
			sl := sls.At(j)
			lrs := sl.LogRecords()
			for k := 0; k < lrs.Len(); k++ {
				lr := lrs.At(k)
				tCtx := ottllog.NewTransformContext(rl, sl, lr)

				isMatch, evalErr := condSeq.Eval(ctx, tCtx)
				tCtx.Close()
				if evalErr != nil {
					return nil, 0, 0, fmt.Errorf("evaluate condition on log %d/%d/%d: %w", i, j, k, evalErr)
				}

				var keep bool
				switch mode {
				case FilterModeInclude:
					keep = isMatch
				case FilterModeExclude:
					keep = !isMatch
				default:
					return nil, 0, 0, fmt.Errorf("unknown filter mode: %s", mode)
				}

				if keep {
					matched[logKey{i, j, k}] = true
				}
				totalLogs++
			}
		}
	}

	// Step 5: build a new ExportLogsServiceRequest containing only matched logs.
	var filteredReq collectorlogsv1.ExportLogsServiceRequest
	for i, rls := range srcReq.ResourceLogs {
		var outRls logsv1.ResourceLogs
		outRls.Resource = rls.Resource
		outRls.SchemaUrl = rls.SchemaUrl

		for j, sls := range rls.ScopeLogs {
			var outSls logsv1.ScopeLogs
			outSls.Scope = sls.Scope
			outSls.SchemaUrl = sls.SchemaUrl

			for k, lr := range sls.LogRecords {
				if matched[logKey{i, j, k}] {
					outSls.LogRecords = append(outSls.LogRecords, lr)
				}
			}

			if len(outSls.LogRecords) > 0 {
				outRls.ScopeLogs = append(outRls.ScopeLogs, &outSls)
			}
		}

		if len(outRls.ScopeLogs) > 0 {
			filteredReq.ResourceLogs = append(filteredReq.ResourceLogs, &outRls)
		}
	}

	outJSON, err := protojson.MarshalOptions{
		UseProtoNames: false,
		Indent:        "",
	}.Marshal(&filteredReq)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("marshal result JSON: %w", err)
	}

	filteredCount := len(matched)
	return outJSON, totalLogs, filteredCount, nil
}
