package engine

import (
	"context"
	"log"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if err := EnableOTTLLambdaFunctions(); err != nil {
		log.Fatalf("enable ottl.functions.enableLambda: %v", err)
	}
	os.Exit(m.Run())
}

func TestValidateStatements_LambdaFilter(t *testing.T) {
	stmt := `set(attributes["prod_tags"], Filter(attributes["tags"], (_, v) => v == "prod"))`
	res := ValidateStatements([]string{stmt})
	if !res.Valid {
		t.Fatalf("expected valid lambda statement, errors=%v", res.Errors)
	}
	if res.Parsed != 1 {
		t.Fatalf("parsed=%d want 1", res.Parsed)
	}
}

func TestTransformRunData_LambdaFilter(t *testing.T) {
	traceJSON := []byte(`{
  "resourceSpans": [{
    "resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "svc"}}]},
    "scopeSpans": [{
      "spans": [{
        "traceId": "0102030405060708090a0b0c0d0e0f10",
        "spanId": "0102030405060708",
        "name": "span",
        "kind": 1,
        "startTimeUnixNano": "1",
        "endTimeUnixNano": "2",
        "attributes": [{
          "key": "tags",
          "value": {
            "arrayValue": {
              "values": [
                {"stringValue": "prod"},
                {"stringValue": "dev"},
                {"stringValue": "prod"}
              ]
            }
          }
        }]
      }]
    }]
  }]
}`)

	stmt := `set(attributes["prod_tags"], Filter(attributes["tags"], (_, v) => v == "prod"))`
	out, count, err := TransformRunData(context.Background(), traceJSON, []string{stmt})
	if err != nil {
		t.Fatalf("TransformRunData: %v", err)
	}
	if count != 1 {
		t.Fatalf("count=%d want 1", count)
	}
	s := string(out)
	want := `"key":"prod_tags","value":{"arrayValue":{"values":[{"stringValue":"prod"},{"stringValue":"prod"}]`
	if !contains(s, want) {
		t.Fatalf("expected filtered prod_tags in output, want substring %q in %s", want, s)
	}
}
