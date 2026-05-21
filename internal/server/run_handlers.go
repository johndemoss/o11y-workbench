package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/logging"
	"github.com/johndemoss/o11y-workbench/internal/region"
)

func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Seed              int64            `json:"seed"`
		TraceCount        int              `json:"trace_count"`
		Rate              int              `json:"rate"`
		Duration          int              `json:"duration"`
		Topology          string           `json:"topology"`
		Scenario          string           `json:"scenario"`
		ScenarioSpanCount int              `json:"scenario_span_count"`
		SpanCount         int              `json:"span_count"`
		MaxDepth          int              `json:"max_depth"`
		ServiceName       string           `json:"service_name"`
		Output            string           `json:"output"`
		Format            string           `json:"format"`
		Endpoints         []string         `json:"endpoints"`
		Insecure          *bool            `json:"insecure"`
		SignalType        string           `json:"signal_type"` // DEPRECATED: use Signals
		Signals           *gen.Signals     `json:"signals"`
		LogConfig         gen.LogConfig    `json:"log_config"`
		MetricConfig      gen.MetricConfig `json:"metric_config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	logging.Debugf("StartRun: Received request body: signals=%+v, log_config=%+v", body.Signals, body.LogConfig)

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

	// Normalize signal selection: prefer new Signals format, fall back to SignalType
	signals := body.Signals
	if signals == nil {
		// Backwards compat: derive from SignalType string
		signalType := body.SignalType
		if signalType == "" {
			signalType = "traces"
		}
		s := gen.NewSignalsFromType(signalType)
		signals = &s
	}

	var topoType gen.TopologyType
	if body.Scenario == "" {
		if signals.Traces {
			var err error
			topoType, err = gen.ParseTopology(body.Topology)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		}
	}

	cfg := gen.Config{
		Seed: body.Seed, TraceCount: body.TraceCount, Rate: body.Rate,
		Duration:  time.Duration(body.Duration) * time.Second,
		Topology: topoType, SpanCount: body.SpanCount, MaxDepth: body.MaxDepth,
		ServiceName: body.ServiceName,
		BaseTime:    time.Now(),
		TraceDuration: 5 * time.Second,
		Scenario: body.Scenario, ScenarioSpanCount: body.ScenarioSpanCount,
		Signals:      *signals,
		SignalType:   gen.DeriveSignalType(*signals), // for backwards compat in responses
		LogConfig:    body.LogConfig,
		MetricConfig: body.MetricConfig,
	}

	// Validate log config.
	if cfg.IncludesLogs() {
		cfg.LogConfig.Enabled = true
		if cfg.LogConfig.LogCount == 0 {
			cfg.LogConfig.LogCount = gen.DefaultLogConfig().LogCount
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
	if cfg.IncludesMetrics() {
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
