package engine

import "fmt"

// ValidatePipelineOTTL walks transform/filter processors and validates their
// OTTL statements/conditions against this collector train.
func ValidatePipelineOTTL(pipeline Pipeline) ValidateResult {
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
		switch stageType {
		case StageTypeTransform:
			stmts, err := ParseTransformStatements(cfgYAML)
			if err != nil {
				allErrors = append(allErrors, fmt.Sprintf("processor %s: %v", name, err))
				continue
			}
			if len(stmts) == 0 {
				continue
			}
			res := ValidateStatements(stmts)
			parsed += res.Parsed
			for _, e := range res.Errors {
				allErrors = append(allErrors, fmt.Sprintf("processor %s: %s", name, e))
			}
		case StageTypeFilter:
			conds, err := ParseFilterConditions(cfgYAML)
			if err != nil {
				allErrors = append(allErrors, fmt.Sprintf("processor %s: %v", name, err))
				continue
			}
			if len(conds) == 0 {
				continue
			}
			sigs := Signals{Traces: true, Logs: true, Metrics: true}
			res := ValidateFilterConditionsParse(conds, sigs)
			parsed += res.Parsed
			for _, e := range res.Errors {
				allErrors = append(allErrors, fmt.Sprintf("processor %s: %s", name, e))
			}
		}
	}

	if len(allErrors) > 0 {
		return ValidateResult{Valid: false, Errors: allErrors, Parsed: parsed}
	}
	return ValidateResult{Valid: true, Parsed: parsed}
}
