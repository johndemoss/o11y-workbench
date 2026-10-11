package engine

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type TransformConfig struct {
	TraceStatements  []ContextStatements `yaml:"trace_statements"`
	LogStatements    []ContextStatements `yaml:"log_statements"`
	MetricStatements []ContextStatements `yaml:"metric_statements"`
}

type ContextStatements struct {
	Context    string   `yaml:"context"`
	Conditions []string `yaml:"conditions,omitempty"`
	Statements []string `yaml:"statements"`
}

type FilterConfig struct {
	Traces  TraceFilters     `yaml:"traces"`
	Logs    LogFilters       `yaml:"logs"`
	Metrics DataPointFilters `yaml:"metrics"`
}

type DataPointFilters struct {
	Conditions []string `yaml:"datapoint"`
}

type TraceFilters struct {
	SpanConditions []string `yaml:"span"`
}

type LogFilters struct {
	LogRecordConditions []string `yaml:"log_record"`
}

type transformWrapper struct {
	Transform TransformConfig `yaml:"transform"`
}

type filterWrapper struct {
	Filter FilterConfig `yaml:"filter"`
}

func ParseTransformStatements(configYAML string) ([]string, error) {
	configYAML = strings.TrimSpace(configYAML)
	if configYAML == "" {
		return nil, fmt.Errorf("empty transform config")
	}
	cleaned := trimTrailingSpaces(configYAML)
	if cleaned == "" {
		return nil, fmt.Errorf("empty transform config")
	}

	var cfg TransformConfig
	if err := yaml.Unmarshal([]byte(cleaned), &cfg); err == nil {
		if stmts := cfg.statements(); len(stmts) > 0 {
			return stmts, nil
		}
	}

	var wrapped transformWrapper
	if err := yaml.Unmarshal([]byte(cleaned), &wrapped); err == nil {
		if stmts := wrapped.Transform.statements(); len(stmts) > 0 {
			return stmts, nil
		}
	}

	var simple struct {
		Statements []string `yaml:"statements"`
	}
	if err := yaml.Unmarshal([]byte(cleaned), &simple); err == nil && len(simple.Statements) > 0 {
		return simple.Statements, nil
	}

	var flat []string
	if err := yaml.Unmarshal([]byte(cleaned), &flat); err == nil && len(flat) > 0 {
		return flat, nil
	}

	if LooksLikeFilterConfig(cleaned) {
		return nil, ErrLooksLikeFilterConfig
	}

	stmts := parseLines(cleaned)
	if len(stmts) > 0 {
		return stmts, nil
	}

	return nil, fmt.Errorf("no OTTL statements found in config")
}

func ParseFilterConditions(configYAML string) ([]string, error) {
	configYAML = strings.TrimSpace(configYAML)
	if configYAML == "" {
		return nil, fmt.Errorf("empty filter config")
	}

	var cfg FilterConfig
	if err := yaml.Unmarshal([]byte(configYAML), &cfg); err == nil {
		if conds := cfg.Traces.SpanConditions; len(conds) > 0 {
			return conds, nil
		}
		if conds := cfg.Logs.LogRecordConditions; len(conds) > 0 {
			return conds, nil
		}
	}

	var wrapped filterWrapper
	if err := yaml.Unmarshal([]byte(configYAML), &wrapped); err == nil {
		if conds := wrapped.Filter.Traces.SpanConditions; len(conds) > 0 {
			return conds, nil
		}
		if conds := wrapped.Filter.Logs.LogRecordConditions; len(conds) > 0 {
			return conds, nil
		}
	}

	var flat []string
	if err := yaml.Unmarshal([]byte(configYAML), &flat); err == nil && len(flat) > 0 {
		return flat, nil
	}

	conds := parseLines(configYAML)
	if len(conds) > 0 {
		return conds, nil
	}

	return nil, fmt.Errorf("no OTTL conditions found in config")
}

func (tc *TransformConfig) statements() []string {
	var out []string
	for _, g := range tc.TraceStatements {
		out = append(out, g.Statements...)
	}
	for _, g := range tc.LogStatements {
		out = append(out, g.Statements...)
	}
	for _, g := range tc.MetricStatements {
		out = append(out, g.Statements...)
	}
	return out
}

// ExtractStatements flattens a slice of ContextStatements into a single string slice.
func ExtractStatements(groups []ContextStatements) []string {
	var out []string
	for _, g := range groups {
		out = append(out, g.Statements...)
	}
	return out
}

// ParseCombinedTransformConfig parses a YAML config and returns separate trace and log
// statement slices. This enables executing both trace and log transformations from a
// single config. For unstructured formats (flat list, lines), all statements are
// returned as trace statements for backward compatibility.
func ParseCombinedTransformConfig(configYAML string) (traceStmts, logStmts []string, err error) {
	configYAML = strings.TrimSpace(configYAML)
	cleaned := trimTrailingSpaces(configYAML)
	if cleaned == "" {
		return nil, nil, fmt.Errorf("empty transform config")
	}

	var cfg TransformConfig
	if err := yaml.Unmarshal([]byte(cleaned), &cfg); err == nil {
		t := ExtractStatements(cfg.TraceStatements)
		l := ExtractStatements(cfg.LogStatements)
		if len(t) > 0 || len(l) > 0 {
			return t, l, nil
		}
	}

	var wrapped transformWrapper
	if err := yaml.Unmarshal([]byte(cleaned), &wrapped); err == nil {
		t := ExtractStatements(wrapped.Transform.TraceStatements)
		l := ExtractStatements(wrapped.Transform.LogStatements)
		if len(t) > 0 || len(l) > 0 {
			return t, l, nil
		}
	}

	// Fallback to generic parse (all statements treated as trace for backward compat)
	if LooksLikeFilterConfig(cleaned) {
		return nil, nil, ErrLooksLikeFilterConfig
	}
	stmts, err := ParseTransformStatements(configYAML)
	if err != nil {
		return nil, nil, err
	}
	return stmts, nil, nil
}

