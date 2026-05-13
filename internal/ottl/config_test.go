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
