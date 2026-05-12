package gen_test

import (
	"testing"

	"github.com/johndemoss/mocktracegen/internal/gen"
)

func TestGenerateScenario_Microservice(t *testing.T) {
	rng := gen.NewSeededRNG(42)
	roots, err := gen.GenerateScenario("microservice", rng, 15, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) < 1 {
		t.Fatal("expected at least one root span tree")
	}
	total := gen.CountScenarioSpans(roots)
	if total < 3 {
		t.Errorf("microservice: expected at least 3 spans, got %d", total)
	}

	names := collectServiceNames(roots)
	if !contains(names, "api-gateway") {
		t.Error("microservice: expected api-gateway service")
	}
}

func TestGenerateScenario_Monolith(t *testing.T) {
	rng := gen.NewSeededRNG(42)
	roots, err := gen.GenerateScenario("monolith", rng, 15, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 {
		t.Errorf("monolith: expected 1 root, got %d", len(roots))
	}
	names := collectServiceNames(roots)
	if !contains(names, "monolith") {
		t.Error("monolith: expected monolith service")
	}
}

func TestGenerateScenario_ThreeTier(t *testing.T) {
	rng := gen.NewSeededRNG(42)
	roots, err := gen.GenerateScenario("3-tier-web-app", rng, 15, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) < 1 {
		t.Fatal("expected at least one root span tree")
	}
	names := collectServiceNames(roots)
	if !contains(names, "web-server") {
		t.Error("3-tier: expected web-server service")
	}
	if !contains(names, "app-server") {
		t.Error("3-tier: expected app-server service")
	}
	if !contains(names, "database") {
		t.Error("3-tier: expected database service")
	}
}

func TestGenerateScenario_EventDriven(t *testing.T) {
	rng := gen.NewSeededRNG(42)
	roots, err := gen.GenerateScenario("event-driven", rng, 15, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) < 2 {
		t.Errorf("event-driven: expected at least 2 roots (publisher + consumer), got %d", len(roots))
	}
	names := collectServiceNames(roots)
	if !contains(names, "publisher") {
		t.Error("event-driven: expected publisher service")
	}
}

func TestGenerateScenario_Determinism(t *testing.T) {
	rng1 := gen.NewSeededRNG(42)
	rng2 := gen.NewSeededRNG(42)

	for _, scenario := range []string{"microservice", "monolith", "3-tier-web-app", "event-driven"} {
		t.Run(scenario, func(t *testing.T) {
			roots1, err := gen.GenerateScenario(scenario, rng1, 15, 5)
			if err != nil {
				t.Fatal(err)
			}
			roots2, err := gen.GenerateScenario(scenario, rng2, 15, 5)
			if err != nil {
				t.Fatal(err)
			}
			c1 := gen.CountScenarioSpans(roots1)
			c2 := gen.CountScenarioSpans(roots2)
			if c1 != c2 {
				t.Errorf("span count mismatch: %d vs %d", c1, c2)
			}
			if len(roots1) != len(roots2) {
				t.Errorf("root count mismatch: %d vs %d", len(roots1), len(roots2))
			}
		})
	}
}

func TestGenerateScenario_Unknown(t *testing.T) {
	rng := gen.NewSeededRNG(42)
	_, err := gen.GenerateScenario("nonexistent", rng, 10, 5)
	if err == nil {
		t.Fatal("expected error for unknown scenario")
	}
}

func TestGenerateScenario_CaseInsensitive(t *testing.T) {
	rng := gen.NewSeededRNG(42)
	variants := []string{"Microservice", "MICROSERVICE", " microservice ", "MicroService"}
	for _, v := range variants {
		t.Run(v, func(t *testing.T) {
			roots, err := gen.GenerateScenario(v, rng, 10, 5)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", v, err)
			}
			if len(roots) == 0 {
				t.Errorf("no roots returned for %q", v)
			}
		})
	}
}

func TestGenerateScenario_EdgeCases(t *testing.T) {
	rng := gen.NewSeededRNG(42)

	t.Run("spanCount=1", func(t *testing.T) {
		for _, s := range []string{"microservice", "monolith", "3-tier-web-app", "event-driven"} {
			roots, err := gen.GenerateScenario(s, rng, 1, 5)
			if err != nil {
				t.Fatalf("%s: %v", s, err)
			}
			if gen.CountScenarioSpans(roots) < 1 {
				t.Errorf("%s: no spans with spanCount=1", s)
			}
		}
	})

	t.Run("maxDepth=0", func(t *testing.T) {
		roots, err := gen.GenerateScenario("monolith", rng, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if gen.CountScenarioSpans(roots) < 1 {
			t.Error("monolith: no spans with maxDepth=0")
		}
	})
}

func TestGenerateScenario_ErrorMarking(t *testing.T) {
	for _, scenario := range []string{"microservice", "monolith", "3-tier-web-app", "event-driven"} {
		t.Run(scenario, func(t *testing.T) {
			rng := gen.NewSeededRNG(42)
			roots, err := gen.GenerateScenario(scenario, rng, 20, 5)
			if err != nil {
				t.Fatal(err)
			}
			hasErrors := false
			var walk func(*gen.ScenarioSpanNode)
			walk = func(n *gen.ScenarioSpanNode) {
				if n.IsError {
					hasErrors = true
				}
				for _, c := range n.Children {
					walk(c)
				}
			}
			for _, r := range roots {
				walk(r)
			}
			if !hasErrors {
				t.Errorf("%s: no error spans marked", scenario)
			}
		})
	}
}

func TestCountScenarioSpans(t *testing.T) {
	if gen.CountScenarioSpans(nil) != 0 {
		t.Error("CountScenarioSpans(nil) should be 0")
	}
	if gen.CountScenarioSpans([]*gen.ScenarioSpanNode{}) != 0 {
		t.Error("CountScenarioSpans([]) should be 0")
	}
}

func TestGenerateScenario_BudgetVariation(t *testing.T) {
	for _, scenario := range []string{"microservice", "monolith", "3-tier-web-app", "event-driven"} {
		t.Run(scenario, func(t *testing.T) {
			rng := gen.NewSeededRNG(42)
			small, _ := gen.GenerateScenario(scenario, rng, 5, 5)

			rng2 := gen.NewSeededRNG(99)
			large, _ := gen.GenerateScenario(scenario, rng2, 30, 5)

			if gen.CountScenarioSpans(large) < gen.CountScenarioSpans(small) {
				t.Errorf("%s: larger budget should produce at least as many spans", scenario)
			}
		})
	}
}

func collectServiceNames(roots []*gen.ScenarioSpanNode) []string {
	var names []string
	seen := make(map[string]bool)
	var walk func(*gen.ScenarioSpanNode)
	walk = func(n *gen.ScenarioSpanNode) {
		if n != nil && !seen[n.ServiceName] {
			seen[n.ServiceName] = true
			names = append(names, n.ServiceName)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	return names
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
