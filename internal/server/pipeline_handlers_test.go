package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/ottl"
	"github.com/johndemoss/o11y-workbench/internal/ottlengine"
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
	if p.Stages[0].ID != "transform/add_env" {
		t.Errorf("expected stage id 'transform/add_env', got %q", p.Stages[0].ID)
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

func TestNativePipelineWithReceivers(t *testing.T) {
	data := json.RawMessage(`{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"00000000000000000000000000000001","spanId":"0000000000000001","name":"hit"}]}]}]}`)
	store := NewRunStore()
	store.Add(&Run{ID: "native", TraceData: &data})
	s := &Server{store: store, pipelineEnabled: true}
	const pipeline = `stages:
  - id: transform
    type: transform
    processor_ref: transform
receivers: [otlp]
processors:
  transform:
    trace_statements:
      - context: span
        statements: ['set(attributes["native"], "yes")']
`
	for _, tt := range []struct {
		name string
		path string
	}{
		{"validate", "/api/ottl/pipeline/validate?signal=traces"},
		{"execute", "/api/ottl/pipeline/execute?signal=traces&run_id=native"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(pipeline))
			req.Header.Set("Content-Type", "application/x-yaml")
			rr := httptest.NewRecorder()
			if tt.name == "validate" {
				s.handlePipelineValidate(rr, req)
			} else {
				s.handlePipelineExecute(rr, req)
			}
			if rr.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rr.Code, rr.Body)
			}
			var result map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if tt.name == "validate" && result["valid"] != true {
				t.Fatalf("validation failed: %s", rr.Body)
			}
			if tt.name == "execute" && !strings.Contains(rr.Body.String(), `"native"`) {
				t.Fatalf("missing transformed attribute: %s", rr.Body)
			}
		})
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
			name:       "yaml with unsupported processor is rejected",
			yaml:       yamlConfig,
			wantStatus: http.StatusBadRequest,
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

func TestScenarioUploadIsAtomic(t *testing.T) {
	const existing = "atomic-upload-existing"
	const added = "atomic-upload-new"
	defer gen.DeleteUserScenario(existing)
	defer gen.DeleteUserScenario(added)
	if err := gen.RegisterUserScenario(existing, []*gen.ScenarioSpanNode{{Name: "original"}}); err != nil {
		t.Fatal(err)
	}
	body := `{"atomic-upload-existing":{"roots":[{"service_name":"svc","name":"replacement","kind":"INTERNAL"}]},"atomic-upload-new":{"roots":[{"service_name":"svc","name":"new","kind":"INTERNAL"}]},"monolith":{"roots":[{"service_name":"svc","name":"bad","kind":"INTERNAL"}]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/scenarios", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	(&Server{}).handleUploadScenarios(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("upload status %d: %s", rr.Code, rr.Body)
	}
	roots, ok := gen.GetUserScenario(existing)
	if !ok || len(roots) != 1 || roots[0].Name != "original" {
		t.Fatalf("existing scenario changed: %#v", roots)
	}
	if _, ok := gen.GetUserScenario(added); ok {
		t.Fatal("new scenario committed despite conflict")
	}
}

func TestCollectorPipelineValidationUsesEngine(t *testing.T) {
	config := `receivers:
  otlp:
    protocols:
      grpc:
processors:
  transform/bad:
    trace_statements:
      - context: span
        statements:
          - definitely_not_an_ottl_function()
exporters:
  debug:
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [transform/bad]
      exporters: [debug]`
	for _, tc := range []struct {
		query  string
		status int
	}{
		{"", http.StatusOK},
		{"?collector_version=0.0.0", http.StatusBadRequest},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/validate"+tc.query, strings.NewReader(config))
		req.Header.Set("Content-Type", "application/yaml")
		rr := httptest.NewRecorder()
		(&Server{}).handlePipelineValidate(rr, req)
		if rr.Code != tc.status {
			t.Fatalf("query %q: status %d body %s", tc.query, rr.Code, rr.Body)
		}
		if tc.status == http.StatusOK {
			var result struct {
				Valid bool `json:"valid"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil || result.Valid {
				t.Fatalf("invalid OTTL accepted: %s (%v)", rr.Body, err)
			}
		}
	}
}

type signalDispatchEngine struct {
	ottlengine.Engine
	called map[string]bool
}

type dispatchContextKey struct{}

func (e *signalDispatchEngine) mark(ctx context.Context, signal string) {
	e.called[signal] = ctx.Value(dispatchContextKey{}) == "request"
}

