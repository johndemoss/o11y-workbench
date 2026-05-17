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

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/ottl"
	"github.com/johndemoss/o11y-workbench/internal/region"
)

//go:embed web
var webFS embed.FS

//go:embed pipeline-ui/dist
var pipelineFS embed.FS

type Server struct {
	store           *RunStore
	mux             *http.ServeMux
	addr            string
	pipelineEnabled bool
}

func New(addr string, pipelineEnabled bool) *Server {
	s := &Server{
		store:           NewRunStore(),
		mux:             http.NewServeMux(),
		addr:            addr,
		pipelineEnabled: pipelineEnabled,
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/scenarios", s.handleScenarios)
	s.mux.HandleFunc("POST /api/scenarios", s.handleUploadScenarios)
	s.mux.HandleFunc("GET /api/config/defaults", s.handleDefaults)
	s.mux.HandleFunc("POST /api/runs", s.handleStartRun)
	s.mux.HandleFunc("GET /api/runs", s.handleListRuns)
	s.mux.HandleFunc("GET /api/runs/{id}", s.handleGetRun)
	s.mux.HandleFunc("POST /api/runs/import", s.handleImportRun)
	s.mux.HandleFunc("POST /api/runs/{id}/stop", s.handleStopRun)
	s.mux.HandleFunc("POST /api/runs/{id}/pause", s.handlePauseRun)
	s.mux.HandleFunc("POST /api/runs/{id}/resume", s.handleResumeRun)
	s.mux.HandleFunc("POST /api/ottl/validate", s.handleOTTLValidate)
	s.mux.HandleFunc("POST /api/ottl/transform", s.handleOTTLTransform)
	s.mux.HandleFunc("POST /api/ottl/filter", s.handleOTTLFilter)
	s.mux.HandleFunc("POST /api/ottl/filter-validate", s.handleOTTLFilterValidate)
	s.mux.HandleFunc("POST /api/ottl/validate-html", s.handleOTTLValidateHTML)
	s.mux.HandleFunc("POST /api/ottl/transform-html", s.handleOTTLTransformHTML)
	s.mux.HandleFunc("POST /api/ottl/filter-html", s.handleOTTLFilterHTML)

	if s.pipelineEnabled {
		s.mux.HandleFunc("POST /api/ottl/pipeline/validate", s.handlePipelineValidate)
		s.mux.HandleFunc("POST /api/ottl/pipeline/execute", s.handlePipelineExecute)
		s.mux.HandleFunc("POST /api/ottl/pipeline/import-config", s.handlePipelineImport)

		pipelineSub, err := fs.Sub(pipelineFS, "pipeline-ui/dist")
		if err != nil {
			log.Fatalf("embed pipeline ui fs: %v", err)
		}
		pipelineServer := http.FileServer(http.FS(pipelineSub))
		s.mux.Handle("GET /pipeline/", http.StripPrefix("/pipeline/", pipelineServer))
	}

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
			if path == "index.html" {
				w.Header().Set("Cache-Control", "no-cache, must-revalidate")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) Store() *RunStore { return s.store }

func (s *Server) ListenAndServe() error {
		log.Printf("Starting o11y-workbench web dashboard on %s", s.addr)
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
	logDef := gen.DefaultLogConfig()
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
		"signal_type": "traces",
		"log_config": map[string]any{
			"enabled":      logDef.Enabled,
			"logs_per_span": logDef.LogsPerSpan,
			"log_count":    logDef.LogCount,
			"severity_dist": map[string]int{
				"info":  logDef.SeverityDist.Info,
				"warn":  logDef.SeverityDist.Warn,
				"error": logDef.SeverityDist.Error,
				"fatal": logDef.SeverityDist.Fatal,
			},
			"body_template": logDef.BodyTemplate,
			"attributes":    []gen.LogAttribute{},
		},
		"metric_config": map[string]any{
			"enabled":       false,
			"metric_count":  20,
			"gauge_ratio":   0.4,
			"sum_ratio":     0.4,
			"value_min":     0,
			"value_max":     100,
			"attributes":    []gen.MetricAttribute{},
		},
	})
}

func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Seed              int64              `json:"seed"`
		TraceCount        int                `json:"trace_count"`
		Rate              int                `json:"rate"`
		Topology          string             `json:"topology"`
		Scenario          string             `json:"scenario"`
		ScenarioSpanCount int                `json:"scenario_span_count"`
		SpanCount         int                `json:"span_count"`
		MaxDepth          int                `json:"max_depth"`
		ServiceName       string             `json:"service_name"`
		Output            string             `json:"output"`
		Format            string             `json:"format"`
		Endpoints         []string           `json:"endpoints"`
		Insecure          *bool              `json:"insecure"`
		SignalType        string             `json:"signal_type"`
		LogConfig         gen.LogConfig      `json:"log_config"`
		MetricConfig      gen.MetricConfig   `json:"metric_config"`
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
		signalIncludesTraces := body.SignalType == "" || body.SignalType == "traces" || body.SignalType == "both" || body.SignalType == "all" || body.SignalType == "traces+metrics"
		if signalIncludesTraces {
			var err error
			topoType, err = gen.ParseTopology(body.Topology)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		}
	}

	signalType := body.SignalType
	if signalType == "" {
		signalType = "traces"
	}

	cfg := gen.Config{
		Seed: body.Seed, TraceCount: body.TraceCount, Rate: body.Rate,
		Topology: topoType, SpanCount: body.SpanCount, MaxDepth: body.MaxDepth,
		ServiceName: body.ServiceName,
		BaseTime:    time.Now(),
		TraceDuration: 5 * time.Second,
		Scenario: body.Scenario, ScenarioSpanCount: body.ScenarioSpanCount,
		SignalType:   signalType,
		LogConfig:    body.LogConfig,
		MetricConfig: body.MetricConfig,
	}

	// Validate log config.
	if signalType == "logs" || signalType == "both" {
		if signalType == "logs" || signalType == "both" {
			cfg.LogConfig.Enabled = true
			if cfg.LogConfig.LogCount == 0 {
				cfg.LogConfig.LogCount = gen.DefaultLogConfig().LogCount
			}
		}
		if cfg.LogConfig.SeverityDist == (gen.SeverityDistribution{}) {
			def := gen.DefaultLogConfig()
			cfg.LogConfig.SeverityDist = def.SeverityDist
		}
		if cfg.LogConfig.BodyTemplate == "" {
			cfg.LogConfig.BodyTemplate = gen.DefaultLogConfig().BodyTemplate
		}
		if cfg.LogConfig.LogsPerSpan == 0 {
			cfg.LogConfig.LogsPerSpan = gen.DefaultLogConfig().LogsPerSpan
		}
	}

	// Validate metric config.
	if signalType == "metrics" || signalType == "all" || signalType == "traces+metrics" || signalType == "logs+metrics" {
		cfg.MetricConfig.Enabled = true
		if cfg.MetricConfig.MetricCount == 0 {
			cfg.MetricConfig.MetricCount = gen.DefaultMetricConfig().MetricCount
		}
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

func (s *Server) handlePauseRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run := s.store.Get(id)
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found: " + id})
		return
	}
	s.store.PauseRun(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused", "id": id})
}

