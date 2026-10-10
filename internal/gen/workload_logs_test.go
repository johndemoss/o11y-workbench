package gen_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"

	"github.com/johndemoss/o11y-workbench/internal/export"
	"github.com/johndemoss/o11y-workbench/internal/gen"
)

type logFailNExporter struct {
	mu       sync.Mutex
	failLeft int
	exports  int
	logs     []*logsv1.ResourceLogs
}

func (m *logFailNExporter) Export(ctx context.Context, logs []*logsv1.ResourceLogs) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exports++
	m.logs = append(m.logs, logs...)
	if m.failLeft > 0 {
		m.failLeft--
		return errors.New("injected log export failure")
	}
	return nil
}

func (m *logFailNExporter) Shutdown(ctx context.Context) error { return nil }

type partialLogReceiptExporter struct {
	rejected int64
	exports  int
}

func (p *partialLogReceiptExporter) Export(ctx context.Context, logs []*logsv1.ResourceLogs) error {
	_, err := p.ExportWithReceipt(ctx, logs)
	return err
}

func (p *partialLogReceiptExporter) ExportWithReceipt(_ context.Context, logs []*logsv1.ResourceLogs) (export.ExportReceipt, error) {
	p.exports++
	return export.ReceiptFromRejected(export.CountLogRecords(logs), p.rejected, "partial"), nil
}

func (p *partialLogReceiptExporter) Shutdown(context.Context) error { return nil }

func TestLogWorkload_LedgerAndPayloadID(t *testing.T) {
	c := gen.Config{
		Seed:        7,
		TraceCount:  3,
		Rate:        1000,
		ServiceName: "evidence-logs",
		Workload: gen.WorkloadProfile{
			Shape:               "flat",
			ExperimentID:        "exp-logs",
			ItemsPerRequest:     4,
			AttributeCount:      2,
			AttributeValueBytes: 3,
			Cardinality:         2,
		},
	}
	g := gen.NewGenerator(c)
	exp := &logFailNExporter{}
	if err := g.RunLogs(context.Background(), exp); err != nil {
		t.Fatalf("RunLogs: %v", err)
	}
	st := g.Stats()
	if st.AttemptedRequests != 3 || st.AckedRequests != 3 || st.FailedRequests != 0 {
		t.Fatalf("request ledger = %+v", st)
	}
	if st.AttemptedItems != 12 || st.AckedItems != 12 || st.FailedItems != 0 {
		t.Fatalf("item ledger = %+v", st)
	}
	var payloadIDs []string
	for _, rl := range exp.logs {
		for _, sl := range rl.ScopeLogs {
			for _, rec := range sl.LogRecords {
				var sawPayload, sawExp bool
				for _, a := range rec.Attributes {
					switch a.Key {
					case "payload.id":
						sawPayload = true
						payloadIDs = append(payloadIDs, a.GetValue().GetStringValue())
					case "experiment.id":
						sawExp = a.GetValue().GetStringValue() == "exp-logs"
					}
				}
				if !sawPayload || !sawExp {
					t.Fatalf("missing evidence attrs on log record")
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
		if !strings.HasPrefix(id, "exp-logs-") {
			t.Fatalf("bad payload prefix %q", id)
		}
	}
}

func TestLogWorkload_PartialAcceptance(t *testing.T) {
	c := gen.Config{
		Seed:       1,
		TraceCount: 1,
		Rate:       1000,
		Workload: gen.WorkloadProfile{
			Shape:           "flat",
			ExperimentID:    "exp-log-partial",
			ItemsPerRequest: 10,
		},
	}
	g := gen.NewGenerator(c)
	exp := &partialLogReceiptExporter{rejected: 2}
	if err := g.RunLogs(context.Background(), exp); err != nil {
		t.Fatalf("RunLogs: %v", err)
	}
	st := g.Stats()
	if st.AttemptedItems != 10 || st.AckedItems != 8 || st.RejectedItems != 2 || st.FailedItems != 0 {
		t.Fatalf("ledger=%+v", st)
	}
}

func TestLogWorkload_ContinueOnExportErr(t *testing.T) {
	c := gen.Config{
		Seed:       1,
		TraceCount: 4,
		Rate:       1000,
		Workload: gen.WorkloadProfile{
			Shape:               "flat",
			ExperimentID:        "exp-log-fail",
			ItemsPerRequest:     2,
			ContinueOnExportErr: true,
		},
	}
	g := gen.NewGenerator(c)
	exp := &logFailNExporter{failLeft: 2}
	if err := g.RunLogs(context.Background(), exp); err != nil {
		t.Fatalf("RunLogs: %v", err)
	}
	st := g.Stats()
	if st.AttemptedRequests != 4 || st.FailedRequests != 2 || st.AckedRequests != 2 {
		t.Fatalf("ledger=%+v", st)
	}
}
