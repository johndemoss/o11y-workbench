package ottl

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

// TransformConfigMeta captures parsed transform groups and whether the YAML was structured.
type TransformConfigMeta struct {
	Structured   bool
	TraceGroups  []ContextStatements
	LogGroups    []ContextStatements
	MetricGroups []ContextStatements
}

// FilterConditionsBySignal holds filter conditions for each signal in a structured config.
type FilterConditionsBySignal struct {
	Traces  []string
	Logs    []string
	Metrics []string
}

func filterConfigHasSections(cfg FilterConfig) bool {
	return len(cfg.Traces.SpanConditions) > 0 ||
		len(cfg.Logs.LogRecordConditions) > 0 ||
		len(cfg.Metrics.Conditions) > 0
}

func parseStructuredFilterConfig(configYAML string) (FilterConfig, bool) {
	var cfg FilterConfig
	if err := yaml.Unmarshal([]byte(configYAML), &cfg); err == nil && filterConfigHasSections(cfg) {
		return cfg, true
	}
	var wrapped filterWrapper
	if err := yaml.Unmarshal([]byte(configYAML), &wrapped); err == nil && filterConfigHasSections(wrapped.Filter) {
		return wrapped.Filter, true
	}
	return FilterConfig{}, false
}

func populatedFilterSections(cfg FilterConfig) int {
	n := 0
	if len(cfg.Traces.SpanConditions) > 0 {
		n++
	}
	if len(cfg.Logs.LogRecordConditions) > 0 {
		n++
	}
	if len(cfg.Metrics.Conditions) > 0 {
		n++
	}
	return n
}

func primaryFilterConditions(cfg FilterConfig) []string {
	if populatedFilterSections(cfg) > 1 {
		return cfg.Traces.SpanConditions
	}
	if len(cfg.Traces.SpanConditions) > 0 {
		return cfg.Traces.SpanConditions
	}
	if len(cfg.Logs.LogRecordConditions) > 0 {
		return cfg.Logs.LogRecordConditions
	}
	return cfg.Metrics.Conditions
}

func transformConfigHasAnySection(cfg TransformConfig) bool {
	return len(cfg.TraceStatements) > 0 ||
		len(cfg.LogStatements) > 0 ||
		len(cfg.MetricStatements) > 0
}

func cloneContextStatementGroups(groups []ContextStatements) []ContextStatements {
	if len(groups) == 0 {
		return nil
	}
	out := make([]ContextStatements, len(groups))
	for i, g := range groups {
		out[i].Context = g.Context
		if len(g.Conditions) > 0 {
			out[i].Conditions = append([]string(nil), g.Conditions...)
		}
		if len(g.Statements) > 0 {
			out[i].Statements = append([]string(nil), g.Statements...)
		}
	}
	return out
}

// ParseTransformConfigMeta parses structured transform YAML and preserves context/condition groups.
// Unstructured inputs return Structured=false with statements placed in TraceGroups.
func ParseTransformConfigMeta(configYAML string) (TransformConfigMeta, error) {
	configYAML = strings.TrimSpace(configYAML)
	cleaned := trimTrailingSpaces(configYAML)
	if cleaned == "" {
		return TransformConfigMeta{}, fmt.Errorf("empty transform config")
	}

	var cfg TransformConfig
	if err := yaml.Unmarshal([]byte(cleaned), &cfg); err == nil && transformConfigHasAnySection(cfg) {
		return TransformConfigMeta{
			Structured:   true,
			TraceGroups:  cloneContextStatementGroups(cfg.TraceStatements),
			LogGroups:    cloneContextStatementGroups(cfg.LogStatements),
			MetricGroups: cloneContextStatementGroups(cfg.MetricStatements),
		}, nil
	}

	var wrapped transformWrapper
	if err := yaml.Unmarshal([]byte(cleaned), &wrapped); err == nil && transformConfigHasAnySection(wrapped.Transform) {
		t := wrapped.Transform
		return TransformConfigMeta{
			Structured:   true,
			TraceGroups:  cloneContextStatementGroups(t.TraceStatements),
			LogGroups:    cloneContextStatementGroups(t.LogStatements),
			MetricGroups: cloneContextStatementGroups(t.MetricStatements),
		}, nil
	}

	if LooksLikeFilterConfig(cleaned) {
		return TransformConfigMeta{}, ErrLooksLikeFilterConfig
	}
	stmts, err := ParseTransformStatements(configYAML)
	if err != nil {
		return TransformConfigMeta{}, err
	}
	if len(stmts) == 0 {
		return TransformConfigMeta{}, fmt.Errorf("no OTTL statements found in config")
	}
	return TransformConfigMeta{
		Structured:  false,
		TraceGroups: []ContextStatements{{Statements: stmts}},
	}, nil
}

