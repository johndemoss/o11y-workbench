package ottl

import (
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
	var srcReq collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(data, &srcReq); err != nil {
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
		UseProtoNames: true,
		Indent:        "  ",
	}.Marshal(&dstReq)
	if err != nil {
		return nil, fmt.Errorf("marshal result JSON: %w", err)
	}

	return json.RawMessage(outJSON), nil
}
