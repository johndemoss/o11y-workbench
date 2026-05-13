package ottl

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestFilterRunData_IncludeMatch(t *testing.T) {
	req := makeMultiSpanTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	// include spans where name == "span-a"
	out, total, kept, err := FilterRunData(traceData, []string{`name == "span-a"`}, FilterModeInclude)
	if err != nil {
		t.Fatalf("FilterRunData: %v", err)
	}

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if kept != 1 {
		t.Errorf("kept = %d, want 1", kept)
	}

	var result collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// should have one span named "span-a"
	spans := flattenSpans(&result)
	if len(spans) != 1 {
		t.Fatalf("result has %d spans, want 1", len(spans))
	}
	if spans[0].Name != "span-a" {
		t.Errorf("span name = %q, want 'span-a'", spans[0].Name)
	}
}

func TestFilterRunData_IncludeNoMatch(t *testing.T) {
	req := makeMultiSpanTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	// include spans where name == "nonexistent"
	out, total, kept, err := FilterRunData(traceData, []string{`name == "nonexistent"`}, FilterModeInclude)
	if err != nil {
		t.Fatalf("FilterRunData: %v", err)
	}

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if kept != 0 {
		t.Errorf("kept = %d, want 0", kept)
	}

	var result collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	spans := flattenSpans(&result)
	if len(spans) != 0 {
		t.Errorf("result has %d spans, want 0", len(spans))
	}
}

func TestFilterRunData_ExcludeMatch(t *testing.T) {
	req := makeMultiSpanTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	// exclude spans where name == "span-b" — should keep 2 spans
	out, total, kept, err := FilterRunData(traceData, []string{`name == "span-b"`}, FilterModeExclude)
	if err != nil {
		t.Fatalf("FilterRunData: %v", err)
	}

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if kept != 2 {
		t.Errorf("kept = %d, want 2", kept)
	}

	var result collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	spans := flattenSpans(&result)
	if len(spans) != 2 {
		t.Fatalf("result has %d spans, want 2", len(spans))
	}
	names := make(map[string]bool)
	for _, s := range spans {
		names[s.Name] = true
	}
	if !names["span-a"] {
		t.Error("missing span-a")
	}
	if !names["span-c"] {
		t.Error("missing span-c")
	}
	if names["span-b"] {
		t.Error("span-b should have been excluded")
	}
}

func TestFilterRunData_AttributeCondition(t *testing.T) {
	req := makeMultiSpanTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	// include spans where http.method == "GET"
	out, total, kept, err := FilterRunData(traceData, []string{`attributes["http.method"] == "GET"`}, FilterModeInclude)
	if err != nil {
		t.Fatalf("FilterRunData: %v", err)
	}

	// span-a has http.method=GET, span-b has http.method=POST, span-c has no http.method
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if kept != 1 {
		t.Errorf("kept = %d, want 1", kept)
	}

	var result collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	spans := flattenSpans(&result)
	if len(spans) != 1 {
		t.Fatalf("result has %d spans, want 1", len(spans))
	}
	if spans[0].Name != "span-a" {
		t.Errorf("span name = %q, want 'span-a'", spans[0].Name)
	}
}

func TestFilterRunData_MultipleConditionsOr(t *testing.T) {
	req := makeMultiSpanTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	// include spans matching either condition (OR default)
	out, total, kept, err := FilterRunData(traceData, []string{
		`name == "span-a"`,
		`name == "span-c"`,
	}, FilterModeInclude)
	if err != nil {
		t.Fatalf("FilterRunData: %v", err)
	}

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if kept != 2 {
		t.Errorf("kept = %d, want 2", kept)
	}

	var result collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	spans := flattenSpans(&result)
	if len(spans) != 2 {
		t.Fatalf("result has %d spans, want 2", len(spans))
	}
}

func TestFilterRunData_InvalidCondition(t *testing.T) {
	req := makeMultiSpanTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	_, _, _, err = FilterRunData(traceData, []string{`this is not a valid condition`}, FilterModeInclude)
	if err == nil {
		t.Fatal("expected error for invalid condition")
	}
}

