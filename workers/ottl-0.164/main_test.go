package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/johndemoss/o11y-workbench/workers/ottl-0.164/engine"
)

func TestHandlePipelineExecute_LogFilter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pipeline/execute", handlePipelineExecute)

	logData := json.RawMessage(`{
  "resourceLogs": [{
    "scopeLogs": [{
      "logRecords": [
        {"body": {"stringValue": "a"}, "attributes": [{"key": "level", "value": {"stringValue": "info"}}]},
        {"body": {"stringValue": "b"}, "attributes": [{"key": "level", "value": {"stringValue": "warn"}}]}
      ]
    }]
  }]
}`)

	body, err := json.Marshal(map[string]any{
		"signal": "logs",
		"data":   logData,
		"pipeline": engine.Pipeline{
			Stages: []engine.Stage{
				{ID: "s1", Type: engine.StageTypeFilter, ProcessorRef: "p1"},
			},
			Processors: map[string]engine.ProcessorConfig{
				"p1": {
					"logs": map[string]interface{}{
						"log_record": []interface{}{`attributes["level"] == "warn"`},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal req: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/pipeline/execute", bytes.NewReader(body))
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var result engine.PipelineResult
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.FinalSpanCount != 1 {
		t.Fatalf("FinalSpanCount = %d, want 1", result.FinalSpanCount)
	}
}

func TestHandlePipelineExecute_MetricTransform(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pipeline/execute", handlePipelineExecute)

	metricData := json.RawMessage(`{
  "resourceMetrics": [{
    "scopeMetrics": [{
      "metrics": [{
        "name": "demo.counter",
        "sum": {
          "aggregationTemporality": 2,
          "isMonotonic": true,
          "dataPoints": [
            {"asInt": "1"},
            {"asInt": "2"}
          ]
        }
      }]
    }]
  }]
}`)

	body, err := json.Marshal(map[string]any{
		"signal": "metrics",
		"data":   metricData,
		"pipeline": engine.Pipeline{
			Stages: []engine.Stage{
				{ID: "s1", Type: engine.StageTypeTransform, ProcessorRef: "p1"},
			},
			Processors: map[string]engine.ProcessorConfig{
				"p1": {
					"metric_statements": []interface{}{
						map[string]interface{}{
							"context":    "datapoint",
							"statements": []interface{}{`set(attributes["pipeline"], "via-route")`},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal req: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/pipeline/execute", bytes.NewReader(body))
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var result engine.PipelineResult
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.FinalSpanCount != 2 {
		t.Fatalf("FinalSpanCount = %d, want 2", result.FinalSpanCount)
	}
	if !strings.Contains(string(result.FinalData), "via-route") {
		t.Fatalf("expected pipeline attribute in final data: %s", result.FinalData)
	}
}

func TestHandlePipelineExecute_UnsupportedSignal(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pipeline/execute", handlePipelineExecute)

	body, err := json.Marshal(map[string]any{
		"signal":   "profiles",
		"data":     json.RawMessage(`{}`),
		"pipeline": engine.Pipeline{},
	})
	if err != nil {
		t.Fatalf("marshal req: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/pipeline/execute", bytes.NewReader(body))
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "unsupported signal") {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestHandlePipelineExecute_DefaultTraces(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pipeline/execute", handlePipelineExecute)

	traceData := json.RawMessage(`{
  "resourceSpans": [{
    "scopeSpans": [{
      "spans": [{
        "traceId": "0102030405060708090a0b0c0d0e0f10",
        "spanId": "0102030405060708",
        "name": "n",
        "startTimeUnixNano": "1",
        "endTimeUnixNano": "2"
      }]
    }]
  }]
}`)

	body, err := json.Marshal(map[string]any{
		"data":     traceData,
		"pipeline": engine.Pipeline{},
	})
	if err != nil {
		t.Fatalf("marshal req: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/pipeline/execute", bytes.NewReader(body))
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var result engine.PipelineResult
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.FinalSpanCount != 1 {
		t.Fatalf("FinalSpanCount = %d, want 1", result.FinalSpanCount)
	}
}
