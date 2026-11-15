package engine

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

	// Extract top-level receiver and exporter names for default fallbacks
	topReceivers := []string{}
	if recvRaw, ok := raw["receivers"].(map[string]interface{}); ok {
		for k := range recvRaw {
			topReceivers = append(topReceivers, k)
			break // use first receiver name as default
		}
	}
	topExporters := []string{}
	if expRaw, ok := raw["exporters"].(map[string]interface{}); ok {
		for k := range expRaw {
			topExporters = append(topExporters, k)
			break // use first exporter name as default
		}
	}

	result := make(PipelinesBySignal)

	for pipelineKey := range pipelinesMap {
		signalName := signalNameFromPipelineKey(pipelineKey)

		pipeline, err := buildPipelineFromConfig(pipelinesMap, topProcessorsMap, pipelineKey, topReceivers, topExporters)
		if err != nil {
			return nil, fmt.Errorf("build pipeline %q: %w", pipelineKey, err)
		}

		if _, ok := result[signalName]; ok {
			return nil, fmt.Errorf("parallel %s pipelines are not supported", signalName)
		}
		pipeline.ReceiverConfigs, _ = raw["receivers"].(map[string]interface{})
		pipeline.ExporterConfigs, _ = raw["exporters"].(map[string]interface{})
		result[signalName] = pipeline
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

func defaultReceivers(signal string) []string {
	switch signal {
	case "traces", "logs":
		return []string{"otlp"}
	default:
		return []string{}
	}
}

func defaultExporters(signal string) []string {
	switch signal {
	case "traces", "logs":
		return []string{"debug"}
	default:
		return []string{}
	}
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

// buildPipelineFromConfig builds a Pipeline for a single pipeline key from the collector config.
func buildPipelineFromConfig(pipelinesMap, topProcessorsMap map[string]interface{}, pipelineKey string, topReceivers, topExporters []string) (*Pipeline, error) {
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

	receivers := parseRefList(pipelineMap["receivers"])
	if len(receivers) == 0 {
		receivers = topReceivers
		if len(receivers) == 0 {
			receivers = defaultReceivers(signalNameFromPipelineKey(pipelineKey))
		}
	}
	exporters := parseRefList(pipelineMap["exporters"])
	if len(exporters) == 0 {
		exporters = topExporters
		if len(exporters) == 0 {
			exporters = defaultExporters(signalNameFromPipelineKey(pipelineKey))
		}
	}
	p := &Pipeline{
		Stages:     make([]Stage, 0, len(procList)),
		Processors: make(map[string]ProcessorConfig, len(procList)),
		Receivers:  receivers,
		Exporters:  exporters,
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
		if stageType == "" || strings.SplitN(ref, "/", 2)[0] != string(stageType) {
			return nil, fmt.Errorf("processor %q has unsupported semantics", ref)
		}
		for key, value := range procConfigMap {
			if key == "error_mode" {
				mode, ok := value.(string)
				if !ok || (mode != "ignore" && mode != "silent" && mode != "propagate") {
					return nil, fmt.Errorf("processor %q: invalid error_mode", ref)
				}
				continue
			}
			if stageType == StageTypeTransform {
				if key != "trace_statements" && key != "log_statements" && key != "metric_statements" {
					return nil, fmt.Errorf("processor %q: unsupported field %q", ref, key)
				}
				groups, ok := value.([]interface{})
				if !ok {
					return nil, fmt.Errorf("processor %q: %s must be a list", ref, key)
				}
				for _, group := range groups {
					m, ok := group.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("processor %q: unsupported statement group", ref)
					}
					for k := range m {
						if k != "context" && k != "conditions" && k != "statements" {
							return nil, fmt.Errorf("processor %q: unsupported statement field %q", ref, k)
						}
					}
				}
			} else {
				want := map[string]string{"traces": "span", "logs": "log_record", "metrics": "datapoint"}[key]
				m, ok := value.(map[string]interface{})
				if want == "" || !ok {
					return nil, fmt.Errorf("processor %q: unsupported filter field %q", ref, key)
				}
				for k := range m {
					if k != want {
						return nil, fmt.Errorf("processor %q: unsupported filter field %s.%s", ref, key, k)
					}
				}
			}
		}

		errorMode := extractErrorMode(procConfigMap)

		stage := Stage{
			ID:           fmt.Sprintf("%s#%d", ref, i),
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
