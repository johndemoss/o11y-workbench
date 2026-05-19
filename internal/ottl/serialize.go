package ottl

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// UnmarshalTracesFromJSON converts OTLP JSON bytes to ptrace.Traces.
// Uses the protojson -> proto -> pdata sequence.
func UnmarshalTracesFromJSON(data json.RawMessage) (ptrace.Traces, error) {
	// Convert hex trace IDs to base64 before protojson unmarshal
	fixedData, err := fixTraceSpanIDsForUnmarshal([]byte(data))
	if err != nil {
		return ptrace.Traces{}, fmt.Errorf("fix trace IDs: %w", err)
	}

	var srcReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(fixedData, &srcReq); err != nil {
		return ptrace.Traces{}, fmt.Errorf("unmarshal trace data: %w", err)
	}

	protoBytes, err := proto.Marshal(&srcReq)
	if err != nil {
		return ptrace.Traces{}, fmt.Errorf("marshal protobuf: %w", err)
	}

	var unmarshaler ptrace.ProtoUnmarshaler
	traces, err := unmarshaler.UnmarshalTraces(protoBytes)
	if err != nil {
		return ptrace.Traces{}, fmt.Errorf("unmarshal to pdata: %w", err)
	}

	return traces, nil
}

// MarshalTracesToJSON converts ptrace.Traces to OTLP JSON bytes.
// Uses the pdata -> proto -> protojson sequence.
func MarshalTracesToJSON(traces ptrace.Traces) (json.RawMessage, error) {
	var marshaler ptrace.ProtoMarshaler
	outBytes, err := marshaler.MarshalTraces(traces)
	if err != nil {
		return nil, fmt.Errorf("marshal pdata: %w", err)
	}

	var dstReq collectortracev1.ExportTraceServiceRequest
	if err := proto.Unmarshal(outBytes, &dstReq); err != nil {
		return nil, fmt.Errorf("unmarshal pdata to protobuf: %w", err)
	}

	outJSON, err := protojson.MarshalOptions{
		UseProtoNames: false,
		Indent:        "  ",
	}.Marshal(&dstReq)
	if err != nil {
		return nil, fmt.Errorf("marshal result JSON: %w", err)
	}

	return json.RawMessage(outJSON), nil
}

func fixTraceSpanIDsForUnmarshal(data []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	fixNodeForTrace(v)
	return json.Marshal(v)
}

func fixNodeForTrace(v any) {
	switch node := v.(type) {
	case map[string]any:
		for key, val := range node {
			if str, ok := val.(string); ok && isTraceIdentityField(key) && len(str) > 0 {
				if decoded, err := hex.DecodeString(str); err == nil && len(decoded) > 0 {
					node[key] = base64.StdEncoding.EncodeToString(decoded)
				}
			} else {
				fixNodeForTrace(val)
			}
		}
	case []any:
		for _, item := range node {
			fixNodeForTrace(item)
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
