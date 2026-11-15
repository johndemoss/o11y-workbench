package engine

import (
	"context"
	"fmt"
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
			for _, groups := range [][]ContextStatements{meta.TraceGroups, meta.LogGroups, meta.MetricGroups} {
				stmts := ExtractStatements(groups)
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
