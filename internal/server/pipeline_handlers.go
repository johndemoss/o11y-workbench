package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/johndemoss/otel-workbench/internal/ottl"
	"gopkg.in/yaml.v3"
)

func parsePipelineFromYAML(pipelineYAML string) (*ottl.Pipeline, error) {
	var pipeline ottl.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineYAML), &pipeline); err != nil {
		return nil, fmt.Errorf("parse pipeline YAML: %w", err)
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

	pipeline, err := ottl.ImportCollectorConfig(configYAML)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, pipeline)
}
