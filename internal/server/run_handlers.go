package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/logging"
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
		Workload          *workloadRequest `json:"workload"`
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

	wl := workloadFromRequest(body.Workload)
	if gen.IsWorkloadScenario(body.Scenario) && wl.Shape == "" {
		wl.Shape = "flat"
	}

	var topoType gen.TopologyType
	if body.Scenario == "" && !strings.EqualFold(wl.Shape, "flat") {
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
		Duration: time.Duration(body.Duration) * time.Second,
		Topology: topoType, SpanCount: body.SpanCount, MaxDepth: body.MaxDepth,
		ServiceName:   body.ServiceName,
		BaseTime:      time.Now(),
		TraceDuration: 5 * time.Second,
		Scenario:      body.Scenario, ScenarioSpanCount: body.ScenarioSpanCount,
		Signals:      *signals,
		SignalType:   gen.DeriveSignalType(*signals), // for backwards compat in responses
		LogConfig:    body.LogConfig,
		MetricConfig: body.MetricConfig,
		Workload:     wl,
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
		status := http.StatusInternalServerError
		if errors.Is(err, ErrRunStoreClosed) {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
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

// workloadRequest is the dashboard/API shape for WorkloadProfile overlays.
// Burst on/off are seconds (not Go Duration nanoseconds).
type workloadRequest struct {
	ExperimentID             string  `json:"experiment_id"`
	ItemsPerRequest          int     `json:"items_per_request"`
	AttributeCount           int     `json:"attribute_count"`
	AttributeValueBytes      int     `json:"attribute_value_bytes"`
	Cardinality              int     `json:"cardinality"`
	Mode                     string  `json:"mode"`
	RateItemsPerSec          float64 `json:"rate_items_per_sec"`
	RequestsPerSec           float64 `json:"requests_per_sec"`
	ContinueOnExportErr      bool    `json:"continue_on_export_err"`
	SingleResource           bool    `json:"single_resource"`
	Shape                    string  `json:"shape"`
	BurstOnS                 float64 `json:"burst_on_s"`
	BurstOffS                float64 `json:"burst_off_s"`
	BurstPeakRateItemsPerSec float64 `json:"burst_peak_rate_items_per_sec"`
}

func workloadFromRequest(req *workloadRequest) gen.WorkloadProfile {
	if req == nil {
		return gen.WorkloadProfile{}
	}
	wl := gen.WorkloadProfile{
		ExperimentID:        strings.TrimSpace(req.ExperimentID),
		ItemsPerRequest:     req.ItemsPerRequest,
		AttributeCount:      req.AttributeCount,
		AttributeValueBytes: req.AttributeValueBytes,
		Cardinality:         req.Cardinality,
		Mode:                req.Mode,
		RateItemsPerSec:     req.RateItemsPerSec,
		RequestsPerSec:      req.RequestsPerSec,
		ContinueOnExportErr: req.ContinueOnExportErr,
		SingleResource:      req.SingleResource,
		Shape:               strings.TrimSpace(req.Shape),
	}
	if strings.EqualFold(wl.Mode, "burst") || req.BurstOnS > 0 || req.BurstOffS > 0 || req.BurstPeakRateItemsPerSec > 0 {
		wl.Burst = gen.BurstConfig{
			On:                  time.Duration(req.BurstOnS * float64(time.Second)),
			Off:                 time.Duration(req.BurstOffS * float64(time.Second)),
			PeakRateItemsPerSec: req.BurstPeakRateItemsPerSec,
		}
	}
	return wl
}
