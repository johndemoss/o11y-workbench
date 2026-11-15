package ottl

import (
	"context"
	"encoding/json"
	"testing"

	"go.opentelemetry.io/collector/pdata/plog"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
)

func makeSampleLogsTwoRecords() *collectorlogsv1.ExportLogsServiceRequest {
	return &collectorlogsv1.ExportLogsServiceRequest{
		ResourceLogs: []*logsv1.ResourceLogs{
			{
				Resource: &resourcev1.Resource{
					Attributes: []*commonv1.KeyValue{
						{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "log-svc"}}},
					},
				},
				ScopeLogs: []*logsv1.ScopeLogs{
					{
						LogRecords: []*logsv1.LogRecord{
							{
								Attributes: []*commonv1.KeyValue{
									{Key: "level", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "info"}}},
								},
							},
							{
								Attributes: []*commonv1.KeyValue{
									{Key: "level", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "debug"}}},
								},
							},
						},
					},
				},
			},
		},
	}
}

func makeSampleLogsTwoScopes() *collectorlogsv1.ExportLogsServiceRequest {
	return &collectorlogsv1.ExportLogsServiceRequest{
		ResourceLogs: []*logsv1.ResourceLogs{
			{
				Resource: &resourcev1.Resource{},
				ScopeLogs: []*logsv1.ScopeLogs{
					{
						Scope: &commonv1.InstrumentationScope{Name: "scope-a"},
						LogRecords: []*logsv1.LogRecord{
							{Body: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "a"}}},
						},
					},
					{
						Scope: &commonv1.InstrumentationScope{Name: "scope-b"},
						LogRecords: []*logsv1.LogRecord{
							{Body: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "b"}}},
						},
					},
				},
			},
		},
	}
}

func logRecordHasAttr(rec *logsv1.LogRecord, key, want string) bool {
	for _, attr := range rec.Attributes {
		if attr.Key != key {
			continue
		}
		v, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue)
		return ok && v.StringValue == want
	}
	return false
}

func makeSampleLogRecord() *collectorlogsv1.ExportLogsServiceRequest {
	return &collectorlogsv1.ExportLogsServiceRequest{
		ResourceLogs: []*logsv1.ResourceLogs{
			{
				Resource: &resourcev1.Resource{
					Attributes: []*commonv1.KeyValue{
						{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "log-svc"}}},
					},
				},
				ScopeLogs: []*logsv1.ScopeLogs{
					{
						LogRecords: []*logsv1.LogRecord{
							{
								Body: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "hello"}},
								Attributes: []*commonv1.KeyValue{
									{Key: "original.key", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "original"}}},
								},
							},
						},
					},
				},
			},
		},
	}
}

func marshalLogsForTest(req *collectorlogsv1.ExportLogsServiceRequest) (json.RawMessage, error) {
	data, err := protojson.MarshalOptions{UseProtoNames: false, Indent: ""}.Marshal(req)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

func logsToPdata(req *collectorlogsv1.ExportLogsServiceRequest) (plog.Logs, error) {
	protoBytes, err := proto.Marshal(req)
	if err != nil {
		return plog.NewLogs(), err
	}
	var unmarshaler plog.ProtoUnmarshaler
	return unmarshaler.UnmarshalLogs(protoBytes)
}

func pdataToLogs(logs plog.Logs) (*collectorlogsv1.ExportLogsServiceRequest, error) {
	var marshaler plog.ProtoMarshaler
	outBytes, err := marshaler.MarshalLogs(logs)
	if err != nil {
		return nil, err
	}
	var dstReq collectorlogsv1.ExportLogsServiceRequest
	if err := proto.Unmarshal(outBytes, &dstReq); err != nil {
		return nil, err
	}
	return &dstReq, nil
}

func TestTransformLogRunData_SetAttribute(t *testing.T) {
	ctx := context.Background()
	logData, err := marshalLogsForTest(makeSampleLogRecord())
	if err != nil {
		t.Fatalf("marshal logs: %v", err)
	}

	out, err := TransformLogRunData(ctx, logData, []string{`set(attributes["new.key"], "new-value")`})
	if err != nil {
		t.Fatalf("TransformLogRunData: %v", err)
	}

	var resultReq collectorlogsv1.ExportLogsServiceRequest
	if err := protojson.Unmarshal(out, &resultReq); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	rec := resultReq.ResourceLogs[0].ScopeLogs[0].LogRecords[0]
	foundNew, foundOriginal := false, false
	for _, attr := range rec.Attributes {
		if attr.Key == "new.key" {
			foundNew = true
			if v, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue); !ok || v.StringValue != "new-value" {
				t.Errorf("new.key = %v, want 'new-value'", attr.Value)
			}
		}
		if attr.Key == "original.key" {
			foundOriginal = true
		}
	}
	if !foundNew {
		t.Error("new.key not found")
	}
	if !foundOriginal {
		t.Error("original.key should remain")
	}
}

