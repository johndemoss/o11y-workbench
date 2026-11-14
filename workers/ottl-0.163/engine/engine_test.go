package engine

import "testing"

func TestValidateStatements_Valid(t *testing.T) {
	res := ValidateStatements([]string{`set(name, "x")`})
	if !res.Valid {
		t.Fatalf("expected valid, errors=%v", res.Errors)
	}
	if res.Parsed != 1 {
		t.Fatalf("parsed=%d want 1", res.Parsed)
	}
}

func TestValidateStatements_Invalid(t *testing.T) {
	res := ValidateStatements([]string{`set(`})
	if res.Valid {
		t.Fatal("expected invalid")
	}
	if len(res.Errors) == 0 {
		t.Fatal("expected errors")
	}
}

func TestTransformRunData_SetAttribute(t *testing.T) {
	// Minimal OTLP JSON with hex IDs (OTLP JSON encoding).
	traceJSON := []byte(`{
  "resourceSpans": [{
    "resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "svc"}}]},
    "scopeSpans": [{
      "spans": [{
        "traceId": "0102030405060708090a0b0c0d0e0f10",
        "spanId": "0102030405060708",
        "name": "original",
        "kind": 1,
        "startTimeUnixNano": "1",
        "endTimeUnixNano": "2",
        "attributes": [{"key": "http.method", "value": {"stringValue": "GET"}}]
      }]
    }]
  }]
}`)

	out, count, err := TransformRunData(traceJSON, []string{`set(attributes["env"], "prod")`, `set(name, "renamed")`})
	if err != nil {
		t.Fatalf("TransformRunData: %v", err)
	}
	if count != 1 {
		t.Fatalf("count=%d want 1", count)
	}
	if len(out) == 0 {
		t.Fatal("empty output")
	}
	s := string(out)
	if !contains(s, `"renamed"`) {
		t.Fatalf("expected renamed span name in output: %s", s)
	}
	if !contains(s, `"prod"`) {
		t.Fatalf("expected env=prod attribute in output: %s", s)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
