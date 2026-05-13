package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/johndemoss/mocktelemetrygen/internal/gen"
	"github.com/johndemoss/mocktelemetrygen/internal/ottl"
	"github.com/johndemoss/mocktelemetrygen/internal/region"
)

//go:embed web
var webFS embed.FS

type Server struct {
	store *RunStore
	mux   *http.ServeMux
	addr  string
}

func New(addr string) *Server {
	s := &Server{
		store: NewRunStore(),
		mux:   http.NewServeMux(),
		addr:  addr,
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /api/scenarios", s.handleScenarios)
	s.mux.HandleFunc("POST /api/scenarios", s.handleUploadScenarios)
	s.mux.HandleFunc("GET /api/config/defaults", s.handleDefaults)
	s.mux.HandleFunc("POST /api/runs", s.handleStartRun)
	s.mux.HandleFunc("GET /api/runs", s.handleListRuns)
	s.mux.HandleFunc("GET /api/runs/{id}", s.handleGetRun)
	s.mux.HandleFunc("POST /api/runs/import", s.handleImportRun)
	s.mux.HandleFunc("POST /api/runs/{id}/stop", s.handleStopRun)
	s.mux.HandleFunc("POST /api/ottl/validate", s.handleOTTLValidate)
	s.mux.HandleFunc("POST /api/ottl/transform", s.handleOTTLTransform)
	s.mux.HandleFunc("POST /api/ottl/filter", s.handleOTTLFilter)
	s.mux.HandleFunc("POST /api/ottl/filter-validate", s.handleOTTLFilterValidate)

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("embed web fs: %v", err)
	}
	fileServer := http.FileServer(http.FS(sub))
	s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(sub, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) Store() *RunStore { return s.store }

func (s *Server) ListenAndServe() error {
	log.Printf("Starting mocktelemetrygen web dashboard on %s", s.addr)
	return http.ListenAndServe(s.addr, s.mux)
}

func (s *Server) handleScenarios(w http.ResponseWriter, r *http.Request) {
	builtIn := []string{"microservice", "monolith", "3-tier-web-app", "event-driven"}
	userDefined := gen.ListUserScenarios()
	all := append([]string{}, builtIn...)
	all = append(all, userDefined...)
	writeJSON(w, http.StatusOK, map[string]any{
		"scenarios": all,
	})
}

func (s *Server) handleUploadScenarios(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read body: " + err.Error()})
		return
	}
	if len(body) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty request body"})
		return
	}

	// Detect format from Content-Type if possible, else auto-detect.
	ct := r.Header.Get("Content-Type")
	var ext string
	switch {
	case strings.Contains(ct, "/yaml") || strings.Contains(ct, "/x-yaml"):
		ext = ".yaml"
	case strings.Contains(ct, "/json") || strings.Contains(ct, "/x-json"):
		ext = ".json"
	}

	scenarios, err := gen.ParseScenarioFile(body, ext)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	registered := make([]string, 0, len(scenarios))
	for name, roots := range scenarios {
		if err := gen.RegisterUserScenario(name, roots); err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("scenario %q: %v", name, err)})
			return
		}
		registered = append(registered, name)
	}

	log.Printf("Registered %d user scenario(s) via API: %v", len(registered), registered)
	writeJSON(w, http.StatusCreated, map[string]any{
		"registered": registered,
	})
}

func (s *Server) handleDefaults(w http.ResponseWriter, r *http.Request) {
	def := gen.DefaultConfig()
	endpoints := make([]string, len(region.All()))
	for i, reg := range region.All() {
		endpoints[i] = region.GetEndpoint(reg)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"seed": def.Seed, "trace_count": def.TraceCount,
		"rate": 0, "topology": def.Topology.String(),
		"span_count": def.SpanCount, "max_depth": def.MaxDepth,
		"service_name": def.ServiceName, "scenario_span_count": 0,
		"output": "file", "format": "json",
		"endpoints": endpoints, "insecure": true,
	})
}

