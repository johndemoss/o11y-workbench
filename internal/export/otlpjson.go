package export

import (
	"bytes"
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
		Indent:        "",
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
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("unmarshal trace JSON: %w", err)
	}
	fixIdentityNodes(v, "", true)
	return json.Marshal(v)
}

// FixTraceSpanIDsForUnmarshal converts hex-encoded trace_id, span_id, and
// parent_span_id values to base64-encoded format. This is the direction needed
// when unmarshaling OTLP JSON → protobuf (protojson expects base64 for bytes).
func FixTraceSpanIDsForUnmarshal(data []byte) ([]byte, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("unmarshal trace JSON: %w", err)
	}
	fixIdentityNodes(v, "", false)
	return json.Marshal(v)
}

// Identity fields occur on spans, span links, log records and metric exemplars.
// Never descend into attributes or other AnyValue payloads.
func fixIdentityNodes(v any, parent string, marshal bool) {
	switch node := v.(type) {
	case map[string]any:
		identity := parent == "spans" || parent == "links" || parent == "logRecords" || parent == "log_records" || parent == "exemplars"
		for key, val := range node {
			if identity && isTraceIdentityField(key) {
				str, ok := val.(string)
				if !ok || str == "" {
					continue
				}
				if marshal {
					if converted, ok := convertIdentityForMarshal(key, str); ok {
						node[key] = converted
					}
				} else if converted, ok := convertIdentityForUnmarshal(key, str); ok {
					node[key] = converted
				}
				continue
			}
			switch key {
			case "attributes", "filteredAttributes", "filtered_attributes", "body":
				continue
			}
			fixIdentityNodes(val, key, marshal)
		}
	case []any:
		for _, item := range node {
			fixIdentityNodes(item, parent, marshal)
		}
	}
}

// isTraceIdentityField returns true for protobuf bytes field names that
// represent trace/span identity — these must be hex-encoded in OTLP JSON.
func isTraceIdentityField(key string) bool {
	return isTraceIDField(key) || isSpanIDField(key)
}

func isTraceIDField(key string) bool {
	switch key {
	case "trace_id", "traceId":
		return true
	}
	return false
}

func isSpanIDField(key string) bool {
	switch key {
	case "span_id", "spanId", "parent_span_id", "parentSpanId":
		return true
	}
	return false
}

// convertIdentityForMarshal converts only base64 of length 24 (trace) or 12
// (span/parent) that decodes to exactly 16 or 8 bytes. Already-hex values are untouched.
func convertIdentityForMarshal(key, str string) (string, bool) {
	var wantLen, wantBytes int
	if isTraceIDField(key) {
		wantLen, wantBytes = 24, 16
	} else if isSpanIDField(key) {
		wantLen, wantBytes = 12, 8
	} else {
		return "", false
	}
	if len(str) != wantLen {
		return "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(str)
	if err != nil || len(decoded) != wantBytes {
		return "", false
	}
	return hex.EncodeToString(decoded), true
}

// convertIdentityForUnmarshal converts only hex of length 32 (trace) or 16
// (span/parent) that decodes to exactly 16 or 8 bytes. Already-base64 values are untouched.
func convertIdentityForUnmarshal(key, str string) (string, bool) {
	var wantLen, wantBytes int
	if isTraceIDField(key) {
		wantLen, wantBytes = 32, 16
	} else if isSpanIDField(key) {
		wantLen, wantBytes = 16, 8
	} else {
		return "", false
	}
	if len(str) != wantLen {
		return "", false
	}
	decoded, err := hex.DecodeString(str)
	if err != nil || len(decoded) != wantBytes {
		return "", false
	}
	return base64.StdEncoding.EncodeToString(decoded), true
}
