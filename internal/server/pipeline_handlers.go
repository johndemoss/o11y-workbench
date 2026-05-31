package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/johndemoss/o11y-workbench/internal/ottl"
	"gopkg.in/yaml.v3"
)

func parsePipelineFromYAML(pipelineYAML string) (*ottl.Pipeline, error) {
	var pipeline ottl.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineYAML), &pipeline); err != nil {
		return nil, fmt.Errorf("parse pipeline YAML: %w", err)
	}
	// If Stages is empty but YAML has top-level "receivers:", this is a full
	// collector config — extract the traces pipeline from service.pipelines.
	if len(pipeline.Stages) == 0 {
		if strings.Contains(pipelineYAML, "receivers:") || strings.Contains(pipelineYAML, "processors:") {
			pipelines, err := ottl.ImportCollectorConfig(pipelineYAML)
			if err != nil {
				return nil, fmt.Errorf("parse as collector config: %w", err)
			}
			if p, ok := pipelines["traces"]; ok && len(p.Stages) > 0 {
				return p, nil
			}
		}
	}
	return &pipeline, nil
}

// parsePipelineFromRequest reads and parses pipeline YAML from the request body.
// Supports:
//   - Content-Type: application/x-yaml or text/yaml (raw YAML body)
//   - Content-Type: application/json (JSON with pipeline_yaml field)
func parsePipelineFromRequest(r *http.Request) (*ottl.Pipeline, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "yaml") {
		return parsePipelineFromYAML(string(body))
	}

	var req struct {
		PipelineYAML string `json:"pipeline_yaml"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid JSON body: %w", err)
	}
	return parsePipelineFromYAML(req.PipelineYAML)
}

func (s *Server) handlePipelineValidate(w http.ResponseWriter, r *http.Request) {
	pipeline, err := parsePipelineFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "errors": []string{err.Error()}})
		return
	}

	if err := pipeline.Validate(); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "errors": []string{err.Error()}})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "errors": []string{}})
}

func (s *Server) handlePipelineExecute(w http.ResponseWriter, r *http.Request) {
	runID := r.URL.Query().Get("run_id")

	if runID == "" {
		body, err := io.ReadAll(r.Body)
		if err == nil {
			r.Body = io.NopCloser(bytes.NewReader(body))
			var jsonReq struct {
				RunID string `json:"run_id"`
			}
			if json.Unmarshal(body, &jsonReq) == nil && jsonReq.RunID != "" {
				runID = jsonReq.RunID
			}
		}
	}

	if runID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id is required (as ?run_id=X or in JSON body)"})
		return
	}

	run := s.store.Get(runID)
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found: " + runID})
		return
	}

	if run.TraceData == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no trace data"})
		return
	}

	pipeline, err := parsePipelineFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := pipeline.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	result, err := ottl.ExecutePipeline(*run.TraceData, *pipeline)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePipelineDiff(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Stages       []ottl.StageResult `json:"stages"`
		OriginalData json.RawMessage    `json:"original_data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	diffs := ottl.ComputeAllStageDiffs(req.Stages, req.OriginalData)

	writeJSON(w, http.StatusOK, diffs)
}

