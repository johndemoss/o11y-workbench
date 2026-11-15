package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/remotetap"
	"golang.org/x/net/websocket"
	"testing"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

func makeResourceSpans(serviceName string, spanKind tracev1.Span_SpanKind) *tracev1.ResourceSpans {
	return &tracev1.ResourceSpans{
		Resource: &resourcev1.Resource{
			Attributes: []*commonv1.KeyValue{
				{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: serviceName}}},
			},
		},
		ScopeSpans: []*tracev1.ScopeSpans{
			{
				Spans: []*tracev1.Span{
					{
						Name: "test-span",
						Kind: spanKind,
					},
				},
			},
		},
	}
}

func makeLogRecord(severity logsv1.SeverityNumber) *logsv1.LogRecord {
	return &logsv1.LogRecord{
		SeverityNumber: severity,
		Body:           &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "test log"}},
	}
}

func TestRingBuffer_FIFO(t *testing.T) {
	rb := newRingBuffer(3, 1024*1024)

	rb.Add(makeResourceSpans("service-1", tracev1.Span_SPAN_KIND_CLIENT))
	rb.Add(makeResourceSpans("service-2", tracev1.Span_SPAN_KIND_SERVER))
	rb.Add(makeResourceSpans("service-3", tracev1.Span_SPAN_KIND_PRODUCER))
	rb.Add(makeResourceSpans("service-4", tracev1.Span_SPAN_KIND_CONSUMER))
	rb.Add(makeResourceSpans("service-5", tracev1.Span_SPAN_KIND_INTERNAL))

	items := rb.GetAll()
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	svc0 := getResourceAttr(items[0], "service.name")
	svc1 := getResourceAttr(items[1], "service.name")
	svc2 := getResourceAttr(items[2], "service.name")
	if svc0 != "service-3" {
		t.Errorf("expected service-3 at index 0, got %s", svc0)
	}
	if svc1 != "service-4" {
		t.Errorf("expected service-4 at index 1, got %s", svc1)
	}
	if svc2 != "service-5" {
		t.Errorf("expected service-5 at index 2, got %s", svc2)
	}
}

