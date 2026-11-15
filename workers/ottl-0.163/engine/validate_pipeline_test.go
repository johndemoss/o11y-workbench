package engine

import (
	"context"
	"strings"
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

func TestValidatePipelineGroupConditionContext(t *testing.T) {
	tests := []struct {
		name, section, context, condition string
		valid                             bool
	}{
		{"trace scope", "trace_statements", "scope", `version == "1.0"`, true},
		{"trace resource", "trace_statements", "resource", `attributes["service.name"] == "app"`, true},
		{"trace span rejects scope path", "trace_statements", "span", `version == "1.0"`, false},
		{"trace default", "trace_statements", "", `name == "hit"`, true},
		{"log record", "log_statements", "log_record", `body == "hit"`, true},
		{"metric datapoint", "metric_statements", "datapoint", `attributes["key"] == "value"`, true},
		{"log scope", "log_statements", "scope", `version == "1.0"`, true},
		{"metric resource", "metric_statements", "resource", `attributes["service.name"] == "app"`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ProcessorConfig{tt.section: []interface{}{map[string]interface{}{
				"context": tt.context, "conditions": []interface{}{tt.condition},
				"statements": []interface{}{`set(attributes["x"], "y")`},
			}}}
			res := ValidatePipelineOTTL(context.Background(), Pipeline{Processors: map[string]ProcessorConfig{"transform": cfg}})
			if res.Valid != tt.valid {
				t.Fatalf("Valid = %v, want %v; errors: %v", res.Valid, tt.valid, res.Errors)
			}
			if !tt.valid && (len(res.Errors) == 0 || !strings.Contains(strings.Join(res.Errors, " "), "span")) {
				t.Fatalf("expected span context error, got %v", res.Errors)
			}
		})
	}
}
