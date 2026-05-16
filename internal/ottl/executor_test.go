package ottl

import (
	"encoding/json"
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// makeSampleTrace builds a minimal OTLP ExportTraceServiceRequest with one
// resource, one scope, and one span carrying attributes.
func makeSampleTrace() *collectortracev1.ExportTraceServiceRequest {
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
						Scope: &commonv1.InstrumentationScope{
							Name:    "test-scope",
							Version: "1.0.0",
						},
						Spans: []*tracev1.Span{
							{
								TraceId: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
								SpanId:  []byte{0, 0, 0, 0, 0, 0, 0, 1},
								Name:    "test-span",
								Kind:    tracev1.Span_SPAN_KIND_SERVER,
								Attributes: []*commonv1.KeyValue{
									{Key: "original.key", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "original"}}},
								},
							},
						},
					},
				},
			},
		},
	}
}

// marshalTrace serializes a ExportTraceServiceRequest into OTLP JSON bytes,
// matching the format stored in run.TraceData.
func marshalTrace(req *collectortracev1.ExportTraceServiceRequest) (json.RawMessage, error) {
	data, err := protojson.MarshalOptions{UseProtoNames: true, Indent: "  "}.Marshal(req)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

func TestTransformRunData_SetAttribute(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	statements := []string{`set(attributes["new.key"], "new-value")`}
	result, _, err := TransformRunData(traceData, statements)
	if err != nil {
		t.Fatalf("TransformRunData: %v", err)
	}

	var resultReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(result, &resultReq); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(resultReq.ResourceSpans) == 0 {
		t.Fatal("result has no ResourceSpans")
	}
	spans := resultReq.ResourceSpans[0].ScopeSpans[0].Spans
	if len(spans) == 0 {
		t.Fatal("result has no Spans")
	}
	span := spans[0]

	foundNew := false
	foundOriginal := false
	for _, attr := range span.Attributes {
		if attr.Key == "new.key" {
			foundNew = true
			if v, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue); !ok || v.StringValue != "new-value" {
				t.Errorf("new.key = %v, want 'new-value'", attr.Value)
			}
		}
		if attr.Key == "original.key" {
			foundOriginal = true
		}
	}
	if !foundNew {
		t.Error("new.key attribute not found after set()")
	}
	if !foundOriginal {
		t.Error("original.key attribute missing after transform (should be preserved)")
	}
}

func TestTransformRunData_InvalidStatement(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	statements := []string{`not_a_real_function()`}
	_, _, err = TransformRunData(traceData, statements)
	if err == nil {
		t.Fatal("expected error for invalid function, got nil")
	}
}

func TestTransformRunData_NoStatements(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	_, _, err = TransformRunData(traceData, nil)
	if err == nil {
		t.Fatal("expected error for nil statements, got nil")
	}

	_, _, err = TransformRunData(traceData, []string{})
	if err == nil {
		t.Fatal("expected error for empty statements, got nil")
	}
}

func TestTransformRunData_InvalidTraceData(t *testing.T) {
	_, _, err := TransformRunData(json.RawMessage(`{"invalid": "data"}`), []string{`set(attributes["x"], "y")`})
	if err == nil {
		t.Fatal("expected error for invalid trace data, got nil")
	}
}

func TestTransformRunData_DeleteKey(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	statements := []string{`delete_key(attributes, "original.key")`}
	result, _, err := TransformRunData(traceData, statements)
	if err != nil {
		t.Fatalf("TransformRunData: %v", err)
	}

	var resultReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(result, &resultReq); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	span := resultReq.ResourceSpans[0].ScopeSpans[0].Spans[0]
	for _, attr := range span.Attributes {
		if attr.Key == "original.key" {
			t.Error("original.key should have been deleted")
		}
	}
}

