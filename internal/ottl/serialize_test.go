package ottl

import (
	"bytes"
	"encoding/json"
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestSerializeRoundTrip(t *testing.T) {
	req := makeSampleTrace()
	originalJSON, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshalTrace: %v", err)
	}

	traces, err := UnmarshalTracesFromJSON(originalJSON)
	if err != nil {
		t.Fatalf("UnmarshalTracesFromJSON: %v", err)
	}

	_, err = MarshalTracesToJSON(traces)
	if err != nil {
		t.Fatalf("MarshalTracesToJSON: %v", err)
	}

	// Verify the round-tripped pdata matches the original structurally.
	// We compare pdata fields rather than proto.Equal because the pdata
	// round-trip can initialize zero-value fields (e.g. Span.Status).
	rss := traces.ResourceSpans()
	if rss.Len() != 1 {
		t.Fatalf("expected 1 ResourceSpans, got %d", rss.Len())
	}
	sss := rss.At(0).ScopeSpans()
	if sss.Len() != 1 {
		t.Fatalf("expected 1 ScopeSpans, got %d", sss.Len())
	}
	spans := sss.At(0).Spans()
	if spans.Len() != 1 {
		t.Fatalf("expected 1 Span, got %d", spans.Len())
	}
	span := spans.At(0)
	if span.Name() != "test-span" {
		t.Errorf("expected span name 'test-span', got %q", span.Name())
	}
	if span.Kind() != 2 { // SPAN_KIND_SERVER = 2
		t.Errorf("expected SPAN_KIND_SERVER (2), got %d", span.Kind())
	}
	attr, ok := span.Attributes().Get("original.key")
	if !ok {
		t.Fatal("expected attribute 'original.key'")
	}
	if attr.Str() != "original" {
		t.Errorf("expected attribute value 'original', got %q", attr.Str())
	}

	// Also verify we can unmarshal the original back to proto and it matches.
	var origProto collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(originalJSON, &origProto); err != nil {
		t.Fatalf("unmarshal original: %v", err)
	}
	if len(origProto.ResourceSpans) != 1 {
		t.Fatalf("expected 1 ResourceSpans in proto, got %d", len(origProto.ResourceSpans))
	}
	if len(origProto.ResourceSpans[0].ScopeSpans) != 1 {
		t.Fatalf("expected 1 ScopeSpans in proto, got %d", len(origProto.ResourceSpans[0].ScopeSpans))
	}
	origSpan := origProto.ResourceSpans[0].ScopeSpans[0].Spans[0]
	if origSpan.Name != "test-span" {
		t.Errorf("expected span name 'test-span', got %q", origSpan.Name)
	}
	if origSpan.Kind != tracev1.Span_SPAN_KIND_SERVER {
		t.Errorf("expected SPAN_KIND_SERVER, got %v", origSpan.Kind)
	}
	if len(origSpan.Attributes) != 1 || origSpan.Attributes[0].Key != "original.key" {
		t.Errorf("expected attribute 'original.key'")
	}
}

func TestSerializeInvalidData(t *testing.T) {
	_, err := UnmarshalTracesFromJSON(json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestSerialize_EmptyTraces(t *testing.T) {
	traces := ptrace.NewTraces()
	data, err := MarshalTracesToJSON(traces)
	if err != nil {
		t.Fatalf("MarshalTracesToJSON: %v", err)
	}

	roundTripped, err := UnmarshalTracesFromJSON(data)
	if err != nil {
		t.Fatalf("UnmarshalTracesFromJSON: %v", err)
	}

	if roundTripped.ResourceSpans().Len() != 0 {
		t.Errorf("expected 0 ResourceSpans, got %d", roundTripped.ResourceSpans().Len())
	}
}

func TestSerialize_NilData(t *testing.T) {
	_, err := UnmarshalTracesFromJSON(nil)
	if err == nil {
		t.Fatal("expected error for nil data")
	}
}

func TestMarshalTracesToJSON_Compact(t *testing.T) {
	req := makeSampleTrace()
	originalJSON, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshalTrace: %v", err)
	}

	traces, err := UnmarshalTracesFromJSON(originalJSON)
	if err != nil {
		t.Fatalf("UnmarshalTracesFromJSON: %v", err)
	}

	data, err := MarshalTracesToJSON(traces)
	if err != nil {
		t.Fatalf("MarshalTracesToJSON: %v", err)
	}

	if bytes.Contains(data, []byte("\n  ")) {
		t.Errorf("MarshalTracesToJSON produced pretty-print indent whitespace: %s", data)
	}

	roundTripped, err := UnmarshalTracesFromJSON(data)
	if err != nil {
		t.Fatalf("round-trip UnmarshalTracesFromJSON: %v", err)
	}
	if roundTripped.ResourceSpans().Len() != 1 {
		t.Fatalf("expected 1 ResourceSpans after round-trip, got %d", roundTripped.ResourceSpans().Len())
	}
	span := roundTripped.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	if span.Name() != "test-span" {
		t.Errorf("expected span name 'test-span', got %q", span.Name())
	}
}