// ParseFilterConditionsBySignal returns per-signal filter conditions from structured YAML.
// For unstructured configs, conditions are returned only under Traces; structured is false.
func ParseFilterConditionsBySignal(configYAML string) (FilterConditionsBySignal, bool, error) {
	configYAML = strings.TrimSpace(configYAML)
	if configYAML == "" {
		return FilterConditionsBySignal{}, false, fmt.Errorf("empty filter config")
	}
	if cfg, ok := parseStructuredFilterConfig(configYAML); ok {
		return FilterConditionsBySignal{
			Traces:  append([]string(nil), cfg.Traces.SpanConditions...),
			Logs:    append([]string(nil), cfg.Logs.LogRecordConditions...),
			Metrics: append([]string(nil), cfg.Metrics.Conditions...),
		}, true, nil
	}
	conds, err := ParseFilterConditions(configYAML)
	if err != nil {
		return FilterConditionsBySignal{}, false, err
	}
	return FilterConditionsBySignal{Traces: conds}, false, nil
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
	if err := yaml.Unmarshal([]byte(cleaned), &cfg); err == nil && transformConfigHasAnySection(cfg) {
		stmts := ExtractStatements(cfg.TraceStatements)
		if len(stmts) > 0 {
			return stmts, nil
		}
		return nil, fmt.Errorf("no OTTL statements found in config")
	}

	var wrapped transformWrapper
	if err := yaml.Unmarshal([]byte(cleaned), &wrapped); err == nil && transformConfigHasAnySection(wrapped.Transform) {
		stmts := ExtractStatements(wrapped.Transform.TraceStatements)
		if len(stmts) > 0 {
			return stmts, nil
		}
		return nil, fmt.Errorf("no OTTL statements found in config")
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

	if cfg, ok := parseStructuredFilterConfig(configYAML); ok {
		if conds := primaryFilterConditions(cfg); len(conds) > 0 {
			return conds, nil
		}
		return nil, fmt.Errorf("no OTTL conditions found in config")
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

	meta, err := ParseTransformConfigMeta(configYAML)
	if err != nil {
		return nil, nil, err
	}
	if meta.Structured {
		return ExtractStatements(meta.TraceGroups), ExtractStatements(meta.LogGroups), nil
	}
	return ExtractStatements(meta.TraceGroups), nil, nil
}

// ParseCombinedTransformConfigAll parses a YAML config and returns separate trace, log,
// and metric statement slices. This enables executing trace, log, and metric
// transformations from a single config. For unstructured formats (flat list, lines),
// all statements are returned as trace statements for backward compatibility.
func ParseCombinedTransformConfigAll(configYAML string) (traceStmts, logStmts, metricStmts []string, err error) {
	meta, err := ParseTransformConfigMeta(configYAML)
	if err != nil {
		return nil, nil, nil, err
	}
	if meta.Structured {
		return ExtractStatements(meta.TraceGroups),
			ExtractStatements(meta.LogGroups),
			ExtractStatements(meta.MetricGroups),
			nil
	}
	return ExtractStatements(meta.TraceGroups), nil, nil, nil
}

// ParseCombinedFilterConfig parses a YAML filter config returning separate trace and
// log condition slices.
func ParseCombinedFilterConfig(configYAML string) (traceConditions, logConditions []string, err error) {
	configYAML = strings.TrimSpace(configYAML)
	if configYAML == "" {
		return nil, nil, fmt.Errorf("empty filter config")
	}

	if cfg, ok := parseStructuredFilterConfig(configYAML); ok {
		return cfg.Traces.SpanConditions, cfg.Logs.LogRecordConditions, nil
	}

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

	if cfg, ok := parseStructuredFilterConfig(configYAML); ok {
		return cfg.Traces.SpanConditions, cfg.Logs.LogRecordConditions, cfg.Metrics.Conditions, nil
	}

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
