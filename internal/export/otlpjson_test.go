package export

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"
)

func TestFixTraceSpanIDsForMarshal(t *testing.T) {
	base64TraceID := "AAECAwQFBgcICQoLDA0ODw=="
	hexTraceID := "000102030405060708090a0b0c0d0e0f"
	base64SpanID := "CgsMDQ4PEBE="
	hexSpanID := "0a0b0c0d0e0f1011"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "snake_case trace_id and span_id",
			input:    `{"trace_id":"` + base64TraceID + `","span_id":"` + base64SpanID + `"}`,
			expected: `{"span_id":"` + hexSpanID + `","trace_id":"` + hexTraceID + `"}`,
		},
		{
			name:     "camelCase traceId and spanId",
			input:    `{"traceId":"` + base64TraceID + `","spanId":"` + base64SpanID + `"}`,
			expected: `{"spanId":"` + hexSpanID + `","traceId":"` + hexTraceID + `"}`,
		},
		{
			name:     "parentSpanId and parent_span_id",
			input:    `{"parentSpanId":"` + base64SpanID + `","parent_span_id":"` + base64SpanID + `"}`,
			expected: `{"parentSpanId":"` + hexSpanID + `","parent_span_id":"` + hexSpanID + `"}`,
		},
		{
			name:     "non-identity fields untouched",
			input:    `{"name":"hello","duration":"1s","trace_id":"` + base64TraceID + `"}`,
			expected: `{"duration":"1s","name":"hello","trace_id":"` + hexTraceID + `"}`,
		},
		{
			name:     "invalid base64 left untouched",
			input:    `{"trace_id":"not-valid-base64!!"}`,
			expected: `{"trace_id":"not-valid-base64!!"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FixTraceSpanIDsForMarshal([]byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var got, want map[string]any
			if err := json.Unmarshal(result, &got); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.expected), &want); err != nil {
				t.Fatalf("unmarshal expected: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}

func TestFixTraceSpanIDsForMarshal_NoIndentWhitespace(t *testing.T) {
	input := `{"trace_id":"AAECAwQFBgcICQoLDA0ODw==","span_id":"CgsMDQ4PEBE="}`
	result, err := FixTraceSpanIDsForMarshal([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bytes.Contains(result, []byte("\n  ")) {
		t.Errorf("result has pretty-print indent whitespace: %s", result)
	}
	if bytes.Contains(result, []byte("\n")) {
		t.Errorf("result contains newlines: %s", result)
	}
}

func TestFixTraceSpanIDsForUnmarshal(t *testing.T) {
	base64TraceID := "AAECAwQFBgcICQoLDA0ODw=="
	hexTraceID := "000102030405060708090a0b0c0d0e0f"
	base64SpanID := "CgsMDQ4PEBE="
	hexSpanID := "0a0b0c0d0e0f1011"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "snake_case trace_id and span_id",
			input:    `{"trace_id":"` + hexTraceID + `","span_id":"` + hexSpanID + `"}`,
			expected: `{"span_id":"` + base64SpanID + `","trace_id":"` + base64TraceID + `"}`,
		},
		{
			name:     "camelCase traceId and spanId",
			input:    `{"traceId":"` + hexTraceID + `","spanId":"` + hexSpanID + `"}`,
			expected: `{"spanId":"` + base64SpanID + `","traceId":"` + base64TraceID + `"}`,
		},
		{
			name:     "parentSpanId and parent_span_id",
			input:    `{"parentSpanId":"` + hexSpanID + `","parent_span_id":"` + hexSpanID + `"}`,
			expected: `{"parentSpanId":"` + base64SpanID + `","parent_span_id":"` + base64SpanID + `"}`,
		},
		{
			name:     "non-identity fields untouched",
			input:    `{"name":"hello","duration":"1s","trace_id":"` + hexTraceID + `"}`,
			expected: `{"duration":"1s","name":"hello","trace_id":"` + base64TraceID + `"}`,
		},
		{
			name:     "invalid hex left untouched",
			input:    `{"trace_id":"not-valid-hex!!"}`,
			expected: `{"trace_id":"not-valid-hex!!"}`,
		},
		{
			name:     "empty string left untouched",
			input:    `{"trace_id":""}`,
			expected: `{"trace_id":""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FixTraceSpanIDsForUnmarshal([]byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(result) != tt.expected {
				t.Errorf("got:\n  %s\nwant:\n  %s", string(result), tt.expected)
			}
		})
	}
}

func TestFixTraceSpanIDsRoundTrip(t *testing.T) {
	base64TraceID := "AAECAwQFBgcICQoLDA0ODw=="
	base64SpanID := "CgsMDQ4PEBE="

	input := `{"trace_id":"` + base64TraceID + `","span_id":"` + base64SpanID + `","parentSpanId":"` + base64SpanID + `"}`

	marshaled, err := FixTraceSpanIDsForMarshal([]byte(input))
	if err != nil {
		t.Fatalf("FixTraceSpanIDsForMarshal error: %v", err)
	}

	unmarshaled, err := FixTraceSpanIDsForUnmarshal(marshaled)
	if err != nil {
		t.Fatalf("FixTraceSpanIDsForUnmarshal error: %v", err)
	}

	var originalMap, resultMap map[string]any
	if err := json.Unmarshal([]byte(input), &originalMap); err != nil {
		t.Fatalf("unmarshal original: %v", err)
	}
	if err := json.Unmarshal(unmarshaled, &resultMap); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	for _, field := range []string{"trace_id", "traceId", "span_id", "spanId", "parentSpanId", "parent_span_id"} {
		origVal, _ := originalMap[field].(string)
		resVal, _ := resultMap[field].(string)
		if origVal != "" && origVal != resVal {
			t.Errorf("round-trip mismatch for %q: got %q, want %q", field, resVal, origVal)
		}
	}
}

func TestTraceIDVectors(t *testing.T) {
	raw := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}
	hexEnc := hex.EncodeToString(raw)
	if hexEnc != "000102030405060708090a0b0c0d0e0f" {
		t.Fatalf("bad hex encoding: got %q", hexEnc)
	}
}