func (s *Server) handleResumeRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run := s.store.Get(id)
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found: " + id})
		return
	}
	s.store.ResumeRun(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed", "id": id})
}

func (s *Server) handleOTTLValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Config     string   `json:"config"`
		Statements []string `json:"statements"` // backward compat
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	var statements []string
	if req.Config != "" {
		var err error
		statements, err = ottl.ParseTransformStatements(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.ValidateResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
	} else {
		statements = req.Statements
	}

	result := ottl.ValidateStatements(statements)
	status := http.StatusOK
	if !result.Valid {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, result)
}

func (s *Server) handleOTTLTransform(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RunID      string   `json:"run_id"`
		Config     string   `json:"config"`
		Statements []string `json:"statements"` // backward compat
		SignalType string   `json:"signal_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.RunID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id is required"})
		return
	}

	run := s.store.Get(req.RunID)
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found: " + req.RunID})
		return
	}

	signalType := req.SignalType
	if signalType == "" {
		signalType = "traces"
	}

	if signalType == "both" {
		traceStmts, logStmts, err := ottl.ParseCombinedTransformConfig(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
		result := ottl.TransformResult{Valid: true}
		if len(traceStmts) > 0 && run.TraceData != nil {
			transformed, spanCount, err := ottl.TransformRunData(*run.TraceData, traceStmts)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.TransformResult{
					Valid:  false,
					Errors: []string{"trace: " + err.Error()},
				})
				return
			}
			result.TransformedData = &transformed
			result.StatementsParsed = len(traceStmts)
			result.SpanCount = spanCount
			result.TransformedCount = spanCount
		}
		if len(logStmts) > 0 && run.LogData != nil {
			transformed, err := ottl.TransformLogRunData(*run.LogData, logStmts)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.TransformResult{
					Valid:  false,
					Errors: []string{"log: " + err.Error()},
				})
				return
			}
			result.LogTransformedData = &transformed
			result.LogStatementsParsed = len(logStmts)
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	if signalType == "all" {
		traceStmts, logStmts, metricStmts, err := ottl.ParseCombinedTransformConfigAll(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
		result := ottl.TransformResult{Valid: true}
		if len(traceStmts) > 0 && run.TraceData != nil {
			transformed, spanCount, err := ottl.TransformRunData(*run.TraceData, traceStmts)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.TransformResult{
					Valid:  false,
					Errors: []string{"trace: " + err.Error()},
				})
				return
			}
			result.TransformedData = &transformed
			result.StatementsParsed = len(traceStmts)
			result.SpanCount = spanCount
			result.TransformedCount = spanCount
		}
		if len(logStmts) > 0 && run.LogData != nil {
			transformed, err := ottl.TransformLogRunData(*run.LogData, logStmts)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.TransformResult{
					Valid:  false,
					Errors: []string{"log: " + err.Error()},
				})
				return
			}
			result.LogTransformedData = &transformed
			result.LogStatementsParsed = len(logStmts)
		}
		if len(metricStmts) > 0 && run.MetricData != nil {
			transformed, err := ottl.TransformMetricRunData(*run.MetricData, metricStmts)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.TransformResult{
					Valid:  false,
					Errors: []string{"metric: " + err.Error()},
				})
				return
			}
			result.MetricTransformedData = &transformed
			result.MetricStatementsParsed = len(metricStmts)
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	if signalType == "logs" {
		if run.LogData == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no log data; only completed runs with log data can be transformed"})
			return
		}
		var statements []string
		if req.Config != "" {
			var err error
			statements, err = ottl.ParseTransformStatements(req.Config)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{err.Error()}})
				return
			}
		} else {
			statements = req.Statements
		}
		transformed, err := ottl.TransformLogRunData(*run.LogData, statements)
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
			StatementsParsed: len(statements),
		})
		return
	}

	if signalType == "metrics" {
		if run.MetricData == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no metric data; only completed runs with metric data can be transformed"})
			return
		}
		var statements []string
		if req.Config != "" {
			var err error
			statements, err = ottl.ParseTransformStatements(req.Config)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{err.Error()}})
				return
			}
		} else {
			statements = req.Statements
		}
		transformed, err := ottl.TransformMetricRunData(*run.MetricData, statements)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.TransformResult{
				Valid:  false,
				Errors: []string{err.Error()},
			})
			return
		}
		writeJSON(w, http.StatusOK, ottl.TransformResult{
			Valid:                 true,
			MetricTransformedData: &transformed,
			MetricStatementsParsed: len(statements),
		})
		return
	}

	// Traces (default)
	if run.TraceData == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no trace data; only completed runs with trace data can be transformed"})
		return
	}
	var statements []string
	if req.Config != "" {
		var err error
		statements, err = ottl.ParseTransformStatements(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
	} else {
		statements = req.Statements
	}
	transformed, spanCount, err := ottl.TransformRunData(*run.TraceData, statements)
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
		StatementsParsed: len(statements),
		SpanCount:        spanCount,
		TransformedCount: spanCount,
	})
}

func (s *Server) handleOTTLFilter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RunID      string         `json:"run_id"`
		Config     string         `json:"config"`
		Mode       ottl.FilterMode `json:"mode"`
		Conditions []string       `json:"conditions"` // backward compat
		SignalType string         `json:"signal_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.RunID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id is required"})
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

	signalType := req.SignalType
	if signalType == "" {
		signalType = "traces"
	}

	if signalType == "both" {
		traceConds, logConds, err := ottl.ParseCombinedFilterConfig(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
		result := ottl.FilterResult{Valid: true}
		if len(traceConds) > 0 && run.TraceData != nil {
			filtered, total, kept, err := ottl.FilterRunData(*run.TraceData, traceConds, req.Mode)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{"trace: " + err.Error()}})
				return
			}
			result.FilteredData = &filtered
			result.ConditionsParsed = len(traceConds)
			result.TotalSpanCount = total
			result.FilteredSpanCount = kept
		}
		if len(logConds) > 0 && run.LogData != nil {
			filtered, total, kept, err := ottl.FilterLogRunData(*run.LogData, logConds, req.Mode)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{"log: " + err.Error()}})
				return
			}
			result.LogFilteredData = &filtered
			result.LogConditionsParsed = len(logConds)
			result.TotalLogCount = total
			result.FilteredLogCount = kept
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	if signalType == "all" {
		traceConds, logConds, metricConds, err := ottl.ParseCombinedFilterConfigAll(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
		result := ottl.FilterResult{Valid: true}
		if len(traceConds) > 0 && run.TraceData != nil {
			filtered, total, kept, err := ottl.FilterRunData(*run.TraceData, traceConds, req.Mode)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{"trace: " + err.Error()}})
				return
			}
			result.FilteredData = &filtered
			result.ConditionsParsed = len(traceConds)
			result.TotalSpanCount = total
			result.FilteredSpanCount = kept
		}
		if len(logConds) > 0 && run.LogData != nil {
			filtered, total, kept, err := ottl.FilterLogRunData(*run.LogData, logConds, req.Mode)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{"log: " + err.Error()}})
				return
			}
			result.LogFilteredData = &filtered
			result.LogConditionsParsed = len(logConds)
			result.TotalLogCount = total
			result.FilteredLogCount = kept
		}
		if len(metricConds) > 0 && run.MetricData != nil {
			metricFiltered, metricTotal, metricKept, err := ottl.FilterMetricRunData(*run.MetricData, metricConds, req.Mode)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{"metric: " + err.Error()}})
				return
			}
			result.MetricFilteredData = &metricFiltered
			result.MetricConditionsParsed = len(metricConds)
			result.TotalMetricCount = metricTotal
			result.FilteredMetricCount = metricKept
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	if signalType == "logs" {
		if run.LogData == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no log data; only completed runs with log data can be filtered"})
			return
		}
		var conditions []string
		if req.Config != "" {
			var err error
			conditions, err = ottl.ParseFilterConditions(req.Config)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{err.Error()}})
				return
			}
		} else {
			conditions = req.Conditions
		}
		filtered, total, kept, err := ottl.FilterLogRunData(*run.LogData, conditions, req.Mode)
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
			ConditionsParsed:  len(conditions),
			TotalSpanCount:    total,
			FilteredSpanCount: kept,
		})
		return
	}

	if signalType == "metrics" {
		if run.MetricData == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no metric data; only completed runs with metric data can be filtered"})
			return
		}
		var conditions []string
		if req.Config != "" {
			var err error
			conditions, err = ottl.ParseFilterConditions(req.Config)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{err.Error()}})
				return
			}
		} else {
			conditions = req.Conditions
		}
		filtered, total, kept, err := ottl.FilterMetricRunData(*run.MetricData, conditions, req.Mode)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.FilterResult{
				Valid:  false,
				Errors: []string{err.Error()},
			})
			return
		}
		writeJSON(w, http.StatusOK, ottl.FilterResult{
			Valid:                true,
			MetricFilteredData:   &filtered,
			MetricConditionsParsed: len(conditions),
			TotalMetricCount:     total,
			FilteredMetricCount:  kept,
		})
		return
	}

	// Traces (default)
	if run.TraceData == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no trace data; only completed runs with trace data can be filtered"})
		return
	}
	var conditions []string
	if req.Config != "" {
		var err error
		conditions, err = ottl.ParseFilterConditions(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
	} else {
		conditions = req.Conditions
	}
	filtered, total, kept, err := ottl.FilterRunData(*run.TraceData, conditions, req.Mode)
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
		ConditionsParsed:  len(conditions),
		TotalSpanCount:    total,
		FilteredSpanCount: kept,
	})
}

