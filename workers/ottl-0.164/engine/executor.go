package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"

	collectorottl "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlresource"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlscope"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlspan"
)

// TransformRunData parses and executes OTTL transform statements against trace
// data stored in a completed run. It returns the transformed trace data as
// OTLP JSON, plus the total number of spans processed. Each OTTL statement is
// executed against every span.
func TransformRunData(ctx context.Context, traceData json.RawMessage, statements []string) (json.RawMessage, int, error) {
	return TransformRunDataSelected(ctx, traceData, statements, nil)
}

// TransformRunDataSelected is like TransformRunData but only counts spans
// at the given flat indexes for the returned span count. spanIndexes nil or
// empty means all spans are counted. The transform is applied to all spans.
// The flat index walks ResourceSpans(i) → ScopeSpans(j) → Spans(k) depth-first.
func TransformRunDataSelected(ctx context.Context, traceData json.RawMessage, statements []string, spanIndexes []int) (json.RawMessage, int, error) {
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
	traces, flatIdx, err := TransformSpans(ctx, traces, statements, ErrorModePropagate)
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
func TransformSpans(ctx context.Context, traces ptrace.Traces, statements []string, errorMode ErrorMode) (ptrace.Traces, int, error) {
	if len(statements) == 0 {
		return traces, 0, fmt.Errorf("no statements provided")
	}
	return TransformSpansWithGroups(ctx, traces, TraceStatementGroups(statements), errorMode)
}

// TraceStatementGroups wraps a flat statement list as a single span-context group.
func TraceStatementGroups(statements []string) []ContextStatements {
	if len(statements) == 0 {
		return nil
	}
	return []ContextStatements{{Statements: statements}}
}

// TransformSpansWithGroups runs each ContextStatements group against the appropriate
// trace signal context (span, scope, or resource). Group conditions are ORed together.
func TransformSpansWithGroups(ctx context.Context, traces ptrace.Traces, groups []ContextStatements, errorMode ErrorMode) (ptrace.Traces, int, error) {
	if len(groups) == 0 || totalTraceStatements(groups) == 0 {
		return traces, 0, fmt.Errorf("no statements provided")
	}

	mode := traceCollectorErrorMode(errorMode)

	for _, group := range groups {
		if len(group.Statements) == 0 {
			continue
		}

		if err := executeTraceStatementGroup(ctx, traces, group, mode, errorMode); err != nil {
			return traces, 0, err
		}
	}

	return traces, countSpans(traces), nil
}

func totalTraceStatements(groups []ContextStatements) int {
	n := 0
	for _, g := range groups {
		n += len(g.Statements)
	}
	return n
}

func traceCollectorErrorMode(errorMode ErrorMode) collectorottl.ErrorMode {
	switch errorMode {
	case ErrorModeIgnore:
		return collectorottl.IgnoreError
	case ErrorModeSilent:
		return collectorottl.SilentError
	default:
		return collectorottl.PropagateError
	}
}

func parseTraceGroupContext(contextName string) (traceGroupKind, error) {
	switch strings.ToLower(strings.TrimSpace(contextName)) {
	case "":
		return traceGroupSpan, nil
	case "span":
		return traceGroupSpan, nil
	case "resource":
		return traceGroupResource, nil
	case "scope", "instrumentation_scope", "instrumentationscope":
		return traceGroupScope, nil
	default:
		return 0, fmt.Errorf("unsupported trace statement context: %q", contextName)
	}
}

func traceSpanPathContextNamesEnabled(contextName string) bool {
	return strings.EqualFold(strings.TrimSpace(contextName), "span")
}

type traceGroupKind int

const (
	traceGroupSpan traceGroupKind = iota
	traceGroupScope
	traceGroupResource
)

func executeTraceStatementGroup(
	ctx context.Context,
	traces ptrace.Traces,
	group ContextStatements,
	mode collectorottl.ErrorMode,
	errorMode ErrorMode,
) error {
	settings := component.TelemetrySettings{
		Logger:         zap.NewNop(),
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  noop.NewMeterProvider(),
		Resource:       pcommon.NewResource(),
	}

	kind, err := parseTraceGroupContext(group.Context)
	if err != nil {
		return err
	}

	switch kind {
	case traceGroupResource:
		return executeTraceResourceGroup(ctx, traces, group, settings, mode, errorMode)
	case traceGroupScope:
		return executeTraceScopeGroup(ctx, traces, group, settings, mode, errorMode)
	default:
		return executeTraceSpanGroup(ctx, traces, group, settings, mode, errorMode, traceSpanPathContextNamesEnabled(group.Context))
	}
}

func executeTraceSpanGroup(
	ctx context.Context,
	traces ptrace.Traces,
	group ContextStatements,
	settings component.TelemetrySettings,
	mode collectorottl.ErrorMode,
	errorMode ErrorMode,
	enablePathContextNames bool,
) error {
	var parserOpts []collectorottl.Option[*ottlspan.TransformContext]
	if enablePathContextNames {
		parserOpts = append(parserOpts, ottlspan.EnablePathContextNames())
	}

	parser, err := ottlspan.NewParser(
		parserFunctions[*ottlspan.TransformContext](),
		settings,
		parserOpts...,
	)
	if err != nil {
		return fmt.Errorf("create OTTL span parser: %w", err)
	}

	parsedStmts, err := parser.ParseStatements(group.Statements)
	if err != nil {
		return fmt.Errorf("parse OTTL statements: %w", err)
	}

	stmtSeq := ottlspan.NewStatementSequence(parsedStmts, settings, ottlspan.WithStatementSequenceErrorMode(mode))

	var condSeq collectorottl.ConditionSequence[*ottlspan.TransformContext]
	hasConds := len(group.Conditions) > 0
	if hasConds {
		parsedConds, err := parser.ParseConditions(group.Conditions)
		if err != nil {
			return fmt.Errorf("parse OTTL conditions: %w", err)
		}
		condSeq = newTraceSpanConditionSequence(parsedConds, settings, mode)
	}

	logger := zap.NewExample()
	rss := traces.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		sss := rs.ScopeSpans()
		for j := 0; j < sss.Len(); j++ {
			ss := sss.At(j)
			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				if err := ctx.Err(); err != nil {
					return err
				}
				span := spans.At(k)
				tCtx := ottlspan.NewTransformContext(rs, ss, span)
				if hasConds {
					match, evalErr := condSeq.Eval(ctx, tCtx)
					if evalErr != nil {
						tCtx.Close()
						switch errorMode {
						case ErrorModeIgnore:
							logger.Warn("OTTL condition error", zap.Error(evalErr))
							continue
						case ErrorModeSilent:
							continue
						default:
							return fmt.Errorf("evaluate condition on span %d/%d/%d: %w", i, j, k, evalErr)
						}
					}
					if !match {
						tCtx.Close()
						continue
					}
				}
				execErr := stmtSeq.Execute(ctx, tCtx)
				tCtx.Close()
				if execErr != nil {
					switch errorMode {
					case ErrorModeIgnore:
						logger.Warn("OTTL execution error", zap.Error(execErr))
					case ErrorModeSilent:
					default:
						return fmt.Errorf("execute on span %d/%d/%d: %w", i, j, k, execErr)
					}
				}
			}
		}
	}
	return nil
}