func (s *Server) handlePipelineImport(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")

	var configYAML string
	if strings.Contains(ct, "yaml") {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		configYAML = string(body)
	} else {
		var req struct {
			ConfigYAML string `json:"config_yaml"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		configYAML = req.ConfigYAML
	}

	pipelines, err := ottl.ImportCollectorConfig(configYAML)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, pipelines)
}

func (s *Server) handlePipelineExport(w http.ResponseWriter, r *http.Request) {
	if !s.pipelineEnabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pipeline endpoint not enabled"})
		return
	}

	var pipelines ottl.PipelinesBySignal
	if err := json.NewDecoder(r.Body).Decode(&pipelines); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body: " + err.Error()})
		return
	}

	yamlOut, err := ottl.ExportPipelinesToYaml(pipelines)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(yamlOut))
}

// graph algorithm helpers (mirroring internal/ottl/graph.go for server-side use)

type flowNode struct {
	ID string
}

type flowEdge struct {
	Source string
	Target string
}

func topologicalSort(nodes []flowNode, edges []flowEdge) ([]string, error) {
	adjacency := make(map[string][]string)
	inDegree := make(map[string]int)
	for _, node := range nodes {
		adjacency[node.ID] = []string{}
		inDegree[node.ID] = 0
	}
	for _, edge := range edges {
		adjacency[edge.Source] = append(adjacency[edge.Source], edge.Target)
		inDegree[edge.Target]++
	}
	queue := []string{}
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	sorted := []string{}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		sorted = append(sorted, id)
		for _, neighbor := range adjacency[id] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}
	if len(sorted) != len(nodes) {
		return nil, fmt.Errorf("cycle detected in pipeline graph")
	}
	return sorted, nil
}

func wouldCreateCycle(edges []flowEdge, newEdge flowEdge) bool {
	if newEdge.Source == newEdge.Target {
		return true
	}
	for _, edge := range edges {
		if edge.Source == newEdge.Source && edge.Target == newEdge.Target {
			return true
		}
	}
	nodeIds := make(map[string]struct{})
	for _, edge := range edges {
		nodeIds[edge.Source] = struct{}{}
		nodeIds[edge.Target] = struct{}{}
	}
	nodeIds[newEdge.Source] = struct{}{}
	nodeIds[newEdge.Target] = struct{}{}
	nodes := make([]flowNode, 0, len(nodeIds))
	for id := range nodeIds {
		nodes = append(nodes, flowNode{ID: id})
	}
	testEdges := make([]flowEdge, len(edges)+1)
	copy(testEdges, edges)
	testEdges[len(edges)] = newEdge
	_, err := topologicalSort(nodes, testEdges)
	return err != nil
}

func (s *Server) handleTopologicalSort(w http.ResponseWriter, r *http.Request) {
	if !s.pipelineEnabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pipeline endpoint not enabled"})
		return
	}

	var req struct {
		Nodes []struct {
			ID string `json:"id"`
		} `json:"nodes"`
		Edges []struct {
			Source string `json:"source"`
			Target string `json:"target"`
		} `json:"edges"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	nodes := make([]flowNode, len(req.Nodes))
	edges := make([]flowEdge, len(req.Edges))
	for i, n := range req.Nodes {
		nodes[i] = flowNode{ID: n.ID}
	}
	for i, e := range req.Edges {
		edges[i] = flowEdge{Source: e.Source, Target: e.Target}
	}

	sorted, err := topologicalSort(nodes, edges)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"sorted_ids": []string{}, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"sorted_ids": sorted})
}

func (s *Server) handleWouldCycle(w http.ResponseWriter, r *http.Request) {
	if !s.pipelineEnabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pipeline endpoint not enabled"})
		return
	}

	var req struct {
		Edges []struct {
			Source string `json:"source"`
			Target string `json:"target"`
		} `json:"edges"`
		NewEdge struct {
			Source string `json:"source"`
			Target string `json:"target"`
		} `json:"new_edge"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	edges := make([]flowEdge, len(req.Edges))
	for i, e := range req.Edges {
		edges[i] = flowEdge{Source: e.Source, Target: e.Target}
	}
	newEdge := flowEdge{Source: req.NewEdge.Source, Target: req.NewEdge.Target}
	wouldCycle := wouldCreateCycle(edges, newEdge)
	writeJSON(w, http.StatusOK, map[string]any{"would_cycle": wouldCycle})
}

func (s *Server) handleFromYaml(w http.ResponseWriter, r *http.Request) {
	if !s.pipelineEnabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pipeline endpoint not enabled"})
		return
	}

	ct := r.Header.Get("Content-Type")
	var configYAML string
	if strings.Contains(ct, "yaml") {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		configYAML = string(body)
	} else {
		var req struct {
			ConfigYAML string `json:"config_yaml"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		configYAML = req.ConfigYAML
	}

	pipelines, err := ottl.ImportCollectorConfig(configYAML)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, pipelines)
}