func TestTransformRunData_MultipleStatements(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	statements := []string{
		`set(attributes["a"], "1")`,
		`set(attributes["b"], "2")`,
	}
	result, _, err := TransformRunData(traceData, statements)
	if err != nil {
		t.Fatalf("TransformRunData: %v", err)
	}

	var resultReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(result, &resultReq); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	span := resultReq.ResourceSpans[0].ScopeSpans[0].Spans[0]
	attrs := make(map[string]string)
	for _, attr := range span.Attributes {
		if v, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue); ok {
			attrs[attr.Key] = v.StringValue
		}
	}
	if attrs["a"] != "1" {
		t.Errorf("a = %q, want '1'", attrs["a"])
	}
	if attrs["b"] != "2" {
		t.Errorf("b = %q, want '2'", attrs["b"])
	}
	if attrs["original.key"] != "original" {
		t.Errorf("original.key = %q, want 'original'", attrs["original.key"])
	}
}

// TestTransformRunData_IsMatch applies a transform only where condition matches.
func TestTransformRunData_IsMatch(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	// This should match because the span name is "test-span"
	statements := []string{`set(attributes["matched"], "yes") where name == "test-span"`}
	result, _, err := TransformRunData(traceData, statements)
	if err != nil {
		t.Fatalf("TransformRunData: %v", err)
	}

	var resultReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(result, &resultReq); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	span := resultReq.ResourceSpans[0].ScopeSpans[0].Spans[0]
	found := false
	for _, attr := range span.Attributes {
		if attr.Key == "matched" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'matched' attribute not set — where clause may have failed")
	}
}

func TestTransformRunData_SetResourceAttribute(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	statements := []string{`set(resource.attributes["env"], "staging")`}
	result, _, err := TransformRunData(traceData, statements)
	if err != nil {
		t.Fatalf("TransformRunData: %v", err)
	}

	var resultReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(result, &resultReq); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	resource := resultReq.ResourceSpans[0].Resource
	found := false
	for _, attr := range resource.Attributes {
		if attr.Key == "env" {
			found = true
			if v, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue); !ok || v.StringValue != "staging" {
				t.Errorf("env = %v, want 'staging'", attr.Value)
			}
		}
	}
	if !found {
		t.Error("resource attribute 'env' not found after set()")
	}
}

// --- TransformSpans tests ---

func traceToPdata(req *collectortracev1.ExportTraceServiceRequest) (ptrace.Traces, error) {
	protoBytes, err := proto.Marshal(req)
	if err != nil {
		return ptrace.NewTraces(), err
	}
	var unmarshaler ptrace.ProtoUnmarshaler
	return unmarshaler.UnmarshalTraces(protoBytes)
}

func pdataToTrace(traces ptrace.Traces) (*collectortracev1.ExportTraceServiceRequest, error) {
	var marshaler ptrace.ProtoMarshaler
	outBytes, err := marshaler.MarshalTraces(traces)
	if err != nil {
		return nil, err
	}
	var dstReq collectortracev1.ExportTraceServiceRequest
	if err := proto.Unmarshal(outBytes, &dstReq); err != nil {
		return nil, err
	}
	return &dstReq, nil
}

func TestTransformSpans_SetAttribute(t *testing.T) {
	req := makeSampleTrace()
	traces, err := traceToPdata(req)
	if err != nil {
		t.Fatalf("traceToPdata: %v", err)
	}

	statements := []string{`set(attributes["new.key"], "new-value")`}
	result, count, err := TransformSpans(traces, statements, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformSpans: %v", err)
	}

	if count != 1 {
		t.Errorf("span count = %d, want 1", count)
	}

	resultReq, err := pdataToTrace(result)
	if err != nil {
		t.Fatalf("pdataToTrace: %v", err)
	}

	spans := resultReq.ResourceSpans[0].ScopeSpans[0].Spans
	if len(spans) == 0 {
		t.Fatal("result has no Spans")
	}
	span := spans[0]

	foundNew := false
	foundOriginal := false
	for _, attr := range span.Attributes {
		if attr.Key == "new.key" {
			foundNew = true
			if v, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue); !ok || v.StringValue != "new-value" {
				t.Errorf("new.key = %v, want 'new-value'", attr.Value)
			}
		}
		if attr.Key == "original.key" {
			foundOriginal = true
		}
	}
	if !foundNew {
		t.Error("new.key attribute not found after set()")
	}
	if !foundOriginal {
		t.Error("original.key attribute missing after transform")
	}
}