func TestRingBuffer_MaxSpans(t *testing.T) {
	rb := newRingBuffer(2, 1024*1024)

	for range 10 {
		rb.Add(makeResourceSpans("service", tracev1.Span_SPAN_KIND_CLIENT))
	}

	items := rb.GetAll()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestRingBuffer_MaxBytes(t *testing.T) {
	rb := newRingBuffer(100, 50)

	rb.Add(makeResourceSpans("service-very-long-name-1", tracev1.Span_SPAN_KIND_CLIENT))
	rb.Add(makeResourceSpans("service-very-long-name-2", tracev1.Span_SPAN_KIND_SERVER))

	items := rb.GetAll()
	if len(items) > 2 {
		t.Fatalf("expected at most 2 items with small maxBytes, got %d", len(items))
	}
}

func TestRingBuffer_EvictedCount(t *testing.T) {
	rb := newRingBuffer(2, 1024*1024)

	rb.Add(makeResourceSpans("s1", tracev1.Span_SPAN_KIND_CLIENT))
	rb.Add(makeResourceSpans("s2", tracev1.Span_SPAN_KIND_SERVER))
	rb.Add(makeResourceSpans("s3", tracev1.Span_SPAN_KIND_PRODUCER))
	rb.Add(makeResourceSpans("s4", tracev1.Span_SPAN_KIND_CONSUMER))
	rb.Add(makeResourceSpans("s5", tracev1.Span_SPAN_KIND_INTERNAL))

	evicted := rb.EvictedCount()
	if evicted != 3 {
		t.Errorf("expected evicted count 3, got %d", evicted)
	}
}

func TestSampler_None(t *testing.T) {
	s := newSampler("none", 0)

	for i := range 100 {
		if !s.ShouldSample() {
			t.Errorf("expected true for none sampler, got false at iteration %d", i)
		}
	}
}

func TestSampler_Head(t *testing.T) {
	s := newSampler("head", 5)

	passes := 0
	for range 10 {
		if s.ShouldSample() {
			passes++
		}
	}

	if passes != 5 {
		t.Errorf("expected 5 passes, got %d", passes)
	}
}

func TestSampler_Tail(t *testing.T) {
	s := newSampler("tail", 3)

	passes := 0
	for range 9 {
		if s.ShouldSample() {
			passes++
		}
	}

	if passes != 3 {
		t.Errorf("expected 3 passes for tail sampling, got %d", passes)
	}
}

func TestSampler_Random(t *testing.T) {
	s := newSampler("random", 0.5)

	trials := 10000
	passes := 0
	for range trials {
		if s.ShouldSample() {
			passes++
		}
	}

	percentage := float64(passes) / float64(trials)
	if percentage < 0.4 || percentage > 0.6 {
		t.Errorf("expected ~50%% passes, got %.2f%%", percentage*100)
	}
}

func TestShouldKeepTrace_Services(t *testing.T) {
	m := &RemotetapManager{
		filters: RemotetapFilters{
			Services: []string{"service-a", "service-b"},
		},
	}

	rsA := makeResourceSpans("service-a", tracev1.Span_SPAN_KIND_CLIENT)
	if !m.shouldKeepTrace(rsA) {
		t.Error("expected to keep service-a")
	}

	rsB := makeResourceSpans("service-b", tracev1.Span_SPAN_KIND_SERVER)
	if !m.shouldKeepTrace(rsB) {
		t.Error("expected to keep service-b")
	}

	rsC := makeResourceSpans("service-c", tracev1.Span_SPAN_KIND_SERVER)
	if m.shouldKeepTrace(rsC) {
		t.Error("expected to reject service-c")
	}
}

func TestShouldKeepTrace_SpanKinds(t *testing.T) {
	m := &RemotetapManager{
		filters: RemotetapFilters{
			SpanKinds: []string{"SPAN_KIND_CLIENT", "SPAN_KIND_SERVER"},
		},
	}

	rsClient := makeResourceSpans("test", tracev1.Span_SPAN_KIND_CLIENT)
	if !m.shouldKeepTrace(rsClient) {
		t.Error("expected to keep SPAN_KIND_CLIENT")
	}

	rsServer := makeResourceSpans("test", tracev1.Span_SPAN_KIND_SERVER)
	if !m.shouldKeepTrace(rsServer) {
		t.Error("expected to keep SPAN_KIND_SERVER")
	}

	rsProducer := makeResourceSpans("test", tracev1.Span_SPAN_KIND_PRODUCER)
	if m.shouldKeepTrace(rsProducer) {
		t.Error("expected to reject SPAN_KIND_PRODUCER")
	}
}

func TestShouldKeepLog_Severity(t *testing.T) {
	m := &RemotetapManager{
		filters: RemotetapFilters{
			MinLogSeverity: "WARN",
		},
	}

	lrWarn := makeLogRecord(logsv1.SeverityNumber_SEVERITY_NUMBER_WARN)
	if !m.shouldKeepLog(lrWarn) {
		t.Error("expected to keep WARN severity")
	}

	lrError := makeLogRecord(logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR)
	if !m.shouldKeepLog(lrError) {
		t.Error("expected to keep ERROR severity")
	}

	lrDebug := makeLogRecord(logsv1.SeverityNumber_SEVERITY_NUMBER_DEBUG)
	if m.shouldKeepLog(lrDebug) {
		t.Error("expected to reject DEBUG severity")
	}

	lrInfo := makeLogRecord(logsv1.SeverityNumber_SEVERITY_NUMBER_INFO)
	if m.shouldKeepLog(lrInfo) {
		t.Error("expected to reject INFO severity")
	}
}

func TestTAPRecordsFilteringAndBoundedPayloads(t *testing.T) {
	store := NewRunStore()
	run := &Run{ID: "capture", Status: RunStatusStreaming}
	store.Add(run)
	m := NewRemotetapManager(store)
	m.run = run
	m.filters = RemotetapFilters{Services: []string{"service"}, SpanKinds: []string{"SERVER"}, MinLogSeverity: "WARN", MaxSpans: 1, MaxBytes: 1 << 20}
	m.sampler = newSampler("none", 0)
	m.buffer = newRingBuffer(1, 1<<20)
	rs := makeResourceSpans("service", tracev1.Span_SPAN_KIND_SERVER)
	rs.ScopeSpans[0].Spans = append(rs.ScopeSpans[0].Spans,
		&tracev1.Span{Name: "excluded", Kind: tracev1.Span_SPAN_KIND_CLIENT},
		&tracev1.Span{Name: "second", Kind: tracev1.Span_SPAN_KIND_SERVER})
	logs := &logsv1.ResourceLogs{Resource: rs.Resource, ScopeLogs: []*logsv1.ScopeLogs{{LogRecords: []*logsv1.LogRecord{
		makeLogRecord(logsv1.SeverityNumber_SEVERITY_NUMBER_INFO), makeLogRecord(logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR),
	}}}}
	metrics := &metricsv1.ResourceMetrics{Resource: rs.Resource, ScopeMetrics: []*metricsv1.ScopeMetrics{{Metrics: []*metricsv1.Metric{{Name: "requests"}, {Name: "errors"}}}}}
	m.processData(&remotetap.RemotetapData{Traces: []*tracev1.ResourceSpans{rs}, Logs: []*logsv1.ResourceLogs{logs}, Metrics: []*metricsv1.ResourceMetrics{metrics}})
	snapshot := run.snapshot()
	if snapshot.SpanCount != 2 || snapshot.LogCount != 1 || snapshot.MetricCount != 2 {
		t.Fatalf("counts = %d/%d/%d, want 2/1/2", snapshot.SpanCount, snapshot.LogCount, snapshot.MetricCount)
	}
	if snapshot.LogCountCompleted != 1 || snapshot.MetricCountCompleted != 2 {
		t.Fatalf("completed logs/metrics = %d/%d, want 1/2", snapshot.LogCountCompleted, snapshot.MetricCountCompleted)
	}
	traces, retainedLogs, retainedMetrics := m.buffer.payloads()
	if len(traces) != 1 || len(retainedLogs) != 1 || len(retainedMetrics) != 2 || m.buffer.EvictedCount() != 1 {
		t.Fatalf("retained traces/logs/metrics=%d/%d/%d, evictions=%d", len(traces), len(retainedLogs), len(retainedMetrics), m.buffer.EvictedCount())
	}
}

func TestTAPConnectDrainAndUsageHandler(t *testing.T) {
	release := make(chan struct{})
	peer := httptest.NewServer(websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		frames := []string{
			`{"resourceSpans":[{"scopeSpans":[{"spans":[{"name":"one"},{"name":"two"}]}]}]}`,
			`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"one"}},{"body":{"stringValue":"two"}}]}]}]}`,
			`{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"one","gauge":{"dataPoints":[{"asDouble":1}]}},{"name":"two","gauge":{"dataPoints":[{"asDouble":2}]}}]}]}]}`,
		}
		for _, frame := range frames {
			if err := websocket.Message.Send(conn, frame); err != nil {
				return
			}
		}
		<-release
	}))
	defer peer.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	store := NewRunStore()
	manager := NewRemotetapManager(store)
	defer manager.Close()
	s := &Server{store: store, remotetap: manager, mux: http.NewServeMux()}
	if err := s.registerRoutes(); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"endpoint": strings.TrimPrefix(peer.URL, "http://"), "filters": map[string]any{"maxSpans": 10, "maxBytes": 65536}})
	connect := httptest.NewRecorder()
	s.mux.ServeHTTP(connect, httptest.NewRequest(http.MethodPost, "/api/remotetap/connect", bytes.NewReader(body)))
	if connect.Code != http.StatusOK {
		t.Fatalf("connect: %d %s", connect.Code, connect.Body)
	}
	var result struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(connect.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/remotetap/usage/not-current", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("wrong run usage: %d", w.Code)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		snapshot := store.Get(result.RunID)
		if snapshot != nil && snapshot.Status == RunStatusStreaming && snapshot.LogCountCompleted == 2 && snapshot.MetricCountCompleted == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("streaming counts did not arrive: %+v", snapshot)
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	deadline = time.Now().Add(3 * time.Second)
	for {
		snapshot := store.Get(result.RunID)
		if snapshot != nil && snapshot.Status == RunStatusCompleted {
			if snapshot.SpanCount != 2 || snapshot.LogCount != 2 || snapshot.MetricCount != 2 || snapshot.TraceData == nil || snapshot.LogData == nil || snapshot.MetricData == nil {
				t.Fatalf("capture not drained: %+v", snapshot)
			}
			if snapshot.LogRecordCount != 2 || snapshot.MetricRecordCount != 2 {
				t.Fatalf("retained logs/metrics = %d/%d, want 2/2", snapshot.LogRecordCount, snapshot.MetricRecordCount)
			}
			for _, signal := range []string{"traces", "logs", "metrics"} {
				if _, err := runPayloadForSignal(snapshot, signal); err != nil {
					t.Errorf("%s payload: %v", signal, err)
				}
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("capture did not finish")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestTAPSamplingValidation(t *testing.T) {
	for _, rate := range []float64{0, 0.5, -1} {
		if err := validateTAPFilters(RemotetapFilters{SamplingType: "tail", SamplingRate: rate, MaxSpans: 1, MaxBytes: 1}); err == nil {
			t.Errorf("accepted tail sampling rate %v", rate)
		}
	}
}
