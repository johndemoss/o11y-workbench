package ottl

import (
	"testing"
)

func TestParseTransformStatements_FullConfig(t *testing.T) {
	yamlStr := `
trace_statements:
  - context: span
    statements:
      - set(attributes["env"], "prod")
      - delete_key(attributes, "secret")
`
	stmts, err := ParseTransformStatements(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stmts) != 2 {
		t.Fatalf("got %d statements, want 2", len(stmts))
	}
	if stmts[0] != `set(attributes["env"], "prod")` {
		t.Errorf("stmts[0] = %q, want %q", stmts[0], `set(attributes["env"], "prod")`)
	}
	if stmts[1] != `delete_key(attributes, "secret")` {
		t.Errorf("stmts[1] = %q, want %q", stmts[1], `delete_key(attributes, "secret")`)
	}
}

func TestParseTransformStatements_Wrapped(t *testing.T) {
	yamlStr := `
transform:
  trace_statements:
    - context: span
      statements:
        - set(attributes["env"], "prod")
`
	stmts, err := ParseTransformStatements(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("got %d statements, want 1", len(stmts))
	}
}

func TestParseTransformStatements_Simplified(t *testing.T) {
	yamlStr := `
statements:
  - set(attributes["env"], "prod")
  - keep_keys(attributes, ["key1"])
`
	stmts, err := ParseTransformStatements(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stmts) != 2 {
		t.Fatalf("got %d statements, want 2", len(stmts))
	}
}

func TestParseTransformStatements_FlatYAMLList(t *testing.T) {
	yamlStr := `
- set(attributes["env"], "prod")
- delete_key(attributes, "secret")
`
	stmts, err := ParseTransformStatements(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stmts) != 2 {
		t.Fatalf("got %d statements, want 2", len(stmts))
	}
}

func TestParseTransformStatements_FlatPerLine(t *testing.T) {
	input := `
set(attributes["env"], "prod")
delete_key(attributes, "secret")
`
	stmts, err := ParseTransformStatements(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stmts) != 2 {
		t.Fatalf("got %d statements, want 2", len(stmts))
	}
}

