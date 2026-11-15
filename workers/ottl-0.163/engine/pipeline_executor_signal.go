package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ExecutePipelineForSignal runs a pipeline against the given signal's OTLP JSON payload.
// Traces use ExecutePipeline; logs and metrics reuse Transform*/Filter* run-data helpers.
func ExecutePipelineForSignal(ctx context.Context, data json.RawMessage, pipeline Pipeline, signal string) (*PipelineResult, error) {
	switch signal {
	case "", "traces":
		return ExecutePipeline(ctx, data, pipeline)
	case "logs":
		return ExecuteLogPipeline(ctx, data, pipeline)
	case "metrics":
		return ExecuteMetricPipeline(ctx, data, pipeline)
	default:
		return nil, fmt.Errorf("unsupported signal: %s", signal)
	}
}

// ExecuteLogPipeline runs a pipeline of OTTL stages against log data.
func ExecuteLogPipeline(ctx context.Context, logData json.RawMessage, pipeline Pipeline) (*PipelineResult, error) {
	if _, err := countLogRecordsJSON(logData); err != nil {
		return nil, fmt.Errorf("unmarshal log data: %w", err)
	}
	return executeJSONPipeline(ctx, logData, pipeline, countLogRecordsJSON, executeLogTransformStage, executeLogFilterStage)
}

// ExecuteMetricPipeline runs a pipeline of OTTL stages against metric data.
func ExecuteMetricPipeline(ctx context.Context, metricData json.RawMessage, pipeline Pipeline) (*PipelineResult, error) {
	if _, err := countMetricDataPointsJSON(metricData); err != nil {
		return nil, fmt.Errorf("unmarshal metric data: %w", err)
	}
	return executeJSONPipeline(ctx, metricData, pipeline, countMetricDataPointsJSON, executeMetricTransformStage, executeMetricFilterStage)
}

type countFn func(json.RawMessage) (int, error)
type stageFn func(context.Context, json.RawMessage, ProcessorConfig, ErrorMode) (json.RawMessage, int, int, error)

func executeJSONPipeline(
	ctx context.Context,
	data json.RawMessage,
	pipeline Pipeline,
	count countFn,
	transform stageFn,
	filter stageFn,
) (*PipelineResult, error) {
	if len(pipeline.Stages) == 0 {
		n, _ := count(data)
		return &PipelineResult{
			Stages:         []StageResult{},
			FinalData:      data,
			FinalSpanCount: n,
		}, nil
	}

	var stageResults []StageResult
	current := data

	for i, stage := range pipeline.Stages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		startTime := time.Now()
		errorMode := stage.ErrorMode
		if errorMode == "" {
			errorMode = ErrorModePropagate
		}

		procCfg, ok := pipeline.Processors[stage.ProcessorRef]
		if !ok {
			return nil, fmt.Errorf("stage %s: undefined processor: %s", stage.ID, stage.ProcessorRef)
		}

		inputCount, _ := count(current)
		output := current
		outputCount := inputCount
		var stageErr error
		statementsParsed := 0

		switch stage.Type {
		case StageTypeTransform:
			output, outputCount, statementsParsed, stageErr = transform(ctx, current, procCfg, errorMode)
		case StageTypeFilter:
			output, outputCount, statementsParsed, stageErr = filter(ctx, current, procCfg, errorMode)
		default:
			return nil, fmt.Errorf("stage %s: unknown stage type: %s", stage.ID, stage.Type)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Per-record evaluation errors are handled by the stage itself.
		// Errors returned here include invalid configuration and must be fatal.
		if stageErr != nil {
			return nil, fmt.Errorf("stage %s: %w", stage.ID, stageErr)
		}
		intermediate := output
		var errors []string

		stageResults = append(stageResults, StageResult{
			StageIndex:       i,
			StageID:          stage.ID,
			InputSpanCount:   inputCount,
			OutputSpanCount:  outputCount,
			StatementsParsed: statementsParsed,
			Errors:           errors,
			IntermediateData: intermediate,
			DurationNs:       time.Since(startTime).Nanoseconds(),
		})

		if stageErr == nil {
			current = output
		}
	}

	finalCount, _ := count(current)
	return &PipelineResult{
		Stages:         stageResults,
		FinalData:      current,
		FinalSpanCount: finalCount,
	}, nil
}

func executeLogTransformStage(ctx context.Context, logData json.RawMessage, cfg ProcessorConfig, errorMode ErrorMode) (json.RawMessage, int, int, error) {
	if len(cfg) == 0 {
		n, _ := countLogRecordsJSON(logData)
		return logData, n, 0, nil
	}
	cfgYAML, err := mapToYAML(cfg)
	if err != nil {
		return logData, 0, 0, fmt.Errorf("marshal processor config: %w", err)
	}
	groups, err := extractSignalTransformGroups(cfgYAML, "logs")
	if err != nil {
		return logData, 0, 0, err
	}
	logStmts := ExtractStatements(groups)
	if len(logStmts) == 0 {
		n, _ := countLogRecordsJSON(logData)
		return logData, n, 0, nil
	}
	out, _, err := TransformLogRunDataWithGroups(ctx, logData, groups, errorMode)
	if err != nil {
		return logData, 0, len(logStmts), err
	}
	n, _ := countLogRecordsJSON(out)
	return out, n, len(logStmts), nil
}