func executeTraceScopeGroup(
	ctx context.Context,
	traces ptrace.Traces,
	group ContextStatements,
	settings component.TelemetrySettings,
	mode collectorottl.ErrorMode,
	errorMode ErrorMode,
) error {
	parser, err := ottlscope.NewParser(
		parserFunctions[*ottlscope.TransformContext](),
		settings,
	)
	if err != nil {
		return fmt.Errorf("create OTTL scope parser: %w", err)
	}

	parsedStmts, err := parser.ParseStatements(group.Statements)
	if err != nil {
		return fmt.Errorf("parse OTTL statements: %w", err)
	}

	stmtSeq := ottlscope.NewStatementSequence(parsedStmts, settings, ottlscope.WithStatementSequenceErrorMode(mode))

	var condSeq collectorottl.ConditionSequence[*ottlscope.TransformContext]
	hasConds := len(group.Conditions) > 0
	if hasConds {
		parsedConds, err := parser.ParseConditions(group.Conditions)
		if err != nil {
			return fmt.Errorf("parse OTTL conditions: %w", err)
		}
		condSeq = newTraceScopeConditionSequence(parsedConds, settings, mode)
	}

	logger := zap.NewExample()
	rss := traces.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		sss := rs.ScopeSpans()
		for j := 0; j < sss.Len(); j++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			ss := sss.At(j)
			if ss.Spans().Len() == 0 {
				continue
			}
			tCtx := ottlscope.NewTransformContext(ss.Scope(), rs.Resource(), ss, rs)
			if hasConds {
				match, evalErr := condSeq.Eval(ctx, tCtx)
				if evalErr != nil {
					tCtx.Close()
					switch errorMode {
					case ErrorModeIgnore:
						logger.Warn("OTTL condition error", zap.Error(evalErr))
						continue
					case ErrorModeSilent:
						continue
					default:
						return fmt.Errorf("evaluate condition on scope %d/%d: %w", i, j, evalErr)
					}
				}
				if !match {
					tCtx.Close()
					continue
				}
			}
			execErr := stmtSeq.Execute(ctx, tCtx)
			tCtx.Close()
			if execErr != nil {
				switch errorMode {
				case ErrorModeIgnore:
					logger.Warn("OTTL execution error", zap.Error(execErr))
				case ErrorModeSilent:
				default:
					return fmt.Errorf("execute on scope %d/%d: %w", i, j, execErr)
				}
			}
		}
	}
	return nil
}