func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Seed              int64    `json:"seed"`
		TraceCount        int      `json:"trace_count"`
		Rate              int      `json:"rate"`
		Topology          string   `json:"topology"`
		Scenario          string   `json:"scenario"`
		ScenarioSpanCount int      `json:"scenario_span_count"`
		SpanCount         int      `json:"span_count"`
		MaxDepth          int      `json:"max_depth"`
		ServiceName       string   `json:"service_name"`
		Output            string   `json:"output"`
		Format            string   `json:"format"`
		Endpoints         []string `json:"endpoints"`
		Insecure          *bool    `json:"insecure"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	insecure := true
	if body.Insecure != nil {
		insecure = *body.Insecure
	}

	endpoints := body.Endpoints
	if len(endpoints) > 0 {
		region.SetEndpoints(endpoints)
	} else {
		region.SetEndpoints(nil)
		endpoints = make([]string, len(region.All()))
		for i, reg := range region.All() {
			endpoints[i] = region.GetEndpoint(reg)
		}
	}

	var topoType gen.TopologyType
	if body.Scenario == "" {
		var err error
		topoType, err = gen.ParseTopology(body.Topology)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}

	cfg := gen.Config{
		Seed: body.Seed, TraceCount: body.TraceCount, Rate: body.Rate,
		Topology: topoType, SpanCount: body.SpanCount, MaxDepth: body.MaxDepth,
		ServiceName: body.ServiceName,
		BaseTime:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: 5 * time.Second,
		Scenario: body.Scenario, ScenarioSpanCount: body.ScenarioSpanCount,
	}

	run, err := StartRun(r.Context(), s.store, cfg, endpoints, insecure, body.Output)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"runs": s.store.List()})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run := s.store.Get(id)
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found: " + id})
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleStopRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run := s.store.Get(id)
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found: " + id})
		return
	}
	s.store.StopRun(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped", "id": id})
}

func (s *Server) handleOTTLValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Statements []string `json:"statements"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if len(req.Statements) == 0 {
		writeJSON(w, http.StatusBadRequest, ottl.ValidateResult{Valid: false, Errors: []string{"no statements provided"}})
		return
	}

	result := ottl.ValidateStatements(req.Statements)
	status := http.StatusOK
	if !result.Valid {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, result)
}

func (s *Server) handleOTTLTransform(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RunID      string   `json:"run_id"`
		Statements []string `json:"statements"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.RunID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id is required"})
		return
	}
	if len(req.Statements) == 0 {
		writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{"no statements provided"}})
		return
	}

	run := s.store.Get(req.RunID)
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found: " + req.RunID})
		return
	}
	if run.TraceData == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no trace data; only completed runs with data can be transformed"})
		return
	}

	transformed, err := ottl.TransformRunData(*run.TraceData, req.Statements)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ottl.TransformResult{
			Valid:  false,
			Errors: []string{err.Error()},
		})
		return
	}

	writeJSON(w, http.StatusOK, ottl.TransformResult{
		Valid:            true,
		TransformedData:  &transformed,
		StatementsParsed: len(req.Statements),
	})
}

func (s *Server) handleOTTLFilter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RunID      string         `json:"run_id"`
		Conditions []string       `json:"conditions"`
		Mode       ottl.FilterMode `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.RunID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id is required"})
		return
	}
	if len(req.Conditions) == 0 {
		writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{"no conditions provided"}})
		return
	}
	if req.Mode == "" {
		req.Mode = ottl.FilterModeInclude
	}

	run := s.store.Get(req.RunID)
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found: " + req.RunID})
		return
	}
	if run.TraceData == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no trace data; only completed runs with data can be filtered"})
		return
	}

	filtered, total, kept, err := ottl.FilterRunData(*run.TraceData, req.Conditions, req.Mode)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ottl.FilterResult{
			Valid:  false,
			Errors: []string{err.Error()},
		})
		return
	}

	writeJSON(w, http.StatusOK, ottl.FilterResult{
		Valid:             true,
		FilteredData:      &filtered,
		ConditionsParsed:  len(req.Conditions),
		TotalSpanCount:    total,
		FilteredSpanCount: kept,
	})
}

func (s *Server) handleOTTLFilterValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Conditions []string `json:"conditions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if len(req.Conditions) == 0 {
		writeJSON(w, http.StatusBadRequest, ottl.ValidateResult{Valid: false, Errors: []string{"no conditions provided"}})
		return
	}

	result := ottl.ValidateFilterConditionsParse(req.Conditions)
	status := http.StatusOK
	if !result.Valid {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, result)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
