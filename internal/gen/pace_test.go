package gen_test

import (
	"context"
	"testing"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/johndemoss/o11y-workbench/internal/gen"
)

type timedExporter struct {
	starts []time.Time
	cancel context.CancelFunc
}

func (s *timedExporter) Export(_ context.Context, _ []*tracev1.ResourceSpans) error {
	s.starts = append(s.starts, time.Now())
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

func (s *timedExporter) Shutdown(context.Context) error { return nil }

func TestFlatFinalExportDoesNotWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exp := &timedExporter{cancel: cancel}
	cfg := gen.Config{TraceCount: 1, Workload: gen.WorkloadProfile{Shape: "flat", RequestsPerSec: 0.1}}
	if err := gen.NewGenerator(cfg).Run(ctx, exp); err != nil {
		t.Fatalf("final successful export must complete despite cancellation: %v", err)
	}
	if len(exp.starts) != 1 {
		t.Fatalf("exports = %d, want 1", len(exp.starts))
	}
}

func TestTopologyPacesEveryBatch(t *testing.T) {
	exp := &timedExporter{}
	cfg := gen.Config{TraceCount: 1, SpanCount: 4, MaxDepth: 1, Workload: gen.WorkloadProfile{
		SingleResource: true, ItemsPerRequest: 1, RequestsPerSec: 50,
	}}
	if err := gen.NewGenerator(cfg).Run(context.Background(), exp); err != nil {
		t.Fatal(err)
	}
	if len(exp.starts) < 2 {
		t.Fatalf("expected multiple export batches, got %d", len(exp.starts))
	}
	elapsed := exp.starts[len(exp.starts)-1].Sub(exp.starts[0])
	minimum := time.Duration(len(exp.starts)-1) * 15 * time.Millisecond
	if elapsed < minimum {
		t.Fatalf("batch starts spanned %v, want at least %v", elapsed, minimum)
	}
}
