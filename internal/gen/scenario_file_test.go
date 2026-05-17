package gen_test

import (
	"testing"

	"github.com/johndemoss/o11y-workbench/internal/gen"
)

func TestParseScenarioFile_MultiYAML(t *testing.T) {
	yaml := `
service-mesh:
  roots:
    - service_name: ingress
      name: GET /api/orders
      kind: SERVER
      min_duration: 10ms
      max_duration: 50ms
      children:
        - service_name: orders
          name: process.order
          kind: INTERNAL
          attributes:
            - key: db.system
              value: postgresql

worker:
  roots:
    - service_name: worker
      name: process.job
      kind: CONSUMER
      is_error: true
`
	scenarios, err := gen.ParseScenarioFile([]byte(yaml), ".yaml")
	if err != nil {
		t.Fatalf("ParseScenarioFile(.yaml): %v", err)
	}
	if len(scenarios) != 2 {
		t.Fatalf("expected 2 scenarios, got %d", len(scenarios))
	}
	if _, ok := scenarios["service-mesh"]; !ok {
		t.Fatal("expected scenario 'service-mesh'")
	}
	if _, ok := scenarios["worker"]; !ok {
		t.Fatal("expected scenario 'worker'")
	}
	sm := scenarios["service-mesh"]
	if len(sm) != 1 {
		t.Fatalf("service-mesh: expected 1 root, got %d", len(sm))
	}
	if sm[0].ServiceName != "ingress" {
		t.Errorf("service-mesh root: expected service_name 'ingress', got %q", sm[0].ServiceName)
	}
	if sm[0].IsError {
		t.Error("service-mesh root: expected is_error=false")
	}
	wk := scenarios["worker"]
	if len(wk) != 1 {
		t.Fatalf("worker: expected 1 root, got %d", len(wk))
	}
	if !wk[0].IsError {
		t.Error("worker root: expected is_error=true")
	}
}

func TestParseScenarioFile_MultiJSON(t *testing.T) {
	json := `{
		"service-mesh": {
			"roots": [
				{
					"service_name": "ingress",
					"name": "GET /api/orders",
					"kind": "SERVER",
					"children": [
						{
							"service_name": "orders",
							"name": "process.order",
							"kind": "INTERNAL"
						}
					]
				}
			]
		},
		"worker": {
			"roots": [
				{
					"service_name": "worker",
					"name": "process.job",
					"kind": "CONSUMER",
					"is_error": true
				}
			]
		}
	}`
	scenarios, err := gen.ParseScenarioFile([]byte(json), ".json")
	if err != nil {
		t.Fatalf("ParseScenarioFile(.json): %v", err)
	}
	if len(scenarios) != 2 {
		t.Fatalf("expected 2 scenarios, got %d", len(scenarios))
	}
	if _, ok := scenarios["service-mesh"]; !ok {
		t.Fatal("expected scenario 'service-mesh'")
	}
	if _, ok := scenarios["worker"]; !ok {
		t.Fatal("expected scenario 'worker'")
	}
}

func TestParseScenarioFile_SingleLegacyJSON(t *testing.T) {
	json := `{
		"name": "my-scenario",
		"roots": [
			{
				"service_name": "frontend",
				"name": "GET /users",
				"kind": "SERVER"
			}
		]
	}`
	scenarios, err := gen.ParseScenarioFile([]byte(json), ".json")
	if err != nil {
		t.Fatalf("ParseScenarioFile legacy JSON: %v", err)
	}
	if len(scenarios) != 1 {
		t.Fatalf("expected 1 scenario, got %d", len(scenarios))
	}
	roots, ok := scenarios["my-scenario"]
	if !ok {
		t.Fatal("expected scenario 'my-scenario'")
	}
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0].ServiceName != "frontend" {
		t.Errorf("expected service_name 'frontend', got %q", roots[0].ServiceName)
	}
}

func TestParseScenarioFile_SingleLegacyYAML(t *testing.T) {
	yaml := `
name: my-yaml-scenario
roots:
  - service_name: backend
    name: query.db
    kind: CLIENT
`
	scenarios, err := gen.ParseScenarioFile([]byte(yaml), ".yaml")
	if err != nil {
		t.Fatalf("ParseScenarioFile legacy YAML: %v", err)
	}
	if len(scenarios) != 1 {
		t.Fatalf("expected 1 scenario, got %d", len(scenarios))
	}
	roots, ok := scenarios["my-yaml-scenario"]
	if !ok {
		t.Fatal("expected scenario 'my-yaml-scenario'")
	}
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0].ServiceName != "backend" {
		t.Errorf("expected service_name 'backend', got %q", roots[0].ServiceName)
	}
}

func TestParseScenarioFile_AutoDetect(t *testing.T) {
	json := `{"my-scenario": {"roots": [{"service_name": "test", "name": "op", "kind": "INTERNAL"}]}}`
	scenarios, err := gen.ParseScenarioFile([]byte(json), "")
	if err != nil {
		t.Fatalf("ParseScenarioFile auto-detect JSON: %v", err)
	}
	if len(scenarios) != 1 {
		t.Fatalf("expected 1 scenario, got %d", len(scenarios))
	}
}

func TestParseScenarioFile_Errors(t *testing.T) {
	tests := []struct {
		name string
		data string
		ext  string
	}{
		{"empty JSON", "", ".json"},
		{"empty YAML", "", ".yaml"},
		{"invalid JSON", "{bad json}", ".json"},
		{"invalid YAML", ": bad yaml ::", ".yaml"},
		{"JSON empty object", "{}", ".json"},
		{"YAML empty object", "{}", ".yaml"},
		{"missing roots in JSON", `{"my-scenario": {}}`, ".json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gen.ParseScenarioFile([]byte(tc.data), tc.ext)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestParseScenarioFile_BackwardCompat(t *testing.T) {
	json := `{"name": "compat-test", "roots": [{"service_name": "srv", "name": "op", "kind": "SERVER"}]}`
	scenarios, err := gen.ParseScenarioFile([]byte(json), ".json")
	if err != nil {
		t.Fatalf("backward compat parse: %v", err)
	}
	if len(scenarios) != 1 {
		t.Fatalf("expected 1 scenario, got %d", len(scenarios))
	}
	if _, ok := scenarios["compat-test"]; !ok {
		t.Fatal("expected scenario 'compat-test'")
	}
}

func TestParseScenarioFile_Kinds(t *testing.T) {
	yaml := `
kind-test:
  roots:
    - service_name: srv
      name: server-op
      kind: SERVER
    - service_name: srv
      name: client-op
      kind: CLIENT
    - service_name: srv
      name: internal-op
      kind: INTERNAL
    - service_name: srv
      name: producer-op
      kind: PRODUCER
    - service_name: srv
      name: consumer-op
      kind: CONSUMER
`
	scenarios, err := gen.ParseScenarioFile([]byte(yaml), ".yaml")
	if err != nil {
		t.Fatalf("ParseScenarioFile kinds: %v", err)
	}
	roots := scenarios["kind-test"]
	if len(roots) != 5 {
		t.Fatalf("expected 5 roots, got %d", len(roots))
	}
}