func (s *Server) handleOTTLFilterValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Config     string `json:"config"`
		Conditions []string `json:"conditions"` // backward compat
		SignalType string `json:"signal_type"`  // OTTL context (traces/logs/metrics)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	var conditions []string
	if req.Config != "" {
		var err error
		conditions, err = ottl.ParseFilterConditions(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.ValidateResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
	} else {
		conditions = req.Conditions
	}

	signalType := req.SignalType
	if signalType == "" {
		signalType = "traces"
	}

	result := ottl.ValidateFilterConditionsParse(conditions, signalType)
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

func writeHTML(w http.ResponseWriter, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

// --- HTMX HTML endpoint helpers ---

func renderValidateHTML(result ottl.ValidateResult) string {
	if result.Valid {
		return fmt.Sprintf(`<div class="ottl-success">All %d statement(s) valid.</div>`, result.Parsed)
	}
	var sb strings.Builder
	sb.WriteString(`<div class="ottl-error">`)
	for _, e := range result.Errors {
		sb.WriteString(fmt.Sprintf(`<div class="ottl-error-item">%s</div>`, escapeHTMLEntity(e)))
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

func renderTransformHTML(result ottl.TransformResult, signalType string) string {
	if !result.Valid {
		var sb strings.Builder
		sb.WriteString(`<div class="ottl-error">`)
		for _, e := range result.Errors {
			sb.WriteString(fmt.Sprintf(`<div class="ottl-error-item">%s</div>`, escapeHTMLEntity(e)))
		}
		sb.WriteString(`</div>`)
		return sb.String()
	}

	var parts []string

	if signalType == "both" || signalType == "all" {
		if result.StatementsParsed > 0 {
			parts = append(parts, fmt.Sprintf(`<span>✓ %d trace statement(s) parsed, %d spans transformed</span>`, result.StatementsParsed, result.TransformedCount))
		}
		if result.LogStatementsParsed > 0 {
			parts = append(parts, fmt.Sprintf(`<span>✓ %d log statement(s) parsed</span>`, result.LogStatementsParsed))
		}
		if result.MetricStatementsParsed > 0 {
			parts = append(parts, fmt.Sprintf(`<span>✓ %d metric statement(s) parsed</span>`, result.MetricStatementsParsed))
		}
	} else {
		switch signalType {
		case "logs":
			parts = append(parts, fmt.Sprintf(`<span>✓ %d log statement(s) parsed</span>`, result.LogStatementsParsed))
		case "metrics":
			parts = append(parts, fmt.Sprintf(`<span>✓ %d metric statement(s) parsed</span>`, result.MetricStatementsParsed))
		default:
			parts = append(parts, fmt.Sprintf(`<span>✓ %d statement(s) parsed, %d spans transformed</span>`, result.StatementsParsed, result.TransformedCount))
		}
	}

	var sb strings.Builder

	sb.WriteString(`<div data-tab-content="summary">`)
	sb.WriteString(`<div class="ottl-summary">`)
	for i, p := range parts {
		if i > 0 {
			sb.WriteString(`<br>`)
		}
		sb.WriteString(p)
	}
	sb.WriteString(`</div>`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<div data-tab-content="json" style="display:none">`)
	if result.TransformedData != nil {
		sb.WriteString(`<div class="ottl-section"><h4>Transformed Trace Data</h4><pre class="ottl-json">`)
		sb.WriteString(escapeHTMLEntity(prettyJSON(*result.TransformedData)))
		sb.WriteString(`</pre></div>`)
	}
	if result.LogTransformedData != nil {
		sb.WriteString(`<div class="ottl-section"><h4>Transformed Log Data</h4><pre class="ottl-json">`)
		sb.WriteString(escapeHTMLEntity(prettyJSON(*result.LogTransformedData)))
		sb.WriteString(`</pre></div>`)
	}
	if result.MetricTransformedData != nil {
		sb.WriteString(`<div class="ottl-section"><h4>Transformed Metric Data</h4><pre class="ottl-json">`)
		sb.WriteString(escapeHTMLEntity(prettyJSON(*result.MetricTransformedData)))
		sb.WriteString(`</pre></div>`)
	}
	sb.WriteString(`</div>`)

	// Always (re)set __ottlTransformData to clear stale trace data from prior transform results.
	// For composite signal types, embed per-signal data so the client can render diffs for each.
	if signalType == "both" || signalType == "all" {
		var scriptBuf strings.Builder
		scriptBuf.WriteString(`<script>window.__ottlTransformData={_composite:true`)
		if result.OriginalData != nil || result.TransformedData != nil {
			orig := "null"; if result.OriginalData != nil { orig = string(*result.OriginalData) }
			trans := "null"; if result.TransformedData != nil { trans = string(*result.TransformedData) }
			fmt.Fprintf(&scriptBuf, `,traces:{original:%s,transformed:%s}`, orig, trans)
		}
		if result.LogOriginalData != nil || result.LogTransformedData != nil {
			orig := "null"; if result.LogOriginalData != nil { orig = string(*result.LogOriginalData) }
			trans := "null"; if result.LogTransformedData != nil { trans = string(*result.LogTransformedData) }
			fmt.Fprintf(&scriptBuf, `,logs:{original:%s,transformed:%s}`, orig, trans)
		}
		if result.MetricOriginalData != nil || result.MetricTransformedData != nil {
			orig := "null"; if result.MetricOriginalData != nil { orig = string(*result.MetricOriginalData) }
			trans := "null"; if result.MetricTransformedData != nil { trans = string(*result.MetricTransformedData) }
			fmt.Fprintf(&scriptBuf, `,metrics:{original:%s,transformed:%s}`, orig, trans)
		}
		scriptBuf.WriteString(`}</script>`)
		sb.WriteString(scriptBuf.String())
	} else {
		origJSON := "null"
		transJSON := "null"
		if result.OriginalData != nil {
			origJSON = string(*result.OriginalData)
		}
		if result.TransformedData != nil {
			transJSON = string(*result.TransformedData)
		} else if result.LogTransformedData != nil {
			transJSON = string(*result.LogTransformedData)
		} else if result.MetricTransformedData != nil {
			transJSON = string(*result.MetricTransformedData)
		}
		sb.WriteString(fmt.Sprintf(`<script>window.__ottlTransformData={original:%s,transformed:%s}</script>`, origJSON, transJSON))
	}

	return sb.String()
}

func renderFilterHTML(result ottl.FilterResult, mode string, signalType string) string {
	if !result.Valid {
		var sb strings.Builder
		sb.WriteString(`<div class="ottl-error">`)
		for _, e := range result.Errors {
			sb.WriteString(fmt.Sprintf(`<div class="ottl-error-item">%s</div>`, escapeHTMLEntity(e)))
		}
		sb.WriteString(`</div>`)
		return sb.String()
	}

	var sb strings.Builder

	// Summary tab
	sb.WriteString(`<div data-tab-content="summary">`)

	if signalType == "both" || signalType == "all" {
		if result.ConditionsParsed > 0 {
			sb.WriteString(fmt.Sprintf(`<div class="ottl-summary">✓ %d condition(s) applied, kept %d of %d spans (%s mode)</div>`,
				result.ConditionsParsed, result.FilteredSpanCount, result.TotalSpanCount, mode))
		}
		if result.LogConditionsParsed > 0 {
			sb.WriteString(fmt.Sprintf(`<div class="ottl-summary" style="margin-top:4px">✓ %d log condition(s) applied, kept %d of %d logs (%s mode)</div>`,
				result.LogConditionsParsed, result.FilteredLogCount, result.TotalLogCount, mode))
		}
		if result.MetricConditionsParsed > 0 {
			sb.WriteString(fmt.Sprintf(`<div class="ottl-summary" style="margin-top:4px">✓ %d metric condition(s) applied, kept %d of %d metrics (%s mode)</div>`,
				result.MetricConditionsParsed, result.FilteredMetricCount, result.TotalMetricCount, mode))
		}
	} else {
		switch signalType {
		case "logs":
			sb.WriteString(fmt.Sprintf(`<div class="ottl-summary">✓ %d condition(s) applied, kept %d of %d logs (%s mode)</div>`,
				result.LogConditionsParsed, result.FilteredLogCount, result.TotalLogCount, mode))
		case "metrics":
			sb.WriteString(fmt.Sprintf(`<div class="ottl-summary">✓ %d condition(s) applied, kept %d of %d metrics (%s mode)</div>`,
				result.MetricConditionsParsed, result.FilteredMetricCount, result.TotalMetricCount, mode))
		default:
			sb.WriteString(fmt.Sprintf(`<div class="ottl-summary">✓ %d condition(s) applied, kept %d of %d spans (%s mode)</div>`,
				result.ConditionsParsed, result.FilteredSpanCount, result.TotalSpanCount, mode))
		}
	}

	// Filtered data preview in summary
	if result.FilteredData != nil {
		sb.WriteString(`<div class="ottl-section"><h4>Filtered Trace Data</h4><pre class="ottl-json">`)
		sb.WriteString(escapeHTMLEntity(prettyJSON(*result.FilteredData)))
		sb.WriteString(`</pre></div>`)
	}
	if result.LogFilteredData != nil {
		sb.WriteString(`<div class="ottl-section"><h4>Filtered Log Data</h4><pre class="ottl-json">`)
		sb.WriteString(escapeHTMLEntity(prettyJSON(*result.LogFilteredData)))
		sb.WriteString(`</pre></div>`)
	}
	if result.MetricFilteredData != nil {
		sb.WriteString(`<div class="ottl-section"><h4>Filtered Metric Data</h4><pre class="ottl-json">`)
		sb.WriteString(escapeHTMLEntity(prettyJSON(*result.MetricFilteredData)))
		sb.WriteString(`</pre></div>`)
	}

	sb.WriteString(`</div>`) // end summary tab

	// Raw JSON tab
	sb.WriteString(`<div data-tab-content="json" style="display:none">`)
	if result.FilteredData != nil {
		sb.WriteString(`<div class="ottl-section"><h4>Filtered Trace Data</h4><pre class="ottl-json">`)
		sb.WriteString(escapeHTMLEntity(prettyJSON(*result.FilteredData)))
		sb.WriteString(`</pre></div>`)
	}
	if result.LogFilteredData != nil {
		sb.WriteString(`<div class="ottl-section"><h4>Filtered Log Data</h4><pre class="ottl-json">`)
		sb.WriteString(escapeHTMLEntity(prettyJSON(*result.LogFilteredData)))
		sb.WriteString(`</pre></div>`)
	}
	if result.MetricFilteredData != nil {
		sb.WriteString(`<div class="ottl-section"><h4>Filtered Metric Data</h4><pre class="ottl-json">`)
		sb.WriteString(escapeHTMLEntity(prettyJSON(*result.MetricFilteredData)))
		sb.WriteString(`</pre></div>`)
	}
	sb.WriteString(`</div>`) // end json tab

	// Always (re)set __ottlTransformData to clear stale trace data from prior filter results.
	// For composite signal types, embed per-signal data so the client can render diffs for each.
	if signalType == "both" || signalType == "all" {
		var scriptBuf strings.Builder
		scriptBuf.WriteString(`<script>window.__ottlTransformData={_composite:true`)
		if result.OriginalData != nil || result.FilteredData != nil {
			orig := "null"; if result.OriginalData != nil { orig = string(*result.OriginalData) }
			trans := "null"; if result.FilteredData != nil { trans = string(*result.FilteredData) }
			fmt.Fprintf(&scriptBuf, `,traces:{original:%s,transformed:%s}`, orig, trans)
		}
		if result.LogOriginalData != nil || result.LogFilteredData != nil {
			orig := "null"; if result.LogOriginalData != nil { orig = string(*result.LogOriginalData) }
			trans := "null"; if result.LogFilteredData != nil { trans = string(*result.LogFilteredData) }
			fmt.Fprintf(&scriptBuf, `,logs:{original:%s,transformed:%s}`, orig, trans)
		}
		if result.MetricOriginalData != nil || result.MetricFilteredData != nil {
			orig := "null"; if result.MetricOriginalData != nil { orig = string(*result.MetricOriginalData) }
			trans := "null"; if result.MetricFilteredData != nil { trans = string(*result.MetricFilteredData) }
			fmt.Fprintf(&scriptBuf, `,metrics:{original:%s,transformed:%s}`, orig, trans)
		}
		scriptBuf.WriteString(`}</script>`)
		sb.WriteString(scriptBuf.String())
	} else {
		origJSON := "null"
		filteredJSON := "null"
		if result.OriginalData != nil {
			origJSON = string(*result.OriginalData)
		}
		if result.FilteredData != nil {
			filteredJSON = string(*result.FilteredData)
		} else if result.LogFilteredData != nil {
			filteredJSON = string(*result.LogFilteredData)
		} else if result.MetricFilteredData != nil {
			filteredJSON = string(*result.MetricFilteredData)
		}
		sb.WriteString(fmt.Sprintf(`<script>window.__ottlTransformData={original:%s,transformed:%s}</script>`, origJSON, filteredJSON))
	}

	return sb.String()
}

func escapeHTMLEntity(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

func prettyJSON(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(b)
}

func (s *Server) handleOTTLValidateHTML(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeHTML(w, `<div class="ottl-error"><div class="ottl-error-item">invalid form data</div></div>`)
		return
	}
	config := r.PostFormValue("config")
	mode := r.PostFormValue("mode")

	if mode == "filter" {
		conditions, err := ottl.ParseFilterConditions(config)
		if err != nil {
			writeHTML(w, renderValidateHTML(ottl.ValidateResult{Valid: false, Errors: []string{err.Error()}}))
			return
		}
		signalType := r.PostFormValue("signal_type")
		if signalType == "" {
			signalType = "traces"
		}
		result := ottl.ValidateFilterConditionsParse(conditions, signalType)
		writeHTML(w, renderValidateHTML(result))
		return
	}

	// Transform mode (default)
	statements, err := ottl.ParseTransformStatements(config)
	if err != nil {
		writeHTML(w, renderValidateHTML(ottl.ValidateResult{Valid: false, Errors: []string{err.Error()}}))
		return
	}
	result := ottl.ValidateStatements(statements)
	writeHTML(w, renderValidateHTML(result))
}

func (s *Server) handleOTTLTransformHTML(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeHTML(w, `<div class="ottl-error"><div class="ottl-error-item">invalid form data</div></div>`)
		return
	}
	runID := r.PostFormValue("run_id")
	config := r.PostFormValue("config")
	signalType := r.PostFormValue("signal_type")
	if runID == "" {
		writeHTML(w, `<div class="ottl-error"><div class="ottl-error-item">run_id is required</div></div>`)
		return
	}

	run := s.store.Get(runID)
	if run == nil {
		writeHTML(w, `<div class="ottl-error"><div class="ottl-error-item">run not found</div></div>`)
		return
	}

	if signalType == "" {
		signalType = "traces"
	}

	var result ottl.TransformResult

	if signalType == "both" {
		traceStmts, logStmts, err := ottl.ParseCombinedTransformConfig(config)
		if err != nil {
			writeHTML(w, renderTransformHTML(ottl.TransformResult{Valid: false, Errors: []string{err.Error()}}, signalType))
			return
		}
		result = ottl.TransformResult{Valid: true}
		if len(traceStmts) > 0 && run.TraceData != nil {
			origCopy := make(json.RawMessage, len(*run.TraceData))
			copy(origCopy, *run.TraceData)
			transformed, spanCount, err := ottl.TransformRunData(*run.TraceData, traceStmts)
			if err != nil {
				writeHTML(w, renderTransformHTML(ottl.TransformResult{Valid: false, Errors: []string{"trace: " + err.Error()}}, signalType))
				return
			}
			result.OriginalData = &origCopy
			result.TransformedData = &transformed
			result.StatementsParsed = len(traceStmts)
			result.SpanCount = spanCount
			result.TransformedCount = spanCount
		}
		if len(logStmts) > 0 && run.LogData != nil {
			transformed, err := ottl.TransformLogRunData(*run.LogData, logStmts)
			if err != nil {
				writeHTML(w, renderTransformHTML(ottl.TransformResult{Valid: false, Errors: []string{"log: " + err.Error()}}, signalType))
				return
			}
			logOrigCopy := make(json.RawMessage, len(*run.LogData))
			copy(logOrigCopy, *run.LogData)
			result.LogOriginalData = &logOrigCopy
			if result.OriginalData == nil {
				result.OriginalData = &logOrigCopy
			}
			result.LogTransformedData = &transformed
			result.LogStatementsParsed = len(logStmts)
		}
		writeHTML(w, renderTransformHTML(result, signalType))
		return
	}

	if signalType == "all" {
		traceStmts, logStmts, metricStmts, err := ottl.ParseCombinedTransformConfigAll(config)
		if err != nil {
			writeHTML(w, renderTransformHTML(ottl.TransformResult{Valid: false, Errors: []string{err.Error()}}, signalType))
			return
		}
		result = ottl.TransformResult{Valid: true}
		if len(traceStmts) > 0 && run.TraceData != nil {
			origCopy := make(json.RawMessage, len(*run.TraceData))
			copy(origCopy, *run.TraceData)
			transformed, spanCount, err := ottl.TransformRunData(*run.TraceData, traceStmts)
			if err != nil {
				writeHTML(w, renderTransformHTML(ottl.TransformResult{Valid: false, Errors: []string{"trace: " + err.Error()}}, signalType))
				return
			}
			result.OriginalData = &origCopy
			result.TransformedData = &transformed
			result.StatementsParsed = len(traceStmts)
			result.SpanCount = spanCount
			result.TransformedCount = spanCount
		}
		if len(logStmts) > 0 && run.LogData != nil {
			transformed, err := ottl.TransformLogRunData(*run.LogData, logStmts)
			if err != nil {
				writeHTML(w, renderTransformHTML(ottl.TransformResult{Valid: false, Errors: []string{"log: " + err.Error()}}, signalType))
				return
			}
			logOrigCopy := make(json.RawMessage, len(*run.LogData))
			copy(logOrigCopy, *run.LogData)
			result.LogOriginalData = &logOrigCopy
			if result.OriginalData == nil {
				result.OriginalData = &logOrigCopy
			}
			result.LogTransformedData = &transformed
			result.LogStatementsParsed = len(logStmts)
		}
		if len(metricStmts) > 0 && run.MetricData != nil {
			transformed, err := ottl.TransformMetricRunData(*run.MetricData, metricStmts)
			if err != nil {
				writeHTML(w, renderTransformHTML(ottl.TransformResult{Valid: false, Errors: []string{"metric: " + err.Error()}}, signalType))
				return
			}
			metricOrigCopy := make(json.RawMessage, len(*run.MetricData))
			copy(metricOrigCopy, *run.MetricData)
			result.MetricOriginalData = &metricOrigCopy
			if result.OriginalData == nil {
				result.OriginalData = &metricOrigCopy
			}
			result.MetricTransformedData = &transformed
			result.MetricStatementsParsed = len(metricStmts)
		}
		writeHTML(w, renderTransformHTML(result, signalType))
		return
	}

	if signalType == "logs" {
		if run.LogData == nil {
			writeHTML(w, `<div class="ottl-error"><div class="ottl-error-item">run has no log data</div></div>`)
			return
		}
		var statements []string
		var err error
		statements, err = ottl.ParseTransformStatements(config)
		if err != nil {
			writeHTML(w, renderTransformHTML(ottl.TransformResult{Valid: false, Errors: []string{err.Error()}}, signalType))
			return
		}
		transformed, err := ottl.TransformLogRunData(*run.LogData, statements)
		if err != nil {
			writeHTML(w, renderTransformHTML(ottl.TransformResult{Valid: false, Errors: []string{err.Error()}}, signalType))
			return
		}
		origCopy := make(json.RawMessage, len(*run.LogData))
		copy(origCopy, *run.LogData)
		result = ottl.TransformResult{Valid: true, OriginalData: &origCopy, LogTransformedData: &transformed, LogStatementsParsed: len(statements)}
		writeHTML(w, renderTransformHTML(result, signalType))
		return
	}

	if signalType == "metrics" {
		if run.MetricData == nil {
			writeHTML(w, `<div class="ottl-error"><div class="ottl-error-item">run has no metric data</div></div>`)
			return
		}
		var statements []string
		var err error
		statements, err = ottl.ParseTransformStatements(config)
		if err != nil {
			writeHTML(w, renderTransformHTML(ottl.TransformResult{Valid: false, Errors: []string{err.Error()}}, signalType))
			return
		}
		transformed, err := ottl.TransformMetricRunData(*run.MetricData, statements)
		if err != nil {
			writeHTML(w, renderTransformHTML(ottl.TransformResult{Valid: false, Errors: []string{err.Error()}}, signalType))
			return
		}
		origCopy := make(json.RawMessage, len(*run.MetricData))
		copy(origCopy, *run.MetricData)
		result = ottl.TransformResult{Valid: true, OriginalData: &origCopy, MetricTransformedData: &transformed, MetricStatementsParsed: len(statements)}
		writeHTML(w, renderTransformHTML(result, signalType))
		return
	}

	// Traces (default)
	if run.TraceData == nil {
		writeHTML(w, `<div class="ottl-error"><div class="ottl-error-item">run has no trace data</div></div>`)
		return
	}
	var statements []string
	var err error
	statements, err = ottl.ParseTransformStatements(config)
	if err != nil {
		writeHTML(w, renderTransformHTML(ottl.TransformResult{Valid: false, Errors: []string{err.Error()}}, signalType))
		return
	}
	origCopy := make(json.RawMessage, len(*run.TraceData))
	copy(origCopy, *run.TraceData)
	transformed, spanCount, err := ottl.TransformRunData(*run.TraceData, statements)
	if err != nil {
		writeHTML(w, renderTransformHTML(ottl.TransformResult{Valid: false, Errors: []string{err.Error()}}, signalType))
		return
	}
	result = ottl.TransformResult{Valid: true, OriginalData: &origCopy, TransformedData: &transformed, StatementsParsed: len(statements), SpanCount: spanCount, TransformedCount: spanCount}
	writeHTML(w, renderTransformHTML(result, signalType))
}

func (s *Server) handleOTTLFilterHTML(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeHTML(w, `<div class="ottl-error"><div class="ottl-error-item">invalid form data</div></div>`)
		return
	}
	runID := r.PostFormValue("run_id")
	config := r.PostFormValue("config")
	mode := ottl.FilterMode(r.PostFormValue("mode"))
	signalType := r.PostFormValue("signal_type")
	if runID == "" {
		writeHTML(w, `<div class="ottl-error"><div class="ottl-error-item">run_id is required</div></div>`)
		return
	}
	if mode == "" {
		mode = ottl.FilterModeInclude
	}

	run := s.store.Get(runID)
	if run == nil {
		writeHTML(w, `<div class="ottl-error"><div class="ottl-error-item">run not found</div></div>`)
		return
	}

	if signalType == "" {
		signalType = "traces"
	}

	if signalType == "both" {
		traceConds, logConds, err := ottl.ParseCombinedFilterConfig(config)
		if err != nil {
			writeHTML(w, renderFilterHTML(ottl.FilterResult{Valid: false, Errors: []string{err.Error()}}, string(mode), signalType))
			return
		}
		result := ottl.FilterResult{Valid: true}
		if len(traceConds) > 0 && run.TraceData != nil {
			origCopy := make(json.RawMessage, len(*run.TraceData))
			copy(origCopy, *run.TraceData)
			filtered, total, kept, err := ottl.FilterRunData(*run.TraceData, traceConds, mode)
			if err != nil {
				writeHTML(w, renderFilterHTML(ottl.FilterResult{Valid: false, Errors: []string{"trace: " + err.Error()}}, string(mode), signalType))
				return
			}
			result.OriginalData = &origCopy
			result.FilteredData = &filtered
			result.ConditionsParsed = len(traceConds)
			result.TotalSpanCount = total
			result.FilteredSpanCount = kept
		}
		if len(logConds) > 0 && run.LogData != nil {
			filtered, total, kept, err := ottl.FilterLogRunData(*run.LogData, logConds, mode)
			if err != nil {
				writeHTML(w, renderFilterHTML(ottl.FilterResult{Valid: false, Errors: []string{"log: " + err.Error()}}, string(mode), signalType))
				return
			}
			logOrigCopy := make(json.RawMessage, len(*run.LogData))
			copy(logOrigCopy, *run.LogData)
			result.LogOriginalData = &logOrigCopy
			if result.OriginalData == nil {
				result.OriginalData = &logOrigCopy
			}
			result.LogFilteredData = &filtered
			result.LogConditionsParsed = len(logConds)
			result.TotalLogCount = total
			result.FilteredLogCount = kept
		}
		writeHTML(w, renderFilterHTML(result, string(mode), signalType))
		return
	}

	if signalType == "all" {
		traceConds, logConds, metricConds, err := ottl.ParseCombinedFilterConfigAll(config)
		if err != nil {
			writeHTML(w, renderFilterHTML(ottl.FilterResult{Valid: false, Errors: []string{err.Error()}}, string(mode), signalType))
			return
		}
		result := ottl.FilterResult{Valid: true}
		if len(traceConds) > 0 && run.TraceData != nil {
			origCopy := make(json.RawMessage, len(*run.TraceData))
			copy(origCopy, *run.TraceData)
			filtered, total, kept, err := ottl.FilterRunData(*run.TraceData, traceConds, mode)
			if err != nil {
				writeHTML(w, renderFilterHTML(ottl.FilterResult{Valid: false, Errors: []string{"trace: " + err.Error()}}, string(mode), signalType))
				return
			}
			result.OriginalData = &origCopy
			result.FilteredData = &filtered
			result.ConditionsParsed = len(traceConds)
			result.TotalSpanCount = total
			result.FilteredSpanCount = kept
		}
		if len(logConds) > 0 && run.LogData != nil {
			filtered, total, kept, err := ottl.FilterLogRunData(*run.LogData, logConds, mode)
			if err != nil {
				writeHTML(w, renderFilterHTML(ottl.FilterResult{Valid: false, Errors: []string{"log: " + err.Error()}}, string(mode), signalType))
				return
			}
			logOrigCopy := make(json.RawMessage, len(*run.LogData))
			copy(logOrigCopy, *run.LogData)
			result.LogOriginalData = &logOrigCopy
			if result.OriginalData == nil {
				result.OriginalData = &logOrigCopy
			}
			result.LogFilteredData = &filtered
			result.LogConditionsParsed = len(logConds)
			result.TotalLogCount = total
			result.FilteredLogCount = kept
		}
		if len(metricConds) > 0 && run.MetricData != nil {
			filtered, total, kept, err := ottl.FilterMetricRunData(*run.MetricData, metricConds, mode)
			if err != nil {
				writeHTML(w, renderFilterHTML(ottl.FilterResult{Valid: false, Errors: []string{"metric: " + err.Error()}}, string(mode), signalType))
				return
			}
			metricOrigCopy := make(json.RawMessage, len(*run.MetricData))
			copy(metricOrigCopy, *run.MetricData)
			result.MetricOriginalData = &metricOrigCopy
			if result.OriginalData == nil {
				result.OriginalData = &metricOrigCopy
			}
			result.MetricFilteredData = &filtered
			result.MetricConditionsParsed = len(metricConds)
			result.TotalMetricCount = total
			result.FilteredMetricCount = kept
		}
		writeHTML(w, renderFilterHTML(result, string(mode), signalType))
		return
	}

	if signalType == "logs" {
		if run.LogData == nil {
			writeHTML(w, `<div class="ottl-error"><div class="ottl-error-item">run has no log data</div></div>`)
			return
		}
		conds, err := ottl.ParseFilterConditions(config)
		if err != nil {
			writeHTML(w, renderFilterHTML(ottl.FilterResult{Valid: false, Errors: []string{err.Error()}}, string(mode), signalType))
			return
		}
		filtered, total, kept, err := ottl.FilterLogRunData(*run.LogData, conds, mode)
		if err != nil {
			writeHTML(w, renderFilterHTML(ottl.FilterResult{Valid: false, Errors: []string{err.Error()}}, string(mode), signalType))
			return
		}
		origCopy := make(json.RawMessage, len(*run.LogData))
		copy(origCopy, *run.LogData)
		writeHTML(w, renderFilterHTML(ottl.FilterResult{
			Valid: true, LogFilteredData: &filtered,
			OriginalData: &origCopy,
			LogConditionsParsed: len(conds),
			TotalLogCount: total, FilteredLogCount: kept,
		}, string(mode), signalType))
		return
	}

	if signalType == "metrics" {
		if run.MetricData == nil {
			writeHTML(w, `<div class="ottl-error"><div class="ottl-error-item">run has no metric data</div></div>`)
			return
		}
		conds, err := ottl.ParseFilterConditions(config)
		if err != nil {
			writeHTML(w, renderFilterHTML(ottl.FilterResult{Valid: false, Errors: []string{err.Error()}}, string(mode), signalType))
			return
		}
		filtered, total, kept, err := ottl.FilterMetricRunData(*run.MetricData, conds, mode)
		if err != nil {
			writeHTML(w, renderFilterHTML(ottl.FilterResult{Valid: false, Errors: []string{err.Error()}}, string(mode), signalType))
			return
		}
		origCopy := make(json.RawMessage, len(*run.MetricData))
		copy(origCopy, *run.MetricData)
		writeHTML(w, renderFilterHTML(ottl.FilterResult{
			Valid: true, MetricFilteredData: &filtered,
			OriginalData: &origCopy,
			MetricConditionsParsed: len(conds),
			TotalMetricCount: total, FilteredMetricCount: kept,
		}, string(mode), signalType))
		return
	}

	// Traces (default)
	if run.TraceData == nil {
		writeHTML(w, `<div class="ottl-error"><div class="ottl-error-item">run has no trace data</div></div>`)
		return
	}
	conds, err := ottl.ParseFilterConditions(config)
	if err != nil {
		writeHTML(w, renderFilterHTML(ottl.FilterResult{Valid: false, Errors: []string{err.Error()}}, string(mode), signalType))
		return
	}
	origCopy := make(json.RawMessage, len(*run.TraceData))
	copy(origCopy, *run.TraceData)
	filtered, total, kept, err := ottl.FilterRunData(*run.TraceData, conds, mode)
	if err != nil {
		writeHTML(w, renderFilterHTML(ottl.FilterResult{Valid: false, Errors: []string{err.Error()}}, string(mode), signalType))
		return
	}
	writeHTML(w, renderFilterHTML(ottl.FilterResult{
		Valid: true, OriginalData: &origCopy, FilteredData: &filtered,
		ConditionsParsed: len(conds),
		TotalSpanCount: total, FilteredSpanCount: kept,
	}, string(mode), signalType))
}
