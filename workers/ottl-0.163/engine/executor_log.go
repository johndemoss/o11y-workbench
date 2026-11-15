package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	collectorottl "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottllog"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlresource"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlscope"
)

// TransformLogRunData parses and executes OTTL transform statements against log
// data stored in a completed run. It returns the transformed log data as
// OTLP JSON. Each OTTL statement is executed against every log record.
func TransformLogRunData(ctx context.Context, logData json.RawMessage, statements []string) (json.RawMessage, error) {
	out, _, err := TransformLogRunDataWithErrorMode(ctx, logData, statements, ErrorModePropagate)
	return out, err
}

// TransformLogRunDataWithErrorMode is the pipeline-oriented entry point: it applies
// statements with the given error mode and returns the number of log records processed.
func TransformLogRunDataWithErrorMode(ctx context.Context, logData json.RawMessage, statements []string, errorMode ErrorMode) (json.RawMessage, int, error) {
	if len(statements) == 0 {
		return nil, 0, fmt.Errorf("no statements provided")
	}

	logs, err := UnmarshalLogsFromJSON(logData)
	if err != nil {
		return nil, 0, err
	}

	transformed, count, err := TransformLogs(ctx, logs, statements, errorMode)
	if err != nil {
		return nil, 0, err
	}

	outJSON, err := MarshalLogsToJSON(transformed)
	if err != nil {
		return nil, 0, err
	}
	return outJSON, count, nil
}

// TransformLogRunDataWithGroups executes grouped log_statements (context + conditions)
// using upstream ottllog path context names when a context is specified.
func TransformLogRunDataWithGroups(ctx context.Context, logData json.RawMessage, groups []ContextStatements, errorMode ErrorMode) (json.RawMessage, int, error) {
	if len(groups) == 0 {
		return nil, 0, fmt.Errorf("no statement groups provided")
	}

	logs, err := UnmarshalLogsFromJSON(logData)
	if err != nil {
		return nil, 0, err
	}

	transformed, count, err := TransformLogsWithGroups(ctx, logs, groups, errorMode)
	if err != nil {
		return nil, 0, err
	}

	outJSON, err := MarshalLogsToJSON(transformed)
	if err != nil {
		return nil, 0, err
	}
	return outJSON, count, nil
}

// TransformLogs executes OTTL transform statements against every log record in logs.
func TransformLogs(ctx context.Context, logs plog.Logs, statements []string, errorMode ErrorMode) (plog.Logs, int, error) {
	if len(statements) == 0 {
		return logs, 0, fmt.Errorf("no statements provided")
	}

	settings := component.TelemetrySettings{
		Logger:         zap.NewNop(),
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  noop.NewMeterProvider(),
		Resource:       pcommon.NewResource(),
	}

	parser, err := ottllog.NewParser(
		ParserFunctions[*ottllog.TransformContext](),
		settings,
	)
	if err != nil {
		return logs, 0, fmt.Errorf("create OTTL log parser: %w", err)
	}

	parsedStmts, err := parser.ParseStatements(statements)
	if err != nil {
		return logs, 0, fmt.Errorf("parse OTTL statements: %w", err)
	}

	mode := logCollectorErrorMode(errorMode)
	seq := ottllog.NewStatementSequence(parsedStmts, settings, ottllog.WithStatementSequenceErrorMode(mode))

	return executeLogStatementSequence(ctx, logs, seq, errorMode)
}

// TransformLogsWithGroups runs each ContextStatements group against the appropriate
// upstream OTTL log context (log record, instrumentation scope, or resource).
func TransformLogsWithGroups(ctx context.Context, logs plog.Logs, groups []ContextStatements, errorMode ErrorMode) (plog.Logs, int, error) {
	if len(groups) == 0 {
		return logs, 0, fmt.Errorf("no statement groups provided")
	}

	settings := component.TelemetrySettings{
		Logger:         zap.NewNop(),
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  noop.NewMeterProvider(),
		Resource:       pcommon.NewResource(),
	}

	mode := logCollectorErrorMode(errorMode)
	total := 0

	for _, group := range groups {
		if len(group.Statements) == 0 {
			continue
		}

		var (
			n   int
			err error
		)
		switch logGroupExecutionKind(group.Context) {
		case logGroupResource:
			n, err = runLogResourceStatementGroup(ctx, logs, group, settings, mode, errorMode)
		case logGroupScope:
			n, err = runLogScopeStatementGroup(ctx, logs, group, settings, mode, errorMode)
		default:
			n, err = runLogRecordStatementGroup(ctx, logs, group, settings, mode, errorMode)
		}
		total += n
		if err != nil {
			return logs, total, err
		}
	}

	return logs, total, nil
}

