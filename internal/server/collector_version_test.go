package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/ottlengine"
)

func TestPipelineExecute_UnsupportedCollectorVersion(t *testing.T) {
	store := NewRunStore()
	now := time.Now()
	raw := json.RawMessage(`{"resourceSpans":[]}`)
	run := &Run{
		ID:          fmt.Sprintf("run-%d", runIDCounter.Add(1)),
		Status:      RunStatusCompleted,
		StartedAt:   now,
		CompletedAt: &now,
		TraceData:   &raw,
	}
	store.Add(run)

	srv := &Server{store: store, pipelineEnabled: true}

	pipelineYAML := `
stages:
  - id: t1
    type: transform
    processor_ref: p1
processors:
  p1:
    statements:
      - set(name, "x")
`
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/ottl/pipeline/execute?run_id="+run.ID+"&collector_version=9.9.9",
		bytes.NewBufferString(pipelineYAML),
	)
	req.Header.Set("Content-Type", "application/x-yaml")
	rr := httptest.NewRecorder()

	srv.handlePipelineExecute(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "unsupported collector_version") {
		t.Fatalf("body missing unsupported collector_version: %s", rr.Body.String())
	}
}

func TestPipelineValidate_InvalidOTTL_CollectorVersion(t *testing.T) {
	srv := &Server{pipelineEnabled: true}

	pipelineYAML := `
stages:
  - id: t1
    type: transform
    processor_ref: p1
processors:
  p1:
    statements:
      - this is not valid ottl
`
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/ottl/pipeline/validate?collector_version=0.165.0",
		bytes.NewBufferString(pipelineYAML),
	)
	req.Header.Set("Content-Type", "application/x-yaml")
	rr := httptest.NewRecorder()

	srv.handlePipelineValidate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rr.Body.String())
	}
	if resp.Valid {
		t.Fatalf("expected valid=false, body=%s", rr.Body.String())
	}
	if len(resp.Errors) == 0 {
		t.Fatal("expected OTTL validation errors")
	}
	joined := strings.ToLower(strings.Join(resp.Errors, " "))
	if !strings.Contains(joined, "parse") && !strings.Contains(joined, "ottl") {
		t.Fatalf("errors should mention parse/OTTL, got %v", resp.Errors)
	}
}

func TestCollectorVersions_DefaultInProcess(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/collector-versions", nil)
	rr := httptest.NewRecorder()

	srv.handleCollectorVersions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Default  string                   `json:"default"`
		Versions []ottlengine.VersionInfo `json:"versions"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rr.Body.String())
	}
	if resp.Default != ottlengine.DefaultContribVersion {
		t.Fatalf("default = %q, want %q", resp.Default, ottlengine.DefaultContribVersion)
	}
	found := false
	for _, v := range resp.Versions {
		if v.Contrib == ottlengine.DefaultContribVersion && v.Source == "in-process" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("versions missing in-process %s: %+v", ottlengine.DefaultContribVersion, resp.Versions)
	}
}

func TestServerClose_RunsEngineStops(t *testing.T) {
	called := 0
	srv := &Server{
		engineStops: []func() error{
			func() error {
				called++
				return nil
			},
		},
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if called != 1 {
		t.Fatalf("stop calls = %d, want 1", called)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if called != 1 {
		t.Fatalf("stop calls after second Close = %d, want 1", called)
	}
}

func TestNew_CloseStopsConfiguredWorkers(t *testing.T) {
	bin := filepath.Join("..", "..", "bin", "ottl-worker-0.164.0")
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("worker binary not found at %s (run make ottl-workers)", bin)
	}

	t.Setenv("O11Y_OTTL_WORKERS", "0.164.0="+bin)
	srv, err := New("127.0.0.1:0", false, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.Close()

	eng, err := srv.ensureEngines().Get("0.164.0")
	if err != nil {
		t.Fatalf("Get(0.164.0): %v", err)
	}
	wc, ok := eng.(*ottlengine.WorkerClient)
	if !ok {
		t.Fatalf("engine type %T, want *ottlengine.WorkerClient", eng)
	}
	if _, _, err := wc.Health(context.Background()); err != nil {
		t.Fatalf("worker health before Close: %v", err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, _, err := wc.Health(context.Background()); err == nil {
		t.Fatal("worker health succeeded after Close; stopper was not retained/run")
	}
}
