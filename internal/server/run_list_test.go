package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func seedCompletedRun(t *testing.T, store *RunStore, id string) *Run {
	t.Helper()
	trace := json.RawMessage(`{"resourceSpans":[]}`)
	logData := json.RawMessage(`{"resourceLogs":[]}`)
	metric := json.RawMessage(`{"resourceMetrics":[]}`)
	run := &Run{
		ID:         id,
		Status:     RunStatusCompleted,
		StartedAt:  time.Now().Add(-time.Minute),
		SpanCount:  3,
		TraceData:  &trace,
		LogData:    &logData,
		MetricData: &metric,
		Endpoints:  []string{"localhost:4317"},
	}
	store.Add(run)
	return run
}

func TestRunStore_PruneCompleted(t *testing.T) {
	store := NewRunStore()
	base := time.Now().Add(-2 * time.Hour)
	var oldestID string
	for i := 0; i < 51; i++ {
		id := fmt.Sprintf("completed-%d", i)
		if i == 0 {
			oldestID = id
		}
		run := &Run{
			ID:        id,
			Status:    RunStatusCompleted,
			StartedAt: base.Add(time.Duration(i) * time.Minute),
			Endpoints: []string{"localhost:4317"},
		}
		store.Add(run)
	}
	running := &Run{
		ID:        "running-1",
		Status:    RunStatusRunning,
		StartedAt: base.Add(-time.Hour),
		Endpoints: []string{"localhost:4317"},
	}
	store.Add(running)

	store.PruneCompleted(50)

	list := store.List()
	var completed, stillRunning int
	for _, r := range list {
		switch r.Status {
		case RunStatusCompleted:
			completed++
		case RunStatusRunning:
			stillRunning++
		}
	}
	if completed != 50 {
		t.Fatalf("completed count = %d, want 50", completed)
	}
	if stillRunning != 1 {
		t.Fatalf("running count = %d, want 1", stillRunning)
	}
	if got := store.Get(oldestID); got != nil {
		t.Fatalf("Get(%s) = %#v, want nil", oldestID, got)
	}
	if got := store.Get("running-1"); got == nil || got.Status != RunStatusRunning {
		t.Fatalf("running run missing or wrong status: %#v", got)
	}
}

func TestRunStore_ListOmitsPayloads(t *testing.T) {
	store := NewRunStore()
	seedCompletedRun(t, store, "run-1")

	list := store.List()
	if len(list) != 1 {
		t.Fatalf("List len = %d, want 1", len(list))
	}
	got := list[0]
	if got.TraceData != nil || got.LogData != nil || got.MetricData != nil {
		t.Fatalf("List included payloads: trace=%v log=%v metric=%v", got.TraceData != nil, got.LogData != nil, got.MetricData != nil)
	}
	if !got.HasTraceData || !got.HasLogData || !got.HasMetricData {
		t.Fatalf("Has* flags = trace:%v log:%v metric:%v, want all true", got.HasTraceData, got.HasLogData, got.HasMetricData)
	}
}

func TestRunStore_GetIncludesPayloads(t *testing.T) {
	store := NewRunStore()
	seed := seedCompletedRun(t, store, "run-2")

	got := store.Get("run-2")
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.TraceData == nil || got.LogData == nil || got.MetricData == nil {
		t.Fatal("Get omitted payloads")
	}
	if string(*got.TraceData) != string(*seed.TraceData) {
		t.Fatalf("TraceData = %s, want %s", *got.TraceData, *seed.TraceData)
	}
	if !got.HasTraceData || !got.HasLogData || !got.HasMetricData {
		t.Fatalf("Has* flags = trace:%v log:%v metric:%v, want all true", got.HasTraceData, got.HasLogData, got.HasMetricData)
	}
}

func TestHandleListRuns_OmitsPayloads(t *testing.T) {
	srv, err := New(":0", false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	seedCompletedRun(t, srv.store, "run-list")

	req := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	var body struct {
		Runs []map[string]json.RawMessage `json:"runs"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Runs) != 1 {
		t.Fatalf("runs len = %d, want 1", len(body.Runs))
	}
	run := body.Runs[0]
	if _, ok := run["trace_data"]; ok {
		t.Fatalf("list JSON included trace_data: %s", run["trace_data"])
	}
	if _, ok := run["log_data"]; ok {
		t.Fatalf("list JSON included log_data: %s", run["log_data"])
	}
	if _, ok := run["metric_data"]; ok {
		t.Fatalf("list JSON included metric_data: %s", run["metric_data"])
	}
	var hasTrace bool
	if err := json.Unmarshal(run["has_trace_data"], &hasTrace); err != nil || !hasTrace {
		t.Fatalf("has_trace_data = %s, want true", run["has_trace_data"])
	}
}

func TestHandleGetRun_IncludesPayloads(t *testing.T) {
	srv, err := New(":0", false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	seedCompletedRun(t, srv.store, "run-get")

	req := httptest.NewRequest(http.MethodGet, "/api/runs/run-get", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	var run map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := run["trace_data"]; !ok {
		t.Fatal("get JSON missing trace_data")
	}
	if _, ok := run["log_data"]; !ok {
		t.Fatal("get JSON missing log_data")
	}
	if _, ok := run["metric_data"]; !ok {
		t.Fatal("get JSON missing metric_data")
	}
}
