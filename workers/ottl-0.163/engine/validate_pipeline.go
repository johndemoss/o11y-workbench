package engine

import (
	"context"
	"fmt"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlmetric"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlresource"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlscope"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"
)

// ValidatePipelineOTTL walks transform/filter processors and validates their
// OTTL statements/conditions against this collector train.
func ValidatePipelineOTTL(ctx context.Context, pipeline Pipeline) ValidateResult {
	if err := ctx.Err(); err != nil {
		return ValidateResult{Valid: false, Errors: []string{err.Error()}}
	}
	if len(pipeline.Processors) == 0 {
		return ValidateResult{Valid: true, Parsed: 0}
	}

	var allErrors []string
	parsed := 0

	for name, cfg := range pipeline.Processors {
		if len(cfg) == 0 {
			continue
		}
		cfgYAML, err := mapToYAML(cfg)
		if err != nil {
			allErrors = append(allErrors, fmt.Sprintf("processor %s: %v", name, err))
			continue
		}

		stageType := detectStageType(map[string]interface{}(cfg))
		if _, ok := cfg["statements"]; ok {
			stageType = StageTypeTransform
		}
		switch stageType {
		case StageTypeTransform:
			meta, err := ParseTransformConfigMeta(cfgYAML)
			if err != nil {
				allErrors = append(allErrors, fmt.Sprintf("processor %s: %v", name, err))
				continue
			}
			for _, section := range []struct {
				groups  []ContextStatements
				signals Signals
			}{
				{meta.TraceGroups, Signals{Traces: true}},
				{meta.LogGroups, Signals{Logs: true}},
				{meta.MetricGroups, Signals{Metrics: true}},
			} {
				for _, group := range section.groups {
					if len(group.Conditions) == 0 {
						continue
					}
					res := validateGroupConditions(group, section.signals)
					for _, e := range res.Errors {
						allErrors = append(allErrors, fmt.Sprintf("processor %s: %s", name, e))
					}
				}
				stmts := ExtractStatements(section.groups)
				if len(stmts) == 0 {
					continue
				}
				res := ValidateStatements(stmts)
				parsed += res.Parsed
				for _, e := range res.Errors {
					allErrors = append(allErrors, fmt.Sprintf("processor %s: %s", name, e))
				}
			}
		case StageTypeFilter:
			conds, _, err := ParseFilterConditionsBySignal(cfgYAML)
			if err != nil {
				allErrors = append(allErrors, fmt.Sprintf("processor %s: %v", name, err))
				continue
			}
			for _, section := range []struct {
				conditions []string
				signals    Signals
			}{
				{conds.Traces, Signals{Traces: true}},
				{conds.Logs, Signals{Logs: true}},
				{conds.Metrics, Signals{Metrics: true}},
			} {
				if len(section.conditions) == 0 {
					continue
				}
				res := ValidateFilterConditionsParse(section.conditions, section.signals)
				parsed += res.Parsed
				for _, e := range res.Errors {
					allErrors = append(allErrors, fmt.Sprintf("processor %s: %s", name, e))
				}
			}
		}
	}

	if len(allErrors) > 0 {
		return ValidateResult{Valid: false, Errors: allErrors, Parsed: parsed}
	}
	return ValidateResult{Valid: true, Parsed: parsed}
}

func validateGroupConditions(group ContextStatements, signals Signals) ValidateResult {
	kind := ""
	if signals.Traces {
		traceKind, err := parseTraceGroupContext(group.Context)
		if err != nil {
			return ValidateResult{Errors: []string{err.Error()}}
		}
		switch traceKind {
		case traceGroupScope:
			kind = "scope"
		case traceGroupResource:
			kind = "resource"
		}
	} else if signals.Logs {
		switch logGroupExecutionKind(group.Context) {
		case logGroupScope:
			kind = "scope"
		case logGroupResource:
			kind = "resource"
		}
	} else if signals.Metrics {
		switch metricGroupExecutionKind(group.Context) {
		case metricGroupScope:
			kind = "scope"
		case metricGroupResource:
			kind = "resource"
		case metricGroupMetric:
			kind = "metric"
		}
	}
	settings := component.TelemetrySettings{Logger: zap.NewNop()}
	var err error
	switch kind {
	case "scope":
		parser, parseErr := ottlscope.NewParser(ParserFunctions[*ottlscope.TransformContext](), settings)
		err = parseErr
		if err == nil {
			_, err = parser.ParseConditions(group.Conditions)
		}
	case "resource":
		parser, parseErr := ottlresource.NewParser(ParserFunctions[*ottlresource.TransformContext](), settings)
		err = parseErr
		if err == nil {
			_, err = parser.ParseConditions(group.Conditions)
		}
	case "metric":
		parser, parseErr := ottlmetric.NewParser(ParserFunctions[*ottlmetric.TransformContext](), settings)
		err = parseErr
		if err == nil {
			_, err = parser.ParseConditions(group.Conditions)
		}
	default:
		return ValidateTransformConditionsParse(group.Conditions, signals)
	}
	if err != nil {
		return ValidateResult{Errors: []string{fmt.Sprintf("%s context: %v", kind, err)}}
	}
	return ValidateResult{Valid: true}
}
