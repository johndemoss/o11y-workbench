package ottlengine

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/ottl"
)

// LocalEngine runs OTTL against the in-process collector train linked into this binary.
type LocalEngine struct{}

func (e *LocalEngine) Info() VersionInfo {
	return VersionInfo{
		Contrib: DefaultContribVersion,
		Stable:  DefaultStableVersion,
		Source:  "in-process",
	}
}

func (e *LocalEngine) ValidateStatements(statements []string) ottl.ValidateResult {
	return ottl.ValidateStatements(statements)
}

func (e *LocalEngine) ValidateFilterConditions(conditions []string, signals gen.Signals) ottl.ValidateResult {
	return ottl.ValidateFilterConditionsParse(conditions, signals)
}

func (e *LocalEngine) TransformTraces(data json.RawMessage, statements []string) (json.RawMessage, int, error) {
	return ottl.TransformRunData(data, statements)
}

func (e *LocalEngine) TransformLogs(data json.RawMessage, statements []string) (json.RawMessage, error) {
	return ottl.TransformLogRunData(data, statements)
}

func (e *LocalEngine) TransformMetrics(data json.RawMessage, statements []string) (json.RawMessage, error) {
	return ottl.TransformMetricRunData(data, statements)
}

func (e *LocalEngine) FilterTraces(data json.RawMessage, conditions []string, mode ottl.FilterMode) (json.RawMessage, int, int, error) {
	return ottl.FilterRunData(data, conditions, mode)
}

func (e *LocalEngine) FilterLogs(data json.RawMessage, conditions []string, mode ottl.FilterMode) (json.RawMessage, int, int, error) {
	return ottl.FilterLogRunData(data, conditions, mode)
}

func (e *LocalEngine) FilterMetrics(data json.RawMessage, conditions []string, mode ottl.FilterMode) (json.RawMessage, int, int, error) {
	return ottl.FilterMetricRunData(data, conditions, mode)
}

func (e *LocalEngine) ExecutePipeline(data json.RawMessage, pipeline ottl.Pipeline) (*ottl.PipelineResult, error) {
	return ottl.ExecutePipeline(data, pipeline)
}

func (e *LocalEngine) ExecutePipelineForSignal(data json.RawMessage, pipeline ottl.Pipeline, signal string) (*ottl.PipelineResult, error) {
	return ottl.ExecutePipelineForSignal(data, pipeline, signal)
}

func (e *LocalEngine) ImportCollectorConfig(configYAML string) (ottl.PipelinesBySignal, error) {
	return ottl.ImportCollectorConfig(configYAML)
}

// ValidatePipelineOTTL walks processor configs and validates transform statements
// and filter conditions against this train.
func (e *LocalEngine) ValidatePipelineOTTL(pipeline ottl.Pipeline) ottl.ValidateResult {
	if len(pipeline.Processors) == 0 {
		return ottl.ValidateResult{Valid: true, Parsed: 0}
	}

	var (
		errors []string
		parsed int
	)

	for name, cfg := range pipeline.Processors {
		kind := classifyProcessor(cfg)
		switch kind {
		case processorTransform:
			stmts, err := extractTransformStatements(cfg)
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			if len(stmts) == 0 {
				continue
			}
			res := ottl.ValidateStatements(stmts)
			parsed += res.Parsed
			for _, msg := range res.Errors {
				errors = append(errors, fmt.Sprintf("%s: %s", name, msg))
			}
		case processorFilter:
			conds, signals, err := extractFilterConditions(cfg)
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			if len(conds) == 0 {
				continue
			}
			res := ottl.ValidateFilterConditionsParse(conds, signals)
			parsed += res.Parsed
			for _, msg := range res.Errors {
				errors = append(errors, fmt.Sprintf("%s: %s", name, msg))
			}
		}
	}

	if len(errors) > 0 {
		return ottl.ValidateResult{Valid: false, Errors: errors, Parsed: parsed}
	}
	return ottl.ValidateResult{Valid: true, Parsed: parsed}
}

type processorKind int

const (
	processorUnknown processorKind = iota
	processorTransform
	processorFilter
)

func classifyProcessor(cfg ottl.ProcessorConfig) processorKind {
	hasTransform := false
	hasFilter := false
	for key := range cfg {
		switch key {
		case "statements", "trace_statements", "log_statements", "metric_statements":
			hasTransform = true
		case "traces", "logs", "metrics":
			hasFilter = true
		}
	}
	if hasTransform {
		return processorTransform
	}
	if hasFilter {
		return processorFilter
	}
	return processorUnknown
}

func extractTransformStatements(cfg ottl.ProcessorConfig) ([]string, error) {
	cfgYAML, err := marshalProcessorYAML(cfg)
	if err != nil {
		return nil, err
	}
	return ottl.ParseTransformStatements(cfgYAML)
}

func extractFilterConditions(cfg ottl.ProcessorConfig) ([]string, gen.Signals, error) {
	cfgYAML, err := marshalProcessorYAML(cfg)
	if err != nil {
		return nil, gen.Signals{}, err
	}
	conds, err := ottl.ParseFilterConditions(cfgYAML)
	if err != nil {
		return nil, gen.Signals{}, err
	}
	return conds, filterSignals(cfg), nil
}

func filterSignals(cfg ottl.ProcessorConfig) gen.Signals {
	_, hasTraces := cfg["traces"]
	_, hasLogs := cfg["logs"]
	_, hasMetrics := cfg["metrics"]
	n := 0
	if hasTraces {
		n++
	}
	if hasLogs {
		n++
	}
	if hasMetrics {
		n++
	}
	if n == 1 {
		return gen.Signals{Traces: hasTraces, Logs: hasLogs, Metrics: hasMetrics}
	}
	return gen.Signals{Traces: true, Logs: true, Metrics: true}
}

func marshalProcessorYAML(cfg ottl.ProcessorConfig) (string, error) {
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal processor config: %w", err)
	}
	return string(out), nil
}
