package ottl

import (
	"encoding/json"
	"testing"
)

func TestPipelineResultJSON(t *testing.T) {
	result := PipelineResult{
		Stages: []StageResult{
			{
				StageIndex:       0,
				StageID:          "stage1",
				InputSpanCount:   10,
				OutputSpanCount:  10,
				StatementsParsed: 2,
				DurationNs:       1500,
			},
			{
				StageIndex:       1,
				StageID:          "stage2",
				InputSpanCount:   10,
				OutputSpanCount:  5,
				StatementsParsed: 1,
				Errors:           []string{`condition "foo" evaluated to false`},
				DurationNs:       800,
			},
		},
		FinalData:      json.RawMessage(`{"resource_spans":[{"spans":[{"name":"test"}]}]}`),
		FinalSpanCount: 5,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := decoded["stages"]; !ok {
		t.Error("missing stages field")
	}
	if _, ok := decoded["final_data"]; !ok {
		t.Error("missing final_data field")
	}
	if _, ok := decoded["final_span_count"]; !ok {
		t.Error("missing final_span_count field")
	}

	stages := decoded["stages"].([]interface{})
	if len(stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(stages))
	}
}

func TestPipelineResultEmpty(t *testing.T) {
	result := PipelineResult{}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	stages, _ := decoded["stages"].([]interface{})
	if len(stages) != 0 {
		t.Errorf("expected empty stages, got %d", len(stages))
	}
}

func TestPipelineErrorResult(t *testing.T) {
	errResult := PipelineErrorResult{Error: "execution failed: bad statement"}
	data, err := json.Marshal(errResult)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["error"] != "execution failed: bad statement" {
		t.Errorf("unexpected error value: %v", decoded["error"])
	}
}

func TestStageResultJSON(t *testing.T) {
	sr := StageResult{
		StageIndex:       0,
		StageID:          "add_env",
		InputSpanCount:   5,
		OutputSpanCount:  5,
		StatementsParsed: 1,
		DurationNs:       1200,
	}

	data, err := json.Marshal(sr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	checks := []struct {
		key  string
		want interface{}
	}{
		{"stage_index", float64(0)},
		{"stage_id", "add_env"},
		{"input_span_count", float64(5)},
		{"output_span_count", float64(5)},
		{"statements_parsed", float64(1)},
		{"duration_ns", float64(1200)},
	}
	for _, c := range checks {
		if got := decoded[c.key]; got != c.want {
			t.Errorf("%s: expected %v, got %v", c.key, c.want, got)
		}
	}
}
