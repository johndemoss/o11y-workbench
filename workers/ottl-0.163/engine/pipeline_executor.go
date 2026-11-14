package engine

import (
	"encoding/json"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"gopkg.in/yaml.v3"
)

// ExecutePipeline runs a pipeline of OTTL stages against trace data.
// Each stage is executed in order, applying the configured processor with
// the specified error_mode. Returns a PipelineResult with per-stage results
// and final trace data.
func ExecutePipeline(traceData json.RawMessage, pipeline Pipeline) (*PipelineResult, error) {
	traces, err := UnmarshalTracesFromJSON(traceData)
	if err != nil {
		return nil, fmt.Errorf("unmarshal trace data: %w", err)
	}

	if len(pipeline.Stages) == 0 {
		finalData, err := MarshalTracesToJSON(traces)
		if err != nil {
			return nil, fmt.Errorf("marshal final data: %w", err)
		}
		return &PipelineResult{
			Stages:         []StageResult{},
			FinalData:      finalData,
			FinalSpanCount: countSpans(traces),
		}, nil
	}

	var stageResults []StageResult
	currentTraces := traces

	for i, stage := range pipeline.Stages {
		startTime := time.Now()
		errorMode := stage.ErrorMode
		if errorMode == "" {
			errorMode = ErrorModePropagate
		}

		procCfg, ok := pipeline.Processors[stage.ProcessorRef]
		if !ok {
			return nil, fmt.Errorf("stage %s: undefined processor: %s", stage.ID, stage.ProcessorRef)
		}

		inputSpanCount := countSpans(currentTraces)
		outputTraces := currentTraces
		outputSpanCount := inputSpanCount
		var stageErr error
		statementsParsed := 0

		switch stage.Type {
		case StageTypeTransform:
			outputTraces, outputSpanCount, statementsParsed, stageErr = executeTransformStage(currentTraces, procCfg, errorMode)
		case StageTypeFilter:
			outputTraces, outputSpanCount, statementsParsed, stageErr = executeFilterStage(currentTraces, procCfg, errorMode)
		default:
			return nil, fmt.Errorf("stage %s: unknown stage type: %s", stage.ID, stage.Type)
		}

		var intermediate json.RawMessage
		if stageErr == nil {
			intermediate, _ = MarshalTracesToJSON(outputTraces)
		} else {
			intermediate, _ = MarshalTracesToJSON(currentTraces)
		}

		var errors []string
		if stageErr != nil {
			switch errorMode {
			case ErrorModePropagate:
				return nil, fmt.Errorf("stage %s: %w", stage.ID, stageErr)
			case ErrorModeIgnore:
				errors = append(errors, stageErr.Error())
				outputTraces = currentTraces
				outputSpanCount = inputSpanCount
			case ErrorModeSilent:
				outputTraces = currentTraces
				outputSpanCount = inputSpanCount
			}
		}

		if stage.Type == StageTypeTransform && outputSpanCount == 0 && stageErr == nil {
			outputSpanCount = inputSpanCount
		}

		stageResults = append(stageResults, StageResult{
			StageIndex:       i,
			StageID:          stage.ID,
			InputSpanCount:   inputSpanCount,
			OutputSpanCount:  outputSpanCount,
			StatementsParsed: statementsParsed,
			Errors:           errors,
			IntermediateData: intermediate,
			DurationNs:       time.Since(startTime).Nanoseconds(),
		})

		if stageErr == nil {
			currentTraces = outputTraces
		}
	}

	finalData, err := MarshalTracesToJSON(currentTraces)
	if err != nil {
		return nil, fmt.Errorf("marshal final data: %w", err)
	}

	return &PipelineResult{
		Stages:         stageResults,
		FinalData:      finalData,
		FinalSpanCount: countSpans(currentTraces),
	}, nil
}

func executeTransformStage(traces ptrace.Traces, cfg ProcessorConfig, errorMode ErrorMode) (ptrace.Traces, int, int, error) {
	if len(cfg) == 0 {
		return traces, countSpans(traces), 0, nil
	}

	cfgYAML, err := mapToYAML(cfg)
	if err != nil {
		return traces, 0, 0, fmt.Errorf("marshal processor config: %w", err)
	}

	stmts, err := ParseTransformStatements(cfgYAML)
	if err != nil {
		return traces, 0, 0, err
	}

	if len(stmts) == 0 {
		return traces, countSpans(traces), 0, nil
	}

	out, spanCount, execErr := TransformSpans(traces, stmts, errorMode)
	if execErr != nil {
		return traces, 0, len(stmts), execErr
	}

	return out, spanCount, len(stmts), nil
}

func executeFilterStage(traces ptrace.Traces, cfg ProcessorConfig, errorMode ErrorMode) (ptrace.Traces, int, int, error) {
	if len(cfg) == 0 {
		return traces, countSpans(traces), 0, nil
	}

	cfgYAML, err := mapToYAML(cfg)
	if err != nil {
		return traces, 0, 0, fmt.Errorf("marshal processor config: %w", err)
	}

	conds, err := ParseFilterConditions(cfgYAML)
	if err != nil {
		return traces, 0, 0, err
	}

	if len(conds) == 0 {
		return traces, countSpans(traces), 0, nil
	}

	out, _, kept, execErr := FilterSpans(traces, conds, FilterModeInclude, errorMode)
	if execErr != nil {
		return traces, 0, len(conds), execErr
	}

	return out, kept, len(conds), nil
}

func countSpans(traces ptrace.Traces) int {
	count := 0
	rss := traces.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		sss := rss.At(i).ScopeSpans()
		for j := 0; j < sss.Len(); j++ {
			count += sss.At(j).Spans().Len()
		}
	}
	return count
}

func mapToYAML(cfg ProcessorConfig) (string, error) {
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal to yaml: %w", err)
	}
	return string(out), nil
}