// ParseCombinedTransformConfigAll parses a YAML config and returns separate trace, log,
// and metric statement slices. This enables executing trace, log, and metric
// transformations from a single config. For unstructured formats (flat list, lines),
// all statements are returned as trace statements for backward compatibility.
func ParseCombinedTransformConfigAll(configYAML string) (traceStmts, logStmts, metricStmts []string, err error) {
	configYAML = strings.TrimSpace(configYAML)
	cleaned := trimTrailingSpaces(configYAML)
	if cleaned == "" {
		return nil, nil, nil, fmt.Errorf("empty transform config")
	}

	var cfg TransformConfig
	if err := yaml.Unmarshal([]byte(cleaned), &cfg); err == nil {
		t := ExtractStatements(cfg.TraceStatements)
		l := ExtractStatements(cfg.LogStatements)
		m := ExtractStatements(cfg.MetricStatements)
		if len(t) > 0 || len(l) > 0 || len(m) > 0 {
			return t, l, m, nil
		}
	}

	var wrapped transformWrapper
	if err := yaml.Unmarshal([]byte(cleaned), &wrapped); err == nil {
		t := ExtractStatements(wrapped.Transform.TraceStatements)
		l := ExtractStatements(wrapped.Transform.LogStatements)
		m := ExtractStatements(wrapped.Transform.MetricStatements)
		if len(t) > 0 || len(l) > 0 || len(m) > 0 {
			return t, l, m, nil
		}
	}

	// Fallback to generic parse (all statements treated as trace for backward compat)
	if LooksLikeFilterConfig(cleaned) {
		return nil, nil, nil, ErrLooksLikeFilterConfig
	}
	stmts, err := ParseTransformStatements(configYAML)
	if err != nil {
		return nil, nil, nil, err
	}
	return stmts, nil, nil, nil
}

// ParseCombinedFilterConfig parses a YAML filter config returning separate trace and
// log condition slices.
func ParseCombinedFilterConfig(configYAML string) (traceConditions, logConditions []string, err error) {
	configYAML = strings.TrimSpace(configYAML)
	if configYAML == "" {
		return nil, nil, fmt.Errorf("empty filter config")
	}

	var cfg FilterConfig
	if err := yaml.Unmarshal([]byte(configYAML), &cfg); err == nil {
		return cfg.Traces.SpanConditions, cfg.Logs.LogRecordConditions, nil
	}

	var wrapped filterWrapper
	if err := yaml.Unmarshal([]byte(configYAML), &wrapped); err == nil {
		return wrapped.Filter.Traces.SpanConditions, wrapped.Filter.Logs.LogRecordConditions, nil
	}

	// Fallback to generic parse
	conds, err := ParseFilterConditions(configYAML)
	if err != nil {
		return nil, nil, err
	}
	return conds, nil, nil
}

// ParseCombinedFilterConfigAll parses a YAML filter config returning separate trace,
// log, and metric datapoint condition slices.
func ParseCombinedFilterConfigAll(configYAML string) (traceConditions, logConditions, metricConditions []string, err error) {
	configYAML = strings.TrimSpace(configYAML)
	if configYAML == "" {
		return nil, nil, nil, fmt.Errorf("empty filter config")
	}

	var cfg FilterConfig
	if err := yaml.Unmarshal([]byte(configYAML), &cfg); err == nil {
		return cfg.Traces.SpanConditions, cfg.Logs.LogRecordConditions, cfg.Metrics.Conditions, nil
	}

	var wrapped filterWrapper
	if err := yaml.Unmarshal([]byte(configYAML), &wrapped); err == nil {
		return wrapped.Filter.Traces.SpanConditions, wrapped.Filter.Logs.LogRecordConditions, wrapped.Filter.Metrics.Conditions, nil
	}

	// Fallback to generic parse
	conds, err := ParseFilterConditions(configYAML)
	if err != nil {
		return nil, nil, nil, err
	}
	return conds, nil, nil, nil
}

// LooksLikeFilterConfig checks whether the given YAML config matches the
// filter configuration structure (traces → span conditions, logs → log_record
// conditions, metrics → datapoint conditions). This is used to provide
// helpful error messages when users accidentally enter filter-format YAML
// in the Transform handler.
func LooksLikeFilterConfig(configYAML string) bool {
	configYAML = strings.TrimSpace(configYAML)
	if configYAML == "" {
		return false
	}

	var cfg FilterConfig
	if err := yaml.Unmarshal([]byte(configYAML), &cfg); err == nil {
		if len(cfg.Traces.SpanConditions) > 0 || len(cfg.Logs.LogRecordConditions) > 0 || len(cfg.Metrics.Conditions) > 0 {
			return true
		}
	}

	var wrapped filterWrapper
	if err := yaml.Unmarshal([]byte(configYAML), &wrapped); err == nil {
		if len(wrapped.Filter.Traces.SpanConditions) > 0 || len(wrapped.Filter.Logs.LogRecordConditions) > 0 || len(wrapped.Filter.Metrics.Conditions) > 0 {
			return true
		}
	}

	return false
}

// ErrLooksLikeFilterConfig is returned when the input YAML matches the filter
// configuration structure but was submitted to a transform parser.
var ErrLooksLikeFilterConfig = fmt.Errorf("configuration appears to use filter format (uses 'traces:', 'span:', or 'log_record:' keys). Switch to Filter mode in the OTTL Prototyper, or reformat as transform statements")

func trimTrailingSpaces(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func parseLines(s string) []string {
	lines := strings.Split(s, "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}
