package gen_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/johndemoss/o11y-workbench/internal/export"
	"github.com/johndemoss/o11y-workbench/internal/gen"
)

type metricFailNExporter struct {
	mu       sync.Mutex
	failLeft int
	exports  int
	metrics  []*metricspb.ResourceMetrics
}

func (m *metricFailNExporter) Export(ctx context.Context, metrics []*metricspb.ResourceMetrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exports++
	m.metrics = append(m.metrics, metrics...)
	if m.failLeft > 0 {
		m.failLeft--
		return errors.New("injected metric export failure")
	}
	return nil
}

func (m *metricFailNExporter) Shutdown(ctx context.Context) error { return nil }

type partialMetricReceiptExporter struct {
	rejected int64
	exports  int
}

func (p *partialMetricReceiptExporter) Export(ctx context.Context, metrics []*metricspb.ResourceMetrics) error {
	_, err := p.ExportWithReceipt(ctx, metrics)
	return err
}

func (p *partialMetricReceiptExporter) ExportWithReceipt(_ context.Context, metrics []*metricspb.ResourceMetrics) (export.ExportReceipt, error) {
	p.exports++
	return export.ReceiptFromRejected(export.CountDataPoints(metrics), p.rejected, "partial"), nil
}

func (p *partialMetricReceiptExporter) Shutdown(context.Context) error { return nil }

func TestMetricWorkload_LedgerAndPayloadID(t *testing.T) {
	c := gen.Config{
		Seed:        7,
		TraceCount:  3,
		Rate:        1000,
		ServiceName: "evidence-metrics",
		Workload: gen.WorkloadProfile{
			Shape:               "flat",
			ExperimentID:        "exp-metrics",
			ItemsPerRequest:     4,
			AttributeCount:      2,
			AttributeValueBytes: 3,
			Cardinality:         2,
		},
	}
	g := gen.NewGenerator(c)
	exp := &metricFailNExporter{}
	if err := g.RunMetrics(context.Background(), exp); err != nil {
		t.Fatalf("RunMetrics: %v", err)
	}
	st := g.Stats()
	if st.AttemptedRequests != 3 || st.AckedRequests != 3 || st.FailedRequests != 0 {
		t.Fatalf("request ledger = %+v", st)
	}
	if st.AttemptedItems != 12 || st.AckedItems != 12 || st.FailedItems != 0 {
		t.Fatalf("item ledger = %+v", st)
	}
	var payloadIDs []string
	for _, rm := range exp.metrics {
		for _, sm := range rm.ScopeMetrics {
			for _, metric := range sm.Metrics {
				gauge := metric.GetGauge()
				if gauge == nil {
					t.Fatal("expected gauge metric")
				}
				for _, dp := range gauge.DataPoints {
					var sawPayload, sawExp bool
					for _, a := range dp.Attributes {
						switch a.Key {
						case "payload.id":
							sawPayload = true
							payloadIDs = append(payloadIDs, a.GetValue().GetStringValue())
						case "experiment.id":
							sawExp = a.GetValue().GetStringValue() == "exp-metrics"
						}
					}
					if !sawPayload || !sawExp {
						t.Fatalf("missing evidence attrs on metric data point")
					}
				}
			}
		}
	}
	if len(payloadIDs) != 12 {
		t.Fatalf("payload ids=%d", len(payloadIDs))
	}
	seen := map[string]bool{}
	for _, id := range payloadIDs {
		if seen[id] {
			t.Fatalf("duplicate payload.id %q", id)
		}
		seen[id] = true
		if !strings.HasPrefix(id, "exp-metrics-") {
			t.Fatalf("bad payload prefix %q", id)
		}
	}
}

func TestMetricWorkload_PartialAcceptance(t *testing.T) {
	c := gen.Config{
		Seed:       1,
		TraceCount: 1,
		Rate:       1000,
		Workload: gen.WorkloadProfile{
			Shape:           "flat",
			ExperimentID:    "exp-metric-partial",
			ItemsPerRequest: 10,
		},
	}
	g := gen.NewGenerator(c)
	exp := &partialMetricReceiptExporter{rejected: 2}
	if err := g.RunMetrics(context.Background(), exp); err != nil {
		t.Fatalf("RunMetrics: %v", err)
	}
	st := g.Stats()
	if st.AttemptedItems != 10 || st.AckedItems != 8 || st.RejectedItems != 2 || st.FailedItems != 0 {
		t.Fatalf("ledger=%+v", st)
	}
}

func TestMetricWorkload_ContinueOnExportErr(t *testing.T) {
	c := gen.Config{
		Seed:       1,
		TraceCount: 4,
		Rate:       1000,
		Workload: gen.WorkloadProfile{
			Shape:               "flat",
			ExperimentID:        "exp-metric-fail",
			ItemsPerRequest:     2,
			ContinueOnExportErr: true,
		},
	}
	g := gen.NewGenerator(c)
	exp := &metricFailNExporter{failLeft: 2}
	if err := g.RunMetrics(context.Background(), exp); err != nil {
		t.Fatalf("RunMetrics: %v", err)
	}
	st := g.Stats()
	if st.AttemptedRequests != 4 || st.FailedRequests != 2 || st.AckedRequests != 2 {
		t.Fatalf("ledger=%+v", st)
	}
}
