package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/johndemoss/o11y-workbench/workers/ottl-0.163/engine"
)

const (
	contribVersion = "0.163.0"
	stableVersion  = "1.69.0"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:0", "listen address")
	parentStdin := flag.Bool("parent-stdin", false, "exit when supervising parent closes stdin")
	flag.Parse()
	if *parentStdin {
		go func() { _, _ = io.Copy(io.Discard, os.Stdin); os.Exit(0) }()
	}

	if err := engine.EnableOTTLLambdaGate(); err != nil {
		log.Fatalf("enable OTTL lambda gate: %v", err)
	}

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
	server := &http.Server{Handler: workerHandler(mux), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	if err := server.Serve(ln); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func workerHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() || r.Header.Get("Origin") != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost && !strings.EqualFold(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]), "application/json") {
			http.Error(w, "application/json required", http.StatusUnsupportedMediaType)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128<<20)
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	if _, ok := err.(*http.MaxBytesError); ok {
		status = http.StatusRequestEntityTooLarge
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
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
		out, count, err := engine.TransformRunData(r.Context(), req.Data, req.Statements)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out, "span_count": count})
	case "logs":
		out, err := engine.TransformLogRunData(r.Context(), req.Data, req.Statements)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out})
	case "metrics":
		out, err := engine.TransformMetricRunData(r.Context(), req.Data, req.Statements)
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
		out, total, kept, err = engine.FilterRunData(r.Context(), req.Data, req.Conditions, mode)
	case "logs":
		out, total, kept, err = engine.FilterLogRunData(r.Context(), req.Data, req.Conditions, mode)
	case "metrics":
		out, total, kept, err = engine.FilterMetricRunData(r.Context(), req.Data, req.Conditions, mode)
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
	writeJSON(w, http.StatusOK, engine.ValidatePipelineOTTL(r.Context(), req.Pipeline))
}

func handlePipelineExecute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Signal   string          `json:"signal"`
		Data     json.RawMessage `json:"data"`
		Pipeline engine.Pipeline `json:"pipeline"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	result, err := engine.ExecutePipelineForSignal(r.Context(), req.Data, req.Pipeline, req.Signal)
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
