package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/johndemoss/o11y-workbench/internal/ottl"
)

func TestParsePipelineFromYAML_CollectorConfigFallback(t *testing.T) {
	collectorConfigYAML := `receivers:
  otlp:
    protocols:
      grpc:
processors:
  transform/add_env:
    error_mode: ignore
    trace_statements:
      - context: span
        statements:
          - set(attributes["env"], "prod")
exporters:
  debug:
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [transform/add_env]
      exporters: [debug]`

	p, err := parsePipelineFromYAML(collectorConfigYAML)
	if err != nil {
		t.Fatalf("expected no error for collector config YAML, got %v", err)
	}
	if len(p.Stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(p.Stages))
	}
	if p.Stages[0].ID != "add_env" {
		t.Errorf("expected stage id 'add_env', got %q", p.Stages[0].ID)
	}
	if p.Stages[0].Type != ottl.StageTypeTransform {
		t.Errorf("expected stage type transform, got %v", p.Stages[0].Type)
	}
	if p.Stages[0].ProcessorRef != "transform/add_env" {
		t.Errorf("expected processor ref 'transform/add_env', got %q", p.Stages[0].ProcessorRef)
	}
}

func TestParsePipelineFromYAML_PipelineFormatStillWorks(t *testing.T) {
	pipelineYAML := `stages:
  - id: stage-1
    type: transform
    processor_ref: proc-1
processors:
  proc-1:
    error_mode: ignore
    trace_statements:
      - context: span
        statements: []`

	p, err := parsePipelineFromYAML(pipelineYAML)
	if err != nil {
		t.Fatalf("expected no error for pipeline format YAML, got %v", err)
	}
	if len(p.Stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(p.Stages))
	}
	if p.Stages[0].ID != "stage-1" {
		t.Errorf("expected stage id 'stage-1', got %q", p.Stages[0].ID)
	}
}

