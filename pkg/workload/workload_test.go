package workload_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"

	"github.com/johndemoss/o11y-workbench/pkg/workload"
)

// recordingServer is a loopback OTLP traces receiver with deterministic
// synchronization hooks: onExport signals each arrival, block (when set)
// holds responses until closed or the RPC context ends.
type recordingServer struct {
	collectortracev1.UnimplementedTraceServiceServer

	mu       sync.Mutex
	requests int
	items    int
	rejected int64

	onExport chan struct{}
	block    chan struct{}
}

func (s *recordingServer) Export(ctx context.Context, req *collectortracev1.ExportTraceServiceRequest) (*collectortracev1.ExportTraceServiceResponse, error) {
	s.mu.Lock()
	s.requests++
	for _, rs := range req.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			s.items += len(ss.Spans)
		}
	}
	s.mu.Unlock()
	if s.onExport != nil {
		s.onExport <- struct{}{}
	}
	if s.block != nil {
		select {
		case <-s.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.rejected > 0 {
		return &collectortracev1.ExportTraceServiceResponse{
			PartialSuccess: &collectortracev1.ExportTracePartialSuccess{
				RejectedSpans: s.rejected,
			},
		}, nil
	}
	return &collectortracev1.ExportTraceServiceResponse{}, nil
}

func (s *recordingServer) snapshot() (requests, items int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests, s.items
}

// newLoopback serves srv on an ephemeral loopback TCP port and returns the
// dial target.
func newLoopback(t *testing.T, srv collectortracev1.TraceServiceServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer()
	collectortracev1.RegisterTraceServiceServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	return lis.Addr().String()
}

func validConfig(endpoint string) workload.Config {
	return workload.Config{
		Endpoint:        endpoint,
		Insecure:        true,
		Seed:            7,
		Duration:        300 * time.Millisecond,
		ItemsPerRequest: 4,
		RequestsPerSec:  200,
	}
}

// TestRunPartialRejectionAccounting proves transport-acceptance accounting
// through the facade: one request of 10 items with an OTLP partial success
// rejecting 2 yields attempted 10, acked 8, rejected 2. The 2s request
// interval against a 1s duration pins exactly one export, deterministically.
func TestRunPartialRejectionAccounting(t *testing.T) {
	srv := &recordingServer{rejected: 2}
	endpoint := newLoopback(t, srv)

	cfg := validConfig(endpoint)
	cfg.Duration = 1 * time.Second
	cfg.ItemsPerRequest = 10
	cfg.RequestsPerSec = 0.5
	r, err := workload.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	st := r.Stats()
	want := workload.Stats{
		AttemptedRequests: 1,
		AckedRequests:     1,
		AttemptedItems:    10,
		AckedItems:        8,
		RejectedItems:     2,
	}
	if st != want {
		t.Fatalf("stats = %+v, want %+v", st, want)
	}
	reqs, items := srv.snapshot()
	if reqs != 1 || items != 10 {
		t.Fatalf("server saw %d requests / %d items, want 1 / 10", reqs, items)
	}
}

// TestRunCancelBlockedExport verifies owner cancellation of a blocked
// export: Run returns promptly with the cancellation, and once it has
// returned no further requests arrive and the counters are frozen.
func TestRunCancelBlockedExport(t *testing.T) {
	srv := &recordingServer{onExport: make(chan struct{}, 1), block: make(chan struct{})}
	endpoint := newLoopback(t, srv)

	cfg := validConfig(endpoint)
	cfg.Duration = 30 * time.Second
	cfg.RequestsPerSec = 100
	r, err := workload.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()

	select {
	case <-srv.onExport:
	case <-time.After(10 * time.Second):
		t.Fatal("first export never arrived")
	}
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil for canceled blocked export")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	before := r.Stats()
	close(srv.block) // would let a next request through if generation lived on
	reqsBefore, _ := srv.snapshot()
	after := r.Stats()
	reqsAfter, _ := srv.snapshot()
	if before != after {
		t.Fatalf("counters changed after Run returned: %+v -> %+v", before, after)
	}
	if reqsBefore != reqsAfter {
		t.Fatalf("server requests changed after Run returned: %d -> %d", reqsBefore, reqsAfter)
	}
	if before.AttemptedRequests == 0 || before.AttemptedItems == 0 {
		t.Fatalf("canceled run recorded no attempts: %+v", before)
	}
}

// TestRunFinalSnapshotAllAcked checks the normal-completion contract: every
// attempt carries exactly ItemsPerRequest items and every item is acked.
func TestRunFinalSnapshotAllAcked(t *testing.T) {
	srv := &recordingServer{}
	endpoint := newLoopback(t, srv)

	cfg := validConfig(endpoint)
	r, err := workload.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	st := r.Stats()
	if st.AttemptedRequests < 2 {
		t.Fatalf("expected multiple requests, stats %+v", st)
	}
	if st.AttemptedItems != st.AttemptedRequests*4 {
		t.Fatalf("attempted items %d != %d requests * 4", st.AttemptedItems, st.AttemptedRequests)
	}
	if st.AckedRequests != st.AttemptedRequests || st.AckedItems != st.AttemptedItems {
		t.Fatalf("not all acked: %+v", st)
	}
	if st.FailedRequests != 0 || st.RejectedItems != 0 || st.FailedItems != 0 || st.UncertainItems != 0 {
		t.Fatalf("unexpected failures: %+v", st)
	}
	_, items := srv.snapshot()
	if uint64(items) != st.AckedItems {
		t.Fatalf("server items %d != acked items %d", items, st.AckedItems)
	}
}

// TestRunOneShotGuard proves repeat and concurrent Run calls never start a
// second generation.
func TestRunOneShotGuard(t *testing.T) {
	srv := &recordingServer{onExport: make(chan struct{}, 1), block: make(chan struct{})}
	endpoint := newLoopback(t, srv)

	cfg := validConfig(endpoint)
	cfg.Duration = 30 * time.Second
	r, err := workload.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	select {
	case <-srv.onExport:
	case <-time.After(10 * time.Second):
		t.Fatal("first export never arrived")
	}

	concurrent := make(chan error, 1)
	go func() { concurrent <- r.Run(context.Background()) }()
	select {
	case err := <-concurrent:
		if err == nil || !strings.Contains(err.Error(), "workload runner already started") {
			t.Fatalf("concurrent Run error = %v, want already started", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent Run did not return")
	}

	cancel()
	<-done
	if err := r.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "workload runner already started") {
		t.Fatalf("repeat Run error = %v, want already started", err)
	}
}

// TestNewRejectsInvalidConfigs keeps the pre-allocation validation contract.
func TestNewRejectsInvalidConfigs(t *testing.T) {
	burst := func(peak float64) workload.Config {
		c := validConfig("127.0.0.1:4317")
		c.Mode = "burst"
		c.RequestsPerSec = 0
		c.RateItemsPerSec = 0
		c.Burst = workload.Burst{On: time.Second, Off: time.Second, PeakRateItemsPerSec: peak}
		return c
	}
	cases := []struct {
		name string
		cfg  workload.Config
		want string
	}{
		{"empty endpoint", func() workload.Config { c := validConfig(""); return c }(), "endpoint"},
		{"zero duration", func() workload.Config { c := validConfig("127.0.0.1:1"); c.Duration = 0; return c }(), "duration"},
		{"negative duration", func() workload.Config { c := validConfig("127.0.0.1:1"); c.Duration = -time.Second; return c }(), "duration"},
		{"zero items", func() workload.Config { c := validConfig("127.0.0.1:1"); c.ItemsPerRequest = 0; return c }(), "items per request"},
		{"unknown mode", func() workload.Config { c := validConfig("127.0.0.1:1"); c.Mode = "spiky"; return c }(), "constant or burst"},
		{"no rate", func() workload.Config {
			c := validConfig("127.0.0.1:1")
			c.RequestsPerSec = 0
			c.RateItemsPerSec = 0
			return c
		}(), "positive effective request rate"},
		{"burst without peak", burst(0), "positive effective request rate"},
		{"burst missing off", func() workload.Config { c := burst(100); c.Burst.Off = 0; return c }(), "burst"},
		{"generator bounds", func() workload.Config { c := validConfig("127.0.0.1:1"); c.AttributeCount = 4096; return c }(), "invalid workload dimensions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := workload.New(tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("New error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

// TestNewOpensNoConnection pins lazy dialing: an endpoint with no listener
// still constructs a Runner.
func TestNewOpensNoConnection(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	dead := lis.Addr().String()
	lis.Close()

	if _, err := workload.New(validConfig(dead)); err != nil {
		t.Fatalf("New against dead endpoint: %v", err)
	}
}

// TestStatsJSONSchema pins the public JSON field names.
func TestStatsJSONSchema(t *testing.T) {
	b, err := json.Marshal(workload.Stats{
		AttemptedRequests: 1, AckedRequests: 1, FailedRequests: 2,
		AttemptedItems: 3, AckedItems: 4, RejectedItems: 5,
		FailedItems: 6, UncertainItems: 7,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"attempted_requests":1,"acked_requests":1,"failed_requests":2,` +
		`"attempted_items":3,"acked_items":4,"rejected_items":5,` +
		`"failed_items":6,"uncertain_items":7}`
	if string(b) != want {
		t.Fatalf("json = %s, want %s", b, want)
	}
}