func TestParseTransformStatements_Empty(t *testing.T) {
	_, err := ParseTransformStatements("")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseTransformStatements_CommentsIgnored(t *testing.T) {
	input := `
# this is a comment
set(attributes["env"], "prod")
# another comment
`
	stmts, err := ParseTransformStatements(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("got %d statements, want 1", len(stmts))
	}
}

func TestParseFilterConditions_FullConfig(t *testing.T) {
	yamlStr := `
traces:
  span:
    - kind == SPAN_KIND_INTERNAL
    - attributes["http.status_code"] == 200
`
	conds, err := ParseFilterConditions(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conds) != 2 {
		t.Fatalf("got %d conditions, want 2", len(conds))
	}
	if conds[0] != `kind == SPAN_KIND_INTERNAL` {
		t.Errorf("conds[0] = %q, want %q", conds[0], `kind == SPAN_KIND_INTERNAL`)
	}
}

func TestParseFilterConditions_Wrapped(t *testing.T) {
	yamlStr := `
filter:
  traces:
    span:
      - kind == SPAN_KIND_INTERNAL
`
	conds, err := ParseFilterConditions(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conds) != 1 {
		t.Fatalf("got %d conditions, want 1", len(conds))
	}
}

func TestParseFilterConditions_MetricsDatapoint(t *testing.T) {
	yamlStr := `
metrics:
  datapoint:
    - attributes["env"] == "prod"
    - name == "demo.counter"
`
	conds, err := ParseFilterConditions(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conds) != 2 {
		t.Fatalf("got %d conditions, want 2", len(conds))
	}
	if conds[0] != `attributes["env"] == "prod"` {
		t.Errorf("conds[0] = %q, want %q", conds[0], `attributes["env"] == "prod"`)
	}
}

func TestParseFilterConditions_MetricsDatapointWrapped(t *testing.T) {
	yamlStr := `
filter:
  metrics:
    datapoint:
      - attributes["env"] == "prod"
`
	conds, err := ParseFilterConditions(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conds) != 1 {
		t.Fatalf("got %d conditions, want 1", len(conds))
	}
	if conds[0] != `attributes["env"] == "prod"` {
		t.Errorf("conds[0] = %q, want %q", conds[0], `attributes["env"] == "prod"`)
	}
}

func TestParseFilterConditions_MetricsWithErrorMode(t *testing.T) {
	yamlStr := `
error_mode: ignore
metrics:
  datapoint:
    - attributes["env"] == "prod"
`
	conds, err := ParseFilterConditions(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conds) != 1 {
		t.Fatalf("got %d conditions, want 1", len(conds))
	}
	if conds[0] != `attributes["env"] == "prod"` {
		t.Errorf("conds[0] = %q, want %q", conds[0], `attributes["env"] == "prod"`)
	}
}

func TestParseFilterConditions_FlatYAMLList(t *testing.T) {
	yamlStr := `
- kind == SPAN_KIND_INTERNAL
- name == "GET /api/users"
`
	conds, err := ParseFilterConditions(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conds) != 2 {
		t.Fatalf("got %d conditions, want 2", len(conds))
	}
}

func TestParseFilterConditions_FlatPerLine(t *testing.T) {
	input := `
kind == SPAN_KIND_INTERNAL
name == "GET /api/users"
`
	conds, err := ParseFilterConditions(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conds) != 2 {
		t.Fatalf("got %d conditions, want 2", len(conds))
	}
}

func TestParseFilterConditions_Empty(t *testing.T) {
	_, err := ParseFilterConditions("")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestMultipleContextGroups(t *testing.T) {
	yamlStr := `
trace_statements:
  - context: span
    statements:
      - set(attributes["a"], "1")
  - context: span
    statements:
      - set(attributes["b"], "2")
      - set(attributes["c"], "3")
`
	stmts, err := ParseTransformStatements(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stmts) != 3 {
		t.Fatalf("got %d statements, want 3", len(stmts))
	}
}

func TestSingleCondition(t *testing.T) {
	conds, err := ParseFilterConditions(`kind == SPAN_KIND_INTERNAL`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conds) != 1 {
		t.Fatalf("got %d conditions, want 1", len(conds))
	}
}

func TestSingleStatement(t *testing.T) {
	stmts, err := ParseTransformStatements(`set(attributes["env"], "prod")`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("got %d statements, want 1", len(stmts))
	}
}

func TestLooksLikeFilterConfig_TracesSpan(t *testing.T) {
	yamlStr := `
traces:
  span:
    - kind == SPAN_KIND_INTERNAL
`
	if !LooksLikeFilterConfig(yamlStr) {
		t.Error("LooksLikeFilterConfig should detect traces/span filter config")
	}
}

func TestLooksLikeFilterConfig_LogsLogRecord(t *testing.T) {
	yamlStr := `
logs:
  log_record:
    - severity == "ERROR"
`
	if !LooksLikeFilterConfig(yamlStr) {
		t.Error("LooksLikeFilterConfig should detect logs/log_record filter config")
	}
}

func TestLooksLikeFilterConfig_MetricsDatapoint(t *testing.T) {
	yamlStr := `
metrics:
  datapoint:
    - value > 100
`
	if !LooksLikeFilterConfig(yamlStr) {
		t.Error("LooksLikeFilterConfig should detect metrics/datapoint filter config")
	}
}

func TestLooksLikeFilterConfig_Wrapped(t *testing.T) {
	yamlStr := `
filter:
  traces:
    span:
      - kind == SPAN_KIND_INTERNAL
`
	if !LooksLikeFilterConfig(yamlStr) {
		t.Error("LooksLikeFilterConfig should detect wrapped filter config")
	}
}

func TestLooksLikeFilterConfig_TransformNotDetected(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "full transform config",
			yaml: `
trace_statements:
  - context: span
    statements:
      - set(attributes["env"], "prod")
`,
		},
		{
			name: "wrapped transform",
			yaml: `
transform:
  trace_statements:
    - context: span
      statements:
        - set(attributes["env"], "prod")
`,
		},
		{
			name: "statements key",
			yaml: `
statements:
  - set(attributes["env"], "prod")
`,
		},
		{
			name: "flat yaml list",
			yaml: `
- set(attributes["env"], "prod")
- delete_key(attributes, "secret")
`,
		},
		{
			name: "plain statements per line",
			yaml: `set(attributes["env"], "prod")`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if LooksLikeFilterConfig(tc.yaml) {
				t.Errorf("LooksLikeFilterConfig should not detect transform config")
			}
		})
	}
}