func TestParsePipelineFromYAML_InvalidYAMLReturnsError(t *testing.T) {
	invalidYAML := `not: valid: yaml: []['`

	_, err := parsePipelineFromYAML(invalidYAML)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestHandleTopologicalSort(t *testing.T) {
	tests := []struct {
		name       string
		request    map[string]interface{}
		wantStatus int
	}{
		{
			name: "linear chain",
			request: map[string]interface{}{
				"nodes": []map[string]interface{}{
					{"id": "a"},
					{"id": "b"},
					{"id": "c"},
				},
				"edges": []map[string]interface{}{
					{"source": "a", "target": "b"},
					{"source": "b", "target": "c"},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "parallel branches",
			request: map[string]interface{}{
				"nodes": []map[string]interface{}{
					{"id": "root"},
					{"id": "branch1"},
					{"id": "branch2"},
					{"id": "merge"},
				},
				"edges": []map[string]interface{}{
					{"source": "root", "target": "branch1"},
					{"source": "root", "target": "branch2"},
					{"source": "branch1", "target": "merge"},
					{"source": "branch2", "target": "merge"},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "cycle detected",
			request: map[string]interface{}{
				"nodes": []map[string]interface{}{
					{"id": "a"},
					{"id": "b"},
					{"id": "c"},
				},
				"edges": []map[string]interface{}{
					{"source": "a", "target": "b"},
					{"source": "b", "target": "c"},
					{"source": "c", "target": "a"},
				},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/topological-sort", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			s := &Server{pipelineEnabled: true}
			s.handleTopologicalSort(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}

			var got map[string]interface{}
			json.Unmarshal(rr.Body.Bytes(), &got)

			if tt.name == "cycle detected" {
				if _, ok := got["error"]; !ok {
					t.Errorf("expected error field for cycle case")
				}
				return
			}

			sorted, ok := got["sorted_ids"].([]interface{})
			if !ok {
				t.Fatalf("response missing sorted_ids: %v", got)
			}
			sortedStr := make([]string, len(sorted))
			for i, v := range sorted {
				sortedStr[i] = v.(string)
			}

			if tt.name == "linear chain" {
				pos := map[string]int{"a": -1, "b": -1, "c": -1}
				for i, id := range sortedStr {
					pos[id] = i
				}
				if pos["a"] >= pos["b"] || pos["b"] >= pos["c"] {
					t.Errorf("expected a<b<c order, got %v", sortedStr)
				}
			}
			if tt.name == "parallel branches" {
				pos := map[string]int{"root": -1, "branch1": -1, "branch2": -1, "merge": -1}
				for i, id := range sortedStr {
					pos[id] = i
				}
				if pos["root"] >= pos["branch1"] || pos["root"] >= pos["branch2"] {
					t.Errorf("root must be before branches, got %v", sortedStr)
				}
				if pos["branch1"] >= pos["merge"] || pos["branch2"] >= pos["merge"] {
					t.Errorf("branches must be before merge, got %v", sortedStr)
				}
			}
		})
	}
}

func TestHandleTopologicalSort_Disabled(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"nodes": []map[string]interface{}{{"id": "a"}},
		"edges": []map[string]interface{}{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/topological-sort", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s := &Server{pipelineEnabled: false}
	s.handleTopologicalSort(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 when disabled, got %d", rr.Code)
	}
}

func TestHandleWouldCycle(t *testing.T) {
	tests := []struct {
		name      string
		request   map[string]interface{}
		wantCycle bool
	}{
		{
			name: "no cycle - adding edge to independent node",
			request: map[string]interface{}{
				"edges": []map[string]interface{}{
					{"source": "a", "target": "b"},
				},
				"new_edge": map[string]interface{}{
					"source": "c", "target": "d",
				},
			},
			wantCycle: false,
		},
		{
			name: "self-loop always creates cycle",
			request: map[string]interface{}{
				"edges": []map[string]interface{}{
					{"source": "a", "target": "b"},
				},
				"new_edge": map[string]interface{}{
					"source": "a", "target": "a",
				},
			},
			wantCycle: true,
		},
		{
			name: "duplicate edge creates cycle",
			request: map[string]interface{}{
				"edges": []map[string]interface{}{
					{"source": "a", "target": "b"},
					{"source": "b", "target": "c"},
				},
				"new_edge": map[string]interface{}{
					"source": "a", "target": "b",
				},
			},
			wantCycle: true,
		},
		{
			name: "cycle would be created",
			request: map[string]interface{}{
				"edges": []map[string]interface{}{
					{"source": "a", "target": "b"},
					{"source": "b", "target": "c"},
				},
				"new_edge": map[string]interface{}{
					"source": "c", "target": "a",
				},
			},
			wantCycle: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/would-cycle", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			s := &Server{pipelineEnabled: true}
			s.handleWouldCycle(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
			}

			var got map[string]interface{}
			json.Unmarshal(rr.Body.Bytes(), &got)

			wc, ok := got["would_cycle"].(bool)
			if !ok {
				t.Fatalf("response missing would_cycle: %v", got)
			}
			if wc != tt.wantCycle {
				t.Errorf("expected would_cycle=%v, got %v", tt.wantCycle, wc)
			}
		})
	}
}

func TestHandleWouldCycle_Disabled(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"edges":    []map[string]interface{}{{"source": "a", "target": "b"}},
		"new_edge": map[string]interface{}{"source": "b", "target": "c"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/would-cycle", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s := &Server{pipelineEnabled: false}
	s.handleWouldCycle(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 when disabled, got %d", rr.Code)
	}
}

func TestHandleFromYaml(t *testing.T) {
	yamlConfig := `
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [logging]
processors:
  batch:
    timeout: 10s
`
	tests := []struct {
		name        string
		yaml        string
		wantStatus  int
		wantSignals []string
	}{
		{
			name:        "valid yaml with traces pipeline",
			yaml:        yamlConfig,
			wantStatus:  http.StatusOK,
			wantSignals: []string{"traces"},
		},
		{
			name:       "invalid yaml",
			yaml:       "not: [valid: yaml",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/from-yaml", bytes.NewReader([]byte(tt.yaml)))
			req.Header.Set("Content-Type", "application/x-yaml")
			rr := httptest.NewRecorder()

			s := &Server{pipelineEnabled: true}
			s.handleFromYaml(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}

			if tt.wantStatus == http.StatusOK {
				var got map[string]*ottl.Pipeline
				json.Unmarshal(rr.Body.Bytes(), &got)
				for _, sig := range tt.wantSignals {
					if _, ok := got[sig]; !ok {
						t.Errorf("expected signal %q in response, got %v", sig, got)
					}
				}
			}
		})
	}
}

func TestHandleFromYaml_JSONBody(t *testing.T) {
	body, _ := json.Marshal(map[string]string{
		"config_yaml": "service:\n  pipelines:\n    traces:\n      receivers: [otlp]\n      processors: []\n      exporters: []\n",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/from-yaml", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s := &Server{pipelineEnabled: true}
	s.handleFromYaml(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var got map[string]*ottl.Pipeline
	json.Unmarshal(rr.Body.Bytes(), &got)
	if _, ok := got["traces"]; !ok {
		t.Errorf("expected traces signal in response")
	}
}

func TestHandleFromYaml_Disabled(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/from-yaml", bytes.NewReader([]byte("service:\n  pipelines:\n    traces:\n      receivers: []\n      processors: []\n      exporters: []\n")))
	req.Header.Set("Content-Type", "application/x-yaml")
	rr := httptest.NewRecorder()

	s := &Server{pipelineEnabled: false}
	s.handleFromYaml(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 when disabled, got %d", rr.Code)
	}
}

func TestHandlePipelineExport(t *testing.T) {
	validPipeline := &ottl.Pipeline{
		Stages: []ottl.Stage{
			{ID: "t1", Type: ottl.StageTypeTransform, ProcessorRef: "transform"},
		},
		Processors: map[string]ottl.ProcessorConfig{
			"transform": {"error_mode": "ignore"},
		},
	}

	emptyPipeline := &ottl.Pipeline{
		Stages:     []ottl.Stage{},
		Processors: map[string]ottl.ProcessorConfig{},
	}

	tests := []struct {
		name            string
		pipelineEnabled bool
		body            interface{}
		wantStatus      int
		checkBody       func(t *testing.T, body string)
	}{
		{
			name:            "pipeline disabled returns 404",
			pipelineEnabled: false,
			body: map[string]interface{}{
				"traces": validPipeline,
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:            "valid PipelinesBySignal returns YAML",
			pipelineEnabled: true,
			body: map[string]interface{}{
				"traces": validPipeline,
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body string) {
				if body == "" {
					t.Fatal("expected non-empty YAML body")
				}
				if !bytes.Contains([]byte(body), []byte("receivers:")) {
					t.Error("expected YAML to contain receivers section")
				}
				if !bytes.Contains([]byte(body), []byte("processors:")) {
					t.Error("expected YAML to contain processors section")
				}
				if !bytes.Contains([]byte(body), []byte("exporters:")) {
					t.Error("expected YAML to contain exporters section")
				}
				if !bytes.Contains([]byte(body), []byte("service:")) {
					t.Error("expected YAML to contain service section")
				}
			},
		},
		{
			name:            "empty pipelines returns valid YAML",
			pipelineEnabled: true,
			body: map[string]interface{}{
				"traces": emptyPipeline,
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body string) {
				if body == "" {
					t.Fatal("expected non-empty YAML body")
				}
				if !bytes.Contains([]byte(body), []byte("receivers:")) {
					t.Error("expected YAML to contain receivers section")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/export", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

			s := &Server{pipelineEnabled: tt.pipelineEnabled}
			w := httptest.NewRecorder()

			s.handlePipelineExport(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.checkBody != nil && w.Code == http.StatusOK {
				ct := w.Header().Get("Content-Type")
				if ct != "text/plain" && ct != "application/x-yaml" {
					t.Errorf("expected Content-Type text/plain or application/x-yaml, got %s", ct)
				}
				tt.checkBody(t, w.Body.String())
			}
		})
	}
}
func TestParsePipelineFromYAMLForSignal_LogsCollectorConfig(t *testing.T) {
	collectorConfigYAML := `receivers:
  otlp:
    protocols:
      grpc:
processors:
  transform/add_env:
    error_mode: ignore
    log_statements:
      - context: log
        statements:
          - set(attributes["env"], "prod")
exporters:
  debug:
service:
  pipelines:
    logs:
      receivers: [otlp]
      processors: [transform/add_env]
      exporters: [debug]`

	p, err := parsePipelineFromYAMLForSignal(collectorConfigYAML, "logs")
	if err != nil {
		t.Fatalf("expected no error for logs collector config, got %v", err)
	}
	if len(p.Stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(p.Stages))
	}
	if p.Stages[0].ProcessorRef != "transform/add_env" {
		t.Errorf("processor_ref = %q, want transform/add_env", p.Stages[0].ProcessorRef)
	}

	// traces-only parse should fail closed for logs-only config
	if _, err := parsePipelineFromYAML(collectorConfigYAML); err == nil {
		t.Fatal("expected traces parse to fail for logs-only collector config")
	}
}

func TestHandlePipelineValidate_LogsAndMetricsCollectorConfig(t *testing.T) {
	collectorConfigYAML := `receivers:
  otlp:
    protocols:
      grpc:
processors:
  transform/logs:
    log_statements:
      - context: log
        statements:
          - set(attributes["env"], "prod")
  transform/metrics:
    metric_statements:
      - context: datapoint
        statements:
          - set(attributes["env"], "prod")
exporters:
  debug:
service:
  pipelines:
    logs:
      receivers: [otlp]
      processors: [transform/logs]
      exporters: [debug]
    metrics:
      receivers: [otlp]
      processors: [transform/metrics]
      exporters: [debug]`

	req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/validate", bytes.NewReader([]byte(collectorConfigYAML)))
	req.Header.Set("Content-Type", "application/x-yaml")
	rr := httptest.NewRecorder()
	s := &Server{pipelineEnabled: true}
	s.handlePipelineValidate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["valid"] != true {
		t.Fatalf("valid = %v, want true; body=%s", got["valid"], rr.Body.String())
	}
}

func TestHandlePipelineExecute_LogsSignal(t *testing.T) {
	logData := json.RawMessage(`{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"cap-svc"}}]},"scopeLogs":[{"logRecords":[{"body":{"stringValue":"hello"},"attributes":[{"key":"level","value":{"stringValue":"info"}}]},{"body":{"stringValue":"warn"},"attributes":[{"key":"level","value":{"stringValue":"warn"}}]}]}]}]}`)
	store := NewRunStore()
	run := &Run{ID: "run-logs-1", Status: RunStatusCompleted, LogData: &logData}
	store.Add(run)
	srv := &Server{store: store, pipelineEnabled: true}

	pipelineYAML := `
stages:
  - id: transform-logs
    type: transform
    processor_ref: transform-proc
processors:
  transform-proc:
    log_statements:
      - context: log
        statements:
          - set(attributes["pipeline"], "applied")
`
	req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/execute?run_id="+run.ID+"&signal=logs", bytes.NewReader([]byte(pipelineYAML)))
	req.Header.Set("Content-Type", "application/x-yaml")
	rr := httptest.NewRecorder()
	srv.handlePipelineExecute(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("execute logs: status %d, body: %s", rr.Code, rr.Body.String())
	}
	var result ottl.PipelineResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Stages) != 1 {
		t.Fatalf("stages = %d, want 1", len(result.Stages))
	}
	if result.FinalSpanCount != 2 {
		t.Fatalf("FinalSpanCount = %d, want 2", result.FinalSpanCount)
	}
}

func TestHandlePipelineExecute_MetricsSignal(t *testing.T) {
	metricData := json.RawMessage(`{"resourceMetrics":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"cap-svc"}}]},"scopeMetrics":[{"metrics":[{"name":"cap.counter","sum":{"dataPoints":[{"asInt":"1","attributes":[{"key":"env","value":{"stringValue":"dev"}}]},{"asInt":"2","attributes":[{"key":"env","value":{"stringValue":"prod"}}]}],"aggregationTemporality":"AGGREGATION_TEMPORALITY_CUMULATIVE","isMonotonic":true}}]}]}]}`)
	store := NewRunStore()
	run := &Run{ID: "run-metrics-1", Status: RunStatusCompleted, MetricData: &metricData}
	store.Add(run)
	srv := &Server{store: store, pipelineEnabled: true}

	pipelineYAML := `
stages:
  - id: transform-metrics
    type: transform
    processor_ref: transform-proc
processors:
  transform-proc:
    metric_statements:
      - context: datapoint
        statements:
          - set(attributes["pipeline"], "applied")
`
	req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/execute?run_id="+run.ID+"&signal=metrics", bytes.NewReader([]byte(pipelineYAML)))
	req.Header.Set("Content-Type", "application/x-yaml")
	rr := httptest.NewRecorder()
	srv.handlePipelineExecute(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("execute metrics: status %d, body: %s", rr.Code, rr.Body.String())
	}
	var result ottl.PipelineResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.FinalSpanCount != 2 {
		t.Fatalf("FinalSpanCount = %d, want 2", result.FinalSpanCount)
	}
}