func executeLogStatementSequence(ctx context.Context, logs plog.Logs, seq collectorottl.StatementSequence[*ottllog.TransformContext], errorMode ErrorMode) (plog.Logs, int, error) {
	logger := zap.NewExample()
	flatIdx := 0

	rls := logs.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		rl := rls.At(i)
		sls := rl.ScopeLogs()
		for j := 0; j < sls.Len(); j++ {
			sl := sls.At(j)
			lrs := sl.LogRecords()
			for k := 0; k < lrs.Len(); k++ {
				if err := ctx.Err(); err != nil {
					return logs, flatIdx, err
				}
				lr := lrs.At(k)
				tCtx := ottllog.NewTransformContext(rl, sl, lr)
				execErr := seq.Execute(ctx, tCtx)
				tCtx.Close()
				if execErr != nil {
					switch errorMode {
					case ErrorModeIgnore:
						logger.Warn("OTTL execution error", zap.Error(execErr))
					case ErrorModeSilent:
					default:
						return logs, flatIdx, fmt.Errorf("execute on log %d/%d/%d: %w", i, j, k, execErr)
					}
				}
				flatIdx++
			}
		}
	}

	return logs, flatIdx, nil
}

type logGroupKind int

const (
	logGroupRecord logGroupKind = iota
	logGroupScope
	logGroupResource
)

func logGroupExecutionKind(contextName string) logGroupKind {
	switch strings.ToLower(strings.TrimSpace(contextName)) {
	case "resource":
		return logGroupResource
	case "scope", "instrumentation_scope", "instrumentationscope":
		return logGroupScope
	default:
		return logGroupRecord
	}
}

func logGroupRequiresPathContextNames(contextName string) bool {
	_ = contextName
	// Group statements execute inside their declared context, so implicit
	// paths (attributes[...], body, ...) must resolve normally.
	return false
}

func runLogRecordStatementGroup(
	ctx context.Context,
	logs plog.Logs,
	group ContextStatements,
	settings component.TelemetrySettings,
	mode collectorottl.ErrorMode,
	errorMode ErrorMode,
) (int, error) {
	var parserOpts []collectorottl.Option[*ottllog.TransformContext]
	if logGroupRequiresPathContextNames(group.Context) {
		parserOpts = append(parserOpts, ottllog.EnablePathContextNames())
	}

	parser, err := ottllog.NewParser(ParserFunctions[*ottllog.TransformContext](), settings, parserOpts...)
	if err != nil {
		return 0, fmt.Errorf("create OTTL log parser: %w", err)
	}

	parsedStmts, err := parser.ParseStatements(group.Statements)
	if err != nil {
		return 0, fmt.Errorf("parse OTTL statements: %w", err)
	}

	stmtSeq := ottllog.NewStatementSequence(parsedStmts, settings, ottllog.WithStatementSequenceErrorMode(mode))

	hasConds := len(group.Conditions) > 0
	var condSeq collectorottl.ConditionSequence[*ottllog.TransformContext]
	if hasConds {
		parsedConds, err := parser.ParseConditions(group.Conditions)
		if err != nil {
			return 0, fmt.Errorf("parse OTTL conditions: %w", err)
		}
		condSeq = newLogRecordConditionSequence(parsedConds, settings, mode)
	}

	logger := zap.NewExample()
	count := 0
	rls := logs.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		rl := rls.At(i)
		sls := rl.ScopeLogs()
		for j := 0; j < sls.Len(); j++ {
			sl := sls.At(j)
			lrs := sl.LogRecords()
			for k := 0; k < lrs.Len(); k++ {
				if err := ctx.Err(); err != nil {
					return count, err
				}
				lr := lrs.At(k)
				tCtx := ottllog.NewTransformContext(rl, sl, lr)
				if hasConds {
					match, evalErr := condSeq.Eval(ctx, tCtx)
					if evalErr != nil {
						tCtx.Close()
						skip, stopErr := handleLogConditionError(errorMode, logger, evalErr, fmt.Sprintf("log %d/%d/%d", i, j, k))
						count++
						if stopErr != nil {
							return count, stopErr
						}
						if skip {
							continue
						}
					}
					if !match {
						tCtx.Close()
						count++
						continue
					}
				}
				execErr := stmtSeq.Execute(ctx, tCtx)
				tCtx.Close()
				if stopErr := handleLogExecutionError(errorMode, logger, execErr, fmt.Sprintf("log %d/%d/%d", i, j, k)); stopErr != nil {
					return count, stopErr
				}
				count++
			}
		}
	}
	return count, nil
}

