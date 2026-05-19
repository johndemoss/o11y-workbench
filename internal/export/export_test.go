package export_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/johndemoss/o11y-workbench/internal/export"
	"github.com/johndemoss/o11y-workbench/internal/region"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

func makeTestSpans() []*tracev1.ResourceSpans {
	return []*tracev1.ResourceSpans{
		{
			Resource: &resourcev1.Resource{
				Attributes: region.ResourceAttributes(region.RegionLocal, "test-svc"),
			},
			ScopeSpans: []*tracev1.ScopeSpans{
				{
					Scope: &commonv1.InstrumentationScope{Name: "test", Version: "1.0"},
					Spans: []*tracev1.Span{
						{Name: "test-span", TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, SpanId: []byte{1, 2, 3, 4, 5, 6, 7, 8}},
					},
				},
			},
		},
	}
}

func TestFormatParseRoundtrip(t *testing.T) {
	cases := []struct {
		str string
		fmt export.Format
	}{
		{"json", export.FormatJSON},
		{"protobuf", export.FormatProtobuf},
	}
	for _, c := range cases {
		got, err := export.ParseFormat(c.str)
		if err != nil {
			t.Fatalf("ParseFormat(%q): %v", c.str, err)
		}
		if got != c.fmt {
			t.Fatalf("ParseFormat(%q) = %v, want %v", c.str, got, c.fmt)
		}
	}
}

func TestFormatParseInvalid(t *testing.T) {
	_, err := export.ParseFormat("invalid")
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
}

func TestFileExporter_JSON(t *testing.T) {
	dir := t.TempDir()

	exp, err := export.NewFileExporter(dir, export.FormatJSON)
	if err != nil {
		t.Fatalf("NewFileExporter: %v", err)
	}

	ctx := context.Background()
	spans := makeTestSpans()
	if err := exp.Export(ctx, spans); err != nil {
		t.Fatalf("Export: %v", err)
	}

	if err := exp.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	data, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile JSON: %v", err)
	}

	var req collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(data, &req); err != nil {
		t.Fatalf("Unmarshal JSON: %v", err)
	}
	if len(req.GetResourceSpans()) != 1 {
		t.Fatalf("expected 1 ResourceSpans in JSON, got %d", len(req.GetResourceSpans()))
	}
}

func TestFileExporter_Protobuf(t *testing.T) {
	dir := t.TempDir()

	exp, err := export.NewFileExporter(dir, export.FormatProtobuf)
	if err != nil {
		t.Fatalf("NewFileExporter: %v", err)
	}

	ctx := context.Background()
	spans := makeTestSpans()
	if err := exp.Export(ctx, spans); err != nil {
		t.Fatalf("Export: %v", err)
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	data, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile protobuf: %v", err)
	}

	var req collectortracev1.ExportTraceServiceRequest
	if err := proto.Unmarshal(data, &req); err != nil {
		t.Fatalf("Unmarshal protobuf: %v", err)
	}
	if len(req.GetResourceSpans()) != 1 {
		t.Fatalf("expected 1 ResourceSpans, got %d", len(req.GetResourceSpans()))
	}
	if req.GetResourceSpans()[0].GetScopeSpans()[0].GetSpans()[0].Name != "test-span" {
		t.Fatalf("expected span name test-span, got %s", req.GetResourceSpans()[0].GetScopeSpans()[0].GetSpans()[0].Name)
	}
}

func TestNewFileExporter_CreatesDir(t *testing.T) {
	tDir := t.TempDir()
	subDir := tDir + "/a/b/c"

	_, err := export.NewFileExporter(subDir, export.FormatJSON)
	if err != nil {
		t.Fatalf("NewFileExporter: %v", err)
	}

	info, err := os.Stat(subDir)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}
}

type mockCountingExporter struct {
	got int
}

func (m *mockCountingExporter) Export(_ context.Context, spans []*tracev1.ResourceSpans) error {
	m.got += len(spans)
	return nil
}

func (m *mockCountingExporter) Shutdown(_ context.Context) error { return nil }

func TestEndpointRouter_DispatchesByEndpoint(t *testing.T) {
	localExp := &mockCountingExporter{}
	usEuExp := &mockCountingExporter{}

	router := export.NewEndpointRouter(map[region.Name]export.TraceExporter{
		region.RegionLocal: localExp,
		region.RegionUSEU:  usEuExp,
	})

	spans := []*tracev1.ResourceSpans{
		{
			Resource: &resourcev1.Resource{
				Attributes: region.ResourceAttributes(region.RegionLocal, "svc"),
			},
		},
		{
			Resource: &resourcev1.Resource{
				Attributes: region.ResourceAttributes(region.RegionUSEU, "svc"),
			},
		},
		{
			Resource: &resourcev1.Resource{
				Attributes: region.ResourceAttributes(region.RegionLocal, "svc"),
			},
		},
	}

	if err := router.Export(context.Background(), spans); err != nil {
		t.Fatalf("Export: %v", err)
	}

	if localExp.got != 2 {
		t.Errorf("local exporter got %d ResourceSpans, want 2", localExp.got)
	}
	if usEuExp.got != 1 {
		t.Errorf("us-eu exporter got %d ResourceSpans, want 1", usEuExp.got)
	}
}

