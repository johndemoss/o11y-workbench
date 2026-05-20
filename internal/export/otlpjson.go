package export

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// marshalOTLPJSON marshals req to OTLP-compliant JSON with hex-encoded
// trace_id, span_id, and parent_span_id bytes fields.
//
// protojson always encodes bytes as base64, but the OTLP JSON specification
// requires hex encoding for these trace identity fields.
func marshalOTLPJSON(req *collectortracev1.ExportTraceServiceRequest) ([]byte, error) {
	opts := protojson.MarshalOptions{
		UseProtoNames: false,
		Indent:        "  ",
	}
	data, err := opts.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	return FixTraceSpanIDsForMarshal(data)
}

// FixTraceSpanIDsForMarshal converts base64-encoded trace_id, span_id, and
// parent_span_id values to hex-encoded format per the OTLP JSON specification.
// This is the direction needed when marshaling protobuf → JSON for OTLP output.
func FixTraceSpanIDsForMarshal(data []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("unmarshal trace JSON: %w", err)
	}
	fixNodeForMarshal(v)
	return json.MarshalIndent(v, "", "  ")
}

// FixTraceSpanIDsForUnmarshal converts hex-encoded trace_id, span_id, and
// parent_span_id values to base64-encoded format. This is the direction needed
// when unmarshaling OTLP JSON → protobuf (protojson expects base64 for bytes).
func FixTraceSpanIDsForUnmarshal(data []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("unmarshal trace JSON: %w", err)
	}
	fixNodeForUnmarshal(v)
	return json.Marshal(v)
}

// fixNodeForMarshal recursively walks the JSON tree and converts base64-encoded
// trace identity fields to hex encoding.
func fixNodeForMarshal(v any) {
	switch node := v.(type) {
	case map[string]any:
		for key, val := range node {
			if str, ok := val.(string); ok && isTraceIdentityField(key) {
				if decoded, err := base64.StdEncoding.DecodeString(str); err == nil && len(decoded) > 0 {
					node[key] = fmt.Sprintf("%x", decoded)
				}
			} else {
				fixNodeForMarshal(val)
			}
		}
	case []any:
		for _, item := range node {
			fixNodeForMarshal(item)
		}
	}
}

// fixNodeForUnmarshal recursively walks the JSON tree and converts hex-encoded
// trace identity fields to base64 encoding.
func fixNodeForUnmarshal(v any) {
	switch node := v.(type) {
	case map[string]any:
		for key, val := range node {
			if str, ok := val.(string); ok && isTraceIdentityField(key) && len(str) > 0 {
				if decoded, err := hex.DecodeString(str); err == nil && len(decoded) > 0 {
					node[key] = base64.StdEncoding.EncodeToString(decoded)
				}
			} else {
				fixNodeForUnmarshal(val)
			}
		}
	case []any:
		for _, item := range node {
			fixNodeForUnmarshal(item)
		}
	}
}

// isTraceIdentityField returns true for protobuf bytes field names that
// represent trace/span identity — these must be hex-encoded in OTLP JSON.
func isTraceIdentityField(key string) bool {
	switch key {
	case "trace_id", "traceId",
		"span_id", "spanId",
		"parent_span_id", "parentSpanId":
		return true
	}
	return false
}
