// Package ottl provides OTTL (OpenTelemetry Transformation Language) statement
// validation, execution, and filtering for the o11y-workbench web dashboard.
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

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottldatapoint"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottllog"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlspan"
)

// ValidateFilterConditionsParse parses and validates OTTL filter conditions
// without executing them. Returns which conditions are syntactically valid.
// The signalType parameter selects the appropriate OTTL context(s) for validation:
// "traces" → span, "logs" → log, "metrics" → datapoint.
// Combined types like "both", "all", "traces+metrics", "logs+metrics" are also
// supported — a condition is valid if it parses correctly in ANY selected context.
func ValidateFilterConditionsParse(conditions []string, signals Signals) ValidateResult {
	if len(conditions) == 0 {
		return ValidateResult{
			Valid:  false,
			Errors: []string{"no conditions provided"},
		}
	}

	settings := component.TelemetrySettings{
		Logger: zap.NewNop(),
	}

	type namedParser struct {
		name  string
		parse func(string) error
	}

	// collectParsers creates parsers for the given signal type names.
	collectParsers := func(sigs []string) ([]namedParser, error) {
		var parsers []namedParser
		for _, sig := range sigs {
			switch sig {
			case "traces":
				p, err := ottlspan.NewParser(
					ParserFunctions[*ottlspan.TransformContext](),
					settings,
				)
				if err != nil {
					return nil, fmt.Errorf("create span parser: %w", err)
				}
				parsers = append(parsers, namedParser{
					name: "span context",
					parse: func(cond string) error {
						_, err := p.ParseCondition(cond)
						return err
					},
				})
			case "logs":
				p, err := ottllog.NewParser(
					ParserFunctions[*ottllog.TransformContext](),
					settings,
				)
				if err != nil {
					return nil, fmt.Errorf("create log parser: %w", err)
				}
				parsers = append(parsers, namedParser{
					name: "log context",
					parse: func(cond string) error {
						_, err := p.ParseCondition(cond)
						return err
					},
				})
			case "metrics":
				p, err := ottldatapoint.NewParser(
					ParserFunctions[*ottldatapoint.TransformContext](),
					settings,
				)
				if err != nil {
					return nil, fmt.Errorf("create metric parser: %w", err)
				}
				parsers = append(parsers, namedParser{
					name: "metric context",
					parse: func(cond string) error {
						_, err := p.ParseCondition(cond)
						return err
					},
				})
			}
		}
		return parsers, nil
	}

	// Collect signal contexts to check based on enabled flags.
	var sigs []string
	if signals.Traces {
		sigs = append(sigs, "traces")
	}
	if signals.Logs {
		sigs = append(sigs, "logs")
	}
	if signals.Metrics {
		sigs = append(sigs, "metrics")
	}
	if len(sigs) == 0 {
		// Default to traces if nothing selected
		sigs = []string{"traces"}
	}

	parsers, err := collectParsers(sigs)
	if err != nil {
		return ValidateResult{
			Valid:  false,
			Errors: []string{fmt.Sprintf("internal error creating parser: %v", err)},
		}
	}

	if len(parsers) == 0 {
		return ValidateResult{
			Valid:  false,
			Errors: []string{fmt.Sprintf("no parsers available for signals: %v", sigs)},
		}
	}

	// Validate each condition against all selected contexts.
	// A condition is valid if it parses in ANY context.
	var allErrors []string
	parsedCount := 0
	for _, cond := range conditions {
		var errs []string
		for _, p := range parsers {
			if err := p.parse(cond); err == nil {
				errs = nil
				break
			} else {
				errs = append(errs, fmt.Sprintf("%s: %v", p.name, err))
			}
		}
		if len(errs) > 0 {
			if len(parsers) == 1 {
				allErrors = append(allErrors, fmt.Sprintf("condition %q: %v", cond, errs[0]))
			} else {
				allErrors = append(allErrors, fmt.Sprintf("condition %q: not valid for any selected context — %s", cond, strings.Join(errs, "; ")))
			}
		} else {
			parsedCount++
		}
	}

	if len(allErrors) > 0 {
		return ValidateResult{
			Valid:  false,
			Errors: allErrors,
		}
	}

	return ValidateResult{
		Valid:  true,
		Parsed: parsedCount,
	}
}

