package export

import (
	"encoding/base64"
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
		UseProtoNames: true,
		Indent:        "  ",
	}
	data, err := opts.Marshal(req)
	if err != nil {
		return nil, err
	}
	return fixTraceSpanIDs(data)
}

// fixTraceSpanIDs converts base64-encoded trace_id, span_id, and parent_span_id
// values to hex-encoded format per the OTLP JSON specification.
func fixTraceSpanIDs(data []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	fixNode(v)
	return json.MarshalIndent(v, "", "  ")
}

func fixNode(v any) {
	switch node := v.(type) {
	case map[string]any:
		for key, val := range node {
			if str, ok := val.(string); ok && isTraceIdentityField(key) {
				if decoded, err := base64.StdEncoding.DecodeString(str); err == nil && len(decoded) > 0 {
					node[key] = fmt.Sprintf("%x", decoded)
				}
			} else {
				fixNode(val)
			}
		}
	case []any:
		for _, item := range node {
			fixNode(item)
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
