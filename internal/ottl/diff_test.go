package ottl

import (
	"encoding/json"
	"testing"
)

func TestSignalDiffRecords(t *testing.T) {
	tests := []struct{ signal, before, after string }{
		{"traces", `{"resourceSpans":[{"resource":{"attributes":[{"key":"env","value":{"stringValue":"a"}}]},"scopeSpans":[{"spans":[{"traceId":"t1","spanId":"s","attributes":[{"key":"x","value":{"stringValue":"a"}}]},{"traceId":"t2","spanId":"s"}]}]}]}`, `{"resourceSpans":[{"resource":{"attributes":[{"key":"env","value":{"stringValue":"b"}}]},"scopeSpans":[{"spans":[{"traceId":"t1","spanId":"s","attributes":[{"key":"x","value":{"stringValue":"b"}}]}]}]}]}`},
		{"logs", `{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"a"}}]}]}]}`, `{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"b"}}]}]}]}`},
		{"metrics", `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"requests","sum":{"dataPoints":[{"asInt":"1"}]}}]}]}]}`, `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"requests","sum":{"dataPoints":[{"asInt":"2"}]}}]}]}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.signal, func(t *testing.T) {
			d := computeStageDiff(json.RawMessage(tt.before), json.RawMessage(tt.after), 1, tt.signal)
			if len(d.Modified) != 1 || len(d.Modified[0].Changes) == 0 {
				t.Fatalf("missing changes: %+v", d)
			}
			if tt.signal == "traces" && (d.InputCount != 2 || len(d.Removed) != 1) {
				t.Fatalf("lost trace identity: %+v", d)
			}
		})
	}
}

func TestDiffPreservesDuplicates(t *testing.T) {
	before := json.RawMessage(`{"spans":[{"traceId":"t","spanId":"s"},{"traceId":"t","spanId":"s"}]}`)
	after := json.RawMessage(`{"spans":[{"traceId":"t","spanId":"s"}]}`)
	d := computeStageDiff(before, after, 0, "traces")
	if d.InputCount != 2 || len(d.Removed) != 1 {
		t.Fatalf("lost duplicate: %+v", d)
	}
}