func TestTransformLogs_ErrorModePropagate(t *testing.T) {
	ctx := context.Background()
	logs, err := logsToPdata(makeSampleLogRecord())
	if err != nil {
		t.Fatalf("logsToPdata: %v", err)
	}

	statements := []string{
		`set(attributes["first"], "ok")`,
		`set(attributes["second"], Substring("hello", -1, 10))`,
	}
	_, _, err = TransformLogs(ctx, logs, statements, ErrorModePropagate)
	if err == nil {
		t.Fatal("expected error for ErrorModePropagate, got nil")
	}
}

func TestTransformLogs_ErrorModeIgnore(t *testing.T) {
	ctx := context.Background()
	logs, err := logsToPdata(makeSampleLogRecord())
	if err != nil {
		t.Fatalf("logsToPdata: %v", err)
	}

	statements := []string{
		`set(attributes["first"], "ok")`,
		`set(attributes["second"], Substring("hello", -1, 10))`,
	}
	result, count, err := TransformLogs(ctx, logs, statements, ErrorModeIgnore)
	if err != nil {
		t.Fatalf("TransformLogs with ErrorModeIgnore: %v", err)
	}
	if count != 1 {
		t.Errorf("log record count = %d, want 1", count)
	}

	resultReq, err := pdataToLogs(result)
	if err != nil {
		t.Fatalf("pdataToLogs: %v", err)
	}
	rec := resultReq.ResourceLogs[0].ScopeLogs[0].LogRecords[0]
	found := false
	for _, attr := range rec.Attributes {
		if attr.Key == "first" {
			found = true
			if v, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue); !ok || v.StringValue != "ok" {
				t.Errorf("first = %v, want 'ok'", attr.Value)
			}
		}
	}
	if !found {
		t.Error("'first' attribute should have been set even with error on later statement")
	}
}

func TestTransformLogs_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	logs, err := logsToPdata(makeSampleLogRecord())
	if err != nil {
		t.Fatalf("logsToPdata: %v", err)
	}

	_, _, err = TransformLogs(ctx, logs, []string{`set(attributes["k"], "v")`}, ErrorModePropagate)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestTransformLogRunDataWithGroups_ResourceContext(t *testing.T) {
	ctx := context.Background()
	logData, err := marshalLogsForTest(makeSampleLogRecord())
	if err != nil {
		t.Fatalf("marshal logs: %v", err)
	}

	groups := []ContextStatements{
		{
			Context:    "resource",
			Statements: []string{`set(attributes["env"], "prod")`},
		},
	}

	out, count, err := TransformLogRunDataWithGroups(ctx, logData, groups, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformLogRunDataWithGroups: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	var resultReq collectorlogsv1.ExportLogsServiceRequest
	if err := protojson.Unmarshal(out, &resultReq); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	resource := resultReq.ResourceLogs[0].Resource
	found := false
	for _, attr := range resource.Attributes {
		if attr.Key == "env" {
			found = true
			if v, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue); !ok || v.StringValue != "prod" {
				t.Errorf("env = %v, want 'prod'", attr.Value)
			}
		}
	}
	if !found {
		t.Error("resource attribute 'env' not found after group transform")
	}
}

func TestTransformLogsWithGroups_ConditionalOR(t *testing.T) {
	ctx := context.Background()
	logs, err := logsToPdata(makeSampleLogsTwoRecords())
	if err != nil {
		t.Fatalf("logsToPdata: %v", err)
	}

	groups := []ContextStatements{
		{
			Context: "log",
			Conditions: []string{
				`log.attributes["level"] == "info"`,
				`log.attributes["level"] == "warn"`,
			},
			Statements: []string{`set(log.attributes["tagged"], "yes")`},
		},
	}

	result, count, err := TransformLogsWithGroups(ctx, logs, groups, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformLogsWithGroups: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	resultReq, err := pdataToLogs(result)
	if err != nil {
		t.Fatalf("pdataToLogs: %v", err)
	}
	recs := resultReq.ResourceLogs[0].ScopeLogs[0].LogRecords
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2", len(recs))
	}
	if !logRecordHasAttr(recs[0], "tagged", "yes") {
		t.Error("info record should match OR condition and be tagged")
	}
	if logRecordHasAttr(recs[1], "tagged", "yes") {
		t.Error("debug record should not be tagged")
	}
}

