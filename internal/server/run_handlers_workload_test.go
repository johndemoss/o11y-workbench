package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleStartRun_WorkloadOverlays(t *testing.T) {
	s, err := New(":0", false)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"seed":        7,
		"trace_count": 1,
		"rate":        0,
		"duration":    1,
		"topology":    "flat",
		"span_count":  2,
		"max_depth":   3,
		"service_name": "dash",
		"output":      "file",
		"signals":     map[string]bool{"traces": true},
		"workload": map[string]any{
			"single_resource":       true,
			"items_per_request":     2,
			"experiment_id":         "exp-dash",
			"attribute_count":       1,
			"attribute_value_bytes": 2,
			"cardinality":           3,
			"continue_on_export_err": true,
			"mode":                  "burst",
			"burst_on_s":            1.5,
			"burst_off_s":           0.5,
			"burst_peak_rate_items_per_sec": 100,
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var run map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	cfg, _ := run["config"].(map[string]any)
	if cfg == nil {
		t.Fatalf("missing config: %#v", run)
	}
	wl, _ := cfg["workload"].(map[string]any)
	if wl == nil {
		t.Fatalf("missing workload: %#v", cfg)
	}
	if wl["experiment_id"] != "exp-dash" {
		t.Fatalf("experiment_id=%v", wl["experiment_id"])
	}
	if wl["single_resource"] != true {
		t.Fatalf("single_resource=%v", wl["single_resource"])
	}
	// Burst durations encode as nanoseconds in JSON.
	burst, _ := wl["burst"].(map[string]any)
	if burst == nil {
		t.Fatalf("missing burst: %#v", wl)
	}
	onNs, _ := burst["on"].(float64)
	if time.Duration(onNs) != 1500*time.Millisecond {
		t.Fatalf("burst.on=%v", onNs)
	}
}

func TestHandleStartRun_FlatShapeSkipsTopologyParse(t *testing.T) {
	s, err := New(":0", false)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"seed":        1,
		"trace_count": 1,
		"topology":    "not-a-real-topology",
		"output":      "file",
		"signals":     map[string]bool{"traces": true},
		"workload":    map[string]any{"shape": "flat", "items_per_request": 1},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}


func TestHandleStartRun_ScenarioWorkload(t *testing.T) {
	s, err := New(":0", false)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"seed":        1,
		"trace_count": 1,
		"topology":    "not-a-real-topology",
		"scenario":    "workload",
		"output":      "file",
		"signals":     map[string]bool{"traces": true},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var run map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	cfg, _ := run["config"].(map[string]any)
	wl, _ := cfg["workload"].(map[string]any)
	if wl == nil || wl["shape"] != "flat" {
		t.Fatalf("expected workload.shape=flat, got %#v", cfg)
	}
	// Config.Scenario has no json tag; assert via Workload.Shape pin instead.
	_ = cfg
}