// FilterMode controls whether conditions include or exclude matching spans.
type FilterMode string

const (
	// FilterModeInclude keeps only spans that match the conditions.
	FilterModeInclude FilterMode = "include"
	// FilterModeExclude removes spans that match the conditions.
	FilterModeExclude FilterMode = "exclude"
)

// FilterRunData evaluates OTTL conditions against each span in the trace data
// and returns a filtered trace containing only the spans that match (include mode)
// or only the spans that do NOT match (exclude mode).
//
// Conditions are ORed together — if ANY condition evaluates to true for a span,
// the span is considered a match.
func FilterRunData(traceData json.RawMessage, conditions []string, mode FilterMode) (json.RawMessage, int, int, error) {
	if len(conditions) == 0 {
		return nil, 0, 0, fmt.Errorf("no conditions provided")
	}

	// Step 1: convert hex trace IDs to base64, then unmarshal JSON trace data into protobuf types.
	fixedData, err := FixTraceSpanIDsForUnmarshal(traceData)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("fix trace IDs: %w", err)
	}
	var srcReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(fixedData, &srcReq); err != nil {
		return nil, 0, 0, fmt.Errorf("unmarshal trace data: %w", err)
	}

	protoBytes, err := proto.Marshal(&srcReq)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("marshal protobuf: %w", err)
	}

	var unmarshaler ptrace.ProtoUnmarshaler
	traces, err := unmarshaler.UnmarshalTraces(protoBytes)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("unmarshal to pdata: %w", err)
	}

	filtered, total, kept, err := FilterSpans(traces, conditions, mode, ErrorModePropagate)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("filter spans: %w", err)
	}

	var marshaler ptrace.ProtoMarshaler
	outProto, err := marshaler.MarshalTraces(filtered)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("marshal filtered traces: %w", err)
	}

	var outReq collectortracev1.ExportTraceServiceRequest
	if err := proto.Unmarshal(outProto, &outReq); err != nil {
		return nil, 0, 0, fmt.Errorf("unmarshal result proto: %w", err)
	}

	outJSON, err := protojson.MarshalOptions{
		UseProtoNames: false,
		Indent:        "",
	}.Marshal(&outReq)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("marshal result JSON: %w", err)
	}

	return outJSON, total, kept, nil
}

