package gen_test

import (
	"context"
	"testing"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/johndemoss/o11y-workbench/internal/gen"
)

func TestBuiltInScenarioNamesIncludesWorkload(t *testing.T) {
	names := gen.BuiltInScenarioNames()
	found := false
	for _, n := range names {
		if n == "workload" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("BuiltInScenarioNames missing workload: %v", names)
	}
	if !gen.IsWorkloadScenario("workload") || !gen.IsWorkloadScenario(" WorkLoad ") {
		t.Fatal("IsWorkloadScenario failed")
	}
	if gen.IsWorkloadScenario("microservice") {
		t.Fatal("microservice must not be workload")
	}
}

func TestGenerateScenario_WorkloadIsNotATree(t *testing.T) {
	_, err := gen.GenerateScenario("workload", gen.NewSeededRNG(1), 10, 3)
	if err == nil {
		t.Fatal("expected error for workload tree generation")
	}
}

func TestRegisterUserScenario_CannotOverrideWorkload(t *testing.T) {
	err := gen.RegisterUserScenario("workload", nil)
	if err == nil {
		t.Fatal("expected conflict with built-in workload")
	}
}

type captureExp struct {
	batches [][]*tracev1.ResourceSpans
}

func (c *captureExp) Export(ctx context.Context, rs []*tracev1.ResourceSpans) error {
	cp := make([]*tracev1.ResourceSpans, len(rs))
	copy(cp, rs)
	c.batches = append(c.batches, cp)
	return nil
}

func (c *captureExp) Shutdown(ctx context.Context) error { return nil }

func TestGenerator_ScenarioWorkloadUsesFlatGrammar(t *testing.T) {
	exp := &captureExp{}
	g := gen.NewGenerator(gen.Config{
		Seed:        9,
		TraceCount:  2,
		Rate:        1000,
		Duration:    time.Second,
		Scenario:    "workload",
		ServiceName: "svc",
		Signals:     gen.Signals{Traces: true},
		Workload:    gen.WorkloadProfile{ItemsPerRequest: 3},
	})
	if err := g.Run(context.Background(), exp); err != nil {
		t.Fatal(err)
	}
	if len(exp.batches) == 0 {
		t.Fatal("expected exports")
	}
	span := exp.batches[0][0].ScopeSpans[0].Spans[0]
	if span.Name != "workload.span" {
		t.Fatalf("span name=%q", span.Name)
	}
	st := g.Stats()
	if st.AttemptedItems == 0 || st.AckedItems == 0 {
		t.Fatalf("stats=%+v", st)
	}
}
