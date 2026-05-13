package gen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ScenarioFileRoots holds the roots for one scenario in a multi-scenario file.
type ScenarioFileRoots struct {
	Roots []ScenarioNodeDef `json:"roots" yaml:"roots"`
}

// ParseScenarioFile parses a YAML or JSON file containing one or more scenario
// definitions. Returns a map of scenario name → parsed tree.
//
// ext is the file extension (".yaml", ".yml", ".json"). When empty, attempts
// JSON first, then YAML.
func ParseScenarioFile(data []byte, ext string) (map[string][]*ScenarioSpanNode, error) {
	switch strings.ToLower(ext) {
	case ".yaml", ".yml":
		return parseYAML(data)
	case ".json":
		return parseJSON(data)
	default:
		r, e := parseJSON(data)
		if e == nil {
			return r, nil
		}
		return parseYAML(data)
	}
}

func parseJSON(data []byte) (map[string][]*ScenarioSpanNode, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON scenario file: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("scenario file is empty")
	}
	if _, hasName := raw["name"]; hasName {
		if _, hasRoots := raw["roots"]; hasRoots {
			var def ScenarioDef
			if err := json.Unmarshal(data, &def); err != nil {
				return nil, fmt.Errorf("parse single scenario: %w", err)
			}
			return singleToMap(&def)
		}
	}
	result := make(map[string][]*ScenarioSpanNode, len(raw))
	for name, rawEntry := range raw {
		var entry ScenarioFileRoots
		if err := json.Unmarshal(rawEntry, &entry); err != nil {
			return nil, fmt.Errorf("scenario %q: %w", name, err)
		}
		r, err := validateAndConvert(name, &entry)
		if err != nil {
			return nil, err
		}
		result[name] = r
	}
	return result, nil
}

func parseYAML(data []byte) (map[string][]*ScenarioSpanNode, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid YAML scenario file: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("scenario file is empty")
	}
	if _, hasName := raw["name"]; hasName {
		if _, hasRoots := raw["roots"]; hasRoots {
			var def ScenarioDef
			if err := yaml.Unmarshal(data, &def); err != nil {
				return nil, fmt.Errorf("parse single scenario: %w", err)
			}
			return singleToMap(&def)
		}
	}
	var multi map[string]ScenarioFileRoots
	if err := yaml.Unmarshal(data, &multi); err != nil {
		return nil, fmt.Errorf("parse multi-scenario: %w", err)
	}
	result := make(map[string][]*ScenarioSpanNode, len(multi))
	for name, entry := range multi {
		r, err := validateAndConvert(name, &entry)
		if err != nil {
			return nil, err
		}
		result[name] = r
	}
	return result, nil
}

func singleToMap(def *ScenarioDef) (map[string][]*ScenarioSpanNode, error) {
	name := strings.TrimSpace(def.Name)
	if name == "" {
		return nil, fmt.Errorf("scenario name is required")
	}
	if len(def.Roots) == 0 {
		return nil, fmt.Errorf("scenario %q: at least one root node is required", name)
	}
	roots, err := convertRoots(def.Roots)
	if err != nil {
		return nil, err
	}
	return map[string][]*ScenarioSpanNode{name: roots}, nil
}

func validateAndConvert(name string, entry *ScenarioFileRoots) ([]*ScenarioSpanNode, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("scenario name cannot be empty")
	}
	if len(entry.Roots) == 0 {
		return nil, fmt.Errorf("scenario %q: at least one root node is required", name)
	}
	roots, err := convertRoots(entry.Roots)
	if err != nil {
		return nil, fmt.Errorf("scenario %q: %w", name, err)
	}
	return roots, nil
}

func convertRoots(nodes []ScenarioNodeDef) ([]*ScenarioSpanNode, error) {
	result := make([]*ScenarioSpanNode, 0, len(nodes))
	for i, r := range nodes {
		node, err := nodeDefToSpanNode(r)
		if err != nil {
			return nil, fmt.Errorf("root[%d]: %w", i, err)
		}
		result = append(result, node)
	}
	return result, nil
}

// LoadScenarioFile reads a scenario file from path, parses, and registers all
// scenarios. Returns the registered scenarios map.
func LoadScenarioFile(path string) (map[string][]*ScenarioSpanNode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scenario file: %w", err)
	}
	ext := filepath.Ext(path)
	scenarios, err := ParseScenarioFile(data, ext)
	if err != nil {
		return nil, fmt.Errorf("parse scenario file: %w", err)
	}
	for name, roots := range scenarios {
		if err := RegisterUserScenario(name, roots); err != nil {
			return nil, fmt.Errorf("register %q: %w", name, err)
		}
	}
	return scenarios, nil
}

