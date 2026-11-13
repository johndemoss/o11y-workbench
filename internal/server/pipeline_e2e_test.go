package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/ottl"
)

func TestPipelineE2E_DiffOutput(t *testing.T) {
	fixtureData, err := os.ReadFile("../ottl/testdata/pipeline-fixture.json")
	if err != nil {
		t.Fatalf("read pipeline-fixture.json: %v", err)
	}

	var fixture struct {
		ID        string          `json:"id"`
		TraceData json.RawMessage `json:"trace_data"`
	}
	if err := json.Unmarshal(fixtureData, &fixture); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	store := NewRunStore()
	now := time.Now()
	run := &Run{
		ID:          fmt.Sprintf("run-%d", runIDCounter.Add(1)),
		Status:      RunStatusCompleted,
		StartedAt:   now,
		CompletedAt: &now,
		TraceData:   &fixture.TraceData,
	}
	store.Add(run)

	srv := &Server{store: store, pipelineEnabled: true}

	pipelineYAML := `
stages:
  - id: transform-all
    type: transform
    processor_ref: transform-proc
    error_mode: propagate
processors:
  transform-proc:
    trace_statements:
      - context: span
        statements:
          - set(attributes["pipeline"], "applied")
`

	executeBody, _ := json.Marshal(map[string]string{
		"pipeline_yaml": pipelineYAML,
	})
	executeReq := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/execute?run_id="+run.ID, bytes.NewReader(executeBody))
	executeReq.Header.Set("Content-Type", "application/json")
	executeRR := httptest.NewRecorder()

	srv.handlePipelineExecute(executeRR, executeReq)

	if executeRR.Code != http.StatusOK {
		t.Fatalf("execute pipeline: status %d, body: %s", executeRR.Code, executeRR.Body.String())
	}

	var execResult ottl.PipelineResult
	if err := json.Unmarshal(executeRR.Body.Bytes(), &execResult); err != nil {
		t.Fatalf("unmarshal execute result: %v", err)
	}

	diffBody, _ := json.Marshal(map[string]interface{}{
		"stages":        execResult.Stages,
		"original_data": fixture.TraceData,
	})
	diffReq := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/diff", bytes.NewReader(diffBody))
	diffReq.Header.Set("Content-Type", "application/json")
	diffRR := httptest.NewRecorder()

	srv.handlePipelineDiff(diffRR, diffReq)

	if diffRR.Code != http.StatusOK {
		t.Fatalf("compute diff: status %d, body: %s", diffRR.Code, diffRR.Body.String())
	}

	var diffs []map[string]interface{}
	if err := json.Unmarshal(diffRR.Body.Bytes(), &diffs); err != nil {
		t.Fatalf("diff response is not valid JSON array: %v", err)
	}

	if len(diffs) == 0 {
		t.Fatal("expected at least one stage diff, got none")
	}

	firstDiff := diffs[0]
	if stageIndex, ok := firstDiff["stage_index"].(float64); !ok || int(stageIndex) != 0 {
		t.Errorf("expected first diff to have stage_index 0, got %v", firstDiff["stage_index"])
	}

	modified, ok := firstDiff["modified"].([]interface{})
	if !ok {
		t.Fatalf("modified field is not an array: %v", firstDiff["modified"])
	}

	if len(modified) == 0 {
		t.Error("expected at least one modified span after transform, got none")
	}
}

func TestPipelineE2E_ExportImportRoundTrip(t *testing.T) {
	original := ottl.PipelinesBySignal{
		"traces": {
			Stages: []ottl.Stage{
				{ID: "add_env", Type: ottl.StageTypeTransform, ErrorMode: ottl.ErrorModeIgnore, ProcessorRef: "transform/add_env"},
				{ID: "drop_internal", Type: ottl.StageTypeFilter, ErrorMode: ottl.ErrorModePropagate, ProcessorRef: "filter/drop_internal"},
			},
			Processors: map[string]ottl.ProcessorConfig{
				"transform/add_env": {
					"error_mode": "ignore",
					"trace_statements": []interface{}{
						map[string]interface{}{
							"context":    "span",
							"statements": []interface{}{`set(attributes["env"], "prod")`},
						},
					},
				},
				"filter/drop_internal": {
					"error_mode": "propagate",
					"traces": map[string]interface{}{
						"span": []interface{}{"kind != SPAN_KIND_INTERNAL"},
					},
				},
			},
			Receivers: []string{"otlp"},
			Exporters: []string{"debug"},
		},
	}

	srv := &Server{pipelineEnabled: true}

	exportBody, _ := json.Marshal(original)
	exportReq := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/export", bytes.NewReader(exportBody))
	exportReq.Header.Set("Content-Type", "application/json")
	exportRR := httptest.NewRecorder()

	srv.handlePipelineExport(exportRR, exportReq)

	if exportRR.Code != http.StatusOK {
		t.Fatalf("export pipeline: status %d, body: %s", exportRR.Code, exportRR.Body.String())
	}

	exportedYAML := exportRR.Body.String()

	importReq := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/from-yaml", bytes.NewReader([]byte(exportedYAML)))
	importReq.Header.Set("Content-Type", "application/yaml")
	importRR := httptest.NewRecorder()

	srv.handleFromYaml(importRR, importReq)

	if importRR.Code != http.StatusOK {
		t.Fatalf("from-yaml import: status %d, body: %s", importRR.Code, importRR.Body.String())
	}

	var imported ottl.PipelinesBySignal
	if err := json.Unmarshal(importRR.Body.Bytes(), &imported); err != nil {
		t.Fatalf("unmarshal imported: %v", err)
	}

	origJSON, _ := json.Marshal(original)
	importedJSON, _ := json.Marshal(imported)

	if string(origJSON) != string(importedJSON) {
		t.Errorf("export/import round-trip mismatch:\n--- original:\n%s\n--- after round-trip:\n%s", origJSON, importedJSON)
	}
}

