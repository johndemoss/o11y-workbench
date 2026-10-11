package ottl

import (
	"encoding/json"
	"fmt"
	"time"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// ExecutePipelineForSignal runs a pipeline against the given signal's OTLP JSON payload.
// Traces use ExecutePipeline; logs and metrics reuse Transform*/Filter* run-data helpers.
func ExecutePipelineForSignal(data json.RawMessage, pipeline Pipeline, signal string) (*PipelineResult, error) {
	switch signal {
	case "", "traces":
		return ExecutePipeline(data, pipeline)
	case "logs":
		return ExecuteLogPipeline(data, pipeline)
	case "metrics":
		return ExecuteMetricPipeline(data, pipeline)
	default:
		return nil, fmt.Errorf("unsupported signal: %s", signal)
	}
}

// ExecuteLogPipeline runs a pipeline of OTTL stages against log data.
func ExecuteLogPipeline(logData json.RawMessage, pipeline Pipeline) (*PipelineResult, error) {
	if _, err := countLogRecordsJSON(logData); err != nil {
		return nil, fmt.Errorf("unmarshal log data: %w", err)
	}
	return executeJSONPipeline(logData, pipeline, countLogRecordsJSON, executeLogTransformStage, executeLogFilterStage)
}

// ExecuteMetricPipeline runs a pipeline of OTTL stages against metric data.
func ExecuteMetricPipeline(metricData json.RawMessage, pipeline Pipeline) (*PipelineResult, error) {
	if _, err := countMetricDataPointsJSON(metricData); err != nil {
		return nil, fmt.Errorf("unmarshal metric data: %w", err)
	}
	return executeJSONPipeline(metricData, pipeline, countMetricDataPointsJSON, executeMetricTransformStage, executeMetricFilterStage)
}

type countFn func(json.RawMessage) (int, error)
type stageFn func(json.RawMessage, ProcessorConfig) (json.RawMessage, int, int, error)

func executeJSONPipeline(
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
			output, outputCount, statementsParsed, stageErr = transform(current, procCfg)
		case StageTypeFilter:
			output, outputCount, statementsParsed, stageErr = filter(current, procCfg)
		default:
			return nil, fmt.Errorf("stage %s: unknown stage type: %s", stage.ID, stage.Type)
		}

		intermediate := current
		if stageErr == nil {
			intermediate = output
		}

		var errors []string
		if stageErr != nil {
			switch errorMode {
			case ErrorModePropagate:
				return nil, fmt.Errorf("stage %s: %w", stage.ID, stageErr)
			case ErrorModeIgnore:
				errors = append(errors, stageErr.Error())
				output = current
				outputCount = inputCount
			case ErrorModeSilent:
				output = current
				outputCount = inputCount
			}
		}

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

func executeLogTransformStage(logData json.RawMessage, cfg ProcessorConfig) (json.RawMessage, int, int, error) {
	if len(cfg) == 0 {
		n, _ := countLogRecordsJSON(logData)
		return logData, n, 0, nil
	}
	cfgYAML, err := mapToYAML(cfg)
	if err != nil {
		return logData, 0, 0, fmt.Errorf("marshal processor config: %w", err)
	}
	logStmts, err := extractSignalTransformStatements(cfgYAML, "logs")
	if err != nil {
		return logData, 0, 0, err
	}
	if len(logStmts) == 0 {
		n, _ := countLogRecordsJSON(logData)
		return logData, n, 0, nil
	}
	out, err := TransformLogRunData(logData, logStmts)
	if err != nil {
		return logData, 0, len(logStmts), err
	}
	n, _ := countLogRecordsJSON(out)
	return out, n, len(logStmts), nil
}

func executeLogFilterStage(logData json.RawMessage, cfg ProcessorConfig) (json.RawMessage, int, int, error) {
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
	out, _, kept, err := FilterLogRunData(logData, logConds, FilterModeInclude)
	if err != nil {
		return logData, 0, len(logConds), err
	}
	return out, kept, len(logConds), nil
}

func executeMetricTransformStage(metricData json.RawMessage, cfg ProcessorConfig) (json.RawMessage, int, int, error) {
	if len(cfg) == 0 {
		n, _ := countMetricDataPointsJSON(metricData)
		return metricData, n, 0, nil
	}
	cfgYAML, err := mapToYAML(cfg)
	if err != nil {
		return metricData, 0, 0, fmt.Errorf("marshal processor config: %w", err)
	}
	metricStmts, err := extractSignalTransformStatements(cfgYAML, "metrics")
	if err != nil {
		return metricData, 0, 0, err
	}
	if len(metricStmts) == 0 {
		n, _ := countMetricDataPointsJSON(metricData)
		return metricData, n, 0, nil
	}
	out, err := TransformMetricRunData(metricData, metricStmts)
	if err != nil {
		return metricData, 0, len(metricStmts), err
	}
	n, _ := countMetricDataPointsJSON(out)
	return out, n, len(metricStmts), nil
}

func executeMetricFilterStage(metricData json.RawMessage, cfg ProcessorConfig) (json.RawMessage, int, int, error) {
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
	out, _, kept, err := FilterMetricRunData(metricData, metricConds, FilterModeInclude)
	if err != nil {
		return metricData, 0, len(metricConds), err
	}
	return out, kept, len(metricConds), nil
}

// extractSignalTransformStatements returns statements for the requested signal.
// Unstructured configs (flat lists) are treated as belonging to that signal.
func extractSignalTransformStatements(cfgYAML, signal string) ([]string, error) {
	traceStmts, logStmts, metricStmts, err := ParseCombinedTransformConfigAll(cfgYAML)
	if err == nil && (len(traceStmts) > 0 || len(logStmts) > 0 || len(metricStmts) > 0) {
		switch signal {
		case "logs":
			if len(logStmts) > 0 {
				return logStmts, nil
			}
			// Unstructured fallback lands in traceStmts.
			return traceStmts, nil
		case "metrics":
			if len(metricStmts) > 0 {
				return metricStmts, nil
			}
			return traceStmts, nil
		default:
			return traceStmts, nil
		}
	}
	return ParseTransformStatements(cfgYAML)
}

// extractSignalFilterConditions returns filter conditions for the requested signal.
// Unstructured configs (flat lists) are treated as belonging to that signal.
func extractSignalFilterConditions(cfgYAML, signal string) ([]string, error) {
	traceConds, logConds, metricConds, err := ParseCombinedFilterConfigAll(cfgYAML)
	if err == nil && (len(traceConds) > 0 || len(logConds) > 0 || len(metricConds) > 0) {
		switch signal {
		case "logs":
			if len(logConds) > 0 {
				return logConds, nil
			}
			return traceConds, nil
		case "metrics":
			if len(metricConds) > 0 {
				return metricConds, nil
			}
			return traceConds, nil
		default:
			return traceConds, nil
		}
	}
	return ParseFilterConditions(cfgYAML)
}

func countLogRecordsJSON(data json.RawMessage) (int, error) {
	var req collectorlogsv1.ExportLogsServiceRequest
	if err := protojson.Unmarshal(data, &req); err != nil {
		return 0, err
	}
	count := 0
	for _, rl := range req.ResourceLogs {
		for _, sl := range rl.ScopeLogs {
			count += len(sl.LogRecords)
		}
	}
	return count, nil
}

func countMetricDataPointsJSON(data json.RawMessage) (int, error) {
	var req collectormetricsv1.ExportMetricsServiceRequest
	if err := protojson.Unmarshal(data, &req); err != nil {
		return 0, err
	}
	count := 0
	for _, rm := range req.ResourceMetrics {
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				count += countMetricProtoDataPoints(m)
			}
		}
	}
	return count, nil
}

func countMetricProtoDataPoints(m *metricspb.Metric) int {
	if m == nil {
		return 0
	}
	switch data := m.Data.(type) {
	case *metricspb.Metric_Gauge:
		if data.Gauge == nil {
			return 0
		}
		return len(data.Gauge.DataPoints)
	case *metricspb.Metric_Sum:
		if data.Sum == nil {
			return 0
		}
		return len(data.Sum.DataPoints)
	case *metricspb.Metric_Histogram:
		if data.Histogram == nil {
			return 0
		}
		return len(data.Histogram.DataPoints)
	case *metricspb.Metric_ExponentialHistogram:
		if data.ExponentialHistogram == nil {
			return 0
		}
		return len(data.ExponentialHistogram.DataPoints)
	case *metricspb.Metric_Summary:
		if data.Summary == nil {
			return 0
		}
		return len(data.Summary.DataPoints)
	default:
		return 0
	}
}
