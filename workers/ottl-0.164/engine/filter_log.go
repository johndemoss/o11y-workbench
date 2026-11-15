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

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottllog"
)

// FilterLogRunData evaluates OTTL conditions against each log record in the log data
// and returns a filtered result containing only the logs that match (include mode)
// or only the logs that do NOT match (exclude mode).
//
// Conditions are ORed together — if ANY condition evaluates to true for a log,
// the log is considered a match.
func FilterLogRunData(ctx context.Context, logData json.RawMessage, conditions []string, mode FilterMode) (json.RawMessage, int, int, error) {
	return FilterLogRunDataWithErrorMode(ctx, logData, conditions, mode, ErrorModePropagate)
}

// FilterLogRunDataWithErrorMode is the pipeline entry point; use FilterModeExclude
// for collector-aligned filter processors (drop matching telemetry).
func FilterLogRunDataWithErrorMode(ctx context.Context, logData json.RawMessage, conditions []string, mode FilterMode, errorMode ErrorMode) (json.RawMessage, int, int, error) {
	if err := checkFilterCtx(ctx); err != nil {
		return nil, 0, 0, err
	}
	if len(conditions) == 0 {
		return nil, 0, 0, fmt.Errorf("no conditions provided")
	}

	logs, err := UnmarshalLogsFromJSON(logData)
	if err != nil {
		return nil, 0, 0, err
	}

	filtered, total, kept, err := FilterLogRecords(ctx, logs, conditions, mode, errorMode)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("filter log records: %w", err)
	}

	outJSON, err := MarshalLogsToJSON(filtered)
	if err != nil {
		return nil, 0, 0, err
	}
	return outJSON, total, kept, nil
}

// FilterLogRecords evaluates OTTL conditions against all log records in plog.Logs and
// returns filtered logs. errorMode controls per-item evaluation errors (same as FilterSpans).
func FilterLogRecords(ctx context.Context, logs plog.Logs, conditions []string, mode FilterMode, errorMode ErrorMode) (plog.Logs, int, int, error) {
	if err := checkFilterCtx(ctx); err != nil {
		return logs, 0, 0, err
	}
	if len(conditions) == 0 {
		return logs, 0, 0, fmt.Errorf("no conditions provided")
	}

	settings := component.TelemetrySettings{
		Logger:         zap.NewNop(),
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  noop.NewMeterProvider(),
		Resource:       pcommon.NewResource(),
	}

	parser, err := ottllog.NewParser(
		parserFunctions[*ottllog.TransformContext](),
		settings,
	)
	if err != nil {
		return logs, 0, 0, fmt.Errorf("create OTTL log parser: %w", err)
	}

	parsedConditions, err := parser.ParseConditions(conditions)
	if err != nil {
		return logs, 0, 0, fmt.Errorf("parse OTTL conditions: %w", err)
	}

	condSeq := ottllog.NewConditionSequence(parsedConditions, settings)

	rls := logs.ResourceLogs()

	type logKey struct {
		ri, si, pi int
	}
	matched := make(map[logKey]bool)

	totalLogs := 0

	var ignoreLogger *zap.Logger
	if errorMode == ErrorModeIgnore {
		ignoreLogger = zap.NewExample()
		defer ignoreLogger.Sync()
	}

	for i := 0; i < rls.Len(); i++ {
		if err := checkFilterCtx(ctx); err != nil {
			return logs, 0, 0, err
		}
		rl := rls.At(i)
		sls := rl.ScopeLogs()
		for j := 0; j < sls.Len(); j++ {
			sl := sls.At(j)
			lrs := sl.LogRecords()
			for k := 0; k < lrs.Len(); k++ {
				if err := checkFilterCtx(ctx); err != nil {
					return logs, 0, 0, err
				}
				lr := lrs.At(k)
				tCtx := ottllog.NewTransformContext(rl, sl, lr)

				isMatch, evalErr := condSeq.Eval(ctx, tCtx)
				tCtx.Close()
				if evalErr != nil {
					switch errorMode {
					case ErrorModePropagate:
						return logs, 0, 0, fmt.Errorf("evaluate condition on log %d/%d/%d: %w", i, j, k, evalErr)
					case ErrorModeIgnore:
						ignoreLogger.Warn("failed to evaluate condition, skipping log record",
							zap.Error(evalErr),
							zap.Int("resource", i),
							zap.Int("scope", j),
							zap.Int("log", k),
						)
						isMatch = false
					case ErrorModeSilent:
						isMatch = false
					default:
						return logs, 0, 0, fmt.Errorf("unknown error mode: %s", errorMode)
					}
				}

				keep, err := filterKeep(isMatch, mode)
				if err != nil {
					return logs, 0, 0, err
				}

				if keep {
					matched[logKey{i, j, k}] = true
				}
				totalLogs++
			}
		}
	}

	filtered := plog.NewLogs()
	outRls := filtered.ResourceLogs()

	for i := 0; i < rls.Len(); i++ {
		rl := rls.At(i)
		sls := rl.ScopeLogs()

		hasRlMatch := false
		for j := 0; j < sls.Len() && !hasRlMatch; j++ {
			lrs := sls.At(j).LogRecords()
			for k := 0; k < lrs.Len() && !hasRlMatch; k++ {
				if matched[logKey{i, j, k}] {
					hasRlMatch = true
				}
			}
		}
		if !hasRlMatch {
			continue
		}

		outRl := outRls.AppendEmpty()
		rl.Resource().CopyTo(outRl.Resource())
		outRl.SetSchemaUrl(rl.SchemaUrl())
		outSls := outRl.ScopeLogs()

		for j := 0; j < sls.Len(); j++ {
			sl := sls.At(j)
			lrs := sl.LogRecords()

			hasSlMatch := false
			for k := 0; k < lrs.Len() && !hasSlMatch; k++ {
				if matched[logKey{i, j, k}] {
					hasSlMatch = true
				}
			}
			if !hasSlMatch {
				continue
			}

			outSl := outSls.AppendEmpty()
			sl.Scope().CopyTo(outSl.Scope())
			outSl.SetSchemaUrl(sl.SchemaUrl())
			outLrs := outSl.LogRecords()

			for k := 0; k < lrs.Len(); k++ {
				if matched[logKey{i, j, k}] {
					lr := lrs.At(k)
					outLr := outLrs.AppendEmpty()
					lr.CopyTo(outLr)
				}
			}
		}
	}

	filteredCount := len(matched)
	return filtered, totalLogs, filteredCount, nil
}
