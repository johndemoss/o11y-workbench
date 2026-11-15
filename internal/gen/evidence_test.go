package gen_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/johndemoss/o11y-workbench/internal/gen"
)

type failNExporter struct {
	mu       sync.Mutex
	failLeft int
	exports  int
	spans    []*tracev1.ResourceSpans
}

func (m *failNExporter) Export(ctx context.Context, spans []*tracev1.ResourceSpans) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exports++
	m.spans = append(m.spans, spans...)
	if m.failLeft > 0 {
		m.failLeft--
		return errors.New("injected export failure")
	}
	return nil
}

func (m *failNExporter) Shutdown(ctx context.Context) error { return nil }
func TestWorkloadProfile_LedgerAndPayloadID(t *testing.T) {
	c := gen.Config{
		Regions: testRegions, Signals: testSignals,
		Seed:        7,
		TraceCount:  3,
		Rate:        1000,
		ServiceName: "evidence-svc",
		BaseTime:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Workload: gen.WorkloadProfile{
			Shape:               "flat",
			ExperimentID:        "exp-a",
			ItemsPerRequest:     4,
			AttributeCount:      2,
			AttributeValueBytes: 3,
			Cardinality:         2,
		},
	}
	g := gen.NewGenerator(c)
	exp := &failNExporter{}
	if err := g.Run(context.Background(), exp); err != nil {
		t.Fatalf("Run: %v", err)
	}
	st := g.Stats()
	if st.AttemptedRequests != 3 || st.AckedRequests != 3 || st.FailedRequests != 0 {
		t.Fatalf("request ledger = %+v", st)
	}
	if st.AttemptedItems != 12 || st.AckedItems != 12 || st.FailedItems != 0 {
		t.Fatalf("item ledger = %+v", st)
	}
	if exp.exports != 3 {
		t.Fatalf("exports=%d want 3", exp.exports)
	}
	var payloadIDs []string
	for _, rs := range exp.spans {
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				var sawPayload, sawExp bool
				for _, a := range sp.Attributes {
					switch a.Key {
					case "payload.id":
						sawPayload = true
						payloadIDs = append(payloadIDs, a.GetValue().GetStringValue())
					case "experiment.id":
						sawExp = a.GetValue().GetStringValue() == "exp-a"
					}
				}
				if !sawPayload || !sawExp {
					t.Fatalf("missing evidence attrs")
				}
				attrVals := 0
				for _, at := range sp.Attributes {
					if strings.HasPrefix(at.Key, "attr.") {
						attrVals++
						if got := at.GetValue().GetStringValue(); got != "xxx" {
							t.Fatalf("bad attr value %q", got)
						}
					}
				}
				if attrVals != 2 {
					t.Fatalf("attr count=%d", attrVals)
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
			t.Fatalf("duplicate payload.id")
		}
		seen[id] = true
		if !strings.HasPrefix(id, "exp-a-") {
			t.Fatalf("bad payload prefix")
		}
	}
}

func TestWorkloadProfile_ContinueOnExportErrCountsFailures(t *testing.T) {
	c := gen.Config{
		Regions: testRegions, Signals: testSignals,
		Seed:       1,
		TraceCount: 4,
		Rate:       1000,
		Workload: gen.WorkloadProfile{
			Shape:               "flat",
			ExperimentID:        "exp-fail",
			ItemsPerRequest:     2,
			ContinueOnExportErr: true,
		},
	}
	g := gen.NewGenerator(c)
	exp := &failNExporter{failLeft: 2}
	if err := g.Run(context.Background(), exp); err != nil {
		t.Fatalf("Run: %v", err)
	}
	st := g.Stats()
	if st.AttemptedRequests != 4 || st.FailedRequests != 2 || st.AckedRequests != 2 {
		t.Fatalf("ledger=%+v", st)
	}
	if st.FailedItems != 4 || st.AckedItems != 4 {
		t.Fatalf("item ledger=%+v", st)
	}
}

func TestWorkloadProfile_BurstOffWindowProducesNoExports(t *testing.T) {
	c := gen.Config{
		Regions: testRegions, Signals: testSignals,
		Seed:     1,
		Duration: 35 * time.Millisecond,
		Workload: gen.WorkloadProfile{
			Shape:           "flat",
			ExperimentID:    "exp-burst",
			ItemsPerRequest: 1,
			Mode:            "burst",
			Burst: gen.BurstConfig{
				On:                  0,
				Off:                 time.Hour,
				PeakRateItemsPerSec: 1000,
			},
		},
	}
	g := gen.NewGenerator(c)
	exp := &failNExporter{}
	if err := g.Run(context.Background(), exp); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exp.exports != 0 {
		t.Fatalf("exports=%d want 0", exp.exports)
	}
	if g.Stats().AttemptedRequests != 0 {
		t.Fatalf("attempted=%d", g.Stats().AttemptedRequests)
	}
}

