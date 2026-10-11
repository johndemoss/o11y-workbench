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