func TestLooksLikeFilterConfig_Empty(t *testing.T) {
	if LooksLikeFilterConfig("") {
		t.Error("LooksLikeFilterConfig should return false for empty input")
	}
	if LooksLikeFilterConfig("  ") {
		t.Error("LooksLikeFilterConfig should return false for whitespace-only input")
	}
}

func TestParseTransformStatements_RejectsFilterConfig(t *testing.T) {
	yamlStr := `
traces:
  span:
    - kind == SPAN_KIND_INTERNAL
`
	_, err := ParseTransformStatements(yamlStr)
	if err != ErrLooksLikeFilterConfig {
		t.Fatalf("expected ErrLooksLikeFilterConfig, got: %v", err)
	}
}

func TestParseCombinedTransformConfig_RejectsFilterConfig(t *testing.T) {
	yamlStr := `
traces:
  span:
    - kind == SPAN_KIND_INTERNAL
`
	_, _, err := ParseCombinedTransformConfig(yamlStr)
	if err != ErrLooksLikeFilterConfig {
		t.Fatalf("expected ErrLooksLikeFilterConfig, got: %v", err)
	}
}

func TestParseCombinedTransformConfigAll_RejectsFilterConfig(t *testing.T) {
	yamlStr := `
traces:
  span:
    - kind == SPAN_KIND_INTERNAL
`
	_, _, _, err := ParseCombinedTransformConfigAll(yamlStr)
	if err != ErrLooksLikeFilterConfig {
		t.Fatalf("expected ErrLooksLikeFilterConfig, got: %v", err)
	}
}

func TestParseCombinedFilterConfig_Wrapped(t *testing.T) {
	yamlStr := `
filter:
  traces:
    span:
      - kind == SPAN_KIND_INTERNAL
  logs:
    log_record:
      - severity_text == "ERROR"
`
	traceConds, logConds, err := ParseCombinedFilterConfig(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(traceConds) != 1 {
		t.Fatalf("trace conditions: got %d, want 1", len(traceConds))
	}
	if len(logConds) != 1 {
		t.Fatalf("log conditions: got %d, want 1", len(logConds))
	}
}

func TestParseFilterConditionsBySignal_MultiSignal(t *testing.T) {
	yamlStr := `
traces:
  span:
    - kind == SPAN_KIND_INTERNAL
logs:
  log_record:
    - severity_text == "ERROR"
metrics:
  datapoint:
    - value > 0
`
	bySig, structured, err := ParseFilterConditionsBySignal(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !structured {
		t.Fatal("expected structured parse")
	}
	if len(bySig.Traces) != 1 || len(bySig.Logs) != 1 || len(bySig.Metrics) != 1 {
		t.Fatalf("got traces=%d logs=%d metrics=%d", len(bySig.Traces), len(bySig.Logs), len(bySig.Metrics))
	}
}

func TestParseTransformConfigMeta_PreservesGroups(t *testing.T) {
	yamlStr := `
trace_statements:
  - context: span
    conditions:
      - attributes["keep"] == true
    statements:
      - set(attributes["x"], "1")
`
	meta, err := ParseTransformConfigMeta(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !meta.Structured || len(meta.TraceGroups) != 1 {
		t.Fatalf("structured=%v groups=%d", meta.Structured, len(meta.TraceGroups))
	}
	if meta.TraceGroups[0].Context != "span" {
		t.Fatalf("context=%q", meta.TraceGroups[0].Context)
	}
	if len(meta.TraceGroups[0].Conditions) != 1 {
		t.Fatalf("conditions=%d", len(meta.TraceGroups[0].Conditions))
	}
}

func TestParseTransformStatements_TraceOnlyFromMultiSignal(t *testing.T) {
	yamlStr := `
trace_statements:
  - context: span
    statements:
      - set(attributes["trace"], "1")
log_statements:
  - context: log
    statements:
      - set(attributes["log"], "1")
`
	stmts, err := ParseTransformStatements(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stmts) != 1 || stmts[0] != `set(attributes["trace"], "1")` {
		t.Fatalf("got %#v", stmts)
	}
}

func TestParseCombinedTransformConfigAll_StructuredTraceOnly(t *testing.T) {
	yamlStr := `
trace_statements:
  - context: span
    statements:
      - set(attributes["trace"], "1")
`
	trace, log, metric, err := ParseCombinedTransformConfigAll(yamlStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(trace) != 1 || len(log) != 0 || len(metric) != 0 {
		t.Fatalf("trace=%d log=%d metric=%d", len(trace), len(log), len(metric))
	}
}
