package ottl

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ImportCollectorConfig converts an OTel Collector YAML config into PipelinesBySignal.
// It iterates all pipelines under service.pipelines.*, determines the base signal
// type (traces/logs/metrics) from each pipeline key, and builds a Pipeline per signal.
func ImportCollectorConfig(configYAML string) (PipelinesBySignal, error) {
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

	topProcessorsMap := map[string]interface{}{}
	if tp, ok := raw["processors"]; ok {
		topProcessorsMap, ok = tp.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("'processors' section is not a mapping")
		}
	}

	result := make(PipelinesBySignal)

	for pipelineKey := range pipelinesMap {
		signalName := signalNameFromPipelineKey(pipelineKey)

		pipeline, err := buildPipelineFromConfig(pipelinesMap, topProcessorsMap, pipelineKey)
		if err != nil {
			return nil, err
		}

		// Merge into existing signal if already present (e.g. traces + traces/foo)
		if existing, ok := result[signalName]; ok {
			existing.Stages = append(existing.Stages, pipeline.Stages...)
			for k, v := range pipeline.Processors {
				existing.Processors[k] = v
			}
		} else {
			result[signalName] = pipeline
		}
	}

	return result, nil
}

// signalNameFromPipelineKey extracts the base signal type from a pipeline key.
// "traces" -> "traces", "traces/foo" -> "traces", "logs" -> "logs", etc.
func signalNameFromPipelineKey(key string) string {
	if idx := strings.Index(key, "/"); idx >= 0 {
		return key[:idx]
	}
	return key
}

// buildPipelineFromConfig builds a Pipeline for a single pipeline key from the collector config.
func buildPipelineFromConfig(pipelinesMap, topProcessorsMap map[string]interface{}, pipelineKey string) (*Pipeline, error) {
	pipelineRaw, ok := pipelinesMap[pipelineKey]
	if !ok {
		return nil, fmt.Errorf("pipeline %q not found", pipelineKey)
	}
	pipelineMap, ok := pipelineRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("pipeline %q is not a mapping", pipelineKey)
	}

	procList := []interface{}{}
	if pr, ok := pipelineMap["processors"]; ok {
		procList, ok = pr.([]interface{})
		if !ok {
			return nil, fmt.Errorf("service.pipelines.%s.processors is not a list", pipelineKey)
		}
	}

	p := &Pipeline{
		Stages:     make([]Stage, 0, len(procList)),
		Processors: make(map[string]ProcessorConfig, len(procList)),
	}

	for i, procRef := range procList {
		ref, ok := procRef.(string)
		if !ok {
			return nil, fmt.Errorf("processor reference at index %d in pipeline %q is not a string", i, pipelineKey)
		}

		procConfigRaw, ok := topProcessorsMap[ref]
		if !ok {
			return nil, fmt.Errorf("processor %q referenced in pipeline %q but not defined in 'processors' section", ref, pipelineKey)
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