func executeLogFilterStage(ctx context.Context, logData json.RawMessage, cfg ProcessorConfig, errorMode ErrorMode) (json.RawMessage, int, int, error) {
	if len(cfg) == 0 {
		n, _ := countLogRecordsJSON(logData)
		return logData, n, 0, nil
	}
	cfgYAML, err := mapToYAML(cfg)
	if err != nil {
		return logData, 0, 0, fmt.Errorf("marshal processor config: %w", err)
	}
	logConds, err := extractSignalFilterConditions(cfgYAML, "logs")
	if err != nil {
		return logData, 0, 0, err
	}
	if len(logConds) == 0 {
		n, _ := countLogRecordsJSON(logData)
		return logData, n, 0, nil
	}
	out, _, kept, err := FilterLogRunDataWithErrorMode(ctx, logData, logConds, FilterModeExclude, errorMode)
	if err != nil {
		return logData, 0, len(logConds), err
	}
	return out, kept, len(logConds), nil
}

func executeMetricTransformStage(ctx context.Context, metricData json.RawMessage, cfg ProcessorConfig, errorMode ErrorMode) (json.RawMessage, int, int, error) {
	if len(cfg) == 0 {
		n, _ := countMetricDataPointsJSON(metricData)
		return metricData, n, 0, nil
	}
	cfgYAML, err := mapToYAML(cfg)
	if err != nil {
		return metricData, 0, 0, fmt.Errorf("marshal processor config: %w", err)
	}
	groups, err := extractSignalTransformGroups(cfgYAML, "metrics")
	if err != nil {
		return metricData, 0, 0, err
	}
	metricStmts := ExtractStatements(groups)
	if len(metricStmts) == 0 {
		n, _ := countMetricDataPointsJSON(metricData)
		return metricData, n, 0, nil
	}
	out, _, err := TransformMetricRunDataWithErrorMode(ctx, metricData, groups, errorMode)
	if err != nil {
		return metricData, 0, len(metricStmts), err
	}
	n, _ := countMetricDataPointsJSON(out)
	return out, n, len(metricStmts), nil
}

func executeMetricFilterStage(ctx context.Context, metricData json.RawMessage, cfg ProcessorConfig, errorMode ErrorMode) (json.RawMessage, int, int, error) {
	if len(cfg) == 0 {
		n, _ := countMetricDataPointsJSON(metricData)
		return metricData, n, 0, nil
	}
	cfgYAML, err := mapToYAML(cfg)
	if err != nil {
		return metricData, 0, 0, fmt.Errorf("marshal processor config: %w", err)
	}
	metricConds, err := extractSignalFilterConditions(cfgYAML, "metrics")
	if err != nil {
		return metricData, 0, 0, err
	}
	if len(metricConds) == 0 {
		n, _ := countMetricDataPointsJSON(metricData)
		return metricData, n, 0, nil
	}
	out, _, kept, err := FilterMetricRunDataWithErrorMode(ctx, metricData, metricConds, FilterModeExclude, errorMode)
	if err != nil {
		return metricData, 0, len(metricConds), err
	}
	return out, kept, len(metricConds), nil
}

func extractSignalTransformGroups(cfgYAML, signal string) ([]ContextStatements, error) {
	meta, err := ParseTransformConfigMeta(cfgYAML)
	if err != nil {
		return nil, err
	}
	if !meta.Structured {
		return meta.TraceGroups, nil
	}
	switch signal {
	case "logs":
		return meta.LogGroups, nil
	case "metrics":
		return meta.MetricGroups, nil
	default:
		return meta.TraceGroups, nil
	}
}

func extractSignalFilterConditions(cfgYAML, signal string) ([]string, error) {
	return SelectFilterConditions(cfgYAML, signal)
}

func countLogRecordsJSON(data json.RawMessage) (int, error) {
	logs, err := UnmarshalLogsFromJSON(data)
	if err != nil {
		return 0, err
	}
	count := 0
	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		for j := 0; j < logs.ResourceLogs().At(i).ScopeLogs().Len(); j++ {
			count += logs.ResourceLogs().At(i).ScopeLogs().At(j).LogRecords().Len()
		}
	}
	return count, nil
}

func countMetricDataPointsJSON(data json.RawMessage) (int, error) {
	metrics, err := UnmarshalMetricsFromJSON(data)
	if err != nil {
		return 0, err
	}
	return countMetricDataPointsPdata(metrics), nil
}
