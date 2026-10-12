package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/johndemoss/o11y-workbench/workers/ottl-0.162/engine"
)

const (
	contribVersion = "0.162.0"
	stableVersion  = "1.68.0"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:0", "listen address")
	flag.Parse()

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", handleHealth)
	mux.HandleFunc("POST /v1/validate", handleValidate)
	mux.HandleFunc("POST /v1/filter-validate", handleFilterValidate)
	mux.HandleFunc("POST /v1/transform", handleTransform)
	mux.HandleFunc("POST /v1/filter", handleFilter)
	mux.HandleFunc("POST /v1/pipeline/validate-ottl", handlePipelineValidateOTTL)
	mux.HandleFunc("POST /v1/pipeline/execute", handlePipelineExecute)
	mux.HandleFunc("POST /v1/pipeline/import-config", handlePipelineImportConfig)

	addr := ln.Addr().(*net.TCPAddr)
	fmt.Printf("READY http://127.0.0.1:%d\n", addr.Port)
	if err := http.Serve(ln, mux); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 32<<20))
	return dec.Decode(dst)
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"contrib": contribVersion,
		"stable":  stableVersion,
	})
}

func handleValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Statements []string `json:"statements"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, engine.ValidateStatements(req.Statements))
}

func handleFilterValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Conditions []string       `json:"conditions"`
		Signals    engine.Signals `json:"signals"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, engine.ValidateFilterConditionsParse(req.Conditions, req.Signals))
}

func handleTransform(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Signal     string          `json:"signal"`
		Data       json.RawMessage `json:"data"`
		Statements []string        `json:"statements"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	signal := req.Signal
	if signal == "" {
		signal = "traces"
	}
	switch signal {
	case "traces":
		out, count, err := engine.TransformRunData(req.Data, req.Statements)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out, "count": count})
	case "logs":
		out, err := engine.TransformLogRunData(req.Data, req.Statements)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out})
	case "metrics":
		out, err := engine.TransformMetricRunData(req.Data, req.Statements)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out})
	default:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("unknown signal: %s", signal))
	}
}

func handleFilter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Signal     string            `json:"signal"`
		Data       json.RawMessage   `json:"data"`
		Conditions []string          `json:"conditions"`
		Mode       engine.FilterMode `json:"mode"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	mode := req.Mode
	if mode == "" {
		mode = engine.FilterModeInclude
	}
	signal := req.Signal
	if signal == "" {
		signal = "traces"
	}
	var (
		out         json.RawMessage
		total, kept int
		err         error
	)
	switch signal {
	case "traces":
		out, total, kept, err = engine.FilterRunData(req.Data, req.Conditions, mode)
	case "logs":
		out, total, kept, err = engine.FilterLogRunData(req.Data, req.Conditions, mode)
	case "metrics":
		out, total, kept, err = engine.FilterMetricRunData(req.Data, req.Conditions, mode)
	default:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("unknown signal: %s", signal))
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  out,
		"total": total,
		"kept":  kept,
	})
}

func handlePipelineValidateOTTL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Pipeline engine.Pipeline `json:"pipeline"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, engine.ValidatePipelineOTTL(req.Pipeline))
}

func handlePipelineExecute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data     json.RawMessage `json:"data"`
		Pipeline engine.Pipeline `json:"pipeline"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	result, err := engine.ExecutePipeline(req.Data, req.Pipeline)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func handlePipelineImportConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		// WorkerClient posts config_yaml; keep config as a compatibility alias.
		ConfigYAML string `json:"config_yaml"`
		Config     string `json:"config"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	configYAML := req.ConfigYAML
	if configYAML == "" {
		configYAML = req.Config
	}
	if strings.TrimSpace(configYAML) == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("config_yaml is required"))
		return
	}
	pipelines, err := engine.ImportCollectorConfig(configYAML)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, pipelines)
}