func runLogScopeStatementGroup(
	ctx context.Context,
	logs plog.Logs,
	group ContextStatements,
	settings component.TelemetrySettings,
	mode collectorottl.ErrorMode,
	errorMode ErrorMode,
) (int, error) {
	parser, err := ottlscope.NewParser(ParserFunctions[*ottlscope.TransformContext](), settings)
	if err != nil {
		return 0, fmt.Errorf("create OTTL scope parser: %w", err)
	}

	parsedStmts, err := parser.ParseStatements(group.Statements)
	if err != nil {
		return 0, fmt.Errorf("parse OTTL statements: %w", err)
	}

	stmtSeq := ottlscope.NewStatementSequence(parsedStmts, settings, ottlscope.WithStatementSequenceErrorMode(mode))

	hasConds := len(group.Conditions) > 0
	var condSeq collectorottl.ConditionSequence[*ottlscope.TransformContext]
	if hasConds {
		parsedConds, err := parser.ParseConditions(group.Conditions)
		if err != nil {
			return 0, fmt.Errorf("parse OTTL conditions: %w", err)
		}
		condSeq = newScopeConditionSequence(parsedConds, settings, mode)
	}

	logger := zap.NewExample()
	count := 0
	rls := logs.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		rl := rls.At(i)
		sls := rl.ScopeLogs()
		for j := 0; j < sls.Len(); j++ {
			if err := ctx.Err(); err != nil {
				return count, err
			}
			sl := sls.At(j)
			recordCount := sl.LogRecords().Len()
			tCtx := ottlscope.NewTransformContext(sl.Scope(), rl.Resource(), sl, rl)
			if hasConds {
				match, evalErr := condSeq.Eval(ctx, tCtx)
				if evalErr != nil {
					tCtx.Close()
					skip, stopErr := handleLogConditionError(errorMode, logger, evalErr, fmt.Sprintf("scope %d/%d", i, j))
					count += recordCount
					if stopErr != nil {
						return count, stopErr
					}
					if skip {
						continue
					}
				}
				if !match {
					tCtx.Close()
					count += recordCount
					continue
				}
			}
			execErr := stmtSeq.Execute(ctx, tCtx)
			tCtx.Close()
			if stopErr := handleLogExecutionError(errorMode, logger, execErr, fmt.Sprintf("scope %d/%d", i, j)); stopErr != nil {
				return count, stopErr
			}
			count += recordCount
		}
	}
	return count, nil
}

