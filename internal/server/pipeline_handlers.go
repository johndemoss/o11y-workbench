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
	return parsePipelineFromYAMLForSignal(pipelineYAML, "traces")
}

// parsePipelineFromYAMLForSignal parses stages/processors YAML or extracts the
// requested signal pipeline from a full collector config.
func parsePipelineFromYAMLForSignal(pipelineYAML, signal string) (*ottl.Pipeline, error) {
	if signal == "" {
		signal = "traces"
	}
	var pipeline ottl.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineYAML), &pipeline); err != nil {
		if looksLikeCollectorConfig(pipelineYAML) {
			if p, err := pipelineFromCollectorConfig(pipelineYAML, signal); err == nil {
				return p, nil
			}
		}
		return nil, fmt.Errorf("parse pipeline YAML: %w", err)
	}
	if len(pipeline.Stages) == 0 && looksLikeCollectorConfig(pipelineYAML) {
		return pipelineFromCollectorConfig(pipelineYAML, signal)
	}
	return &pipeline, nil
}

func looksLikeCollectorConfig(pipelineYAML string) bool {
	return strings.Contains(pipelineYAML, "receivers:") ||
		strings.Contains(pipelineYAML, "processors:") ||
		strings.Contains(pipelineYAML, "service:")
}

func pipelineFromCollectorConfig(pipelineYAML, signal string) (*ottl.Pipeline, error) {
	pipelines, err := ottl.ImportCollectorConfig(pipelineYAML)
	if err != nil {
		return nil, fmt.Errorf("parse as collector config: %w", err)
	}
	if signal == "" {
		signal = resolveSignalFromPipelines("", pipelines)
	}
	p, ok := pipelines[signal]
	if !ok || p == nil || len(p.Stages) == 0 {
		return nil, fmt.Errorf("no %s pipeline stages found in collector config", signal)
	}
	return p, nil
}

func resolveSignalFromPipelines(requested string, pipelines ottl.PipelinesBySignal) string {
	if requested != "" {
		return requested
	}
	var withStages []string
	for _, signal := range []string{"traces", "logs", "metrics"} {
		if p, ok := pipelines[signal]; ok && p != nil && len(p.Stages) > 0 {
			withStages = append(withStages, signal)
		}
	}
	if len(withStages) == 1 {
		return withStages[0]
	}
	return "traces"
}

// parsePipelineRequestBody reads the request body once and returns YAML + optional fields.
func parsePipelineRequestBody(r *http.Request) (pipelineYAML, runID, signal string, err error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("read body: %w", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	runID = r.URL.Query().Get("run_id")
	signal = r.URL.Query().Get("signal")

	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "yaml") {
		return string(body), runID, signal, nil
	}

	var req struct {
		PipelineYAML string `json:"pipeline_yaml"`
		RunID        string `json:"run_id"`
		Signal       string `json:"signal"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", "", "", fmt.Errorf("invalid JSON body: %w", err)
	}
	if runID == "" {
		runID = req.RunID
	}
	if signal == "" {
		signal = req.Signal
	}
	return req.PipelineYAML, runID, signal, nil
}

// parsePipelineFromRequest reads and parses pipeline YAML from the request body.
// Supports:
//   - Content-Type: application/x-yaml or text/yaml (raw YAML body)
//   - Content-Type: application/json (JSON with pipeline_yaml field)
//
// Optional signal query/JSON field selects which collector pipeline to extract.
func parsePipelineFromRequest(r *http.Request) (*ottl.Pipeline, error) {
	pipelineYAML, _, signal, err := parsePipelineRequestBody(r)
	if err != nil {
		return nil, err
	}
	if signal == "" {
		signal = "traces"
	}
	return parsePipelineFromYAMLForSignal(pipelineYAML, signal)
}

func (s *Server) handlePipelineValidate(w http.ResponseWriter, r *http.Request) {
	pipelineYAML, _, signal, err := parsePipelineRequestBody(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "errors": []string{err.Error()}})
		return
	}

	// Collector configs: validate every non-empty signal pipeline (or a requested signal).
	if looksLikeCollectorConfig(pipelineYAML) {
		if pipelines, importErr := ottl.ImportCollectorConfig(pipelineYAML); importErr == nil {
			var errs []string
			validated := 0
			for _, sig := range []string{"traces", "logs", "metrics"} {
				if signal != "" && sig != signal {
					continue
				}
				p, ok := pipelines[sig]
				if !ok || p == nil || len(p.Stages) == 0 {
					continue
				}
				validated++
				if err := p.Validate(); err != nil {
					errs = append(errs, fmt.Sprintf("%s: %s", sig, err.Error()))
				}
			}
			if validated > 0 {
				if len(errs) > 0 {
					writeJSON(w, http.StatusOK, map[string]any{"valid": false, "errors": errs})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"valid": true, "errors": []string{}})
				return
			}
		}
	}

	if signal == "" {
		signal = "traces"
	}
	pipeline, err := parsePipelineFromYAMLForSignal(pipelineYAML, signal)
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
	pipelineYAML, runID, signal, err := parsePipelineRequestBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
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

	// Resolve signal from collector config when omitted and only one signal has stages.
	if looksLikeCollectorConfig(pipelineYAML) {
		if pipelines, importErr := ottl.ImportCollectorConfig(pipelineYAML); importErr == nil {
			signal = resolveSignalFromPipelines(signal, pipelines)
		}
	}
	if signal == "" {
		signal = "traces"
	}

	pipeline, err := parsePipelineFromYAMLForSignal(pipelineYAML, signal)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := pipeline.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	data, err := runPayloadForSignal(run, signal)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	result, err := ottl.ExecutePipelineForSignal(data, *pipeline, signal)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func runPayloadForSignal(run *Run, signal string) (json.RawMessage, error) {
	switch signal {
	case "", "traces":
		if run.TraceData == nil {
			return nil, fmt.Errorf("run has no trace data")
		}
		return *run.TraceData, nil
	case "logs":
		if run.LogData == nil {
			return nil, fmt.Errorf("run has no log data")
		}
		return *run.LogData, nil
	case "metrics":
		if run.MetricData == nil {
			return nil, fmt.Errorf("run has no metric data")
		}
		return *run.MetricData, nil
	default:
		return nil, fmt.Errorf("unsupported signal: %s", signal)
	}
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
