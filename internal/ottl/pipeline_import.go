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
			return nil, fmt.Errorf("build pipeline %q: %w", pipelineKey, err)
		}

		// Merge into existing signal if already present (e.g. traces + traces/foo)
		if existing, ok := result[signalName]; ok {
			existing.Stages = append(existing.Stages, pipeline.Stages...)
			for k, v := range pipeline.Processors {
				existing.Processors[k] = v
			}
			existing.Receivers = unionRefs(existing.Receivers, pipeline.Receivers)
			existing.Exporters = unionRefs(existing.Exporters, pipeline.Exporters)
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

// parseRefList converts a YAML sequence (e.g. a pipeline's receivers/exporters
// list) into a []string, preserving order and skipping non-string entries.
// It always returns a non-nil slice so JSON encodes it as [] rather than null.
func parseRefList(v interface{}) []string {
	out := []string{}
	list, ok := v.([]interface{})
	if !ok {
		return out
	}
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// unionRefs concatenates two reference lists, preserving order and dropping
// duplicates. Used when several pipeline keys merge into one signal.
func unionRefs(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := []string{}
	for _, group := range [][]string{a, b} {
		for _, s := range group {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
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
		Receivers:  parseRefList(pipelineMap["receivers"]),
		Exporters:  parseRefList(pipelineMap["exporters"]),
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