func TestFilterRunData_NoConditions(t *testing.T) {
	req := makeMultiSpanTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	_, _, _, err = FilterRunData(traceData, nil, FilterModeInclude)
	if err == nil {
		t.Fatal("expected error for no conditions")
	}
}

func TestFilterRunData_MultipleResourceSpans(t *testing.T) {
	// Build two resource_span groups, each with a scope_span with one span
	req := &collectortracev1.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{
			{
				Resource: &resourcev1.Resource{
					Attributes: []*commonv1.KeyValue{
						{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "svc-a"}}},
					},
				},
				ScopeSpans: []*tracev1.ScopeSpans{
					{
						Scope: &commonv1.InstrumentationScope{Name: "scope-a", Version: "1.0.0"},
						Spans: []*tracev1.Span{
							{
								TraceId: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
								SpanId:  []byte{0, 0, 0, 0, 0, 0, 0, 1},
								Name:    "svc-a-span",
								Kind:    tracev1.Span_SPAN_KIND_SERVER,
							},
						},
					},
				},
			},
			{
				Resource: &resourcev1.Resource{
					Attributes: []*commonv1.KeyValue{
						{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "svc-b"}}},
					},
				},
				ScopeSpans: []*tracev1.ScopeSpans{
					{
						Scope: &commonv1.InstrumentationScope{Name: "scope-b", Version: "1.0.0"},
						Spans: []*tracev1.Span{
							{
								TraceId: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
								SpanId:  []byte{0, 0, 0, 0, 0, 0, 0, 2},
								Name:    "svc-b-span",
								Kind:    tracev1.Span_SPAN_KIND_SERVER,
							},
						},
					},
				},
			},
		},
	}

	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	// include spans named "svc-a-span"
	out, total, kept, err := FilterRunData(traceData, []string{`name == "svc-a-span"`}, FilterModeInclude)
	if err != nil {
		t.Fatalf("FilterRunData: %v", err)
	}

	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if kept != 1 {
		t.Errorf("kept = %d, want 1", kept)
	}

	var result collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// should have 1 resource span, 1 scope span, 1 span
	spans := flattenSpans(&result)
	if len(spans) != 1 {
		t.Fatalf("result has %d spans, want 1", len(spans))
	}
	if spans[0].Name != "svc-a-span" {
		t.Errorf("span name = %q, want 'svc-a-span'", spans[0].Name)
	}
	if len(result.ResourceSpans) != 1 {
		t.Errorf("resource groups = %d, want 1", len(result.ResourceSpans))
	}
}

// makeMultiSpanTrace builds a trace with 3 spans across 1 resource + 1 scope.
func makeMultiSpanTrace() *collectortracev1.ExportTraceServiceRequest {
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
								Name:    "span-a",
								Kind:    tracev1.Span_SPAN_KIND_SERVER,
								Attributes: []*commonv1.KeyValue{
									{Key: "http.method", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "GET"}}},
									{Key: "http.status_code", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 200}}},
								},
							},
							{
								TraceId: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
								SpanId:  []byte{0, 0, 0, 0, 0, 0, 0, 2},
								Name:    "span-b",
								Kind:    tracev1.Span_SPAN_KIND_INTERNAL,
								Attributes: []*commonv1.KeyValue{
									{Key: "http.method", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "POST"}}},
								},
							},
							{
								TraceId: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
								SpanId:  []byte{0, 0, 0, 0, 0, 0, 0, 3},
								Name:    "span-c",
								Kind:    tracev1.Span_SPAN_KIND_CLIENT,
							},
						},
					},
				},
			},
		},
	}
}

// flattenSpans returns all spans from all resource/scope groups.
func flattenSpans(req *collectortracev1.ExportTraceServiceRequest) []*tracev1.Span {
	var out []*tracev1.Span
	for _, rss := range req.ResourceSpans {
		for _, sss := range rss.ScopeSpans {
			out = append(out, sss.Spans...)
		}
	}
	return out
}


