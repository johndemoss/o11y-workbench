package engine

import (
	"encoding/json"
	"testing"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func makeSampleLogs() *collectorlogsv1.ExportLogsServiceRequest {
	return &collectorlogsv1.ExportLogsServiceRequest{
		ResourceLogs: []*logsv1.ResourceLogs{
			{
				Resource: &resourcev1.Resource{
					Attributes: []*commonv1.KeyValue{
						{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "log-svc"}}},
					},
				},
				ScopeLogs: []*logsv1.ScopeLogs{
					{
						LogRecords: []*logsv1.LogRecord{
							{
								Body: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "hello"}},
								Attributes: []*commonv1.KeyValue{
									{Key: "level", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "info"}}},
								},
							},
							{
								Body: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "warn-me"}},
								Attributes: []*commonv1.KeyValue{
									{Key: "level", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "warn"}}},
								},
							},
						},
					},
				},
			},
		},
	}
}

func makeSampleMetrics() *collectormetricsv1.ExportMetricsServiceRequest {
	return &collectormetricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{
			{
				Resource: &resourcev1.Resource{
					Attributes: []*commonv1.KeyValue{
						{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "metric-svc"}}},
					},
				},
				ScopeMetrics: []*metricspb.ScopeMetrics{
					{
						Metrics: []*metricspb.Metric{
							{
								Name: "demo.counter",
								Data: &metricspb.Metric_Sum{
									Sum: &metricspb.Sum{
										AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
										IsMonotonic:            true,
										DataPoints: []*metricspb.NumberDataPoint{
											{
												Attributes: []*commonv1.KeyValue{
													{Key: "env", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "dev"}}},
												},
												Value: &metricspb.NumberDataPoint_AsInt{AsInt: 1},
											},
											{
												Attributes: []*commonv1.KeyValue{
													{Key: "env", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "prod"}}},
												},
												Value: &metricspb.NumberDataPoint_AsInt{AsInt: 2},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func marshalLogs(req *collectorlogsv1.ExportLogsServiceRequest) (json.RawMessage, error) {
	return protojson.Marshal(req)
}

func marshalMetrics(req *collectormetricsv1.ExportMetricsServiceRequest) (json.RawMessage, error) {
	return protojson.Marshal(req)
}

func TestExecuteLogPipeline_Transform(t *testing.T) {
	logData, err := marshalLogs(makeSampleLogs())
	if err != nil {
		t.Fatalf("marshal logs: %v", err)
	}

	pipeline := Pipeline{
		Stages: []Stage{
			{ID: "s1", Type: StageTypeTransform, ProcessorRef: "p1"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {
				"log_statements": []interface{}{
					map[string]interface{}{
						"context":    "log",
						"statements": []interface{}{`set(attributes["pipeline"], "applied")`},
					},
				},
			},
		},
	}

	result, err := ExecuteLogPipeline(logData, pipeline)
	if err != nil {
		t.Fatalf("ExecuteLogPipeline: %v", err)
	}
	if len(result.Stages) != 1 {
		t.Fatalf("stages = %d, want 1", len(result.Stages))
	}
	if result.FinalSpanCount != 2 {
		t.Fatalf("FinalSpanCount = %d, want 2", result.FinalSpanCount)
	}
	if result.Stages[0].StatementsParsed == 0 {
		t.Fatal("expected statements_parsed > 0")
	}

	var out collectorlogsv1.ExportLogsServiceRequest
	if err := protojson.Unmarshal(result.FinalData, &out); err != nil {
		t.Fatalf("unmarshal final: %v", err)
	}
	lr := out.ResourceLogs[0].ScopeLogs[0].LogRecords[0]
	found := false
	for _, attr := range lr.Attributes {
		if attr.Key == "pipeline" {
			found = true
			if v, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue); !ok || v.StringValue != "applied" {
				t.Fatalf("pipeline attr = %v, want applied", attr.Value)
			}
		}
	}
	if !found {
		t.Fatal("pipeline attribute not set on log record")
	}
}

func TestExecuteLogPipeline_Filter(t *testing.T) {
	logData, err := marshalLogs(makeSampleLogs())
	if err != nil {
		t.Fatalf("marshal logs: %v", err)
	}

	pipeline := Pipeline{
		Stages: []Stage{
			{ID: "s1", Type: StageTypeFilter, ProcessorRef: "p1"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {
				"logs": map[string]interface{}{
					"log_record": []interface{}{`attributes["level"] == "warn"`},
				},
			},
		},
	}

	result, err := ExecuteLogPipeline(logData, pipeline)
	if err != nil {
		t.Fatalf("ExecuteLogPipeline: %v", err)
	}
	if result.FinalSpanCount != 1 {
		t.Fatalf("FinalSpanCount = %d, want 1", result.FinalSpanCount)
	}
	if result.Stages[0].InputSpanCount != 2 || result.Stages[0].OutputSpanCount != 1 {
		t.Fatalf("stage counts in=%d out=%d, want 2/1", result.Stages[0].InputSpanCount, result.Stages[0].OutputSpanCount)
	}
}

func TestExecuteMetricPipeline_Transform(t *testing.T) {
	metricData, err := marshalMetrics(makeSampleMetrics())
	if err != nil {
		t.Fatalf("marshal metrics: %v", err)
	}

	pipeline := Pipeline{
		Stages: []Stage{
			{ID: "s1", Type: StageTypeTransform, ProcessorRef: "p1"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {
				"metric_statements": []interface{}{
					map[string]interface{}{
						"context":    "datapoint",
						"statements": []interface{}{`set(attributes["pipeline"], "applied")`},
					},
				},
			},
		},
	}

	result, err := ExecuteMetricPipeline(metricData, pipeline)
	if err != nil {
		t.Fatalf("ExecuteMetricPipeline: %v", err)
	}
	if result.FinalSpanCount != 2 {
		t.Fatalf("FinalSpanCount = %d, want 2", result.FinalSpanCount)
	}

	var out collectormetricsv1.ExportMetricsServiceRequest
	if err := protojson.Unmarshal(result.FinalData, &out); err != nil {
		t.Fatalf("unmarshal final: %v", err)
	}
	dp := out.ResourceMetrics[0].ScopeMetrics[0].Metrics[0].GetSum().DataPoints[0]
	found := false
	for _, attr := range dp.Attributes {
		if attr.Key == "pipeline" {
			found = true
		}
	}
	if !found {
		t.Fatal("pipeline attribute not set on metric datapoint")
	}
}

func TestExecutePipelineForSignal_Dispatch(t *testing.T) {
	traceJSON := []byte(`{
  "resourceSpans": [{
    "resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "svc"}}]},
    "scopeSpans": [{
      "spans": [{
        "traceId": "0102030405060708090a0b0c0d0e0f10",
        "spanId": "0102030405060708",
        "name": "original",
        "kind": 1,
        "startTimeUnixNano": "1",
        "endTimeUnixNano": "2"
      }]
    }]
  }]
}`)
	pipeline := Pipeline{
		Stages: []Stage{
			{ID: "s1", Type: StageTypeTransform, ProcessorRef: "p1"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {"statements": []interface{}{`set(attributes["env"], "prod")`}},
		},
	}
	result, err := ExecutePipelineForSignal(traceJSON, pipeline, "traces")
	if err != nil {
		t.Fatalf("ExecutePipelineForSignal traces: %v", err)
	}
	if result.FinalSpanCount != 1 {
		t.Fatalf("FinalSpanCount = %d, want 1", result.FinalSpanCount)
	}

	if _, err := ExecutePipelineForSignal(traceJSON, pipeline, "unknown"); err == nil {
		t.Fatal("expected error for unsupported signal")
	}
}
