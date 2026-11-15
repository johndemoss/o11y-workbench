package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/export"
	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/logging"
	"github.com/johndemoss/o11y-workbench/internal/region"
	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
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
		SignalType        string           `json:"signal_type"` // Legacy input alias, translated to Signals below.
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
	if body.Format == "" {
		body.Format = "json"
	}
	if body.Format != "json" && body.Format != "protobuf" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "format must be json or protobuf"})
		return
	}
	if body.Duration < 0 || int64(body.Duration) > int64((1<<63-1)/int64(time.Second)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "duration is out of range"})
		return
	}

	insecure := true
	if body.Insecure != nil {
		insecure = *body.Insecure
	}

	endpoints := body.Endpoints
	routes := routesForEndpoints(endpoints)
	regions := make([]region.Name, len(routes))
	for i, route := range routes {
		regions[i] = route.Name
	}

	// Signal selection comes from Signals; the legacy signal_type string is
	// accepted as input only, translated here and never stored in the config.
	signals := body.Signals
	if signals == nil {
		s := signalsFromLegacyType(body.SignalType)
		signals = &s
	}
	if (body.Workload != nil || gen.IsWorkloadScenario(body.Scenario)) && (signals.Logs || signals.Metrics) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "workload profiles support traces only"})
		return
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
		Regions:      regions,
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
	if err := gen.ValidateConfig(cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
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
	s.store.mu.RLock()
	if live := s.store.runs[run.ID]; live != nil {
		live.mu.Lock()
		live.Format = body.Format
		live.mu.Unlock()
	}
	s.store.mu.RUnlock()
	run.Format = body.Format
	writeJSON(w, http.StatusAccepted, run)
}

// signalsFromLegacyType translates the pre-Signals signal_type vocabulary into
// a current signal selection. Empty or unrecognized values select traces.
func signalsFromLegacyType(signalType string) gen.Signals {
	switch strings.ToLower(strings.TrimSpace(signalType)) {
	case "logs":
		return gen.Signals{Logs: true}
	case "metrics":
		return gen.Signals{Metrics: true}
	case "both":
		return gen.Signals{Traces: true, Logs: true}
	case "traces+metrics":
		return gen.Signals{Traces: true, Metrics: true}
	case "logs+metrics":
		return gen.Signals{Logs: true, Metrics: true}
	case "all":
		return gen.Signals{Traces: true, Logs: true, Metrics: true}
	default: // "traces", empty, and unknown legacy values
		return gen.Signals{Traces: true}
	}
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
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping", "id": id})
}

func (s *Server) handlePauseRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.store.PauseRun(r.Context(), id)
	switch {
	case errors.Is(err, ErrRunNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found: " + id})
	case errors.Is(err, context.DeadlineExceeded):
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "pause timed out before in-flight exports finished"})
	case err != nil:
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "paused", "id": id})
	}
}

func (s *Server) handleResumeRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.store.ResumeRun(id)
	switch {
	case errors.Is(err, ErrRunNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found: " + id})
	case err != nil:
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "resumed", "id": id})
	}
}

func (s *Server) handleDownloadRun(w http.ResponseWriter, r *http.Request) {
	run := s.store.Get(r.PathValue("id"))
	if run == nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	signal, format := r.URL.Query().Get("signal"), r.URL.Query().Get("format")
	if signal == "" {
		signal = "traces"
	}
	if format == "" {
		format = "json"
	}
	var data *json.RawMessage
	var msg proto.Message
	switch signal {
	case "traces":
		data, msg = run.TraceData, &collectortracev1.ExportTraceServiceRequest{}
	case "logs":
		data, msg = run.LogData, &collectorlogsv1.ExportLogsServiceRequest{}
	case "metrics":
		data, msg = run.MetricData, &collectormetricsv1.ExportMetricsServiceRequest{}
	default:
		http.Error(w, "invalid signal", http.StatusBadRequest)
		return
	}
	if format != "json" && format != "protobuf" {
		http.Error(w, "invalid format", http.StatusBadRequest)
		return
	}
	if data == nil {
		http.Error(w, "signal capture not available", http.StatusNotFound)
		return
	}
	payload := []byte(*data)
	contentType, ext := "application/json", "json"
	if format == "protobuf" {
		normalized, err := export.FixTraceSpanIDsForUnmarshal(payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := protojson.Unmarshal(normalized, msg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		payload, err = proto.Marshal(msg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		contentType, ext = "application/x-protobuf", "pb"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", run.ID+"-"+signal+"."+ext))
	_, _ = w.Write(payload)
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
