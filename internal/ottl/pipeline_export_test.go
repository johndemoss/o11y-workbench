package ottl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExportPipelinesToYaml(t *testing.T) {
	// Load the golden file
	goldenPath := filepath.Join("testdata", "export-expected.yaml")
	goldenBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}
	expected := string(goldenBytes)

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

	if got != expected {
		t.Errorf("YAML output does not match golden file.\n\nGot:\n%s\n\nExpected:\n%s", got, expected)
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

	// Empty pipelines are skipped in output (matching export-yaml.ts behavior)
	expected := `receivers:
  otlp:
    protocols:
      grpc:

processors:
  {}

exporters:
  debug:

service:
  pipelines:
`
	if got != expected {
		t.Errorf("empty pipelines:\nGot:\n%s\n\nExpected:\n%s", got, expected)
	}
}
