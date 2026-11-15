package ottl

import (
	"context"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
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
	out, total, kept, err := FilterRunData(context.Background(), traceData, []string{`name == "span-a"`}, FilterModeInclude)
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
	out, total, kept, err := FilterRunData(context.Background(), traceData, []string{`name == "nonexistent"`}, FilterModeInclude)
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
	out, total, kept, err := FilterRunData(context.Background(), traceData, []string{`name == "span-b"`}, FilterModeExclude)
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
	out, total, kept, err := FilterRunData(context.Background(), traceData, []string{`attributes["http.method"] == "GET"`}, FilterModeInclude)
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
	out, total, kept, err := FilterRunData(context.Background(), traceData, []string{
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

	_, _, _, err = FilterRunData(context.Background(), traceData, []string{`this is not a valid condition`}, FilterModeInclude)
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

	_, _, _, err = FilterRunData(context.Background(), traceData, nil, FilterModeInclude)
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
	out, total, kept, err := FilterRunData(context.Background(), traceData, []string{`name == "svc-a-span"`}, FilterModeInclude)
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

// makePdataTrace builds a ptrace.Traces equivalent to makeMultiSpanTrace.
func makePdataTrace() ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-svc")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("test-scope")
	ss.Scope().SetVersion("1.0.0")

	spanA := ss.Spans().AppendEmpty()
	spanA.SetName("span-a")
	spanA.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}))
	spanA.SetTraceID(pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}))
	spanA.Attributes().PutStr("http.method", "GET")
	spanA.Attributes().PutInt("http.status_code", 200)

	spanB := ss.Spans().AppendEmpty()
	spanB.SetName("span-b")
	spanB.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2}))
	spanB.SetTraceID(pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}))
	spanB.Attributes().PutStr("http.method", "POST")

	spanC := ss.Spans().AppendEmpty()
	spanC.SetName("span-c")
	spanC.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 3}))
	spanC.SetTraceID(pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}))

	return traces
}