func executeTraceResourceGroup(
	ctx context.Context,
	traces ptrace.Traces,
	group ContextStatements,
	settings component.TelemetrySettings,
	mode collectorottl.ErrorMode,
	errorMode ErrorMode,
) error {
	parser, err := ottlresource.NewParser(
		parserFunctions[*ottlresource.TransformContext](),
		settings,
	)
	if err != nil {
		return fmt.Errorf("create OTTL resource parser: %w", err)
	}

	parsedStmts, err := parser.ParseStatements(group.Statements)
	if err != nil {
		return fmt.Errorf("parse OTTL statements: %w", err)
	}

	stmtSeq := ottlresource.NewStatementSequence(parsedStmts, settings, ottlresource.WithStatementSequenceErrorMode(mode))

	var condSeq collectorottl.ConditionSequence[*ottlresource.TransformContext]
	hasConds := len(group.Conditions) > 0
	if hasConds {
		parsedConds, err := parser.ParseConditions(group.Conditions)
		if err != nil {
			return fmt.Errorf("parse OTTL conditions: %w", err)
		}
		condSeq = newTraceResourceConditionSequence(parsedConds, settings, mode)
	}

	logger := zap.NewExample()
	rss := traces.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		rs := rss.At(i)
		if countSpansInResourceSpan(rs) == 0 {
			continue
		}
		tCtx := ottlresource.NewTransformContext(rs.Resource(), rs)
		if hasConds {
			match, evalErr := condSeq.Eval(ctx, tCtx)
			if evalErr != nil {
				tCtx.Close()
				switch errorMode {
				case ErrorModeIgnore:
					logger.Warn("OTTL condition error", zap.Error(evalErr))
					continue
				case ErrorModeSilent:
					continue
				default:
					return fmt.Errorf("evaluate condition on resource %d: %w", i, evalErr)
				}
			}
			if !match {
				tCtx.Close()
				continue
			}
		}
		execErr := stmtSeq.Execute(ctx, tCtx)
		tCtx.Close()
		if execErr != nil {
			switch errorMode {
			case ErrorModeIgnore:
				logger.Warn("OTTL execution error", zap.Error(execErr))
			case ErrorModeSilent:
			default:
				return fmt.Errorf("execute on resource %d: %w", i, execErr)
			}
		}
	}
	return nil
}

func countSpansInResourceSpan(rs ptrace.ResourceSpans) int {
	n := 0
	sss := rs.ScopeSpans()
	for j := 0; j < sss.Len(); j++ {
		n += sss.At(j).Spans().Len()
	}
	return n
}

func newTraceSpanConditionSequence(
	parsed []*collectorottl.Condition[*ottlspan.TransformContext],
	settings component.TelemetrySettings,
	mode collectorottl.ErrorMode,
) collectorottl.ConditionSequence[*ottlspan.TransformContext] {
	return collectorottl.NewConditionSequence(parsed, settings,
		collectorottl.WithConditionSequenceErrorMode[*ottlspan.TransformContext](mode),
		collectorottl.WithLogicOperation[*ottlspan.TransformContext](collectorottl.Or),
	)
}

func newTraceScopeConditionSequence(
	parsed []*collectorottl.Condition[*ottlscope.TransformContext],
	settings component.TelemetrySettings,
	mode collectorottl.ErrorMode,
) collectorottl.ConditionSequence[*ottlscope.TransformContext] {
	return collectorottl.NewConditionSequence(parsed, settings,
		collectorottl.WithConditionSequenceErrorMode[*ottlscope.TransformContext](mode),
		collectorottl.WithLogicOperation[*ottlscope.TransformContext](collectorottl.Or),
	)
}

func newTraceResourceConditionSequence(
	parsed []*collectorottl.Condition[*ottlresource.TransformContext],
	settings component.TelemetrySettings,
	mode collectorottl.ErrorMode,
) collectorottl.ConditionSequence[*ottlresource.TransformContext] {
	return collectorottl.NewConditionSequence(parsed, settings,
		collectorottl.WithConditionSequenceErrorMode[*ottlresource.TransformContext](mode),
		collectorottl.WithLogicOperation[*ottlresource.TransformContext](collectorottl.Or),
	)
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
