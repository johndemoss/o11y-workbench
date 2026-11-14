package ottlengine

import (
	"encoding/json"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/ottl"
)

const (
	DefaultContribVersion = "0.165.0"
	DefaultStableVersion  = "1.71.0"
)

// VersionInfo describes a registered OTTL engine train.
type VersionInfo struct {
	Contrib string `json:"contrib"` // e.g. "0.165.0"
	Stable  string `json:"stable"`  // e.g. "1.71.0"
	Source  string `json:"source"`  // "in-process" | "worker"
}

// Engine is a version-bound OTTL execution backend.
type Engine interface {
	Info() VersionInfo
	ValidateStatements(statements []string) ottl.ValidateResult
	ValidateFilterConditions(conditions []string, signals gen.Signals) ottl.ValidateResult
	TransformTraces(data json.RawMessage, statements []string) (json.RawMessage, int, error)
	TransformLogs(data json.RawMessage, statements []string) (json.RawMessage, error)
	TransformMetrics(data json.RawMessage, statements []string) (json.RawMessage, error)
	FilterTraces(data json.RawMessage, conditions []string, mode ottl.FilterMode) (json.RawMessage, int, int, error)
	FilterLogs(data json.RawMessage, conditions []string, mode ottl.FilterMode) (json.RawMessage, int, int, error)
	FilterMetrics(data json.RawMessage, conditions []string, mode ottl.FilterMode) (json.RawMessage, int, int, error)
	ExecutePipeline(data json.RawMessage, pipeline ottl.Pipeline) (*ottl.PipelineResult, error)
	ExecutePipelineForSignal(data json.RawMessage, pipeline ottl.Pipeline, signal string) (*ottl.PipelineResult, error)
	ImportCollectorConfig(configYAML string) (ottl.PipelinesBySignal, error)
	ValidatePipelineOTTL(pipeline ottl.Pipeline) ottl.ValidateResult
}
