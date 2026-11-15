package engine

import (
	"context"
	"encoding/json"
	"testing"
)

// TestLogRunDataAcceptsHexIDs guards the stored-run JSON contract: records carry
// hexadecimal trace/span IDs, and every log reader must convert them instead of
// handing them to protojson as base64.
func TestLogRunDataAcceptsHexIDs(t *testing.T) {
	data := json.RawMessage(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"traceId":"b972764f1800c1f48ef4ce148c00299c","spanId":"6a22b8b6c2fade3c","body":{"stringValue":"hi"}}]}]}]}`)
	groups := []ContextStatements{{
		Context:    "log",
		Statements: []string{`set(attributes["x"], "1")`},
	}}

	out, count, err := TransformLogRunDataWithGroups(context.Background(), data, groups, ErrorModeIgnore)
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	var payload struct {
		ResourceLogs []struct {
			ScopeLogs []struct {
				LogRecords []struct {
					TraceID string `json:"traceId"`
					SpanID  string `json:"spanId"`
				} `json:"logRecords"`
			} `json:"scopeLogs"`
		} `json:"resourceLogs"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	rec := payload.ResourceLogs[0].ScopeLogs[0].LogRecords[0]
	if rec.TraceID != "b972764f1800c1f48ef4ce148c00299c" || rec.SpanID != "6a22b8b6c2fade3c" {
		t.Fatalf("IDs changed: trace=%q span=%q", rec.TraceID, rec.SpanID)
	}

	// The same stored payload must survive the filter path.
	filtered, _, kept, err := FilterLogRunDataWithErrorMode(context.Background(), data, []string{`body == "hi"`}, FilterModeInclude, ErrorModeIgnore)
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if kept != 1 {
		t.Fatalf("kept = %d, want 1", kept)
	}
	if err := json.Unmarshal(filtered, &payload); err != nil {
		t.Fatalf("decode filtered: %v", err)
	}
	if got := payload.ResourceLogs[0].ScopeLogs[0].LogRecords[0].TraceID; got != "b972764f1800c1f48ef4ce148c00299c" {
		t.Fatalf("filtered trace ID = %q", got)
	}
}