func TestTransformLogsWithGroups_ResourceOnceForTwoRecords(t *testing.T) {
	ctx := context.Background()
	logs, err := logsToPdata(makeSampleLogsTwoRecords())
	if err != nil {
		t.Fatalf("logsToPdata: %v", err)
	}

	groups := []ContextStatements{
		{
			Context:    "resource",
			Statements: []string{`set(attributes["env"], "prod")`},
		},
	}

	result, count, err := TransformLogsWithGroups(ctx, logs, groups, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformLogsWithGroups: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 (records under resource)", count)
	}

	resultReq, err := pdataToLogs(result)
	if err != nil {
		t.Fatalf("pdataToLogs: %v", err)
	}
	resource := resultReq.ResourceLogs[0].Resource
	envOK := false
	for _, attr := range resource.Attributes {
		if attr.Key == "env" {
			envOK = true
			if v, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue); !ok || v.StringValue != "prod" {
				t.Errorf("env = %v, want prod", attr.Value)
			}
		}
	}
	if !envOK {
		t.Fatal("resource env not set after single resource-group execution")
	}
}

func TestTransformLogsWithGroups_ScopeOncePerScope(t *testing.T) {
	ctx := context.Background()
	logs, err := logsToPdata(makeSampleLogsTwoScopes())
	if err != nil {
		t.Fatalf("logsToPdata: %v", err)
	}

	groups := []ContextStatements{
		{
			Context:    "scope",
			Statements: []string{`set(name, "renamed")`},
		},
	}

	result, count, err := TransformLogsWithGroups(ctx, logs, groups, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformLogsWithGroups: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	resultReq, err := pdataToLogs(result)
	if err != nil {
		t.Fatalf("pdataToLogs: %v", err)
	}
	for i, sl := range resultReq.ResourceLogs[0].ScopeLogs {
		if sl.Scope == nil || sl.Scope.Name != "renamed" {
			name := ""
			if sl.Scope != nil {
				name = sl.Scope.Name
			}
			t.Errorf("scope[%d].name = %q, want renamed (once per ScopeLogs)", i, name)
		}
	}
}

func TestTransformLogs_MixedRecordAndStatementErrors(t *testing.T) {
	ctx := context.Background()
	logs, err := logsToPdata(makeSampleLogsTwoRecords())
	if err != nil {
		t.Fatalf("logsToPdata: %v", err)
	}

	statements := []string{
		`set(attributes["first"], "ok")`,
		`set(attributes["bad"], Substring("x", -1, 3))`,
	}
	result, count, err := TransformLogs(ctx, logs, statements, ErrorModeIgnore)
	if err != nil {
		t.Fatalf("TransformLogs ignore: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	resultReq, err := pdataToLogs(result)
	if err != nil {
		t.Fatalf("pdataToLogs: %v", err)
	}
	recs := resultReq.ResourceLogs[0].ScopeLogs[0].LogRecords
	for i := range recs {
		if !logRecordHasAttr(recs[i], "first", "ok") {
			t.Errorf("record[%d] missing first=ok after statement-level ignore", i)
		}
	}

	logs2, err := logsToPdata(makeSampleLogsTwoRecords())
	if err != nil {
		t.Fatalf("logsToPdata: %v", err)
	}
	_, count, err = TransformLogs(ctx, logs2, statements, ErrorModePropagate)
	if err == nil {
		t.Fatal("expected propagate error across records")
	}
	if count != 0 {
		t.Errorf("propagate count on error = %d, want 0", count)
	}
}

func TestTransformLogsWithGroups_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	logs, err := logsToPdata(makeSampleLogRecord())
	if err != nil {
		t.Fatalf("logsToPdata: %v", err)
	}

	groups := []ContextStatements{
		{Context: "log", Statements: []string{`set(log.attributes["k"], "v")`}},
	}

	_, _, err = TransformLogsWithGroups(ctx, logs, groups, ErrorModePropagate)
	if err == nil {
		t.Fatal("expected cancellation error from group execution")
	}
}
