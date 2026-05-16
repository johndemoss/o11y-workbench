package ottl

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

func attrString(spanAttr []*commonv1.KeyValue, key string) string {
	for _, attr := range spanAttr {
		if attr.Key == key {
			if v, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue); ok {
				return v.StringValue
			}
		}
	}
	return ""
}

func TestExecutePipeline_TwoStageTransform(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	pipeline := Pipeline{
		Stages: []Stage{
			{ID: "s1", Type: StageTypeTransform, ProcessorRef: "p1"},
			{ID: "s2", Type: StageTypeTransform, ProcessorRef: "p2"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {
				"statements": []interface{}{`set(attributes["env"], "prod")`},
			},
			"p2": {
				"statements": []interface{}{`set(attributes["matched"], "yes")`},
			},
		},
	}

	result, err := ExecutePipeline(traceData, pipeline)
	if err != nil {
		t.Fatalf("ExecutePipeline: %v", err)
	}

	if len(result.Stages) != 2 {
		t.Fatalf("got %d stages, want 2", len(result.Stages))
	}

	if result.Stages[0].InputSpanCount != 1 {
		t.Errorf("stage0 input = %d, want 1", result.Stages[0].InputSpanCount)
	}
	if result.Stages[0].OutputSpanCount != 1 {
		t.Errorf("stage0 output = %d, want 1", result.Stages[0].OutputSpanCount)
	}
	if result.Stages[1].InputSpanCount != 1 {
		t.Errorf("stage1 input = %d, want 1", result.Stages[1].InputSpanCount)
	}
	if result.Stages[1].OutputSpanCount != 1 {
		t.Errorf("stage1 output = %d, want 1", result.Stages[1].OutputSpanCount)
	}

	if result.FinalSpanCount != 1 {
		t.Errorf("FinalSpanCount = %d, want 1", result.FinalSpanCount)
	}

	var finalReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(result.FinalData, &finalReq); err != nil {
		t.Fatalf("unmarshal final data: %v", err)
	}

	spans := flattenSpans(&finalReq)
	if len(spans) != 1 {
		t.Fatalf("final span count = %d, want 1", len(spans))
	}

	if v := attrString(spans[0].Attributes, "env"); v != "prod" {
		t.Errorf("env = %q, want 'prod'", v)
	}
	if v := attrString(spans[0].Attributes, "matched"); v != "yes" {
		t.Errorf("matched = %q, want 'yes'", v)
	}
}

func TestExecutePipeline_ThreeStageTransformFilterTransform(t *testing.T) {
	req := makeMultiSpanTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	pipeline := Pipeline{
		Stages: []Stage{
			{ID: "s1", Type: StageTypeTransform, ProcessorRef: "p1"},
			{ID: "s2", Type: StageTypeFilter, ProcessorRef: "p2"},
			{ID: "s3", Type: StageTypeTransform, ProcessorRef: "p3"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {
				"statements": []interface{}{`set(attributes["stage1"], "done")`},
			},
			"p2": {
				"traces": map[string]interface{}{
					"span": []interface{}{`name != "span-c"`},
				},
			},
			"p3": {
				"statements": []interface{}{`set(attributes["checked"], "true")`},
			},
		},
	}

	result, err := ExecutePipeline(traceData, pipeline)
	if err != nil {
		t.Fatalf("ExecutePipeline: %v", err)
	}

	if len(result.Stages) != 3 {
		t.Fatalf("got %d stages, want 3", len(result.Stages))
	}

	if result.Stages[0].InputSpanCount != 3 {
		t.Errorf("stage0 input = %d, want 3", result.Stages[0].InputSpanCount)
	}
	if result.Stages[0].OutputSpanCount != 3 {
		t.Errorf("stage0 output = %d, want 3", result.Stages[0].OutputSpanCount)
	}

	if result.Stages[1].InputSpanCount != 3 {
		t.Errorf("stage1 input = %d, want 3", result.Stages[1].InputSpanCount)
	}
	if result.Stages[1].OutputSpanCount != 2 {
		t.Errorf("stage1 output = %d, want 2", result.Stages[1].OutputSpanCount)
	}

	if result.Stages[2].InputSpanCount != 2 {
		t.Errorf("stage2 input = %d, want 2", result.Stages[2].InputSpanCount)
	}
	if result.Stages[2].OutputSpanCount != 2 {
		t.Errorf("stage2 output = %d, want 2", result.Stages[2].OutputSpanCount)
	}

	if result.FinalSpanCount != 2 {
		t.Errorf("FinalSpanCount = %d, want 2", result.FinalSpanCount)
	}

	var finalReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(result.FinalData, &finalReq); err != nil {
		t.Fatalf("unmarshal final data: %v", err)
	}

	finalSpans := flattenSpans(&finalReq)
	if len(finalSpans) != 2 {
		t.Fatalf("final span count = %d, want 2", len(finalSpans))
	}

	names := make(map[string]bool)
	for _, s := range finalSpans {
		names[s.Name] = true

		if v := attrString(s.Attributes, "stage1"); v != "done" {
			t.Errorf("span %q: stage1 = %q, want 'done'", s.Name, v)
		}
		if v := attrString(s.Attributes, "checked"); v != "true" {
			t.Errorf("span %q: checked = %q, want 'true'", s.Name, v)
		}
	}

	if !names["span-a"] {
		t.Error("missing span-a")
	}
	if !names["span-b"] {
		t.Error("missing span-b")
	}
	if names["span-c"] {
		t.Error("span-c should have been filtered out")
	}
}

func TestExecutePipeline_EmptyStages(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	pipeline := Pipeline{
		Stages:     []Stage{},
		Processors: map[string]ProcessorConfig{},
	}

	result, err := ExecutePipeline(traceData, pipeline)
	if err != nil {
		t.Fatalf("ExecutePipeline: %v", err)
	}

	if len(result.Stages) != 0 {
		t.Errorf("got %d stages, want 0", len(result.Stages))
	}

	if result.FinalSpanCount != 1 {
		t.Errorf("FinalSpanCount = %d, want 1", result.FinalSpanCount)
	}

	var finalReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(result.FinalData, &finalReq); err != nil {
		t.Fatalf("unmarshal final data: %v", err)
	}

	spans := flattenSpans(&finalReq)
	if len(spans) != 1 {
		t.Fatalf("final span count = %d, want 1", len(spans))
	}

	if spans[0].Name != "test-span" {
		t.Errorf("span name = %q, want 'test-span'", spans[0].Name)
	}

	if v := attrString(spans[0].Attributes, "original.key"); v != "original" {
		t.Errorf("original.key = %q, want 'original'", v)
	}
}

func TestExecutePipeline_ErrorModePropagate(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	pipeline := Pipeline{
		Stages: []Stage{
			{ID: "bad", Type: StageTypeTransform, ErrorMode: ErrorModePropagate, ProcessorRef: "badproc"},
		},
		Processors: map[string]ProcessorConfig{
			"badproc": {
				"statements": []interface{}{`this is not valid ottl`},
			},
		},
	}

	_, err = ExecutePipeline(traceData, pipeline)
	if err == nil {
		t.Fatal("expected error for ErrorModePropagate, got nil")
	}
}

func TestExecutePipeline_ErrorModeIgnore(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	pipeline := Pipeline{
		Stages: []Stage{
			{ID: "bad", Type: StageTypeTransform, ErrorMode: ErrorModeIgnore, ProcessorRef: "badproc"},
			{ID: "good", Type: StageTypeTransform, ProcessorRef: "goodproc"},
		},
		Processors: map[string]ProcessorConfig{
			"badproc": {
				"statements": []interface{}{`this is not valid ottl`},
			},
			"goodproc": {
				"statements": []interface{}{`set(attributes["env"], "prod")`},
			},
		},
	}

	result, err := ExecutePipeline(traceData, pipeline)
	if err != nil {
		t.Fatalf("ExecutePipeline with ErrorModeIgnore: %v", err)
	}

	if len(result.Stages) != 2 {
		t.Fatalf("got %d stages, want 2", len(result.Stages))
	}

	if len(result.Stages[0].Errors) == 0 {
		t.Error("expected error in stage 0, got none")
	}

	if result.Stages[0].InputSpanCount != 1 {
		t.Errorf("stage0 input = %d, want 1", result.Stages[0].InputSpanCount)
	}
	if result.Stages[0].OutputSpanCount != 1 {
		t.Errorf("stage0 output = %d, want 1 (unchanged after error)", result.Stages[0].OutputSpanCount)
	}

	if result.Stages[1].InputSpanCount != 1 {
		t.Errorf("stage1 input = %d, want 1", result.Stages[1].InputSpanCount)
	}
	if result.Stages[1].OutputSpanCount != 1 {
		t.Errorf("stage1 output = %d, want 1", result.Stages[1].OutputSpanCount)
	}

	var finalReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(result.FinalData, &finalReq); err != nil {
		t.Fatalf("unmarshal final data: %v", err)
	}

	spans := flattenSpans(&finalReq)
	if len(spans) != 1 {
		t.Fatalf("final span count = %d, want 1", len(spans))
	}

	if v := attrString(spans[0].Attributes, "env"); v != "prod" {
		t.Errorf("env = %q, want 'prod'", v)
	}
}

func TestExecutePipeline_StageWithNoStatements(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	pipeline := Pipeline{
		Stages: []Stage{
			{ID: "empty", Type: StageTypeTransform, ProcessorRef: "empty"},
			{ID: "setter", Type: StageTypeTransform, ProcessorRef: "setter"},
		},
		Processors: map[string]ProcessorConfig{
			"empty":  {},
			"setter": {"statements": []interface{}{`set(attributes["env"], "prod")`}},
		},
	}

	result, err := ExecutePipeline(traceData, pipeline)
	if err != nil {
		t.Fatalf("ExecutePipeline: %v", err)
	}

	if len(result.Stages) != 2 {
		t.Fatalf("got %d stages, want 2", len(result.Stages))
	}

	if result.Stages[0].StatementsParsed != 0 {
		t.Errorf("stage0 statements parsed = %d, want 0", result.Stages[0].StatementsParsed)
	}
	if result.Stages[0].InputSpanCount != 1 {
		t.Errorf("stage0 input = %d, want 1", result.Stages[0].InputSpanCount)
	}
	if result.Stages[0].OutputSpanCount != 1 {
		t.Errorf("stage0 output = %d, want 1", result.Stages[0].OutputSpanCount)
	}

	if result.Stages[1].StatementsParsed != 1 {
		t.Errorf("stage1 statements parsed = %d, want 1", result.Stages[1].StatementsParsed)
	}

	var finalReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(result.FinalData, &finalReq); err != nil {
		t.Fatalf("unmarshal final data: %v", err)
	}

	spans := flattenSpans(&finalReq)
	if len(spans) != 1 {
		t.Fatalf("final span count = %d, want 1", len(spans))
	}

	if v := attrString(spans[0].Attributes, "env"); v != "prod" {
		t.Errorf("env = %q, want 'prod'", v)
	}
}

func TestExecutePipeline_IntermediateDataCaptured(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	pipeline := Pipeline{
		Stages: []Stage{
			{ID: "s1", Type: StageTypeTransform, ProcessorRef: "p1"},
			{ID: "s2", Type: StageTypeTransform, ProcessorRef: "p2"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {
				"statements": []interface{}{`set(attributes["env"], "prod")`},
			},
			"p2": {
				"statements": []interface{}{`set(attributes["matched"], "yes")`},
			},
		},
	}

	result, err := ExecutePipeline(traceData, pipeline)
	if err != nil {
		t.Fatalf("ExecutePipeline: %v", err)
	}

	for i, sr := range result.Stages {
		if len(sr.IntermediateData) == 0 {
			t.Errorf("stage %d has no intermediate data", i)
		}
	}

	if len(result.Stages[0].IntermediateData) > 0 {
		var stage1Req collectortracev1.ExportTraceServiceRequest
		if err := protojson.Unmarshal(result.Stages[0].IntermediateData, &stage1Req); err == nil {
			s1spans := flattenSpans(&stage1Req)
			if len(s1spans) == 1 {
				if v := attrString(s1spans[0].Attributes, "env"); v != "prod" {
					t.Errorf("stage1 intermediate: env = %q, want 'prod'", v)
				}
			}
		}
	}
}

func TestExecutePipeline_ErrorModeSilent(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	pipeline := Pipeline{
		Stages: []Stage{
			{ID: "bad", Type: StageTypeTransform, ErrorMode: ErrorModeSilent, ProcessorRef: "badproc"},
			{ID: "good", Type: StageTypeTransform, ProcessorRef: "goodproc"},
		},
		Processors: map[string]ProcessorConfig{
			"badproc": {
				"statements": []interface{}{`this is not valid ottl`},
			},
			"goodproc": {
				"statements": []interface{}{`set(attributes["env"], "prod")`},
			},
		},
	}

	result, err := ExecutePipeline(traceData, pipeline)
	if err != nil {
		t.Fatalf("ExecutePipeline with ErrorModeSilent: %v", err)
	}

	if len(result.Stages) != 2 {
		t.Fatalf("got %d stages, want 2", len(result.Stages))
	}

	if len(result.Stages[0].Errors) != 0 {
		t.Errorf("expected no errors in stage 0 for silent mode, got %d", len(result.Stages[0].Errors))
	}

	var finalReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(result.FinalData, &finalReq); err != nil {
		t.Fatalf("unmarshal final data: %v", err)
	}

	spans := flattenSpans(&finalReq)
	if len(spans) != 1 {
		t.Fatalf("final span count = %d, want 1", len(spans))
	}

	if v := attrString(spans[0].Attributes, "env"); v != "prod" {
		t.Errorf("env = %q, want 'prod'", v)
	}
}

func TestExecutePipeline_NilTraceData(t *testing.T) {
	pipeline := Pipeline{
		Stages: []Stage{
			{ID: "s1", Type: StageTypeTransform, ProcessorRef: "p1"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {"statements": []interface{}{`set(attributes["env"], "prod")`}},
		},
	}

	_, err := ExecutePipeline(nil, pipeline)
	if err == nil {
		t.Fatal("expected error for nil trace data")
	}
}

func TestExecutePipeline_UnknownStageType(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	pipeline := Pipeline{
		Stages: []Stage{
			{ID: "s1", Type: "bogus", ProcessorRef: "p1"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {"statements": []interface{}{`set(attributes["env"], "prod")`}},
		},
	}

	_, err = ExecutePipeline(traceData, pipeline)
	if err == nil {
		t.Fatal("expected error for unknown stage type")
	}
}

func TestExecutePipeline_MissingProcessorConfig(t *testing.T) {
	req := makeSampleTrace()
	traceData, err := marshalTrace(req)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}

	pipeline := Pipeline{
		Stages: []Stage{
			{ID: "s1", Type: StageTypeTransform, ProcessorRef: "nonexistent"},
		},
		Processors: map[string]ProcessorConfig{
			"p1": {"statements": []interface{}{`set(attributes["env"], "prod")`}},
		},
	}

	_, err = ExecutePipeline(traceData, pipeline)
	if err == nil {
		t.Fatal("expected error for missing processor ref")
	}
}