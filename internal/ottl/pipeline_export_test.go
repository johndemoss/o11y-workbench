package ottl

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestExportPipelinesToYaml(t *testing.T) {

	// Build PipelinesBySignal matching the golden file
	pipelines := PipelinesBySignal{
		"traces": {
			Stages: []Stage{
				{
					ID:           "add_env",
					Type:         StageTypeTransform,
					ErrorMode:    ErrorModeIgnore,
					ProcessorRef: "transform/add_env",
				},
				{
					ID:           "drop_internal",
					Type:         StageTypeFilter,
					ErrorMode:    ErrorModePropagate,
					ProcessorRef: "filter/drop_internal",
				},
			},
			Processors: map[string]ProcessorConfig{
				"transform/add_env": {
					"error_mode": "ignore",
					"trace_statements": []interface{}{
						map[string]interface{}{
							"context":    "span",
							"statements": []interface{}{`set(attributes["env"], "prod")`},
						},
					},
				},
				"filter/drop_internal": {
					"error_mode": "propagate",
					"traces": map[string]interface{}{
						"span": []interface{}{"kind != SPAN_KIND_INTERNAL"},
					},
				},
			},
		},
		"logs": {
			Stages:     []Stage{},
			Processors: map[string]ProcessorConfig{},
		},
		"metrics": {
			Stages:     []Stage{},
			Processors: map[string]ProcessorConfig{},
		},
	}

	got, err := ExportPipelinesToYaml(pipelines)
	if err != nil {
		t.Fatalf("ExportPipelinesToYaml failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := yaml.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("parse exported YAML: %v", err)
	}
	processors := decoded["processors"].(map[string]interface{})
	if !reflect.DeepEqual(processors["transform/add_env"], map[string]interface{}(pipelines["traces"].Processors["transform/add_env"])) {
		t.Errorf("processor config changed: %v", processors)
	}
}

func TestExportPipelinesToYaml_EmptyPipelines(t *testing.T) {
	pipelines := PipelinesBySignal{
		"traces": {
			Stages:     []Stage{},
			Processors: map[string]ProcessorConfig{},
		},
		"logs": {
			Stages:     []Stage{},
			Processors: map[string]ProcessorConfig{},
		},
		"metrics": {
			Stages:     []Stage{},
			Processors: map[string]ProcessorConfig{},
		},
	}

	got, err := ExportPipelinesToYaml(pipelines)
	if err != nil {
		t.Fatalf("ExportPipelinesToYaml failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := yaml.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("parse exported YAML: %v", err)
	}
	service := decoded["service"].(map[string]interface{})
	if len(service["pipelines"].(map[string]interface{})) != 3 {
		t.Errorf("expected all modeled pipelines: %v", service)
	}
}

func TestExportRoundTripAndConflicts(t *testing.T) {
	config := `receivers:
  otlp/custom: {protocols: {grpc: {endpoint: "127.0.0.1:4317"}}}
exporters:
  debug/custom: {verbosity: detailed}
processors:
  transform/foo:
    trace_statements:
      - context: span
        statements: ['set(attributes["message"], "a: b # c")']
service:
  pipelines:
    traces: {receivers: [otlp/custom], processors: [transform/foo], exporters: [debug/custom]}
`
	p, err := ImportCollectorConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ExportPipelinesToYaml(p)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ImportCollectorConfig(out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p["traces"], again["traces"]) {
		t.Fatalf("round trip changed pipeline:\n%s", out)
	}
	p["logs"] = &Pipeline{Stages: []Stage{{ProcessorRef: "transform/foo"}}, Processors: map[string]ProcessorConfig{"transform/foo": {"log_statements": []interface{}{}}}}
	if _, err := ExportPipelinesToYaml(p); err == nil {
		t.Fatal("expected conflicting processor error")
	}
}