func TestTransformSpans_DeleteKey(t *testing.T) {
	req := makeSampleTrace()
	traces, err := traceToPdata(req)
	if err != nil {
		t.Fatalf("traceToPdata: %v", err)
	}

	statements := []string{`delete_key(attributes, "original.key")`}
	result, count, err := TransformSpans(traces, statements, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformSpans: %v", err)
	}

	if count != 1 {
		t.Errorf("span count = %d, want 1", count)
	}

	resultReq, err := pdataToTrace(result)
	if err != nil {
		t.Fatalf("pdataToTrace: %v", err)
	}

	span := resultReq.ResourceSpans[0].ScopeSpans[0].Spans[0]
	for _, attr := range span.Attributes {
		if attr.Key == "original.key" {
			t.Error("original.key should have been deleted")
		}
	}
}

func TestTransformSpans_KeepKeys(t *testing.T) {
	req := makeSampleTrace()
	traces, err := traceToPdata(req)
	if err != nil {
		t.Fatalf("traceToPdata: %v", err)
	}

	statements := []string{`keep_keys(attributes, ["original.key"])`}
	result, count, err := TransformSpans(traces, statements, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformSpans: %v", err)
	}

	if count != 1 {
		t.Errorf("span count = %d, want 1", count)
	}

	resultReq, err := pdataToTrace(result)
	if err != nil {
		t.Fatalf("pdataToTrace: %v", err)
	}

	span := resultReq.ResourceSpans[0].ScopeSpans[0].Spans[0]
	found := false
	for _, attr := range span.Attributes {
		if attr.Key == "original.key" {
			found = true
		}
	}
	if !found {
		t.Error("original.key should be present after keep_keys")
	}
	if len(span.Attributes) != 1 {
		t.Errorf("expected exactly 1 attribute after keep_keys, got %d", len(span.Attributes))
	}
}

func TestTransformSpans_ErrorModePropagate(t *testing.T) {
	req := makeSampleTrace()
	traces, err := traceToPdata(req)
	if err != nil {
		t.Fatalf("traceToPdata: %v", err)
	}

	statements := []string{
		`set(attributes["first"], "ok")`,
		`set(attributes["second"], Substring("hello", -1, 10))`,
	}
	_, _, err = TransformSpans(traces, statements, ErrorModePropagate)
	if err == nil {
		t.Fatal("expected error for ErrorModePropagate, got nil")
	}
}

func TestTransformSpans_ErrorModeIgnore(t *testing.T) {
	req := makeSampleTrace()
	traces, err := traceToPdata(req)
	if err != nil {
		t.Fatalf("traceToPdata: %v", err)
	}

	statements := []string{
		`set(attributes["first"], "ok")`,
		`set(attributes["second"], Substring("hello", -1, 10))`,
	}
	result, count, err := TransformSpans(traces, statements, ErrorModeIgnore)
	if err != nil {
		t.Fatalf("TransformSpans with ErrorModeIgnore: %v", err)
	}

	if count != 1 {
		t.Errorf("span count = %d, want 1", count)
	}

	resultReq, err := pdataToTrace(result)
	if err != nil {
		t.Fatalf("pdataToTrace: %v", err)
	}

	span := resultReq.ResourceSpans[0].ScopeSpans[0].Spans[0]
	found := false
	for _, attr := range span.Attributes {
		if attr.Key == "first" {
			found = true
			if v, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue); !ok || v.StringValue != "ok" {
				t.Errorf("first = %v, want 'ok'", attr.Value)
			}
		}
	}
	if !found {
		t.Error("'first' attribute should have been set even with error")
	}
}
