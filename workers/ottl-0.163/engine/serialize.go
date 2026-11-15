package engine

import (
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// UnmarshalTracesFromJSON converts OTLP JSON bytes to ptrace.Traces.
// Uses the protojson -> proto -> pdata sequence.
func UnmarshalTracesFromJSON(data json.RawMessage) (ptrace.Traces, error) {
	// Convert hex trace IDs to base64 before protojson unmarshal
	fixedData, err := FixTraceSpanIDsForUnmarshal([]byte(data))
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

	data, err := protojson.MarshalOptions{
		UseProtoNames: false,
		Indent:        "",
	}.Marshal(&dstReq)
	if err != nil {
		return nil, fmt.Errorf("marshal result JSON: %w", err)
	}

	return FixTraceSpanIDsForMarshal(data)
}

// UnmarshalLogsFromJSON converts OTLP JSON log data (hex or base64 IDs) to plog.Logs.
func UnmarshalLogsFromJSON(data json.RawMessage) (plog.Logs, error) {
	fixedData, err := FixTraceSpanIDsForUnmarshal([]byte(data))
	if err != nil {
		return plog.Logs{}, fmt.Errorf("fix log IDs: %w", err)
	}
	var srcReq collectorlogsv1.ExportLogsServiceRequest
	if err := protojson.Unmarshal(fixedData, &srcReq); err != nil {
		return plog.Logs{}, fmt.Errorf("unmarshal log data: %w", err)
	}
	protoBytes, err := proto.Marshal(&srcReq)
	if err != nil {
		return plog.Logs{}, fmt.Errorf("marshal protobuf: %w", err)
	}
	var unmarshaler plog.ProtoUnmarshaler
	logs, err := unmarshaler.UnmarshalLogs(protoBytes)
	if err != nil {
		return plog.Logs{}, fmt.Errorf("unmarshal to pdata: %w", err)
	}
	return logs, nil
}

// MarshalLogsToJSON converts plog.Logs to OTLP JSON with hex trace/span IDs.
func MarshalLogsToJSON(logs plog.Logs) (json.RawMessage, error) {
	var marshaler plog.ProtoMarshaler
	outBytes, err := marshaler.MarshalLogs(logs)
	if err != nil {
		return nil, fmt.Errorf("marshal pdata: %w", err)
	}
	var dstReq collectorlogsv1.ExportLogsServiceRequest
	if err := proto.Unmarshal(outBytes, &dstReq); err != nil {
		return nil, fmt.Errorf("unmarshal pdata to protobuf: %w", err)
	}
	data, err := protojson.MarshalOptions{UseProtoNames: false, Indent: ""}.Marshal(&dstReq)
	if err != nil {
		return nil, fmt.Errorf("marshal result JSON: %w", err)
	}
	return FixTraceSpanIDsForMarshal(data)
}

// UnmarshalMetricsFromJSON converts OTLP JSON metric data to pmetric.Metrics.
func UnmarshalMetricsFromJSON(data json.RawMessage) (pmetric.Metrics, error) {
	fixedData, err := FixTraceSpanIDsForUnmarshal([]byte(data))
	if err != nil {
		return pmetric.Metrics{}, fmt.Errorf("fix metric IDs: %w", err)
	}
	var srcReq collectormetricsv1.ExportMetricsServiceRequest
	if err := protojson.Unmarshal(fixedData, &srcReq); err != nil {
		return pmetric.Metrics{}, fmt.Errorf("unmarshal metric data: %w", err)
	}
	protoBytes, err := proto.Marshal(&srcReq)
	if err != nil {
		return pmetric.Metrics{}, fmt.Errorf("marshal protobuf: %w", err)
	}
	var unmarshaler pmetric.ProtoUnmarshaler
	metrics, err := unmarshaler.UnmarshalMetrics(protoBytes)
	if err != nil {
		return pmetric.Metrics{}, fmt.Errorf("unmarshal to pdata: %w", err)
	}
	return metrics, nil
}

// MarshalMetricsToJSON converts pmetric.Metrics to OTLP JSON with hex IDs.
func MarshalMetricsToJSON(metrics pmetric.Metrics) (json.RawMessage, error) {
	var marshaler pmetric.ProtoMarshaler
	outBytes, err := marshaler.MarshalMetrics(metrics)
	if err != nil {
		return nil, fmt.Errorf("marshal pdata: %w", err)
	}
	var dstReq collectormetricsv1.ExportMetricsServiceRequest
	if err := proto.Unmarshal(outBytes, &dstReq); err != nil {
		return nil, fmt.Errorf("unmarshal pdata to protobuf: %w", err)
	}
	data, err := protojson.MarshalOptions{UseProtoNames: false, Indent: ""}.Marshal(&dstReq)
	if err != nil {
		return nil, fmt.Errorf("marshal result JSON: %w", err)
	}
	return FixTraceSpanIDsForMarshal(data)
}

// NormalizeTraceJSONIDs applies OTLP JSON hex encoding to trace/span IDs in protobuf JSON output.
func NormalizeTraceJSONIDs(protojsonOutput []byte) (json.RawMessage, error) {
	return FixTraceSpanIDsForMarshal(protojsonOutput)
}
