package engine

import (
	"context"
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/protobuf/proto"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

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
						Scope: &commonv1.InstrumentationScope{Name: "test-scope"},
						Spans: []*tracev1.Span{
							{
								TraceId:           []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
								SpanId:            []byte{0, 0, 0, 0, 0, 0, 0, 1},
								Name:              "test-span",
								StartTimeUnixNano: 1,
								EndTimeUnixNano:   2,
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

func makeSampleTracesTwoSpans() *collectortracev1.ExportTraceServiceRequest {
	return &collectortracev1.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{
			{
				Resource: &resourcev1.Resource{},
				ScopeSpans: []*tracev1.ScopeSpans{
					{
						Spans: []*tracev1.Span{
							{
								TraceId:           []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
								SpanId:            []byte{0, 0, 0, 0, 0, 0, 0, 1},
								Name:              "span-a",
								StartTimeUnixNano: 1,
								EndTimeUnixNano:   2,
								Attributes: []*commonv1.KeyValue{
									{Key: "mark", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "info"}}},
								},
							},
							{
								TraceId:           []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
								SpanId:            []byte{0, 0, 0, 0, 0, 0, 0, 2},
								Name:              "span-b",
								StartTimeUnixNano: 1,
								EndTimeUnixNano:   2,
								Attributes: []*commonv1.KeyValue{
									{Key: "mark", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "debug"}}},
								},
							},
						},
					},
				},
			},
		},
	}
}

func spanHasAttr(span *tracev1.Span, key, want string) bool {
	for _, attr := range span.Attributes {
		if attr.Key == key {
			if v, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue); ok {
				return v.StringValue == want
			}
		}
	}
	return false
}

func TestTransformSpansWithGroups_ConditionalOR(t *testing.T) {
	ctx := context.Background()
	traces, err := traceToPdata(makeSampleTracesTwoSpans())
	if err != nil {
		t.Fatalf("traceToPdata: %v", err)
	}

	groups := []ContextStatements{
		{
			Conditions: []string{
				`attributes["mark"] == "info"`,
				`attributes["mark"] == "warn"`,
			},
			Statements: []string{`set(attributes["tagged"], "yes")`},
		},
	}

	result, count, err := TransformSpansWithGroups(ctx, traces, groups, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformSpansWithGroups: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	resultReq, err := pdataToTrace(result)
	if err != nil {
		t.Fatalf("pdataToTrace: %v", err)
	}
	spans := resultReq.ResourceSpans[0].ScopeSpans[0].Spans
	if len(spans) != 2 {
		t.Fatalf("spans = %d, want 2", len(spans))
	}
	if !spanHasAttr(spans[0], "tagged", "yes") {
		t.Error("info span should match OR condition")
	}
	if spanHasAttr(spans[1], "tagged", "yes") {
		t.Error("debug span should not be tagged")
	}
}

func TestTransformSpansWithGroups_ResourceOnceForTwoSpans(t *testing.T) {
	ctx := context.Background()
	traces, err := traceToPdata(makeSampleTracesTwoSpans())
	if err != nil {
		t.Fatalf("traceToPdata: %v", err)
	}

	groups := []ContextStatements{
		{Context: "resource", Statements: []string{`set(attributes["tier"], "gold")`}},
	}

	result, count, err := TransformSpansWithGroups(ctx, traces, groups, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformSpansWithGroups: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	resultReq, err := pdataToTrace(result)
	if err != nil {
		t.Fatalf("pdataToTrace: %v", err)
	}
	resource := resultReq.ResourceSpans[0].Resource
	found := false
	for _, attr := range resource.Attributes {
		if attr.Key == "tier" {
			found = true
			if v, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue); !ok || v.StringValue != "gold" {
				t.Errorf("tier = %v, want gold", attr.Value)
			}
		}
	}
	if !found {
		t.Fatal("resource tier not set after single resource-group execution")
	}
}

func TestTransformSpansWithGroups_ScopeOncePerScope(t *testing.T) {
	ctx := context.Background()
	traces, err := traceToPdata(makeSampleTracesTwoSpans())
	if err != nil {
		t.Fatalf("traceToPdata: %v", err)
	}

	groups := []ContextStatements{
		{Context: "scope", Statements: []string{`set(attributes["ver"], "2.0")`}},
	}

	_, count, err := TransformSpansWithGroups(ctx, traces, groups, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformSpansWithGroups: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestTransformSpansWithGroups_ErrorModeIgnoreContinuesStatements(t *testing.T) {
	ctx := context.Background()
	traces, err := traceToPdata(makeSampleTrace())
	if err != nil {
		t.Fatalf("traceToPdata: %v", err)
	}

	groups := []ContextStatements{
		{Statements: []string{
			`set(attributes["first"], "ok")`,
			`set(attributes["bad"], Substring("x", -1, 3))`,
		}},
	}

	result, count, err := TransformSpansWithGroups(ctx, traces, groups, ErrorModeIgnore)
	if err != nil {
		t.Fatalf("TransformSpansWithGroups ignore: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	resultReq, err := pdataToTrace(result)
	if err != nil {
		t.Fatalf("pdataToTrace: %v", err)
	}
	span := resultReq.ResourceSpans[0].ScopeSpans[0].Spans[0]
	if !spanHasAttr(span, "first", "ok") {
		t.Error("first=ok should be set after later statement fails with ignore mode")
	}
}

func TestTransformSpansWithGroups_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	traces, err := traceToPdata(makeSampleTrace())
	if err != nil {
		t.Fatalf("traceToPdata: %v", err)
	}

	groups := []ContextStatements{{Statements: []string{`set(attributes["k"], "v")`}}}
	_, _, err = TransformSpansWithGroups(ctx, traces, groups, ErrorModePropagate)
	if err == nil {
		t.Fatal("expected cancellation error from group execution")
	}
}

func TestTransformSpansWithGroups_UnsupportedContext(t *testing.T) {
	ctx := context.Background()
	traces, err := traceToPdata(makeSampleTrace())
	if err != nil {
		t.Fatalf("traceToPdata: %v", err)
	}

	groups := []ContextStatements{{Context: "log", Statements: []string{`set(attributes["x"], "y")`}}}
	_, _, err = TransformSpansWithGroups(ctx, traces, groups, ErrorModePropagate)
	if err == nil {
		t.Fatal("expected error for unsupported trace context")
	}
}
