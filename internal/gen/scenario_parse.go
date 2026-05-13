package gen

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// ---------------------------------------------------------------------------
// JSON-serializable scenario definition types
// ---------------------------------------------------------------------------

// ScenarioDef is the top-level JSON structure for a user-defined scenario.
type ScenarioDef struct {
	Name  string           `json:"name"`
	Roots []ScenarioNodeDef `json:"roots"`
}

// ScenarioNodeDef is a single node in a user-defined scenario tree.
type ScenarioNodeDef struct {
	ServiceName string            `json:"service_name"`
	Name        string            `json:"name"`
	Kind        string            `json:"kind"`
	Children    []ScenarioNodeDef `json:"children,omitempty"`
	MinDuration string            `json:"min_duration,omitempty"`
	MaxDuration string            `json:"max_duration,omitempty"`
	IsError     bool              `json:"is_error,omitempty"`
	Attributes  []AttributeDef    `json:"attributes,omitempty"`
}

// AttributeDef is a key-value attribute on a scenario node.
type AttributeDef struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// spanKindFromString maps a string to the corresponding SpanKind.
func spanKindFromString(s string) (tracev1.Span_SpanKind, error) {
	switch strings.TrimSpace(s) {
	case "SPAN_KIND_INTERNAL", "INTERNAL", "":
		return tracev1.Span_SPAN_KIND_INTERNAL, nil
	case "SPAN_KIND_SERVER", "SERVER":
		return tracev1.Span_SPAN_KIND_SERVER, nil
	case "SPAN_KIND_CLIENT", "CLIENT":
		return tracev1.Span_SPAN_KIND_CLIENT, nil
	case "SPAN_KIND_PRODUCER", "PRODUCER":
		return tracev1.Span_SPAN_KIND_PRODUCER, nil
	case "SPAN_KIND_CONSUMER", "CONSUMER":
		return tracev1.Span_SPAN_KIND_CONSUMER, nil
	default:
		return 0, fmt.Errorf("unknown span kind %q", s)
	}
}

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

// ParseUserScenario parses a JSON scenario definition and returns the
// corresponding ScenarioSpanNode tree.
func ParseUserScenario(data []byte) ([]*ScenarioSpanNode, error) {
	def, err := parseScenarioDef(data)
	if err != nil {
		return nil, err
	}
	roots := make([]*ScenarioSpanNode, 0, len(def.Roots))
	for _, r := range def.Roots {
		node, err := nodeDefToSpanNode(r)
		if err != nil {
			return nil, fmt.Errorf("scenario %q: %w", def.Name, err)
		}
		roots = append(roots, node)
	}
	return roots, nil
}

// parseScenarioDef validates and parses a JSON scenario definition.
func parseScenarioDef(data []byte) (*ScenarioDef, error) {
	var def ScenarioDef
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("invalid scenario JSON: %w", err)
	}
	if strings.TrimSpace(def.Name) == "" {
		return nil, fmt.Errorf("scenario name is required")
	}
	if len(def.Roots) == 0 {
		return nil, fmt.Errorf("scenario %q: at least one root node is required", def.Name)
	}
	for i, r := range def.Roots {
		if err := validateNodeDef(r); err != nil {
			return nil, fmt.Errorf("scenario %q root[%d]: %w", def.Name, i, err)
		}
	}
	return &def, nil
}

func validateNodeDef(n ScenarioNodeDef) error {
	if strings.TrimSpace(n.ServiceName) == "" {
		return fmt.Errorf("node %q: service_name is required", n.Name)
	}
	if strings.TrimSpace(n.Name) == "" {
		return fmt.Errorf("node: name is required")
	}
	if _, err := spanKindFromString(n.Kind); err != nil {
		return fmt.Errorf("node %q: %w", n.Name, err)
	}
	for i, c := range n.Children {
		if err := validateNodeDef(c); err != nil {
			return fmt.Errorf("children[%d]: %w", i, err)
		}
	}
	return nil
}

