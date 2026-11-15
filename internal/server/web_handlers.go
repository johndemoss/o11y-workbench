package server

import (
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/region"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	reg := s.ensureEngines()
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"ottl": map[string]any{
			"default":  reg.Default(),
			"versions": reg.List(),
		},
	})
}

func (s *Server) handleCollectorVersions(w http.ResponseWriter, r *http.Request) {
	reg := s.ensureEngines()
	writeJSON(w, http.StatusOK, map[string]any{
		"default":  reg.Default(),
		"versions": reg.List(),
	})
}

func (s *Server) handleScenarios(w http.ResponseWriter, r *http.Request) {
	builtIn := gen.BuiltInScenarioNames()
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

	if err := gen.RegisterUserScenarios(scenarios); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	registered := make([]string, 0, len(scenarios))
	for name := range scenarios {
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
		"rate": 0, "duration": 10, "topology": def.Topology.String(),
		"span_count": def.SpanCount, "max_depth": def.MaxDepth,
		"service_name": def.ServiceName, "scenario_span_count": 0,
		"output": "file", "format": "json",
		"endpoints": endpoints, "insecure": true,
		"signal_type": "traces",
		"log_config": map[string]any{
			"enabled":       logDef.Enabled,
			"logs_per_span": logDef.LogsPerSpan,
			"log_count":     logDef.LogCount,
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
			"enabled":      false,
			"metric_count": 20,
			"gauge_ratio":  0.4,
			"sum_ratio":    0.4,
			"value_min":    0,
			"value_max":    100,
			"attributes":   []gen.MetricAttribute{},
		},
	})
}
