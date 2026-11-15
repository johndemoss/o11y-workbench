package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const diffBeforeTraces = `{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"00000000000000000000000000000001","spanId":"0000000000000001","name":"original-span"}]}]}]}`

func TestHandlePipelineDiff(t *testing.T) {
	tests := []struct {
		name       string
		signal     string
		body       string
		wantStatus int
		check      func(t *testing.T, body []byte)
	}{
		{
			name:       "invalid JSON returns 400",
			body:       "not json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unsupported signal returns 400",
			signal:     "sideways",
			body:       `{"stage_index":0,"before_data":null,"after_data":null}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "added record is reported for the stage",
			body:       `{"stage_index":2,"before_data":` + diffBeforeTraces + `,"after_data":{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"00000000000000000000000000000001","spanId":"0000000000000001","name":"original-span"},{"traceId":"00000000000000000000000000000001","spanId":"0000000000000002","name":"added-span"}]}]}]}}`,
			wantStatus: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				var diff struct {
					StageIndex int    `json:"stage_index"`
					Signal     string `json:"signal"`
					Added      []any  `json:"added"`
				}
				if err := json.Unmarshal(body, &diff); err != nil {
					t.Fatalf("decode diff: %v", err)
				}
				if diff.StageIndex != 2 {
					t.Errorf("stage_index = %d, want 2", diff.StageIndex)
				}
				if diff.Signal != "traces" {
					t.Errorf("signal = %q, want traces", diff.Signal)
				}
				if len(diff.Added) != 1 {
					t.Errorf("added = %d records, want 1", len(diff.Added))
				}
			},
		},
		{
			name:       "modified record is reported for the stage",
			body:       `{"stage_index":0,"before_data":` + diffBeforeTraces + `,"after_data":{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"00000000000000000000000000000001","spanId":"0000000000000001","name":"modified-span"}]}]}]}}`,
			wantStatus: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				var diff struct {
					Modified []any `json:"modified"`
				}
				if err := json.Unmarshal(body, &diff); err != nil {
					t.Fatalf("decode diff: %v", err)
				}
				if len(diff.Modified) != 1 {
					t.Errorf("modified = %d records, want 1", len(diff.Modified))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/ottl/pipeline/diff"
			if tt.signal != "" {
				url += "?signal=" + tt.signal
			}
			req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s := &Server{pipelineEnabled: true}
			s.handlePipelineDiff(rr, req)
			if rr.Code != tt.wantStatus {
				t.Fatalf("status %d, want %d: %s", rr.Code, tt.wantStatus, rr.Body)
			}
			if tt.check != nil {
				tt.check(t, rr.Body.Bytes())
			}
		})
	}
}
