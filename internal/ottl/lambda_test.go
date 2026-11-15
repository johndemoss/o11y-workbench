package ottl

import (
	"context"
	"os"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestMain(m *testing.M) {
	if err := EnableLambdaExpressions(); err != nil {
		os.Stderr.WriteString("EnableLambdaExpressions: " + err.Error() + "\n")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func makeSampleTraceWithTagSlice() *collectortracev1.ExportTraceServiceRequest {
	return &collectortracev1.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{
			{
				Resource: &resourcev1.Resource{
					Attributes: []*commonv1.KeyValue{
						{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "test-svc"}}},
					},
				},
				ScopeSpans: []*tracev1.ScopeSpans{
					{
						Spans: []*tracev1.Span{
							{
								TraceId: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
								SpanId:  []byte{0, 0, 0, 0, 0, 0, 0, 1},
								Name:    "test-span",
								Attributes: []*commonv1.KeyValue{
									{Key: "tags", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_ArrayValue{
										ArrayValue: &commonv1.ArrayValue{Values: []*commonv1.AnyValue{
											{Value: &commonv1.AnyValue_StringValue{StringValue: "keep"}},
											{Value: &commonv1.AnyValue_StringValue{StringValue: "drop"}},
										}},
									}}},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestValidateStatements_LambdaFilter(t *testing.T) {
	stmt := `set(attributes["filtered"], Filter(attributes["tags"], (_, v) => v == "keep"))`
	result := ValidateStatements([]string{stmt})
	if !result.Valid {
		t.Fatalf("expected valid lambda Filter statement, got errors: %v", result.Errors)
	}
	if result.Parsed != 1 {
		t.Fatalf("expected 1 parsed statement, got %d", result.Parsed)
	}
}

func TestTransformRunData_LambdaFilterSlice(t *testing.T) {
	req := makeSampleTraceWithTagSlice()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	statements := []string{
		`set(attributes["filtered_tags"], Filter(attributes["tags"], (_, v) => v == "keep"))`,
	}
	result, _, err := TransformRunData(context.Background(), traceData, statements)
	if err != nil {
		t.Fatalf("TransformRunData: %v", err)
	}

	var resultReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(result, &resultReq); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	attrs := resultReq.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes
	var filtered *commonv1.AnyValue
	for _, kv := range attrs {
		if kv.Key == "filtered_tags" {
			filtered = kv.Value
			break
		}
	}
	if filtered == nil {
		t.Fatal("missing filtered_tags attribute after lambda Filter")
	}
	arr := filtered.GetArrayValue()
	if arr == nil || len(arr.Values) != 1 {
		t.Fatalf("expected filtered slice length 1, got %#v", arr)
	}
	if arr.Values[0].GetStringValue() != "keep" {
		t.Fatalf("expected retained value %q, got %q", "keep", arr.Values[0].GetStringValue())
	}
}

func TestExecutePipeline_LambdaFilterTransform(t *testing.T) {
	req := makeSampleTraceWithTagSlice()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	pipeline := Pipeline{
		Stages: []Stage{
			{ID: "lambda-filter", Type: StageTypeTransform, ProcessorRef: "p1"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {
				"statements": []interface{}{
					`set(attributes["filtered_tags"], Filter(attributes["tags"], (_, v) => v == "keep"))`,
				},
			},
		},
	}

	result, err := ExecutePipeline(context.Background(), traceData, pipeline)
	if err != nil {
		t.Fatalf("ExecutePipeline: %v", err)
	}

	var resultReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(result.FinalData, &resultReq); err != nil {
		t.Fatalf("unmarshal pipeline result: %v", err)
	}

	attrs := resultReq.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes
	for _, kv := range attrs {
		if kv.Key != "filtered_tags" {
			continue
		}
		arr := kv.Value.GetArrayValue()
		if arr == nil || len(arr.Values) != 1 || arr.Values[0].GetStringValue() != "keep" {
			t.Fatalf("unexpected filtered_tags: %#v", kv.Value)
		}
		return
	}
	t.Fatal("pipeline did not produce filtered_tags")
}
