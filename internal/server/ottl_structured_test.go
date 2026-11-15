package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/johndemoss/o11y-workbench/internal/ottlengine"
)

// TestStructuredTransformHonorsGroupConditions proves that a structured
// standalone transform routes through pipeline execution, so a group condition
// guards its statements instead of being flattened away.
func TestStructuredTransformHonorsGroupConditions(t *testing.T) {
	data := json.RawMessage(`{"resourceSpans":[{"scopeSpans":[{"spans":[
		{"traceId":"00000000000000000000000000000001","spanId":"0000000000000001","name":"hit"},
		{"traceId":"00000000000000000000000000000001","spanId":"0000000000000002","name":"miss"}
	]}]}]}`)
	store := NewRunStore()
	store.Add(&Run{ID: "structured", TraceData: &data})
	s := &Server{store: store, engines: ottlengine.NewRegistry(ottlengine.DefaultContribVersion, &ottlengine.LocalEngine{})}

	for _, tt := range []struct {
		name   string
		config string
	}{
		{"bare", "trace_statements:\n  - context: span\n    conditions: ['name == \"hit\"']\n    statements: ['set(attributes[\"matched\"], \"yes\")']\n"},
		{"wrapped", "transform:\n  trace_statements:\n    - context: span\n      conditions: ['name == \"hit\"']\n      statements: ['set(attributes[\"matched\"], \"yes\")']\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			validateBody, err := json.Marshal(map[string]any{"config": tt.config})
			if err != nil {
				t.Fatal(err)
			}
			validate := httptest.NewRecorder()
			s.handleOTTLValidate(validate, httptest.NewRequest(http.MethodPost, "/api/ottl/validate", bytes.NewReader(validateBody)))
			if validate.Code != http.StatusOK {
				t.Fatalf("validate status %d: %s", validate.Code, validate.Body)
			}
			body, err := json.Marshal(map[string]any{
				"run_id":  "structured",
				"signals": map[string]bool{"traces": true},
				"config":  tt.config,
			})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/ottl/transform", bytes.NewReader(body)).WithContext(context.Background())
			rr := httptest.NewRecorder()
			s.handleOTTLTransform(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rr.Code, rr.Body)
			}

			var res struct {
				Valid           bool            `json:"valid"`
				TransformedData json.RawMessage `json:"transformed_data"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if !res.Valid || len(res.TransformedData) == 0 {
				t.Fatalf("no transformed data: %s", rr.Body)
			}

			var payload struct {
				ResourceSpans []struct {
					ScopeSpans []struct {
						Spans []struct {
							Name       string `json:"name"`
							Attributes []struct {
								Key   string `json:"key"`
								Value struct {
									StringValue string `json:"stringValue"`
								} `json:"value"`
							} `json:"attributes"`
						} `json:"spans"`
					} `json:"scopeSpans"`
				} `json:"resourceSpans"`
			}
			if err := json.Unmarshal(res.TransformedData, &payload); err != nil {
				t.Fatalf("decode transformed data: %v", err)
			}

			seen := map[string]string{}
			for _, rs := range payload.ResourceSpans {
				for _, ss := range rs.ScopeSpans {
					for _, sp := range ss.Spans {
						for _, attr := range sp.Attributes {
							if attr.Key == "matched" {
								seen[sp.Name] = attr.Value.StringValue
							}
						}
					}
				}
			}
			if seen["hit"] != "yes" {
				t.Errorf("condition-matching span was not transformed: %v", seen)
			}
			if _, ok := seen["miss"]; ok {
				t.Errorf("non-matching span was transformed despite false condition: %v", seen)
			}
		})
	}
}

func TestStructuredTransformValidationAndErrorMode(t *testing.T) {
	data := json.RawMessage(`{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"00000000000000000000000000000001","spanId":"0000000000000001","name":"hit"}]}]}]}`)
	store := NewRunStore()
	store.Add(&Run{ID: "structured", TraceData: &data})
	s := &Server{store: store, engines: ottlengine.NewRegistry(ottlengine.DefaultContribVersion, &ottlengine.LocalEngine{})}
	for _, tt := range []struct {
		name       string
		endpoint   string
		config     string
		wantStatus int
		wantError  string
	}{
		{"invalid condition", "validate", "trace_statements:\n  - context: span\n    conditions: ['definitely_not_a_condition(']\n    statements: ['set(attributes[\"x\"], \"y\")']\n", http.StatusBadRequest, "definitely_not_a_condition"},
		{"ignore execution error", "transform", "transform:\n  error_mode: ignore\n  trace_statements:\n    - context: span\n      statements: ['set(attributes[\"bad\"], Substring(\"x\", -1, 3))']\n", http.StatusOK, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"run_id": "structured", "config": tt.config})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/ottl/"+tt.endpoint, bytes.NewReader(body))
			rr := httptest.NewRecorder()
			if tt.endpoint == "validate" {
				s.handleOTTLValidate(rr, req)
			} else {
				s.handleOTTLTransform(rr, req)
			}
			if rr.Code != tt.wantStatus || !bytes.Contains(rr.Body.Bytes(), []byte(tt.wantError)) {
				t.Fatalf("status %d body %s", rr.Code, rr.Body)
			}
		})
	}
}
