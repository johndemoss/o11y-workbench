package ottlengine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/johndemoss/o11y-workbench/internal/ottl"
)

func TestWorkerClientValidate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/validate", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Statements []string `json:"statements"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(req.Statements) != 1 || req.Statements[0] != `set(name, "x")` {
			http.Error(w, "unexpected statements", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(ottl.ValidateResult{Valid: true, Parsed: 1})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewWorkerClient("0.162.0", "1.68.0", srv.URL)
	res := c.ValidateStatements([]string{`set(name, "x")`})
	if !res.Valid || res.Parsed != 1 {
		t.Fatalf("ValidateStatements: %+v", res)
	}
}

func TestWorkerClientExecutePipeline(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pipeline/execute", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Data     json.RawMessage `json:"data"`
			Pipeline ottl.Pipeline   `json:"pipeline"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if string(req.Data) != `{"ok":true}` {
			http.Error(w, "bad data", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(ottl.PipelineResult{
			Stages:         []ottl.StageResult{},
			FinalData:      req.Data,
			FinalSpanCount: 0,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewWorkerClient("0.162.0", "1.68.0", srv.URL)
	res, err := c.ExecutePipeline(json.RawMessage(`{"ok":true}`), ottl.Pipeline{})
	if err != nil {
		t.Fatalf("ExecutePipeline: %v", err)
	}
	if string(res.FinalData) != `{"ok":true}` {
		t.Fatalf("FinalData = %s", res.FinalData)
	}
}

func TestWorkerClientImportCollectorConfig(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pipeline/import-config", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ConfigYAML string `json:"config_yaml"`
			Config     string `json:"config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.ConfigYAML) == "" {
			http.Error(w, `{"error":"config_yaml is required"}`, http.StatusBadRequest)
			return
		}
		if !strings.Contains(req.ConfigYAML, "service:") {
			http.Error(w, `{"error":"config missing 'service' section"}`, http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(ottl.PipelinesBySignal{
			"traces": &ottl.Pipeline{Receivers: []string{"otlp"}, Exporters: []string{"debug"}},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewWorkerClient("0.162.0", "1.68.0", srv.URL)
	got, err := c.ImportCollectorConfig("service:\n  pipelines:\n    traces:\n      receivers: [otlp]\n      processors: []\n      exporters: [debug]\n")
	if err != nil {
		t.Fatalf("ImportCollectorConfig: %v", err)
	}
	if got["traces"] == nil || len(got["traces"].Receivers) != 1 || got["traces"].Receivers[0] != "otlp" {
		t.Fatalf("unexpected pipelines: %+v", got)
	}
}

func TestWorkerClientExecutePipelineForSignal_Logs(t *testing.T) {
	var gotSignal string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pipeline/execute", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Signal   string          `json:"signal"`
			Data     json.RawMessage `json:"data"`
			Pipeline ottl.Pipeline   `json:"pipeline"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		gotSignal = req.Signal
		if req.Signal != "logs" {
			http.Error(w, "unexpected signal", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(ottl.PipelineResult{
			Stages:         []ottl.StageResult{{StageID: "s1", OutputSpanCount: 1}},
			FinalData:      req.Data,
			FinalSpanCount: 1,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewWorkerClient("0.163.0", "1.69.0", srv.URL)
	res, err := c.ExecutePipelineForSignal(json.RawMessage(`{"resourceLogs":[]}`), ottl.Pipeline{}, "logs")
	if err != nil {
		t.Fatalf("ExecutePipelineForSignal: %v", err)
	}
	if gotSignal != "logs" {
		t.Fatalf("signal = %q, want logs", gotSignal)
	}
	if res.FinalSpanCount != 1 {
		t.Fatalf("FinalSpanCount = %d", res.FinalSpanCount)
	}
}

func TestWorkerClientExecutePipelineForSignal_TracesOmitsSignal(t *testing.T) {
	var gotSignal string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pipeline/execute", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if raw, ok := req["signal"]; ok {
			_ = json.Unmarshal(raw, &gotSignal)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ottl.PipelineResult{FinalSpanCount: 0})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewWorkerClient("0.163.0", "1.69.0", srv.URL)
	if _, err := c.ExecutePipelineForSignal(json.RawMessage(`{}`), ottl.Pipeline{}, ""); err != nil {
		t.Fatalf("ExecutePipelineForSignal: %v", err)
	}
	if gotSignal != "" {
		t.Fatalf("signal = %q, want omitted for empty/traces", gotSignal)
	}
}

func TestWorkerClientExecutePipelineForSignal_UnsupportedPropagates(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pipeline/execute", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"unsupported signal: profiles"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewWorkerClient("0.163.0", "1.69.0", srv.URL)
	_, err := c.ExecutePipelineForSignal(json.RawMessage(`{}`), ottl.Pipeline{}, "profiles")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported signal") {
		t.Fatalf("err = %v", err)
	}
}

func TestWorkerClientNon2xx(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/validate", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewWorkerClient("0.162.0", "1.68.0", srv.URL)
	res := c.ValidateStatements([]string{`set(name, "x")`})
	if res.Valid {
		t.Fatal("expected invalid result on non-2xx")
	}
	if len(res.Errors) == 0 || !strings.Contains(res.Errors[0], "ottl worker 0.162.0:") {
		t.Fatalf("errors = %v", res.Errors)
	}
}