func nodeDefToSpanNode(n ScenarioNodeDef) (*ScenarioSpanNode, error) {
	kind, _ := spanKindFromString(n.Kind) // already validated

	minDur := 1 * time.Millisecond
	maxDur := 10 * time.Millisecond
	if n.MinDuration != "" {
		d, err := time.ParseDuration(n.MinDuration)
		if err != nil {
			return nil, fmt.Errorf("node %q: invalid min_duration: %w", n.Name, err)
		}
		if d > 0 {
			minDur = d
		}
	}
	if n.MaxDuration != "" {
		d, err := time.ParseDuration(n.MaxDuration)
		if err != nil {
			return nil, fmt.Errorf("node %q: invalid max_duration: %w", n.Name, err)
		}
		if d > 0 {
			maxDur = d
		}
	}
	if maxDur < minDur {
		maxDur = minDur
	}

	attrs := make([]*commonv1.KeyValue, 0, len(n.Attributes))
	for _, a := range n.Attributes {
		attrs = append(attrs, stringAttr(a.Key, a.Value))
	}

	children := make([]*ScenarioSpanNode, 0, len(n.Children))
	for _, c := range n.Children {
		child, err := nodeDefToSpanNode(c)
		if err != nil {
			return nil, err
		}
		children = append(children, child)
	}

	return &ScenarioSpanNode{
		ServiceName: n.ServiceName,
		Name:        n.Name,
		Kind:        kind,
		Children:    children,
		MinDuration: minDur,
		MaxDuration: maxDur,
		IsError:     n.IsError,
		Attributes:  attrs,
	}, nil
}

// ---------------------------------------------------------------------------
// In-memory user scenario registry
// ---------------------------------------------------------------------------

var (
	userScenariosMu sync.RWMutex
	userScenarios   = map[string][]*ScenarioSpanNode{}
)

// RegisterUserScenario stores a user-defined scenario by name.
// Returns an error if the name is empty or a built-in scenario with that name exists.
func RegisterUserScenario(name string, roots []*ScenarioSpanNode) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("scenario name is required")
	}
	// Check for conflict with built-in scenarios
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "microservice", "monolith", "3-tier-web-app", "event-driven":
		return fmt.Errorf("cannot override built-in scenario %q", name)
	}
	userScenariosMu.Lock()
	defer userScenariosMu.Unlock()
	userScenarios[name] = roots
	return nil
}

// GetUserScenario retrieves a user-defined scenario by name.
func GetUserScenario(name string) ([]*ScenarioSpanNode, bool) {
	userScenariosMu.RLock()
	defer userScenariosMu.RUnlock()
	roots, ok := userScenarios[name]
	return roots, ok
}

// ListUserScenarios returns the names of all registered user scenarios.
func ListUserScenarios() []string {
	userScenariosMu.RLock()
	defer userScenariosMu.RUnlock()
	names := make([]string, 0, len(userScenarios))
	for n := range userScenarios {
		names = append(names, n)
	}
	return names
}

// DeleteUserScenario removes a user-defined scenario by name.
func DeleteUserScenario(name string) {
	userScenariosMu.Lock()
	defer userScenariosMu.Unlock()
	delete(userScenarios, name)
}

// ---------------------------------------------------------------------------
// Integration with existing scenario system
// ---------------------------------------------------------------------------

// getScenario resolves a scenario name — first checks built-in scenarios,
// then falls back to the user-defined registry.
func getScenario(scenario string, rng *SeededRNG, spanCount, maxDepth int) ([]*ScenarioSpanNode, error) {
	// Try built-in first
	roots, err := GenerateScenario(scenario, rng, spanCount, maxDepth)
	if err == nil {
		return roots, nil
	}
	// Fall back to user-defined
	if roots, ok := GetUserScenario(scenario); ok {
		// User scenarios are pre-built; just mark errors like built-in do
		markScenarioErrors(roots, rng)
		return roots, nil
	}
	return nil, fmt.Errorf("unknown scenario %q", scenario)
}
