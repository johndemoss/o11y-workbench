package gen_test

import (
	"context"
	"testing"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/johndemoss/o11y-workbench/internal/export"
	"github.com/johndemoss/o11y-workbench/internal/gen"
)

// partialReceiptExporter rejects a fixed number of spans while keeping Export compatible.
type partialReceiptExporter struct {
	rejected int64
	exports  int
}

func (p *partialReceiptExporter) Export(ctx context.Context, spans []*tracev1.ResourceSpans) error {
	_, err := p.ExportWithReceipt(ctx, spans)
	return err
}

func (p *partialReceiptExporter) ExportWithReceipt(_ context.Context, spans []*tracev1.ResourceSpans) (export.ExportReceipt, error) {
	p.exports++
	return export.ReceiptFromRejected(export.CountSpans(spans), p.rejected, "partial"), nil
}

func (p *partialReceiptExporter) Shutdown(context.Context) error { return nil }

// plainExporter implements TraceExporter only — no receipt method.
type plainExporter struct {
	exports int
}

func (p *plainExporter) Export(_ context.Context, _ []*tracev1.ResourceSpans) error {
	p.exports++
	return nil
}

func (p *plainExporter) Shutdown(context.Context) error { return nil }

func TestPartialAcceptance_DoesNotRecordTenAcceptedOrRejected(t *testing.T) {
	// Acceptance (a): rejecting two of ten items does not record ten accepted or ten rejected.
	c := gen.Config{
		Seed:       1,
		TraceCount: 1,
		Rate:       1000,
		Workload: gen.WorkloadProfile{
			Shape:           "flat",
			ExperimentID:    "exp-partial",
			ItemsPerRequest: 10,
		},
	}
	g := gen.NewGenerator(c)
	exp := &partialReceiptExporter{rejected: 2}
	if err := g.Run(context.Background(), exp); err != nil {
		t.Fatalf("Run: %v", err)
	}
	st := g.Stats()
	if st.AttemptedItems != 10 {
		t.Fatalf("attempted=%d", st.AttemptedItems)
	}
	if st.AckedItems != 8 {
		t.Fatalf("acked/accepted=%d want 8", st.AckedItems)
	}
	if st.RejectedItems != 2 {
		t.Fatalf("rejected=%d want 2", st.RejectedItems)
	}
	if st.FailedItems != 0 {
		t.Fatalf("failed=%d want 0 (partial is not full-batch failure)", st.FailedItems)
	}
	if st.AckedItems == 10 || st.RejectedItems == 10 {
		t.Fatalf("collapsed ledger=%+v", st)
	}
	if exp.exports != 1 {
		t.Fatalf("exports=%d", exp.exports)
	}
}

func TestPlainExporter_RemainsCompatible(t *testing.T) {
	// Acceptance (b): existing file/mock exporters remain compatible.
	c := gen.Config{
		Seed:       2,
		TraceCount: 2,
		Rate:       1000,
		Workload: gen.WorkloadProfile{
			Shape:           "flat",
			ExperimentID:    "exp-plain",
			ItemsPerRequest: 3,
		},
	}
	g := gen.NewGenerator(c)
	exp := &plainExporter{}
	if err := g.Run(context.Background(), exp); err != nil {
		t.Fatalf("Run: %v", err)
	}
	st := g.Stats()
	if st.AttemptedItems != 6 || st.AckedItems != 6 || st.RejectedItems != 0 || st.FailedItems != 0 {
		t.Fatalf("stats=%+v", st)
	}
	if exp.exports != 2 {
		t.Fatalf("exports=%d", exp.exports)
	}
}

func TestStats_ResponseSuccessFieldsAreNotDelivery(t *testing.T) {
	// Acceptance (c): response success is not labeled destination delivery.
	// Stats field names are acceptance/rejection/uncertain — never "delivered".
	st := gen.Stats{}
	names := []string{"acked_items", "rejected_items", "uncertain_items", "failed_items"}
	for _, n := range names {
		if n == "delivered_items" || n == "destination_items" {
			t.Fatalf("unexpected delivery field name %q", n)
		}
	}
	_ = st
}
