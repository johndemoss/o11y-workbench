package export

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type partialTraceServer struct {
	collectortracev1.UnimplementedTraceServiceServer
	rejected int64
	msg      string
	fail     error
}

func (s *partialTraceServer) Export(_ context.Context, _ *collectortracev1.ExportTraceServiceRequest) (*collectortracev1.ExportTraceServiceResponse, error) {
	if s.fail != nil {
		return nil, s.fail
	}
	resp := &collectortracev1.ExportTraceServiceResponse{}
	if s.rejected > 0 || s.msg != "" {
		resp.PartialSuccess = &collectortracev1.ExportTracePartialSuccess{
			RejectedSpans: s.rejected,
			ErrorMessage:  s.msg,
		}
	}
	return resp, nil
}

func newBufOTLPTraces(t *testing.T, srv collectortracev1.TraceServiceServer) (*TracesExporter, func()) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	collectortracev1.RegisterTraceServiceServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	exp := &TracesExporter{
		conn:     conn,
		client:   collectortracev1.NewTraceServiceClient(conn),
		endpoint: "bufnet",
	}
	return exp, func() {
		_ = exp.Shutdown(context.Background())
		gs.Stop()
		_ = lis.Close()
	}
}

func tenSpans() []*tracev1.ResourceSpans {
	spans := make([]*tracev1.Span, 10)
	for i := range spans {
		spans[i] = &tracev1.Span{Name: "s"}
	}
	return []*tracev1.ResourceSpans{{
		ScopeSpans: []*tracev1.ScopeSpans{{Spans: spans}},
	}}
}

func TestTracesExporter_PartialAcceptanceReceipt(t *testing.T) {
	exp, cleanup := newBufOTLPTraces(t, &partialTraceServer{rejected: 2, msg: "drop two"})
	defer cleanup()

	receipt, err := exp.ExportWithReceipt(context.Background(), tenSpans())
	if err != nil {
		t.Fatalf("ExportWithReceipt: %v", err)
	}
	if receipt.Outcome != OutcomePartialRejection {
		t.Fatalf("outcome=%s", receipt.Outcome)
	}
	if receipt.AcceptedItems != 8 || receipt.RejectedItems != 2 || receipt.AttemptedItems != 10 {
		t.Fatalf("receipt=%+v want accepted=8 rejected=2 attempted=10", receipt)
	}
	// Export itself must not treat partial rejection as full-batch failure.
	if err := exp.Export(context.Background(), tenSpans()); err != nil {
		t.Fatalf("Export on partial: %v", err)
	}
}

func TestTracesExporter_RPCFailureReceipt(t *testing.T) {
	exp, cleanup := newBufOTLPTraces(t, &partialTraceServer{fail: errors.New("unavailable")})
	defer cleanup()

	receipt, err := exp.ExportWithReceipt(context.Background(), tenSpans())
	if err == nil {
		t.Fatal("expected RPC error")
	}
	if receipt.Outcome != OutcomeRPCFailure || receipt.AttemptedItems != 10 || receipt.AcceptedItems != 0 {
		t.Fatalf("receipt=%+v", receipt)
	}
}

func TestTracesExporter_FullSuccessReceipt(t *testing.T) {
	exp, cleanup := newBufOTLPTraces(t, &partialTraceServer{})
	defer cleanup()

	receipt, err := exp.ExportWithReceipt(context.Background(), tenSpans())
	if err != nil {
		t.Fatalf("ExportWithReceipt: %v", err)
	}
	if receipt.Outcome != OutcomeFullSuccess || receipt.AcceptedItems != 10 || receipt.RejectedItems != 0 {
		t.Fatalf("receipt=%+v", receipt)
	}
}

func TestTimeoutWrapper_ForwardsPartialReceipt(t *testing.T) {
	inner, cleanup := newBufOTLPTraces(t, &partialTraceServer{rejected: 2})
	defer cleanup()
	wrapped := WithExportTimeout(inner, time.Second)
	re, ok := wrapped.(interface {
		ExportWithReceipt(context.Context, []*tracev1.ResourceSpans) (ExportReceipt, error)
	})
	if !ok {
		t.Fatal("timeout wrapper must expose ExportWithReceipt")
	}
	receipt, err := re.ExportWithReceipt(context.Background(), tenSpans())
	if err != nil {
		t.Fatalf("ExportWithReceipt: %v", err)
	}
	if receipt.AcceptedItems != 8 || receipt.RejectedItems != 2 {
		t.Fatalf("receipt=%+v", receipt)
	}
}
