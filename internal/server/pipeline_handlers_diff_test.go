package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/johndemoss/o11y-workbench/internal/ottl"
)

func TestHandlePipelineDiff(t *testing.T) {
	tests := []struct {
		name            string
		pipelineEnabled bool
		body            interface{}
		wantStatus      int
		checkBody       func(t *testing.T, body []byte)
	}{
		{
			name:            "invalid JSON returns 400",
			pipelineEnabled: true,
			body:            "not json",
			wantStatus:      http.StatusBadRequest,
		},
		{
			name:            "valid diff request returns stage diffs",
			pipelineEnabled: true,
			body: map[string]interface{}{
				"stages": []map[string]interface{}{
					{
						"stage_index":       0,
						"stage_id":          "stage-1",
						"input_span_count":  1,
						"output_span_count": 2,
						"intermediate_data": json.RawMessage(`{
							"resourceSpans": [{
								"scopeSpans": [{
									"spans": [
										{"spanId": "span-001", "name": "original-span"},
										{"spanId": "span-002", "name": "added-span"}
									]
								}]
							}]
						}`),
					},
				},
				"original_data": json.RawMessage(`{
					"resourceSpans": [{
						"scopeSpans": [{
							"spans": [
								{"spanId": "span-001", "name": "original-span"}
							]
						}]
					}]
				}`),
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var diffs []map[string]interface{}
				if err := json.Unmarshal(body, &diffs); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if len(diffs) != 1 {
					t.Fatalf("expected 1 stage diff, got %d", len(diffs))
				}
				stageDiff := diffs[0]
				added, ok := stageDiff["added"].([]interface{})
				if !ok {
					t.Fatal("expected 'added' field in stage diff")
				}
				if len(added) != 1 {
					t.Errorf("expected 1 added span, got %d", len(added))
				}
			},
		},
		{
			name:            "modified span shows changes",
			pipelineEnabled: true,
			body: map[string]interface{}{
				"stages": []map[string]interface{}{
					{
						"stage_index":       0,
						"stage_id":          "stage-1",
						"input_span_count":  1,
						"output_span_count": 1,
						"intermediate_data": json.RawMessage(`{
							"resourceSpans": [{
								"scopeSpans": [{
									"spans": [
										{"spanId": "span-001", "name": "modified-span", "attributes": {"key": "new-value"}}
									]
								}]
							}]
						}`),
					},
				},
				"original_data": json.RawMessage(`{
					"resourceSpans": [{
						"scopeSpans": [{
							"spans": [
								{"spanId": "span-001", "name": "original-span", "attributes": {"key": "old-value"}}
							]
						}]
					}]
				}`),
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var diffs []map[string]interface{}
				if err := json.Unmarshal(body, &diffs); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if len(diffs) != 1 {
					t.Fatalf("expected 1 stage diff, got %d", len(diffs))
				}
				stageDiff := diffs[0]
				modified, ok := stageDiff["modified"].([]interface{})
				if !ok {
					t.Fatal("expected 'modified' field in stage diff")
				}
				if len(modified) != 1 {
					t.Errorf("expected 1 modified span, got %d", len(modified))
				}
			},
		},
		{
			name:            "empty stages returns nil diffs",
			pipelineEnabled: true,
			body: map[string]interface{}{
				"stages":        []interface{}{},
				"original_data": json.RawMessage(`{"resourceSpans": []}`),
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var diffs []map[string]interface{}
				if err := json.Unmarshal(body, &diffs); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if diffs != nil {
					t.Errorf("expected nil diffs for empty stages, got %v", diffs)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyBytes []byte
			switch v := tt.body.(type) {
			case string:
				bodyBytes = []byte(v)
			default:
				bodyBytes, _ = json.Marshal(tt.body)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/diff", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

			s := &Server{pipelineEnabled: tt.pipelineEnabled}
			w := httptest.NewRecorder()

			s.handlePipelineDiff(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.checkBody != nil && w.Code == http.StatusOK {
				tt.checkBody(t, w.Body.Bytes())
			}
		})
	}
}

func TestHandlePipelineDiff_RouteEnabled(t *testing.T) {
	srv, _ := New(":0", false, nil)
	body, _ := json.Marshal(map[string]interface{}{
		"stages":        []interface{}{},
		"original_data": json.RawMessage(`{}`),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/diff", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 404/405 when pipelineEnabled=false, got %d", rr.Code)
	}
}

func TestComputeAllStageDiffs(t *testing.T) {
	stages := []ottl.StageResult{
		{
			StageIndex:      0,
			StageID:         "s1",
			InputSpanCount:  1,
			OutputSpanCount: 2,
			IntermediateData: json.RawMessage(`{
				"resourceSpans": [{
					"scopeSpans": [{
						"spans": [
							{"spanId": "span-001", "name": "original"},
							{"spanId": "span-002", "name": "added"}
						]
					}]
				}]
			}`),
		},
	}
	original := json.RawMessage(`{
		"resourceSpans": [{
			"scopeSpans": [{
				"spans": [
					{"spanId": "span-001", "name": "original"}
				]
			}]
		}]
	}`)

	diffs := ottl.ComputeAllStageDiffs(stages, original)

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].InputSpanCount != 1 {
		t.Errorf("expected input 1, got %d", diffs[0].InputSpanCount)
	}
	if diffs[0].OutputSpanCount != 2 {
		t.Errorf("expected output 2, got %d", diffs[0].OutputSpanCount)
	}
	if len(diffs[0].Added) != 1 {
		t.Errorf("expected 1 added span, got %d", len(diffs[0].Added))
	}
	if diffs[0].Added[0].SpanID != "span-002" {
		t.Errorf("expected added span-id span-002, got %s", diffs[0].Added[0].SpanID)
	}
}