func TestFilterSpans_Include(t *testing.T) {
	traces := makePdataTrace()
	filtered, total, kept, err := FilterSpans(context.Background(), traces, []string{`name == "span-a"`}, FilterModeInclude, ErrorModePropagate)
	if err != nil {
		t.Fatalf("FilterSpans: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if kept != 1 {
		t.Errorf("kept = %d, want 1", kept)
	}
	rss := filtered.ResourceSpans()
	if rss.Len() != 1 {
		t.Fatalf("resource spans = %d, want 1", rss.Len())
	}
	spans := rss.At(0).ScopeSpans().At(0).Spans()
	if spans.Len() != 1 {
		t.Fatalf("spans = %d, want 1", spans.Len())
	}
	if spans.At(0).Name() != "span-a" {
		t.Errorf("span name = %q, want 'span-a'", spans.At(0).Name())
	}
}

func TestFilterSpans_Exclude(t *testing.T) {
	traces := makePdataTrace()
	filtered, total, kept, err := FilterSpans(context.Background(), traces, []string{`name == "span-b"`}, FilterModeExclude, ErrorModePropagate)
	if err != nil {
		t.Fatalf("FilterSpans: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if kept != 2 {
		t.Errorf("kept = %d, want 2", kept)
	}
	rss := filtered.ResourceSpans()
	if rss.Len() != 1 {
		t.Fatalf("resource spans = %d, want 1", rss.Len())
	}
	spans := rss.At(0).ScopeSpans().At(0).Spans()
	if spans.Len() != 2 {
		t.Fatalf("spans = %d, want 2", spans.Len())
	}
	names := make(map[string]bool)
	for i := 0; i < spans.Len(); i++ {
		names[spans.At(i).Name()] = true
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

func TestFilterSpans_ErrorModePropagate(t *testing.T) {
	traces := makePdataTrace()
	_, _, _, err := FilterSpans(context.Background(), traces, []string{`Substring(name, 10, 1) == "x"`}, FilterModeInclude, ErrorModePropagate)
	if err == nil {
		t.Fatal("expected error for bad substring condition")
	}
}

func TestFilterSpans_ErrorModeIgnore(t *testing.T) {
	traces := makePdataTrace()
	_, total, kept, err := FilterSpans(context.Background(), traces, []string{`Substring(name, 10, 1) == "x"`}, FilterModeInclude, ErrorModeIgnore)
	if err != nil {
		t.Fatalf("FilterSpans: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if kept != 0 {
		t.Errorf("kept = %d, want 0 (all spans should be dropped due to eval errors)", kept)
	}
}

func TestFilterLogRunData_Include(t *testing.T) {
	req := &collectorlogsv1.ExportLogsServiceRequest{
		ResourceLogs: []*logsv1.ResourceLogs{
			{
				ScopeLogs: []*logsv1.ScopeLogs{
					{
						LogRecords: []*logsv1.LogRecord{
							{Body: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "keep"}}},
							{Body: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "drop"}}},
						},
					},
				},
			},
		},
	}
	logData, err := protojson.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	out, total, kept, err := FilterLogRunData(context.Background(), logData, []string{`body == "keep"`}, FilterModeInclude)
	if err != nil {
		t.Fatalf("FilterLogRunData: %v", err)
	}
	if total != 2 || kept != 1 {
		t.Fatalf("total=%d kept=%d, want 2/1", total, kept)
	}
	var filtered collectorlogsv1.ExportLogsServiceRequest
	if err := protojson.Unmarshal(out, &filtered); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(filtered.ResourceLogs[0].ScopeLogs[0].LogRecords) != 1 {
		t.Fatalf("log records = %d, want 1", len(filtered.ResourceLogs[0].ScopeLogs[0].LogRecords))
	}
}

func TestFilterLogRecords_ErrorModeIgnore(t *testing.T) {
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	for _, body := range []string{"a", "b", "c"} {
		lr := sl.LogRecords().AppendEmpty()
		lr.Body().SetStr(body)
	}

	_, total, kept, err := FilterLogRecords(context.Background(), logs, []string{`Substring(body, 10, 1) == "x"`}, FilterModeInclude, ErrorModeIgnore)
	if err != nil {
		t.Fatalf("FilterLogRecords: %v", err)
	}
	if total != 3 || kept != 0 {
		t.Fatalf("total=%d kept=%d, want 3/0", total, kept)
	}
}

func TestFilterMetricRunData_Include(t *testing.T) {
	req := &collectormetricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{
			{
				ScopeMetrics: []*metricspb.ScopeMetrics{
					{
						Metrics: []*metricspb.Metric{
							{
								Name: "m1",
								Metadata: []*commonv1.KeyValue{
									{Key: "code.attrs", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "keep"}}},
								},
								Data: &metricspb.Metric_Sum{
									Sum: &metricspb.Sum{
										DataPoints: []*metricspb.NumberDataPoint{
											{Attributes: []*commonv1.KeyValue{{Key: "env", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "prod"}}}}},
											{Attributes: []*commonv1.KeyValue{{Key: "env", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "dev"}}}}},
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
	metricData, err := protojson.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	out, total, kept, err := FilterMetricRunData(context.Background(), metricData, []string{`attributes["env"] == "prod"`}, FilterModeInclude)
	if err != nil {
		t.Fatalf("FilterMetricRunData: %v", err)
	}
	if total != 2 || kept != 1 {
		t.Fatalf("total=%d kept=%d, want 2/1", total, kept)
	}
	var filtered collectormetricsv1.ExportMetricsServiceRequest
	if err := protojson.Unmarshal(out, &filtered); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	m := filtered.ResourceMetrics[0].ScopeMetrics[0].Metrics[0]
	if len(m.GetMetadata()) != 1 || m.GetMetadata()[0].Key != "code.attrs" {
		t.Fatal("metric metadata not preserved")
	}
	if len(m.GetSum().DataPoints) != 1 {
		t.Fatalf("data points = %d, want 1", len(m.GetSum().DataPoints))
	}
}

func TestSelectFilterConditions_NoTraceFallback(t *testing.T) {
	yamlStr := `
traces:
  span:
    - name == "trace-only"
`
	conds, err := SelectFilterConditions(yamlStr, "logs")
	if err != nil {
		t.Fatalf("SelectFilterConditions: %v", err)
	}
	if len(conds) != 0 {
		t.Fatalf("conds = %v, want empty (must not fall back to trace rules)", conds)
	}
	flat := `
- body == "flat-only"
`
	conds, err = SelectFilterConditions(flat, "logs")
	if err != nil {
		t.Fatalf("SelectFilterConditions flat: %v", err)
	}
	if len(conds) != 1 || conds[0] != `body == "flat-only"` {
		t.Fatalf("flat conds = %v", conds)
	}
}

func TestSelectFilterConditions_Logs(t *testing.T) {
	yamlStr := `
traces:
  span:
    - name == "trace-only"
logs:
  log_record:
    - body == "log-hit"
`
	conds, err := SelectFilterConditions(yamlStr, "logs")
	if err != nil {
		t.Fatalf("SelectFilterConditions: %v", err)
	}
	if len(conds) != 1 || conds[0] != `body == "log-hit"` {
		t.Fatalf("conds = %v, want log condition", conds)
	}
}
