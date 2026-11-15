package ottlengine

import (
	"context"
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
	ValidateStatements(context.Context, []string) ottl.ValidateResult
	ValidateFilterConditions(context.Context, []string, gen.Signals) ottl.ValidateResult
	TransformTraces(context.Context, json.RawMessage, []string) (json.RawMessage, int, error)
	TransformLogs(context.Context, json.RawMessage, []string) (json.RawMessage, error)
	TransformMetrics(context.Context, json.RawMessage, []string) (json.RawMessage, error)
	FilterTraces(context.Context, json.RawMessage, []string, ottl.FilterMode) (json.RawMessage, int, int, error)
	FilterLogs(context.Context, json.RawMessage, []string, ottl.FilterMode) (json.RawMessage, int, int, error)
	FilterMetrics(context.Context, json.RawMessage, []string, ottl.FilterMode) (json.RawMessage, int, int, error)
	ExecutePipeline(context.Context, json.RawMessage, ottl.Pipeline) (*ottl.PipelineResult, error)
	ExecutePipelineForSignal(context.Context, json.RawMessage, ottl.Pipeline, string) (*ottl.PipelineResult, error)
	ImportCollectorConfig(context.Context, string) (ottl.PipelinesBySignal, error)
	ValidatePipelineOTTL(context.Context, ottl.Pipeline) ottl.ValidateResult
}
