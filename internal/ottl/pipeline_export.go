package ottl

import (
	"fmt"
	"sort"
)

func ExportPipelinesToYaml(p PipelinesBySignal) (string, error) {
	var lines []string

	lines = append(lines, "receivers:")
	lines = append(lines, "  otlp:")
	lines = append(lines, "    protocols:")
	lines = append(lines, "      grpc:")
	lines = append(lines, "")

	lines = append(lines, "processors:")

	allProcessorKeys := collectProcessorKeys(p)
	if len(allProcessorKeys) == 0 {
		lines = append(lines, "  {}")
	} else {
		for _, key := range allProcessorKeys {
			for _, signal := range []string{"traces", "logs", "metrics"} {
				cfg, ok := p[signal]
				if !ok {
					continue
				}
				procCfg, exists := cfg.Processors[key]
				if !exists {
					continue
				}
				lines = append(lines, fmt.Sprintf("  %s:", key))
				yamlLines, err := dumpProcessorConfig(procCfg, 4)
				if err != nil {
					return "", fmt.Errorf("processor %s: %w", key, err)
				}
				lines = append(lines, yamlLines...)
				break
			}
		}
	}
	lines = append(lines, "")

	lines = append(lines, "exporters:")
	lines = append(lines, "  debug:")
	lines = append(lines, "")

	lines = append(lines, "service:")
	lines = append(lines, "  pipelines:")
	for _, signal := range []string{"traces", "logs", "metrics"} {
		cfg, ok := p[signal]
		if !ok {
			continue
		}
		if len(cfg.Stages) == 0 && len(cfg.Processors) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("    %s:", signal))
		lines = append(lines, "      receivers: [otlp]")
		if len(cfg.Stages) > 0 {
			procRefs := make([]string, len(cfg.Stages))
			for i, s := range cfg.Stages {
				procRefs[i] = s.ProcessorRef
			}
			lines = append(lines, fmt.Sprintf("      processors: [%s]", joinStrings(procRefs, ", ")))
		} else {
			lines = append(lines, "      processors: []")
		}
		lines = append(lines, "      exporters: [debug]")
	}

	return joinLines(lines), nil
}

func collectProcessorKeys(p PipelinesBySignal) []string {
	stageOrder := []string{}
	seen := make(map[string]bool)

	if traces, ok := p["traces"]; ok {
		for _, stage := range traces.Stages {
			if _, exists := traces.Processors[stage.ProcessorRef]; exists && !seen[stage.ProcessorRef] {
				stageOrder = append(stageOrder, stage.ProcessorRef)
				seen[stage.ProcessorRef] = true
			}
		}
	}

	for _, signal := range []string{"traces", "logs", "metrics"} {
		cfg, ok := p[signal]
		if !ok {
			continue
		}
		for k := range cfg.Processors {
			if !seen[k] {
				stageOrder = append(stageOrder, k)
				seen[k] = true
			}
		}
	}

	return stageOrder
}

func dumpProcessorConfig(cfg ProcessorConfig, depth int) ([]string, error) {
	var lines []string

	keys := sortedKeys(cfg)
	for _, k := range keys {
		v := cfg[k]
		if m, ok := v.(map[string]interface{}); ok {
			lines = append(lines, fmt.Sprintf("%s%s:", spaces(depth), k))
			nested, err := dumpProcessorConfig(m, depth+2)
			if err != nil {
				return nil, err
			}
			lines = append(lines, nested...)
		} else if ar, ok := v.([]interface{}); ok {
			lines = append(lines, fmt.Sprintf("%s%s:", spaces(depth), k))
			for _, item := range ar {
				obj, ok := item.(map[string]interface{})
				if !ok {
					lines = append(lines, fmt.Sprintf("%s- %v", spaces(depth+2), item))
					continue
				}
				keys := make([]string, 0, len(obj))
				for key := range obj {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				hasMultipleKeys := len(keys) > 1
				allSimple := true
				for _, key := range keys {
					if _, ok := obj[key].([]interface{}); ok {
						allSimple = false
						break
					}
					if _, ok := obj[key].(map[string]interface{}); ok {
						allSimple = false
						break
					}
				}
				if hasMultipleKeys || !allSimple {
					for j, key := range keys {
						val := obj[key]
						isFirst := j == 0
						if isFirst {
							if nestedArr, ok := val.([]interface{}); ok {
								lines = append(lines, fmt.Sprintf("%s- %s:", spaces(depth+2), key))
								for _, arrItem := range nestedArr {
									lines = append(lines, fmt.Sprintf("%s- %v", spaces(depth+4), arrItem))
								}
							} else if nestedMap, ok := val.(map[string]interface{}); ok {
								lines = append(lines, fmt.Sprintf("%s- %s:", spaces(depth+2), key))
								for nk, nv := range nestedMap {
									lines = append(lines, fmt.Sprintf("%s%s: %v", spaces(depth+4), nk, nv))
								}
							} else {
								lines = append(lines, fmt.Sprintf("%s- %s: %v", spaces(depth+2), key, val))
							}
						} else {
							if nestedArr, ok := val.([]interface{}); ok {
								lines = append(lines, fmt.Sprintf("%s%s:", spaces(depth+4), key))
								for _, arrItem := range nestedArr {
									lines = append(lines, fmt.Sprintf("%s- %v", spaces(depth+6), arrItem))
								}
							} else if nestedMap, ok := val.(map[string]interface{}); ok {
								lines = append(lines, fmt.Sprintf("%s%s:", spaces(depth+4), key))
								for nk, nv := range nestedMap {
									lines = append(lines, fmt.Sprintf("%s%s: %v", spaces(depth+6), nk, nv))
								}
							} else {
								lines = append(lines, fmt.Sprintf("%s%s: %v", spaces(depth+4), key, val))
							}
						}
					}
				} else {
					first := true
					for _, key := range keys {
						val := obj[key]
						if first {
							lines = append(lines, fmt.Sprintf("%s- %s: %v", spaces(depth+2), key, val))
							first = false
						} else {
							lines = append(lines, fmt.Sprintf("%s%s: %v", spaces(depth+2), key, val))
						}
					}
				}
			}
		} else {
			lines = append(lines, fmt.Sprintf("%s%s: %v", spaces(depth), k, v))
		}
	}
	return lines, nil
}

func sortedKeys(cfg ProcessorConfig) []string {
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	if n == 1 {
		return " "
	}
	return " " + spaces(n-1)
}

func joinStrings(items []string, sep string) string {
	result := ""
	for i, item := range items {
		if i > 0 {
			result += sep
		}
		result += item
	}
	return result
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	result += "\n"
	return result
}