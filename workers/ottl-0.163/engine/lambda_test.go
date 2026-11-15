package engine

import (
	"context"
	"encoding/json"
	"testing"
)

func TestValidateStatements_LambdaFilter(t *testing.T) {
	res := ValidateStatements([]string{
		`set(attributes["filtered"], Filter(attributes["tags"], (_, v) => v == "prod"))`,
	})
	if !res.Valid {
		t.Fatalf("expected valid lambda statement, errors=%v", res.Errors)
	}
	if res.Parsed != 1 {
		t.Fatalf("parsed=%d want 1", res.Parsed)
	}
}

func TestTransformLogRunData_LambdaFilter(t *testing.T) {
	logJSON := []byte(`{
  "resourceLogs": [{
    "scopeLogs": [{
      "logRecords": [{
        "body": {"stringValue": "event"},
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

	stmt := `set(body, Concat(Filter(attributes["tags"], (_, v) => v == "prod"), ","))`
	out, err := TransformLogRunData(context.Background(), logJSON, []string{stmt})
	if err != nil {
		t.Fatalf("TransformLogRunData: %v", err)
	}

	var payload struct {
		ResourceLogs []struct {
			ScopeLogs []struct {
				LogRecords []struct {
					Body struct {
						StringValue string `json:"stringValue"`
					} `json:"body"`
					Attributes []struct {
						Key   string `json:"key"`
						Value struct {
							ArrayValue *struct {
								Values []struct {
									StringValue string `json:"stringValue"`
								} `json:"values"`
							} `json:"arrayValue"`
						} `json:"value"`
					} `json:"attributes"`
				} `json:"logRecords"`
			} `json:"scopeLogs"`
		} `json:"resourceLogs"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(payload.ResourceLogs) == 0 || len(payload.ResourceLogs[0].ScopeLogs) == 0 ||
		len(payload.ResourceLogs[0].ScopeLogs[0].LogRecords) == 0 {
		t.Fatal("expected one log record in output")
	}
	rec := payload.ResourceLogs[0].ScopeLogs[0].LogRecords[0]
	if rec.Body.StringValue != "prod,prod" {
		t.Fatalf("body stringValue = %q, want prod,prod", rec.Body.StringValue)
	}

	var tagValues []string
	for _, attr := range rec.Attributes {
		if attr.Key != "tags" || attr.Value.ArrayValue == nil {
			continue
		}
		for _, v := range attr.Value.ArrayValue.Values {
			tagValues = append(tagValues, v.StringValue)
		}
	}
	if len(tagValues) != 3 {
		t.Fatalf("tags attribute length = %d, want 3 (unchanged source): %v", len(tagValues), tagValues)
	}
	wantTags := map[string]int{"prod": 0, "dev": 0}
	for _, v := range tagValues {
		wantTags[v]++
	}
	if wantTags["prod"] != 2 || wantTags["dev"] != 1 {
		t.Fatalf("tags attribute = %v, want two prod and one dev", tagValues)
	}
}
