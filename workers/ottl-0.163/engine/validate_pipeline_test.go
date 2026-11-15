package engine

import (
	"context"
	"testing"
)

func TestValidatePipelineAllSignalsAndFlat(t *testing.T) {
	for _, cfg := range []ProcessorConfig{
		{"statements": []interface{}{`this is not valid ottl`}},
		{"log_statements": []interface{}{map[string]interface{}{"context": "log", "statements": []interface{}{`this is not valid ottl`}}}},
		{"metric_statements": []interface{}{map[string]interface{}{"context": "datapoint", "statements": []interface{}{`this is not valid ottl`}}}},
		{"traces": map[string]interface{}{"span": []interface{}{`attributes["ok"] == "yes"`}}, "logs": map[string]interface{}{"log_record": []interface{}{`name == "x"`}}},
	} {
		result := ValidatePipelineOTTL(context.Background(), Pipeline{Processors: map[string]ProcessorConfig{"p": cfg}})
		if result.Valid {
			t.Fatalf("invalid config accepted: %#v", cfg)
		}
	}
}
