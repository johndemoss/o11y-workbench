package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/johndemoss/otel-workbench/internal/gen"
)

func TestIntegration_RunLifecycle(t *testing.T) {
	store := NewRunStore()

	// Test 1: Create a finite run
	cfg := gen.Config{
		Seed: 42, TraceCount: 2, Topology: gen.TopologyFlat,
		SpanCount: 5, ServiceName: "test",
		BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: 5 * time.Second,
	}
	run, err := StartRun(context.Background(), store, cfg, nil, true, "file")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	t.Logf("Created run: %s", run.ID)

	time.Sleep(2 * time.Second)

	snap := store.Get(run.ID)
	if snap == nil {
		t.Fatal("run not found in store")
	}
	t.Logf("Run status: %s, spans: %d", snap.Status, snap.SpanCount)
	if snap.Status != RunStatusCompleted {
		t.Errorf("expected completed, got %s", snap.Status)
	}
	if snap.SpanCount == 0 {
		t.Error("expected non-zero span count")
	}

	// Test 2: Create infinite run (TraceCount=0) and cancel it
	cfgZero := gen.Config{
		Seed: 42, TraceCount: 0, Topology: gen.TopologyFlat,
		SpanCount: 5, ServiceName: "test", Rate: 1000,
		BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: 5 * time.Second,
	}
	runZero, err := StartRun(context.Background(), store, cfgZero, nil, true, "file")
	if err != nil {
		t.Fatalf("StartRun infinite: %v", err)
	}
	t.Logf("Created infinite run: %s", runZero.ID)

	time.Sleep(500 * time.Millisecond)

	snapZero := store.Get(runZero.ID)
	if snapZero == nil {
		t.Fatal("infinite run not found in store")
	}
	t.Logf("Infinite run status: %s, completed: %d", snapZero.Status, snapZero.TraceCountCompleted)
	if snapZero.Status != RunStatusRunning {
		t.Errorf("expected running, got %s", snapZero.Status)
	}
	if snapZero.TraceCountCompleted == 0 {
		t.Error("expected some traces completed")
	}

	store.StopRun(runZero.ID)
	time.Sleep(100 * time.Millisecond)
	snapStopped := store.Get(runZero.ID)
	if snapStopped == nil {
		t.Fatal("stopped run not found in store")
	}
	t.Logf("After stop - status: %s, error: %s", snapStopped.Status, snapStopped.Error)
	if snapStopped.Status != RunStatusStopped || snapStopped.Error != "stopped" {
		t.Errorf("expected stopped/stopped, got %s/%s", snapStopped.Status, snapStopped.Error)
	}

	// Test 3: Stopped run persists in store
	persisted := store.Get(runZero.ID)
	if persisted == nil {
		t.Fatal("stopped run should persist in store")
	}
	if persisted.Status != RunStatusStopped {
		t.Errorf("expected stopped status, got %s", persisted.Status)
	}
	t.Log("Persistence OK")
}

func TestIntegration_UserScenarios(t *testing.T) {
	// Test registration and listing
	if err := gen.RegisterUserScenario("my-test", []*gen.ScenarioSpanNode{
		{ServiceName: "svc", Name: "op"},
	}); err != nil {
		t.Fatalf("RegisterUserScenario: %v", err)
	}
	defer gen.DeleteUserScenario("my-test")

	scenarios := gen.ListUserScenarios()
	t.Logf("User scenarios: %v", scenarios)
	if len(scenarios) != 1 || scenarios[0] != "my-test" {
		t.Errorf("expected [my-test], got %v", scenarios)
	}
	roots, ok := gen.GetUserScenario("my-test")
	if !ok || len(roots) != 1 {
		t.Fatal("GetUserScenario failed")
	}

	gen.DeleteUserScenario("my-test")
	if len(gen.ListUserScenarios()) != 0 {
		t.Error("scenario should be deleted")
	}
}