func TestTopologyMode_StillTracksStats(t *testing.T) {
	c := gen.Config{
		Regions: testRegions, Signals: testSignals,
		Seed:          42,
		TraceCount:    2,
		Topology:      gen.TopologyFlat,
		SpanCount:     3,
		MaxDepth:      5,
		ServiceName:   "topo",
		BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: time.Second,
	}
	g := gen.NewGenerator(c)
	exp := &failNExporter{}
	if err := g.Run(context.Background(), exp); err != nil {
		t.Fatalf("Run: %v", err)
	}
	st := g.Stats()
	if st.AttemptedRequests == 0 || st.AckedRequests == 0 || st.FailedRequests != 0 {
		t.Fatalf("stats=%+v", st)
	}
	if st.AttemptedItems != st.AckedItems {
		t.Fatalf("items mismatch %+v", st)
	}
}

func TestWorkloadProfile_ComposesWithTopology(t *testing.T) {
	c := gen.Config{
		Regions: testRegions, Signals: testSignals,
		Seed:          42,
		TraceCount:    1,
		Topology:      gen.TopologyFlat,
		SpanCount:     3,
		MaxDepth:      5,
		ServiceName:   "topo",
		BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: time.Second,
		Workload: gen.WorkloadProfile{
			ExperimentID:        "exp-compose",
			SingleResource:      true,
			AttributeCount:      1,
			AttributeValueBytes: 2,
			Cardinality:         2,
		},
	}
	g := gen.NewGenerator(c)
	exp := &failNExporter{}
	if err := g.Run(context.Background(), exp); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exp.exports != 1 {
		t.Fatalf("exports=%d want 1 (single_resource)", exp.exports)
	}
	var names []string
	for _, rs := range exp.spans {
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				names = append(names, sp.Name)
				var sawExp, sawPayload bool
				for _, a := range sp.Attributes {
					switch a.Key {
					case "experiment.id":
						sawExp = a.GetValue().GetStringValue() == "exp-compose"
					case "payload.id":
						sawPayload = strings.HasPrefix(a.GetValue().GetStringValue(), "exp-compose-")
					}
				}
				if !sawExp || !sawPayload {
					t.Fatalf("missing evidence attrs on topology span %q", sp.Name)
				}
				if sp.Name == "workload.span" {
					t.Fatalf("topology run fell into flat grammar")
				}
			}
		}
	}
	if len(names) != 3 {
		t.Fatalf("spans=%d names=%v", len(names), names)
	}
}

func TestWorkloadProfile_ChunksItemsPerRequest(t *testing.T) {
	c := gen.Config{
		Regions: testRegions, Signals: testSignals,
		Seed:          7,
		TraceCount:    1,
		Topology:      gen.TopologyFlat,
		SpanCount:     5,
		MaxDepth:      5,
		ServiceName:   "chunk",
		BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: time.Second,
		Workload: gen.WorkloadProfile{
			SingleResource:  true,
			ItemsPerRequest: 2,
			ExperimentID:    "exp-chunk",
		},
	}
	g := gen.NewGenerator(c)
	exp := &failNExporter{}
	if err := g.Run(context.Background(), exp); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// 5 spans chunked by 2 => 3 exports
	if exp.exports != 3 {
		t.Fatalf("exports=%d want 3", exp.exports)
	}
	st := g.Stats()
	if st.AttemptedRequests != 3 || st.AttemptedItems != 5 || st.AckedItems != 5 {
		t.Fatalf("stats=%+v", st)
	}
}

func TestWorkloadProfile_ActiveWithoutFlatKeepsTopology(t *testing.T) {
	c := gen.Config{
		Regions: testRegions, Signals: testSignals,
		Seed:          1,
		TraceCount:    1,
		Topology:      gen.TopologyFlat,
		SpanCount:     2,
		ServiceName:   "keep",
		BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: time.Second,
		Workload: gen.WorkloadProfile{
			ContinueOnExportErr: true, // Active, but not Shape=flat
		},
	}
	g := gen.NewGenerator(c)
	exp := &failNExporter{}
	if err := g.Run(context.Background(), exp); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, rs := range exp.spans {
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				if sp.Name == "workload.span" {
					t.Fatalf("Active() without Shape=flat must not use flat grammar")
				}
			}
		}
	}
}
