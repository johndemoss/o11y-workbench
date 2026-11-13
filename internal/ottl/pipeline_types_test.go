package ottl

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPipelineRoundTrip(t *testing.T) {
	p := Pipeline{
		Stages: []Stage{
			{ID: "stage1", Type: StageTypeTransform, ProcessorRef: "transform/add_env"},
			{ID: "stage2", Type: StageTypeFilter, ProcessorRef: "filter/keep_errors"},
		},
		Processors: map[string]ProcessorConfig{
			"transform/add_env":  {"trace_statements": []interface{}{map[string]interface{}{"context": "span", "statements": []interface{}{`set(attributes["env"], "prod")`}}}},
			"filter/keep_errors": {"traces": map[string]interface{}{"span": []interface{}{`status.code == 2`}}},
		},
	}

	data, err := yaml.Marshal(&p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Pipeline
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(got.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(got.Stages))
	}
	if got.Stages[0].ID != "stage1" || got.Stages[0].Type != StageTypeTransform {
		t.Errorf("stage0: expected stage1/transform, got %s/%s", got.Stages[0].ID, got.Stages[0].Type)
	}
	if got.Stages[1].ID != "stage2" || got.Stages[1].Type != StageTypeFilter {
		t.Errorf("stage1: expected stage2/filter, got %s/%s", got.Stages[1].ID, got.Stages[1].Type)
	}
	if len(got.Processors) != 2 {
		t.Fatalf("expected 2 processors, got %d", len(got.Processors))
	}
}

func TestPipelineValidate_EmptyStages(t *testing.T) {
	p := Pipeline{Stages: nil, Processors: map[string]ProcessorConfig{}}
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for empty stages")
	}
	if err.Error() != "pipeline must have at least one stage" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPipelineValidate_UnknownStageType(t *testing.T) {
	p := Pipeline{
		Stages: []Stage{
			{ID: "s1", Type: "unknown", ProcessorRef: "p1"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {},
		},
	}
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for unknown stage type")
	}
	if err.Error() != "unknown stage type: unknown" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPipelineValidate_MissingProcessorRef(t *testing.T) {
	p := Pipeline{
		Stages: []Stage{
			{ID: "s1", Type: StageTypeTransform, ProcessorRef: "missing"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {},
		},
	}
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for missing processor ref")
	}
	if err.Error() != "stage s1 references undefined processor: missing" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPipelineValidate_DuplicateStageID(t *testing.T) {
	p := Pipeline{
		Stages: []Stage{
			{ID: "s1", Type: StageTypeTransform, ProcessorRef: "p1"},
			{ID: "s1", Type: StageTypeFilter, ProcessorRef: "p2"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {},
			"p2": {},
		},
	}
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate stage id")
	}
	if err.Error() != "duplicate stage id: s1" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPipelineValidate_Success(t *testing.T) {
	p := Pipeline{
		Stages: []Stage{
			{ID: "s1", Type: StageTypeTransform, ProcessorRef: "p1"},
			{ID: "s2", Type: StageTypeFilter, ProcessorRef: "p2"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {},
			"p2": {},
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestPipelineValidate_EmptyID(t *testing.T) {
	p := Pipeline{
		Stages: []Stage{
			{ID: "", Type: StageTypeTransform, ProcessorRef: "p1"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {},
		},
	}
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for empty stage id")
	}
	if err.Error() != "stage must have a non-empty id" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPipelineValidate_EmptyProcessorRef(t *testing.T) {
	p := Pipeline{
		Stages: []Stage{
			{ID: "s1", Type: StageTypeTransform, ProcessorRef: ""},
		},
		Processors: map[string]ProcessorConfig{},
	}
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for empty processor_ref")
	}
	if err.Error() != "stage s1 has no processor_ref" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPipelineYAML_InvalidSyntax(t *testing.T) {
	var p Pipeline
	err := yaml.Unmarshal([]byte(`garbage: [`), &p)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