func TestIntegration_ScenarioParser(t *testing.T) {
	jsonData := `{
		"name": "parsed-scenario",
		"roots": [{
			"service_name": "frontend",
			"name": "GET /api",
			"kind": "SPAN_KIND_SERVER",
			"children": [{
				"service_name": "backend",
				"name": "process",
				"kind": "SPAN_KIND_INTERNAL"
			}]
		}]
	}`
	parsed, err := gen.ParseUserScenario([]byte(jsonData))
	if err != nil {
		t.Fatalf("ParseUserScenario: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 root, got %d", len(parsed))
	}
	if parsed[0].ServiceName != "frontend" {
		t.Errorf("expected frontend, got %s", parsed[0].ServiceName)
	}
	if len(parsed[0].Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(parsed[0].Children))
	}
	if parsed[0].Children[0].ServiceName != "backend" {
		t.Errorf("expected backend child, got %s", parsed[0].Children[0].ServiceName)
	}

	// Test error cases
	invalidCases := []struct {
		name string
		data string
	}{
		{"empty name", `{"name":"","roots":[{"service_name":"s","name":"n","kind":"SPAN_KIND_INTERNAL"}]}`},
		{"no roots", `{"name":"test","roots":[]}`},
		{"bad kind", `{"name":"test","roots":[{"service_name":"s","name":"n","kind":"INVALID"}]}`},
		{"missing service_name", `{"name":"test","roots":[{"name":"n","kind":"SPAN_KIND_INTERNAL"}]}`},
	}
	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gen.ParseUserScenario([]byte(tc.data))
			if err == nil {
				t.Error("expected error, got nil")
			}
			t.Logf("Got expected error: %v", err)
		})
	}

	// Test integration: register parsed scenario and run it via getScenario
	if err := gen.RegisterUserScenario("parsed-scenario", parsed); err != nil {
		t.Fatalf("RegisterUserScenario: %v", err)
	}
	defer gen.DeleteUserScenario("parsed-scenario")

	// Run a generator with the user scenario
	store := NewRunStore()
	cfg := gen.Config{
		Seed: 42, TraceCount: 1, Scenario: "parsed-scenario",
		SpanCount: 10, MaxDepth: 5, ServiceName: "test",
		BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: 5 * time.Second,
	}
	run, err := StartRun(context.Background(), store, cfg, nil, true, "file")
	if err != nil {
		t.Fatalf("StartRun user scenario: %v", err)
	}
	time.Sleep(2 * time.Second)
	snap := store.Get(run.ID)
	if snap == nil {
		t.Fatal("user scenario run not found")
	}
	t.Logf("User scenario run: status=%s, spans=%d", snap.Status, snap.SpanCount)
	if snap.Status != RunStatusCompleted {
		t.Errorf("expected completed, got %s", snap.Status)
	}
	if snap.SpanCount == 0 {
		t.Error("expected non-zero span count from user scenario")
	}
}

