// Package ottl provides OTTL (OpenTelemetry Transformation Language) statement
// validation, execution, and filtering for the otel-workbench web dashboard.
package ottl

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
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottldatapoint"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottllog"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlspan"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"
)

// ValidateFilterConditionsParse parses and validates OTTL filter conditions
// without executing them. Returns which conditions are syntactically valid.
// The signalType parameter selects the appropriate OTTL context(s) for validation:
// "traces" → span, "logs" → log, "metrics" → datapoint.
// Combined types like "both", "all", "traces+metrics", "logs+metrics" are also
// supported — a condition is valid if it parses correctly in ANY selected context.
func ValidateFilterConditionsParse(conditions []string, signalType string) ValidateResult {
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
					ottlfuncs.StandardFuncs[*ottlspan.TransformContext](),
					settings,
				)
				if err != nil {
					return nil, err
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
					ottlfuncs.StandardFuncs[*ottllog.TransformContext](),
					settings,
				)
				if err != nil {
					return nil, err
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
					ottlfuncs.StandardFuncs[*ottldatapoint.TransformContext](),
					settings,
				)
				if err != nil {
					return nil, err
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

	// Map combined signal type to the set of parser contexts to check.
	var sigs []string
	switch signalType {
	case "logs":
		sigs = []string{"logs"}
	case "metrics":
		sigs = []string{"metrics"}
	case "both":
		sigs = []string{"traces", "logs"}
	case "all":
		sigs = []string{"traces", "logs", "metrics"}
	case "traces+metrics":
		sigs = []string{"traces", "metrics"}
	case "logs+metrics":
		sigs = []string{"logs", "metrics"}
	default:
		// "traces" (or unknown) — use span context
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
			Errors: []string{fmt.Sprintf("no parsers available for signal type: %s", signalType)},
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

	// Step 1: unmarshal JSON trace data into protobuf types.
	var srcReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(traceData, &srcReq); err != nil {
		return nil, 0, 0, fmt.Errorf("unmarshal trace data: %w", err)
	}

	// Step 2: marshal protobuf to binary wire format, then to pdata for OTTL.
	protoBytes, err := proto.Marshal(&srcReq)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("marshal protobuf: %w", err)
	}

	var unmarshaler ptrace.ProtoUnmarshaler
	traces, err := unmarshaler.UnmarshalTraces(protoBytes)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("unmarshal to pdata: %w", err)
	}

	// Step 3: set up OTTL span-context parser and condition sequence.
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
		return nil, 0, 0, fmt.Errorf("create OTTL parser: %w", err)
	}

	parsedConditions, err := parser.ParseConditions(conditions)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("parse OTTL conditions: %w", err)
	}

	condSeq := ottlspan.NewConditionSequence(parsedConditions, settings)

	// Step 4: evaluate conditions against every span, recording matches.
	ctx := context.Background()
	rss := traces.ResourceSpans()

	// We'll build a set of (resourceIdx, scopeIdx, spanIdx) tuples for matched spans.
	type spanKey struct {
		ri, si, pi int
	}
	matched := make(map[spanKey]bool)

	totalSpans := 0
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		sss := rs.ScopeSpans()
		for j := 0; j < sss.Len(); j++ {
			ss := sss.At(j)
			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				tCtx := ottlspan.NewTransformContextPtr(rs, ss, span)

				isMatch, evalErr := condSeq.Eval(ctx, tCtx)
				tCtx.Close()
				if evalErr != nil {
					return nil, 0, 0, fmt.Errorf("evaluate condition on span %d/%d/%d: %w", i, j, k, evalErr)
				}

				// determine whether this span is kept
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
					matched[spanKey{i, j, k}] = true
				}
				totalSpans++
			}
		}
	}

	// Step 5: build a new ExportTraceServiceRequest containing only matched spans.
	var filteredReq collectortracev1.ExportTraceServiceRequest
	for i, rss := range srcReq.ResourceSpans {
		var outRss tracev1.ResourceSpans
		outRss.Resource = rss.Resource
		outRss.SchemaUrl = rss.SchemaUrl

		for j, sss := range rss.ScopeSpans {
			var outSss tracev1.ScopeSpans
			outSss.Scope = sss.Scope
			outSss.SchemaUrl = sss.SchemaUrl

			for k, span := range sss.Spans {
				if matched[spanKey{i, j, k}] {
					outSss.Spans = append(outSss.Spans, span)
				}
			}

			if len(outSss.Spans) > 0 {
				outRss.ScopeSpans = append(outRss.ScopeSpans, &outSss)
			}
		}

		if len(outRss.ScopeSpans) > 0 {
			filteredReq.ResourceSpans = append(filteredReq.ResourceSpans, &outRss)
		}
	}

	// Step 6: marshal filtered protobuf back to OTLP JSON.
	outJSON, err := protojson.MarshalOptions{
		UseProtoNames: true,
		Indent:        "  ",
	}.Marshal(&filteredReq)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("marshal result JSON: %w", err)
	}

	filteredCount := len(matched)
	return outJSON, totalSpans, filteredCount, nil
}

// FilterResult holds the outcome of a filter request.
type FilterResult struct {
	Valid              bool             `json:"valid"`
	Errors             []string         `json:"errors,omitempty"`
	OriginalData          *json.RawMessage `json:"original_data,omitempty"`
	FilteredData          *json.RawMessage `json:"filtered_data,omitempty"`
	ConditionsParsed      int              `json:"conditions_parsed,omitempty"`
	TotalSpanCount        int              `json:"total_span_count,omitempty"`
	FilteredSpanCount     int              `json:"filtered_span_count,omitempty"`
	LogOriginalData       *json.RawMessage `json:"log_original_data,omitempty"`
	LogFilteredData       *json.RawMessage `json:"log_filtered_data,omitempty"`
	LogConditionsParsed   int              `json:"log_conditions_parsed,omitempty"`
	TotalLogCount         int              `json:"total_log_count,omitempty"`
	FilteredLogCount      int              `json:"filtered_log_count,omitempty"`
	MetricOriginalData    *json.RawMessage `json:"metric_original_data,omitempty"`
	MetricFilteredData    *json.RawMessage `json:"metric_filtered_data,omitempty"`
	MetricConditionsParsed int             `json:"metric_conditions_parsed,omitempty"`
	TotalMetricCount      int              `json:"total_metric_count,omitempty"`
	FilteredMetricCount   int              `json:"filtered_metric_count,omitempty"`
}
