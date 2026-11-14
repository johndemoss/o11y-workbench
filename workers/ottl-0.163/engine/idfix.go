package engine

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// FixTraceSpanIDsForMarshal converts base64-encoded trace_id, span_id, and
// parent_span_id values to hex-encoded format per the OTLP JSON specification.
func FixTraceSpanIDsForMarshal(data []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("unmarshal trace JSON: %w", err)
	}
	fixNodeForMarshal(v)
	return json.Marshal(v)
}

// FixTraceSpanIDsForUnmarshal converts hex-encoded trace_id, span_id, and
// parent_span_id values to base64-encoded format for protojson.
func FixTraceSpanIDsForUnmarshal(data []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("unmarshal trace JSON: %w", err)
	}
	fixNodeForUnmarshal(v)
	return json.Marshal(v)
}

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

func isTraceIdentityField(key string) bool {
	switch key {
	case "trace_id", "traceId",
		"span_id", "spanId",
		"parent_span_id", "parentSpanId":
		return true
	}
	return false
}