func TestIntegration_JobsConfig(t *testing.T) {
	// Test using the same struct as main.go uses for job definitions
	type jobDef struct {
		Seed              int64  `json:"seed"`
		TraceCount        int    `json:"trace_count"`
		Rate              int    `json:"rate"`
		Topology          string `json:"topology"`
		SpanCount         int    `json:"span_count"`
		MaxDepth          int    `json:"max_depth"`
		ServiceName       string `json:"service_name"`
		Scenario          string `json:"scenario"`
		ScenarioSpanCount int    `json:"scenario_span_count"`
	}
	jobsData := `[{"seed":1,"trace_count":2,"topology":"flat","span_count":3,"service_name":"job-svc"}]`
	var jobs []jobDef
	if err := json.Unmarshal([]byte(jobsData), &jobs); err != nil {
		t.Fatalf("unmarshal jobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Seed != 1 || jobs[0].TraceCount != 2 {
		t.Errorf("jobs parse: %+v", jobs[0])
	}
	topo, err := gen.ParseTopology(jobs[0].Topology)
	if err != nil {
		t.Fatalf("parse topology: %v", err)
	}
	if topo != gen.TopologyFlat {
		t.Errorf("expected flat topology, got %v", topo)
	}

	// Test that overwriting built-in scenarios is rejected
	err = gen.RegisterUserScenario("microservice", nil)
	if err == nil {
		t.Error("expected error overriding built-in scenario")
	}
}

func TestIntegration_ImportOTLPJSON(t *testing.T) {
	importSvc := &Server{store: NewRunStore()}

	// Test 1: Import a single ExportTraceServiceRequest
	singleJSON := `{"resourceSpans":[{"resource":{},"scopeSpans":[{"scope":{},"spans":[{"traceId":"","spanId":"","name":"test-span","kind":1,"startTimeUnixNano":"1000000","endTimeUnixNano":"2000000","status":{"code":1}}]}]}]}`
	spans, err := parseOTLPJSON([]byte(singleJSON))
	if err != nil {
		t.Fatalf("parseOTLPJSON single: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("expected 1 ResourceSpans, got %d", len(spans))
	}
	if len(spans[0].ScopeSpans) != 1 || len(spans[0].ScopeSpans[0].Spans) != 1 {
		t.Fatal("expected 1 span")
	}
	if spans[0].ScopeSpans[0].Spans[0].Name != "test-span" {
		t.Errorf("expected test-span, got %s", spans[0].ScopeSpans[0].Spans[0].Name)
	}

	// Test 2: Import NDJSON with hex-encoded IDs (OTLP collector file exporter format)
	ndjsonData := `{"resourceSpans":[{"resource":{},"scopeSpans":[{"scope":{},"spans":[{"traceId":"0102030405060708090a0b0c0d0e0f10","spanId":"0102030405060708","name":"ndjson-1","kind":2,"startTimeUnixNano":"1000000","endTimeUnixNano":"2000000","status":{"code":1}}]}]}]}
{"resourceSpans":[{"resource":{},"scopeSpans":[{"scope":{},"spans":[{"traceId":"0102030405060708090a0b0c0d0e0f10","spanId":"0102030405060709","name":"ndjson-2","kind":3,"startTimeUnixNano":"1000000","endTimeUnixNano":"2000000","status":{"code":1}}]}]}]}`
	ndjsonSpans, err := parseOTLPJSON([]byte(ndjsonData))
	if err != nil {
		t.Fatalf("parseOTLPJSON ndjson: %v", err)
	}
	if len(ndjsonSpans) != 2 {
		t.Fatalf("expected 2 ResourceSpans from NDJSON, got %d", len(ndjsonSpans))
	}

	// Test 3: Import via HTTP handler
	totalSpans := 0
	for _, rs := range ndjsonSpans {
		for _, ss := range rs.ScopeSpans {
			totalSpans += len(ss.Spans)
		}
	}
	if totalSpans != 2 {
		t.Errorf("expected 2 total spans, got %d", totalSpans)
	}

	// Test 4: End-to-end via HTTP handler
	run := importSvc.handleImportRunHelper([]byte(singleJSON))
	if run == nil {
		t.Fatal("handleImportRun returned nil")
	}
	if run.Status != RunStatusCompleted {
		t.Errorf("expected completed, got %s", run.Status)
	}
	if run.SpanCount != 1 {
		t.Errorf("expected 1 span, got %d", run.SpanCount)
	}
	if run.TraceCount != 0 {
		t.Errorf("expected 0 trace IDs (empty traceId), got %d", run.TraceCount)
	}
	if run.Output != "import" {
		t.Errorf("expected output=import, got %s", run.Output)
	}
	t.Logf("Imported run: id=%s spans=%d traces=%d", run.ID, run.SpanCount, run.TraceCount)

	// Verify it was stored
	stored := importSvc.store.Get(run.ID)
	if stored == nil {
		t.Fatal("imported run not in store")
	}
	if stored.TraceData == nil {
		t.Fatal("imported run missing trace_data")
	}

	// Test 5: Error cases
	errCases := []struct {
		name string
		data string
	}{
		{"empty body", ""},
		{"invalid JSON", "{bad json}"},
		{"no resourceSpans", `{"something":"else"}`},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseOTLPJSON([]byte(tc.data))
			if err == nil {
				t.Error("expected error, got nil")
			}
			t.Logf("Got expected error: %v", err)
		})
	}
}

// handleImportRunHelper calls the import handler logic directly and returns the created Run.
func (s *Server) handleImportRunHelper(body []byte) *Run {
	spans, err := parseOTLPJSON(body)
	if err != nil {
		return nil
	}
	totalSpans := 0
	traceIDs := make(map[string]bool)
	for _, rs := range spans {
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				totalSpans++
				if sp.TraceId != nil {
					traceIDs[string(sp.TraceId)] = true
				}
			}
		}
	}
	req := &collectortracev1.ExportTraceServiceRequest{ResourceSpans: spans}
	data, _ := protojson.MarshalOptions{UseProtoNames: true, Indent: "  "}.Marshal(req)
	raw := json.RawMessage(data)
	now := time.Now()
	run := &Run{
		ID:          fmt.Sprintf("run-%d", runIDCounter.Add(1)),
		Status:      RunStatusCompleted,
		StartedAt:   now,
		CompletedAt: &now,
		SpanCount:   totalSpans,
		TraceCount:  len(traceIDs),
		Scenario:    "imported",
		Output:      "import",
		TraceData:   &raw,
	}
	s.store.Add(run)
	return run
}

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}