func TestPipelineE2E_TopologicalSort(t *testing.T) {
	srv := &Server{pipelineEnabled: true}

	tests := []struct {
		name       string
		request    map[string]interface{}
		wantSorted []string
	}{
		{
			name: "linear chain a->b->c",
			request: map[string]interface{}{
				"nodes": []map[string]interface{}{
					{"id": "a"}, {"id": "b"}, {"id": "c"},
				},
				"edges": []map[string]interface{}{
					{"source": "a", "target": "b"},
					{"source": "b", "target": "c"},
				},
			},
			wantSorted: []string{"a", "b", "c"},
		},
		{
			name: "diamond graph",
			request: map[string]interface{}{
				"nodes": []map[string]interface{}{
					{"id": "top"}, {"id": "left"}, {"id": "right"}, {"id": "bottom"},
				},
				"edges": []map[string]interface{}{
					{"source": "top", "target": "left"},
					{"source": "top", "target": "right"},
					{"source": "left", "target": "bottom"},
					{"source": "right", "target": "bottom"},
				},
			},
			wantSorted: []string{"top", "left", "right", "bottom"},
		},
		{
			name: "cycle detected",
			request: map[string]interface{}{
				"nodes": []map[string]interface{}{
					{"id": "x"}, {"id": "y"}, {"id": "z"},
				},
				"edges": []map[string]interface{}{
					{"source": "x", "target": "y"},
					{"source": "y", "target": "z"},
					{"source": "z", "target": "x"},
				},
			},
			wantSorted: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/topological-sort", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			srv.handleTopologicalSort(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", rr.Code)
				return
			}

			var resp map[string]interface{}
			json.Unmarshal(rr.Body.Bytes(), &resp)

			if tt.wantSorted == nil {
				if _, ok := resp["error"]; !ok {
					t.Errorf("expected error for cycle case, got: %v", resp)
				}
				return
			}

			sortedRaw, ok := resp["sorted_ids"].([]interface{})
			if !ok {
				t.Fatalf("missing sorted_ids: %v", resp)
			}
			sorted := make([]string, len(sortedRaw))
			for i, v := range sortedRaw {
				sorted[i] = v.(string)
			}

			if len(sorted) != len(tt.wantSorted) {
				t.Errorf("sorted length mismatch: got %d, want %d", len(sorted), len(tt.wantSorted))
				return
			}

			pos := make(map[string]int)
			for i, id := range sorted {
				pos[id] = i
			}
			for _, edge := range tt.request["edges"].([]map[string]interface{}) {
				src := edge["source"].(string)
				tgt := edge["target"].(string)
				if pos[src] >= pos[tgt] {
					t.Errorf("order violation: %s (%d) should come before %s (%d)", src, pos[src], tgt, pos[tgt])
				}
			}
		})
	}
}

func TestPipelineE2E_WouldCycle(t *testing.T) {
	srv := &Server{pipelineEnabled: true}

	tests := []struct {
		name      string
		edges     [][2]string
		newEdge   [2]string
		wantCycle bool
	}{
		{
			name:      "adding edge creates cycle",
			edges:     [][2]string{{"a", "b"}, {"b", "c"}},
			newEdge:   [2]string{"c", "a"},
			wantCycle: true,
		},
		{
			name:      "adding edge is safe",
			edges:     [][2]string{{"a", "b"}, {"b", "c"}},
			newEdge:   [2]string{"a", "c"},
			wantCycle: false,
		},
		{
			name:      "self-loop",
			edges:     [][2]string{{"a", "b"}},
			newEdge:   [2]string{"a", "a"},
			wantCycle: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edges := make([]map[string]string, len(tt.edges))
			for i, e := range tt.edges {
				edges[i] = map[string]string{"source": e[0], "target": e[1]}
			}

			body, _ := json.Marshal(map[string]interface{}{
				"edges": edges,
				"new_edge": map[string]string{
					"source": tt.newEdge[0],
					"target": tt.newEdge[1],
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/api/ottl/pipeline/would-cycle", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			srv.handleWouldCycle(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", rr.Code)
				return
			}

			var resp map[string]interface{}
			json.Unmarshal(rr.Body.Bytes(), &resp)

			wouldCycle, ok := resp["would_cycle"].(bool)
			if !ok {
				t.Fatalf("missing would_cycle field: %v", resp)
			}

			if wouldCycle != tt.wantCycle {
				t.Errorf("would_cycle = %v, want %v", wouldCycle, tt.wantCycle)
			}
		})
	}
}
