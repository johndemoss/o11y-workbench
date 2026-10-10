package export_test

import (
	"errors"
	"testing"

	"github.com/johndemoss/o11y-workbench/internal/export"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestReceiptFromRejected_PartialSplit(t *testing.T) {
	r := export.ReceiptFromRejected(10, 2, "drop two")
	if r.Outcome != export.OutcomePartialRejection {
		t.Fatalf("outcome=%s", r.Outcome)
	}
	if r.AcceptedItems != 8 || r.RejectedItems != 2 || r.AttemptedItems != 10 {
		t.Fatalf("receipt=%+v", r)
	}
	// Must not collapse to all-accepted or all-rejected.
	if r.AcceptedItems == 10 || r.RejectedItems == 10 {
		t.Fatalf("collapsed receipt=%+v", r)
	}
}

func TestReceiptFromRejected_FullSuccessAndUncertain(t *testing.T) {
	full := export.ReceiptFromRejected(5, 0, "warn")
	if full.Outcome != export.OutcomeFullSuccess || full.AcceptedItems != 5 || full.RejectedItems != 0 {
		t.Fatalf("full=%+v", full)
	}
	if full.ErrorMessage != "warn" {
		t.Fatalf("msg=%q", full.ErrorMessage)
	}
	bad := export.ReceiptFromRejected(3, 9, "too many")
	if bad.Outcome != export.OutcomeUncertain || bad.UncertainItems != 3 || bad.AcceptedItems != 0 || bad.RejectedItems != 0 {
		t.Fatalf("uncertain=%+v", bad)
	}
}

func TestReceiptRPCFailure(t *testing.T) {
	r := export.ReceiptRPCFailure(4, errors.New("boom"))
	if r.Outcome != export.OutcomeRPCFailure || r.AttemptedItems != 4 || r.AcceptedItems != 0 {
		t.Fatalf("rpc=%+v", r)
	}
}

func TestCountSpans(t *testing.T) {
	spans := []*tracev1.ResourceSpans{
		{ScopeSpans: []*tracev1.ScopeSpans{{
			Spans: []*tracev1.Span{{Name: "a"}, {Name: "b"}},
		}}},
		{ScopeSpans: []*tracev1.ScopeSpans{{
			Spans: []*tracev1.Span{{Name: "c"}},
		}}},
	}
	if got := export.CountSpans(spans); got != 3 {
		t.Fatalf("CountSpans=%d", got)
	}
}

func TestCountDataPoints(t *testing.T) {
	metrics := []*metricspb.ResourceMetrics{{
		ScopeMetrics: []*metricspb.ScopeMetrics{{
			Metrics: []*metricspb.Metric{{
				Name: "m",
				Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
					DataPoints: []*metricspb.NumberDataPoint{{}, {}},
				}},
			}},
		}},
	}}
	if got := export.CountDataPoints(metrics); got != 2 {
		t.Fatalf("CountDataPoints=%d", got)
	}
}
