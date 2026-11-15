package ottl

import (
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"
)

// ExportPipelinesToYaml serializes the modeled Collector configuration.
func ExportPipelinesToYaml(p PipelinesBySignal) (string, error) {
	receivers := map[string]interface{}{}
	exporters := map[string]interface{}{}
	processors := map[string]interface{}{}
	pipelines := map[string]interface{}{}
	merge := func(kind string, dst map[string]interface{}, src map[string]interface{}) error {
		for key, value := range src {
			if old, ok := dst[key]; ok && !reflect.DeepEqual(old, value) {
				return fmt.Errorf("conflicting %s configuration for %q", kind, key)
			}
			dst[key] = value
		}
		return nil
	}
	for _, signal := range []string{"traces", "logs", "metrics"} {
		cfg := p[signal]
		if cfg == nil {
			continue
		}
		if err := merge("receiver", receivers, cfg.ReceiverConfigs); err != nil {
			return "", err
		}
		if err := merge("exporter", exporters, cfg.ExporterConfigs); err != nil {
			return "", err
		}
		for ref, config := range cfg.Processors {
			if old, ok := processors[ref]; ok && !reflect.DeepEqual(old, config) {
				return "", fmt.Errorf("conflicting processor configuration for %q", ref)
			}
			processors[ref] = config
		}
		refs := make([]string, 0, len(cfg.Stages))
		for _, stage := range cfg.Stages {
			if _, ok := cfg.Processors[stage.ProcessorRef]; !ok {
				return "", fmt.Errorf("undefined processor %q", stage.ProcessorRef)
			}
			refs = append(refs, stage.ProcessorRef)
		}
		r := cfg.Receivers
		e := cfg.Exporters
		if len(r) == 0 {
			r = []string{"otlp"}
		}
		if len(e) == 0 {
			e = []string{"debug"}
		}
		pipelines[signal] = map[string]interface{}{"receivers": r, "processors": refs, "exporters": e}
	}
	for _, pipeline := range pipelines {
		cfg := pipeline.(map[string]interface{})
		for _, ref := range cfg["receivers"].([]string) {
			if _, ok := receivers[ref]; !ok {
				receivers[ref] = nil
			}
		}
		for _, ref := range cfg["exporters"].([]string) {
			if _, ok := exporters[ref]; !ok {
				exporters[ref] = nil
			}
		}
	}
	tree := map[string]interface{}{"receivers": receivers, "processors": processors, "exporters": exporters, "service": map[string]interface{}{"pipelines": pipelines}}
	data, err := yaml.Marshal(tree)
	if err != nil {
		return "", fmt.Errorf("encode collector config: %w", err)
	}
	return string(data), nil
}