func (e *signalDispatchEngine) ExecutePipelineForSignal(ctx context.Context, data json.RawMessage, _ ottl.Pipeline, signal string) (*ottl.PipelineResult, error) {
	e.mark(ctx, signal)
	return &ottl.PipelineResult{
		Stages:         []ottl.StageResult{{StageIndex: 0, StageID: "transform", InputSpanCount: 1, OutputSpanCount: 1, StatementsParsed: 1}},
		FinalData:      data,
		FinalSpanCount: 1,
	}, nil
}

func (e *signalDispatchEngine) TransformTraces(ctx context.Context, data json.RawMessage, _ []string) (json.RawMessage, int, error) {
	e.mark(ctx, "traces")
	return data, 1, nil
}
func (e *signalDispatchEngine) TransformLogs(ctx context.Context, data json.RawMessage, _ []string) (json.RawMessage, error) {
	e.mark(ctx, "logs")
	return data, nil
}
func (e *signalDispatchEngine) TransformMetrics(ctx context.Context, data json.RawMessage, _ []string) (json.RawMessage, error) {
	e.mark(ctx, "metrics")
	return data, nil
}
func (e *signalDispatchEngine) FilterTraces(ctx context.Context, data json.RawMessage, _ []string, _ ottl.FilterMode) (json.RawMessage, int, int, error) {
	e.mark(ctx, "traces")
	return data, 1, 1, nil
}
func (e *signalDispatchEngine) FilterLogs(ctx context.Context, data json.RawMessage, _ []string, _ ottl.FilterMode) (json.RawMessage, int, int, error) {
	e.mark(ctx, "logs")
	return data, 1, 1, nil
}
func (e *signalDispatchEngine) FilterMetrics(ctx context.Context, data json.RawMessage, _ []string, _ ottl.FilterMode) (json.RawMessage, int, int, error) {
	e.mark(ctx, "metrics")
	return data, 1, 1, nil
}

func TestOTTLSignalCombinationsAndContext(t *testing.T) {
	transformConfig := "trace_statements:\n  - context: span\n    statements: ['set(name, \"a\")']\nlog_statements:\n  - context: log\n    statements: ['set(body, \"a\")']\nmetric_statements:\n  - context: metric\n    statements: ['set(name, \"a\")']\n"
	filterConfig := "traces:\n  span: ['name == \"a\"']\nlogs:\n  log_record: ['body == \"a\"']\nmetrics:\n  datapoint: ['value_int == 1']\n"
	for _, endpoint := range []string{"transform", "filter"} {
		for mask := 1; mask < 8; mask++ {
			for _, withTrace := range []bool{true, false} {
				if mask == 1 && !withTrace {
					continue
				}
				t.Run(fmt.Sprintf("%s-%d-traces-%t", endpoint, mask, withTrace), func(t *testing.T) {
					engine := &signalDispatchEngine{called: map[string]bool{}}
					store := NewRunStore()
					data := json.RawMessage(`{}`)
					run := &Run{ID: "dispatch", LogData: &data, MetricData: &data}
					if withTrace {
						run.TraceData = &data
					}
					store.Add(run)
					s := &Server{store: store, engines: ottlengine.NewRegistry("test", engine)}
					signals := map[string]bool{"traces": mask&1 != 0, "logs": mask&2 != 0, "metrics": mask&4 != 0}
					config := transformConfig
					if endpoint == "filter" {
						config = filterConfig
					}
					if mask == 1 || mask == 2 || mask == 4 {
						config = ""
					}
					body, err := json.Marshal(map[string]any{"run_id": run.ID, "signals": signals, "config": config, "statements": []string{"set(name, \"a\")"}, "conditions": []string{"true"}})
					if err != nil {
						t.Fatal(err)
					}
					req := httptest.NewRequest(http.MethodPost, "/api/ottl/"+endpoint, bytes.NewReader(body))
					req = req.WithContext(context.WithValue(req.Context(), dispatchContextKey{}, "request"))
					rr := httptest.NewRecorder()
					if endpoint == "transform" {
						s.handleOTTLTransform(rr, req)
					} else {
						s.handleOTTLFilter(rr, req)
					}
					if rr.Code != http.StatusOK {
						t.Fatalf("status %d: %s", rr.Code, rr.Body)
					}
					for name, selected := range signals {
						want := selected && (name != "traces" || withTrace)
						if engine.called[name] != want {
							t.Errorf("%s called with request context = %v, want %v", name, engine.called[name], want)
						}
					}
				})
			}
		}
	}
}