func TestEndpointRouter_UnknownRegionFallsBack(t *testing.T) {
	localExp := &mockCountingExporter{}
	router := export.NewEndpointRouter(map[region.Name]export.TraceExporter{
		region.RegionLocal: localExp,
	})

	spans := []*tracev1.ResourceSpans{
		{
			Resource: &resourcev1.Resource{
				Attributes: []*commonv1.KeyValue{
					{Key: "cloud.region", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "unknown"}}},
				},
			},
		},
	}

	if err := router.Export(context.Background(), spans); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if localExp.got != 0 {
		t.Errorf("expected unknown region span to be dropped, got %d", localExp.got)
	}
}

func TestEndpointRouter_SpansWithoutResource(t *testing.T) {
	localExp := &mockCountingExporter{}
	router := export.NewEndpointRouter(map[region.Name]export.TraceExporter{
		region.RegionLocal: localExp,
	})

	spans := []*tracev1.ResourceSpans{
		{ScopeSpans: []*tracev1.ScopeSpans{{Spans: []*tracev1.Span{{Name: "orphan"}}}}},
	}

	if err := router.Export(context.Background(), spans); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if localExp.got != 1 {
		t.Errorf("expected span without resource to route to local, got %d", localExp.got)
	}
}

func makeTraceFileSpan() []*tracev1.ResourceSpans {
	return []*tracev1.ResourceSpans{
		{
			Resource: &resourcev1.Resource{
				Attributes: region.ResourceAttributes(region.RegionLocal, "test-svc"),
			},
			ScopeSpans: []*tracev1.ScopeSpans{
				{
					Scope: &commonv1.InstrumentationScope{Name: "test", Version: "1.0"},
					Spans: []*tracev1.Span{
						{
							Name:    "test-span",
							TraceId: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
							SpanId:  []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
						},
					},
				},
			},
		},
	}
}

func TestTraceFileExporter_WritesJSON(t *testing.T) {
	dir := t.TempDir()
	exp := export.NewTraceFileExporter(dir, export.FormatJSON)

	spans := makeTraceFileSpan()
	if err := exp.Export(context.Background(), spans); err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Expected path: <dir>/0102030405060708090a0b0c0d0e0f10/localhost_4317/spans.json
	expected := filepath.Join(dir, "0102030405060708090a0b0c0d0e0f10", "localhost_4317", "spans.json")
	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", expected, err)
	}

	var req collectortracev1.ExportTraceServiceRequest
	if err := protojson.Unmarshal(data, &req); err != nil {
		t.Fatalf("Unmarshal JSON: %v", err)
	}
	if len(req.GetResourceSpans()) != 1 {
		t.Fatalf("expected 1 ResourceSpans, got %d", len(req.GetResourceSpans()))
	}
}

func TestTraceFileExporter_WritesProtobuf(t *testing.T) {
	dir := t.TempDir()
	exp := export.NewTraceFileExporter(dir, export.FormatProtobuf)

	spans := makeTraceFileSpan()
	if err := exp.Export(context.Background(), spans); err != nil {
		t.Fatalf("Export: %v", err)
	}

	expected := filepath.Join(dir, "0102030405060708090a0b0c0d0e0f10", "localhost_4317", "spans.pb")
	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", expected, err)
	}

	var req collectortracev1.ExportTraceServiceRequest
	if err := proto.Unmarshal(data, &req); err != nil {
		t.Fatalf("Unmarshal protobuf: %v", err)
	}
	if len(req.GetResourceSpans()) != 1 {
		t.Fatalf("expected 1 ResourceSpans, got %d", len(req.GetResourceSpans()))
	}
}

func TestTraceFileExporter_EmptySpans(t *testing.T) {
	dir := t.TempDir()
	exp := export.NewTraceFileExporter(dir, export.FormatJSON)

	if err := exp.Export(context.Background(), nil); err != nil {
		t.Fatalf("Export with nil: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty directory, got %d entries", len(entries))
	}
}

func TestTraceFileExporter_TraceIDDirectory(t *testing.T) {
	dir := t.TempDir()
	exp := export.NewTraceFileExporter(dir, export.FormatJSON)

	spans := makeTraceFileSpan()
	if err := exp.Export(context.Background(), spans); err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Directory structure: <dir>/<hex_trace_id>/<endpoint_slug>/spans.json
	traceDir := filepath.Join(dir, "0102030405060708090a0b0c0d0e0f10")
	info, err := os.Stat(traceDir)
	if err != nil {
		t.Fatalf("Stat trace ID directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("trace ID path is not a directory")
	}

	endpointDir := filepath.Join(traceDir, "localhost_4317")
	info, err = os.Stat(endpointDir)
	if err != nil {
		t.Fatalf("Stat endpoint directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("endpoint path is not a directory")
	}
}
