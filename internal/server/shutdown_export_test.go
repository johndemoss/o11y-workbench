package server

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
)

type stalledShutdownCollector struct {
	collectortracev1.UnimplementedTraceServiceServer
	started, canceled chan struct{}
	once              sync.Once
}

func (s *stalledShutdownCollector) Export(ctx context.Context, _ *collectortracev1.ExportTraceServiceRequest) (*collectortracev1.ExportTraceServiceResponse, error) {
	s.once.Do(func() { close(s.started) })
	<-ctx.Done()
	close(s.canceled)
	return nil, ctx.Err()
}

func TestServerCloseCancelsBlockedOTLPExport(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	collector := &stalledShutdownCollector{started: make(chan struct{}), canceled: make(chan struct{})}
	grpcServer := grpc.NewServer()
	collectortracev1.RegisterTraceServiceServer(grpcServer, collector)
	go func() { _ = grpcServer.Serve(listener) }()
	defer grpcServer.Stop()
	store := NewRunStore()
	srv := &Server{store: store}
	run, err := StartRun(context.Background(), store, gen.Config{Rate: 100, SpanCount: 1, Topology: gen.TopologyFlat, Signals: gen.Signals{Traces: true}}, []string{listener.Addr().String()}, true, "otlp")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-collector.started:
	case <-time.After(3 * time.Second):
		t.Fatal("export never reached the live collector")
	}
	done := make(chan error, 1)
	go func() { done <- srv.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close waited for detached export timeout instead of canceling the RPC")
	}
	select {
	case <-collector.canceled:
	case <-time.After(time.Second):
		t.Error("collector RPC was not canceled")
	}
	if snapshot := store.Get(run.ID); snapshot.Status != RunStatusStopped {
		t.Errorf("shutdown became %s rather than stopped", snapshot.Status)
	}
}