func runLogResourceStatementGroup(
	ctx context.Context,
	logs plog.Logs,
	group ContextStatements,
	settings component.TelemetrySettings,
	mode collectorottl.ErrorMode,
	errorMode ErrorMode,
) (int, error) {
	parser, err := ottlresource.NewParser(ParserFunctions[*ottlresource.TransformContext](), settings)
	if err != nil {
		return 0, fmt.Errorf("create OTTL resource parser: %w", err)
	}

	parsedStmts, err := parser.ParseStatements(group.Statements)
	if err != nil {
		return 0, fmt.Errorf("parse OTTL statements: %w", err)
	}

	stmtSeq := ottlresource.NewStatementSequence(parsedStmts, settings, ottlresource.WithStatementSequenceErrorMode(mode))

	hasConds := len(group.Conditions) > 0
	var condSeq collectorottl.ConditionSequence[*ottlresource.TransformContext]
	if hasConds {
		parsedConds, err := parser.ParseConditions(group.Conditions)
		if err != nil {
			return 0, fmt.Errorf("parse OTTL conditions: %w", err)
		}
		condSeq = newResourceConditionSequence(parsedConds, settings, mode)
	}

	logger := zap.NewExample()
	count := 0
	rls := logs.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		rl := rls.At(i)
		recordCount := countLogRecordsInResource(rl)
		tCtx := ottlresource.NewTransformContext(rl.Resource(), rl)
		if hasConds {
			match, evalErr := condSeq.Eval(ctx, tCtx)
			if evalErr != nil {
				tCtx.Close()
				skip, stopErr := handleLogConditionError(errorMode, logger, evalErr, fmt.Sprintf("resource %d", i))
				count += recordCount
				if stopErr != nil {
					return count, stopErr
				}
				if skip {
					continue
				}
			}
			if !match {
				tCtx.Close()
				count += recordCount
				continue
			}
		}
		execErr := stmtSeq.Execute(ctx, tCtx)
		tCtx.Close()
		if stopErr := handleLogExecutionError(errorMode, logger, execErr, fmt.Sprintf("resource %d", i)); stopErr != nil {
			return count, stopErr
		}
		count += recordCount
	}
	return count, nil
}

func newLogRecordConditionSequence(
	parsed []*collectorottl.Condition[*ottllog.TransformContext],
	settings component.TelemetrySettings,
	mode collectorottl.ErrorMode,
) collectorottl.ConditionSequence[*ottllog.TransformContext] {
	return collectorottl.NewConditionSequence(parsed, settings,
		collectorottl.WithConditionSequenceErrorMode[*ottllog.TransformContext](mode),
		collectorottl.WithLogicOperation[*ottllog.TransformContext](collectorottl.Or),
	)
}

func newScopeConditionSequence(
	parsed []*collectorottl.Condition[*ottlscope.TransformContext],
	settings component.TelemetrySettings,
	mode collectorottl.ErrorMode,
) collectorottl.ConditionSequence[*ottlscope.TransformContext] {
	return collectorottl.NewConditionSequence(parsed, settings,
		collectorottl.WithConditionSequenceErrorMode[*ottlscope.TransformContext](mode),
		collectorottl.WithLogicOperation[*ottlscope.TransformContext](collectorottl.Or),
	)
}

func newResourceConditionSequence(
	parsed []*collectorottl.Condition[*ottlresource.TransformContext],
	settings component.TelemetrySettings,
	mode collectorottl.ErrorMode,
) collectorottl.ConditionSequence[*ottlresource.TransformContext] {
	return collectorottl.NewConditionSequence(parsed, settings,
		collectorottl.WithConditionSequenceErrorMode[*ottlresource.TransformContext](mode),
		collectorottl.WithLogicOperation[*ottlresource.TransformContext](collectorottl.Or),
	)
}

func countLogRecordsInResource(rl plog.ResourceLogs) int {
	total := 0
	sls := rl.ScopeLogs()
	for j := 0; j < sls.Len(); j++ {
		total += sls.At(j).LogRecords().Len()
	}
	return total
}

func handleLogConditionError(errorMode ErrorMode, logger *zap.Logger, evalErr error, where string) (skip bool, stop error) {
	if evalErr == nil {
		return false, nil
	}
	switch errorMode {
	case ErrorModeIgnore:
		logger.Warn("OTTL condition error", zap.Error(evalErr), zap.String("where", where))
		return true, nil
	case ErrorModeSilent:
		return true, nil
	default:
		return true, fmt.Errorf("evaluate condition on %s: %w", where, evalErr)
	}
}

func handleLogExecutionError(errorMode ErrorMode, logger *zap.Logger, execErr error, where string) error {
	if execErr == nil {
		return nil
	}
	switch errorMode {
	case ErrorModeIgnore:
		logger.Warn("OTTL execution error", zap.Error(execErr), zap.String("where", where))
		return nil
	case ErrorModeSilent:
		return nil
	default:
		return fmt.Errorf("execute on %s: %w", where, execErr)
	}
}

func logCollectorErrorMode(errorMode ErrorMode) collectorottl.ErrorMode {
	switch errorMode {
	case ErrorModeIgnore:
		return collectorottl.IgnoreError
	case ErrorModeSilent:
		return collectorottl.SilentError
	default:
		return collectorottl.PropagateError
	}
}
