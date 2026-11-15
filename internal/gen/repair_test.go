package gen_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/region"
)

// logFailNExporter and metricFailNExporter fail the next N exports with a
// captured payload; shared by the routing and correlation regression tests.
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

func TestWorkloadValidationRejectsUnsafeInputs(t *testing.T) {
	for _, p := range []gen.WorkloadProfile{
		{Shape: "flat", AttributeValueBytes: -1},
		{ItemsPerRequest: 100001},
		{ItemsPerRequest: 100000, AttributeCount: 1024, AttributeValueBytes: 1024},
		{ItemsPerRequest: 100000, AttributeCount: 1024},
		{RequestsPerSec: 1e-300},
	} {
		cfg := gen.Config{Regions: testRegions, Signals: testSignals, TraceCount: 1, Workload: p}
		if err := gen.ValidateConfig(cfg); err == nil {
			t.Fatalf("accepted %+v", p)
		}
		if err := gen.NewGenerator(cfg).Run(context.Background(), &mockExporter{}); err == nil {
			t.Fatalf("Run accepted %+v", p)
		}
	}
}

func TestFlatIDsAcrossBatchesAndRuns(t *testing.T) {
	cfg := gen.Config{Regions: testRegions, Signals: testSignals, Seed: 42, TraceCount: 2, Workload: gen.WorkloadProfile{Shape: "flat", ItemsPerRequest: 1001}}
	seen := map[string]bool{}
	for range 2 {
		exp := &mockExporter{}
		if err := gen.NewGenerator(cfg).Run(context.Background(), exp); err != nil {
			t.Fatal(err)
		}
		for _, rs := range exp.spans {
			for _, ss := range rs.ScopeSpans {
				for _, sp := range ss.Spans {
					id := fmt.Sprintf("%x/%x", sp.TraceId, sp.SpanId)
					if seen[id] {
						t.Fatalf("duplicate identity %s", id)
					}
					seen[id] = true
				}
			}
		}
	}
	if len(seen) != 4004 {
		t.Fatalf("got %d identities", len(seen))
	}
}

func TestRegisteredScenarioErrorsArePerTrace(t *testing.T) {
	const name = "regression-error-isolation"
	roots, err := gen.ParseUserScenario([]byte(`{"name":"regression-error-isolation","roots":[{"service_name":"svc","name":"root","children":[{"service_name":"svc","name":"child"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = gen.RegisterUserScenario(name, roots); err != nil {
		t.Fatal(err)
	}
	defer gen.DeleteUserScenario(name)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			exp := &mockExporter{}
			err := gen.NewGenerator(gen.Config{Regions: testRegions, Signals: testSignals, Scenario: name, TraceCount: 10, SpanCount: 2, MaxDepth: 1}).Run(context.Background(), exp)
			if err != nil {
				t.Error(err)
				return
			}
			for _, rs := range exp.spans {
				for _, ss := range rs.ScopeSpans {
					errors := 0
					for _, sp := range ss.Spans {
						if sp.Status.GetCode().String() == "STATUS_CODE_ERROR" {
							errors++
						}
					}
					if errors > 1 {
						t.Errorf("errors accumulated: %d", errors)
					}
				}
			}
		}()
	}
	wg.Wait()
	registered, _ := gen.GetUserScenario(name)
	if registered[0].IsError || registered[0].Children[0].IsError {
		t.Fatal("registry mutated")
	}
}

func TestScenarioFileRejectsInvalidRoots(t *testing.T) {
	for _, input := range []string{`{"name":"x","roots":[{"name":"n"}]}`, `{"x":{"roots":[{"service_name":"s","name":"n","kind":"BOGUS"}]}}`, `{"name":"x","roots":[{"service_name":"s","name":"n","children":[{"service_name":"s"}]}]}`} {
		if _, err := gen.ParseScenarioFile([]byte(input), ".json"); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}

func TestCorrelatedLogResourcesMatchSpans(t *testing.T) {
	traces := &mockExporter{}
	cfg := gen.Config{Regions: testRegions, Signals: testSignals, Scenario: "microservice", TraceCount: 1, SpanCount: 10, MaxDepth: 3}
	if err := gen.NewGenerator(cfg).Run(context.Background(), traces); err != nil {
		t.Fatal(err)
	}
	services := map[string]string{}
	for _, rs := range traces.spans {
		svc := ""
		for _, a := range rs.Resource.Attributes {
			if a.Key == "service.name" {
				svc = a.Value.GetStringValue()
			}
		}
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				services[fmt.Sprintf("%x/%x", sp.TraceId, sp.SpanId)] = svc
			}
		}
	}
	logs := &logFailNExporter{}
	lc := gen.DefaultLogConfig()
	lc.LogsPerSpan = 1
	refs := make(chan []gen.SpanRef, 1)
	var batch []gen.SpanRef
	for _, rs := range traces.spans {
		svc := ""
		var ep region.Name
		for _, a := range rs.Resource.Attributes {
			if a.Key == "service.name" {
				svc = a.Value.GetStringValue()
			}
			if a.Key == "cloud.region" {
				ep = region.Name(a.Value.GetStringValue())
			}
		}
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				var ref gen.SpanRef
				copy(ref.TraceID[:], sp.TraceId)
				copy(ref.SpanID[:], sp.SpanId)
				ref.Service = svc
				ref.Region = ep
				batch = append(batch, ref)
			}
		}
	}
	refs <- batch
	close(refs)
	logGen := gen.NewLogGenerator(lc)
	logGen.Regions = testRegions
	if err := logGen.RunCorrelated(context.Background(), refs, logs); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, rl := range logs.logs {
		svc := ""
		for _, a := range rl.Resource.Attributes {
			if a.Key == "service.name" {
				svc = a.Value.GetStringValue()
			}
		}
		for _, sl := range rl.ScopeLogs {
			for _, r := range sl.LogRecords {
				key := fmt.Sprintf("%x/%x", r.TraceId, r.SpanId)
				if services[key] != svc {
					t.Errorf("%s: log service %s, span service %s", key, svc, services[key])
				}
				count++
			}
		}
	}
	if count != len(services) || count == 0 {
		t.Fatalf("logs=%d spans=%d", count, len(services))
	}
}
