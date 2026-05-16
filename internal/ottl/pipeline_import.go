package ottl

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ImportCollectorConfig converts an OTel Collector YAML config into a Pipeline.
// It extracts the traces pipeline processor chain from the config's
// service.pipelines.traces section and maps each processor reference to a
// pipeline stage with its resolved configuration.
func ImportCollectorConfig(configYAML string) (*Pipeline, error) {
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(configYAML), &raw); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	serviceRaw, ok := raw["service"]
	if !ok {
		return nil, fmt.Errorf("config missing 'service' section")
	}
	serviceMap, ok := serviceRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("'service' section is not a mapping")
	}
	pipelinesRaw, ok := serviceMap["pipelines"]
	if !ok {
		return nil, fmt.Errorf("config missing 'service.pipelines' section")
	}
	pipelinesMap, ok := pipelinesRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("'service.pipelines' is not a mapping")
	}
	tracesRaw, ok := pipelinesMap["traces"]
	if !ok {
		return nil, fmt.Errorf("config missing 'service.pipelines.traces' section")
	}
	tracesMap, ok := tracesRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("'service.pipelines.traces' is not a mapping")
	}

	procList := []interface{}{}
	if pr, ok := tracesMap["processors"]; ok {
		procList, ok = pr.([]interface{})
		if !ok {
			return nil, fmt.Errorf("'service.pipelines.traces.processors' is not a list")
		}
	}

	topProcessorsMap := map[string]interface{}{}
	if tp, ok := raw["processors"]; ok {
		topProcessorsMap, ok = tp.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("'processors' section is not a mapping")
		}
	}

	p := &Pipeline{
		Stages:     make([]Stage, 0, len(procList)),
		Processors: make(map[string]ProcessorConfig, len(procList)),
	}

	for i, procRef := range procList {
		ref, ok := procRef.(string)
		if !ok {
			return nil, fmt.Errorf("processor reference at index %d is not a string", i)
		}

		procConfigRaw, ok := topProcessorsMap[ref]
		if !ok {
			return nil, fmt.Errorf("processor %q referenced in pipeline but not defined in 'processors' section", ref)
		}
		procConfigMap, ok := procConfigRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("processor %q config is not a mapping", ref)
		}

		procConfig := ProcessorConfig(procConfigMap)
		p.Processors[ref] = procConfig

		stageType := detectStageType(procConfigMap)
		if stageType == "" {
			continue
		}

		errorMode := extractErrorMode(procConfigMap)

		stage := Stage{
			ID:           stageIDFromRef(ref),
			Type:         stageType,
			ErrorMode:    errorMode,
			ProcessorRef: ref,
		}
		p.Stages = append(p.Stages, stage)
	}

	return p, nil
}

func detectStageType(config map[string]interface{}) StageType {
	hasTransform := false
	hasFilter := false
	for key := range config {
		switch key {
		case "trace_statements", "log_statements", "metric_statements":
			hasTransform = true
		case "traces", "logs", "metrics":
			hasFilter = true
		}
	}
	if hasTransform {
		return StageTypeTransform
	}
	if hasFilter {
		return StageTypeFilter
	}
	return ""
}

func extractErrorMode(config map[string]interface{}) ErrorMode {
	if emRaw, ok := config["error_mode"]; ok {
		if emStr, ok := emRaw.(string); ok {
			switch ErrorMode(emStr) {
			case ErrorModeIgnore:
				return ErrorModeIgnore
			case ErrorModeSilent:
				return ErrorModeSilent
			case ErrorModePropagate:
				return ErrorModePropagate
			}
		}
	}
	return ErrorModePropagate
}

func stageIDFromRef(ref string) string {
	if idx := strings.Index(ref, "/"); idx >= 0 && idx+1 < len(ref) {
		return ref[idx+1:]
	}
	return ref
}