// FilterSpans evaluates OTTL conditions against all spans in ptrace.Traces and
// returns a filtered ptrace.Traces containing only the spans that match (include
// mode) or only the spans that do NOT match (exclude mode). error_mode controls
// how evaluation errors are handled:
//   - ErrorModePropagate: return error immediately (default)
//   - ErrorModeIgnore: log the error and skip the span
//   - ErrorModeSilent: skip the span without logging
//
// Conditions are ORed together — if ANY condition evaluates to true for a span,
// the span is considered a match.
func FilterSpans(traces ptrace.Traces, conditions []string, mode FilterMode, errorMode ErrorMode) (ptrace.Traces, int, int, error) {
	if len(conditions) == 0 {
		return traces, 0, 0, fmt.Errorf("no conditions provided")
	}

	settings := component.TelemetrySettings{
		Logger:         zap.NewNop(),
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  noop.NewMeterProvider(),
		Resource:       pcommon.NewResource(),
	}

	parser, err := ottlspan.NewParser(
		ParserFunctions[*ottlspan.TransformContext](),
		settings,
	)
	if err != nil {
		return traces, 0, 0, fmt.Errorf("create OTTL parser: %w", err)
	}

	parsedConditions, err := parser.ParseConditions(conditions)
	if err != nil {
		return traces, 0, 0, fmt.Errorf("parse OTTL conditions: %w", err)
	}

	condSeq := ottlspan.NewConditionSequence(parsedConditions, settings)

	ctx := context.Background()
	rss := traces.ResourceSpans()

	type spanKey struct {
		ri, si, pi int
	}
	matched := make(map[spanKey]bool)

	totalSpans := 0

	var ignoreLogger *zap.Logger
	if errorMode == ErrorModeIgnore {
		ignoreLogger = zap.NewExample()
		defer ignoreLogger.Sync()
	}

	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		sss := rs.ScopeSpans()
		for j := 0; j < sss.Len(); j++ {
			ss := sss.At(j)
			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				tCtx := ottlspan.NewTransformContext(rs, ss, span)

				isMatch, evalErr := condSeq.Eval(ctx, tCtx)
				tCtx.Close()
				if evalErr != nil {
					switch errorMode {
					case ErrorModePropagate:
						return traces, 0, 0, fmt.Errorf("evaluate condition on span %d/%d/%d: %w", i, j, k, evalErr)
					case ErrorModeIgnore:
						ignoreLogger.Warn("failed to evaluate condition, skipping span",
							zap.Error(evalErr),
							zap.Int("resource", i),
							zap.Int("scope", j),
							zap.Int("span", k),
						)
						isMatch = false
					case ErrorModeSilent:
						isMatch = false
					default:
						return traces, 0, 0, fmt.Errorf("unknown error mode: %s", errorMode)
					}
				}

				var keep bool
				switch mode {
				case FilterModeInclude:
					keep = isMatch
				case FilterModeExclude:
					keep = !isMatch
				default:
					return traces, 0, 0, fmt.Errorf("unknown filter mode: %s", mode)
				}

				if keep {
					matched[spanKey{i, j, k}] = true
				}
				totalSpans++
			}
		}
	}

	filtered := ptrace.NewTraces()
	outRss := filtered.ResourceSpans()

	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		sss := rs.ScopeSpans()

		hasRsMatch := false
		for j := 0; j < sss.Len() && !hasRsMatch; j++ {
			spans := sss.At(j).Spans()
			for k := 0; k < spans.Len() && !hasRsMatch; k++ {
				if matched[spanKey{i, j, k}] {
					hasRsMatch = true
				}
			}
		}
		if !hasRsMatch {
			continue
		}

		outRs := outRss.AppendEmpty()
		rs.Resource().CopyTo(outRs.Resource())
		outRs.SetSchemaUrl(rs.SchemaUrl())
		outSss := outRs.ScopeSpans()

		for j := 0; j < sss.Len(); j++ {
			ss := sss.At(j)
			spans := ss.Spans()

			hasSsMatch := false
			for k := 0; k < spans.Len() && !hasSsMatch; k++ {
				if matched[spanKey{i, j, k}] {
					hasSsMatch = true
				}
			}
			if !hasSsMatch {
				continue
			}

			outSs := outSss.AppendEmpty()
			ss.Scope().CopyTo(outSs.Scope())
			outSs.SetSchemaUrl(ss.SchemaUrl())
			outSpans := outSs.Spans()

			for k := 0; k < spans.Len(); k++ {
				if matched[spanKey{i, j, k}] {
					span := spans.At(k)
					outSpan := outSpans.AppendEmpty()
					span.CopyTo(outSpan)
				}
			}
		}
	}

	filteredCount := len(matched)
	return filtered, totalSpans, filteredCount, nil
}

// FilterResult holds the outcome of a filter request.
type FilterResult struct {
	Valid                  bool             `json:"valid"`
	Errors                 []string         `json:"errors,omitempty"`
	OriginalData           *json.RawMessage `json:"original_data,omitempty"`
	FilteredData           *json.RawMessage `json:"filtered_data,omitempty"`
	ConditionsParsed       int              `json:"conditions_parsed,omitempty"`
	TotalSpanCount         int              `json:"total_span_count,omitempty"`
	FilteredSpanCount      int              `json:"filtered_span_count,omitempty"`
	LogOriginalData        *json.RawMessage `json:"log_original_data,omitempty"`
	LogFilteredData        *json.RawMessage `json:"log_filtered_data,omitempty"`
	LogConditionsParsed    int              `json:"log_conditions_parsed,omitempty"`
	TotalLogCount          int              `json:"total_log_count,omitempty"`
	FilteredLogCount       int              `json:"filtered_log_count,omitempty"`
	MetricOriginalData     *json.RawMessage `json:"metric_original_data,omitempty"`
	MetricFilteredData     *json.RawMessage `json:"metric_filtered_data,omitempty"`
	MetricConditionsParsed int              `json:"metric_conditions_parsed,omitempty"`
	TotalMetricCount       int              `json:"total_metric_count,omitempty"`
	FilteredMetricCount    int              `json:"filtered_metric_count,omitempty"`
}
