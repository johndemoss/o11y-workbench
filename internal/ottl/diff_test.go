package ottl

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestSignalDiffRecords(t *testing.T) {
	tests := []struct{ signal, before, after string }{
		{"traces", `{"resourceSpans":[{"resource":{"attributes":[{"key":"env","value":{"stringValue":"a"}}]},"scopeSpans":[{"spans":[{"traceId":"t1","spanId":"s","attributes":[{"key":"x","value":{"stringValue":"a"}}]},{"traceId":"t2","spanId":"s"}]}]}]}`, `{"resourceSpans":[{"resource":{"attributes":[{"key":"env","value":{"stringValue":"b"}}]},"scopeSpans":[{"spans":[{"traceId":"t1","spanId":"s","attributes":[{"key":"x","value":{"stringValue":"b"}}]}]}]}]}`},
		{"logs", `{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"a"}}]}]}]}`, `{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"b"}}]}]}]}`},
		{"metrics", `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"requests","sum":{"dataPoints":[{"asInt":"1"}]}}]}]}]}`, `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"requests","sum":{"dataPoints":[{"asInt":"2"}]}}]}]}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.signal, func(t *testing.T) {
			d := computeStageDiff(json.RawMessage(tt.before), json.RawMessage(tt.after), 1, tt.signal)
			if len(d.Modified) != 1 || len(d.Modified[0].Changes) == 0 {
				t.Fatalf("missing changes: %+v", d)
			}
			if tt.signal == "traces" && (d.InputCount != 2 || len(d.Removed) != 1) {
				t.Fatalf("lost trace identity: %+v", d)
			}
		})
	}
}

func TestDiffPreservesDuplicates(t *testing.T) {
	before := json.RawMessage(`{"spans":[{"traceId":"t","spanId":"s"},{"traceId":"t","spanId":"s"}]}`)
	after := json.RawMessage(`{"spans":[{"traceId":"t","spanId":"s"}]}`)
	d := computeStageDiff(before, after, 0, "traces")
	if d.InputCount != 2 || len(d.Removed) != 1 {
		t.Fatalf("lost duplicate: %+v", d)
	}
}

type logDiffCase struct {
	name         string
	before       json.RawMessage
	after        json.RawMessage
	wantModified int
	wantAdded    int
	wantRemoved  int
	wantFields   []string
	wantID       string
}

type metricDiffCase struct {
	name         string
	before       json.RawMessage
	after        json.RawMessage
	wantModified int
	wantAdded    int
	wantRemoved  int
	wantFields   []string
	wantID       string
}

func logRecordsJSON(records ...string) json.RawMessage {
	return json.RawMessage(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[` + strings.Join(records, ",") + `]}]}]}`)
}

func metricSeriesJSON(series ...string) json.RawMessage {
	return json.RawMessage(`{"resourceMetrics":[{"scopeMetrics":[{"metrics":[` + strings.Join(series, ",") + `]}]}]}`)
}

func changeFieldSet(changes []RecordChange) []string {
	fields := make([]string, len(changes))
	for i, c := range changes {
		fields[i] = c.Field
	}
	slices.Sort(fields)
	return fields
}

func TestLogRecordDiffIdentity(t *testing.T) {
	tests := []logDiffCase{
		{name: "reordered correlated records", before: logRecordsJSON(`{"traceId":"t1","spanId":"s1","body":{"stringValue":"one"}}`, `{"traceId":"t2","spanId":"s2","body":{"stringValue":"two"}}`), after: logRecordsJSON(`{"traceId":"t2","spanId":"s2","body":{"stringValue":"two"}}`, `{"traceId":"t1","spanId":"s1","body":{"stringValue":"one"}}`)},
		{name: "reordered uncorrelated records", before: logRecordsJSON(`{"body":{"stringValue":"one"},"timeUnixNano":"1000"}`, `{"body":{"stringValue":"two"},"timeUnixNano":"2000"}`), after: logRecordsJSON(`{"body":{"stringValue":"two"},"timeUnixNano":"2000"}`, `{"body":{"stringValue":"one"},"timeUnixNano":"1000"}`)},
		{name: "reordered records with identical body", before: logRecordsJSON(`{"body":{"stringValue":"one"},"timeUnixNano":"1000","attributes":[{"key":"idx","value":{"stringValue":"1"}}]}`, `{"body":{"stringValue":"one"},"timeUnixNano":"1000","attributes":[{"key":"idx","value":{"stringValue":"2"}}]}`), after: logRecordsJSON(`{"body":{"stringValue":"one"},"timeUnixNano":"1000","attributes":[{"key":"idx","value":{"stringValue":"2"}}]}`, `{"body":{"stringValue":"one"},"timeUnixNano":"1000","attributes":[{"key":"idx","value":{"stringValue":"1"}}]}`)},
		{name: "attribute added to one record", before: logRecordsJSON(`{"traceId":"t2","spanId":"s2","body":{"stringValue":"two"}}`), after: logRecordsJSON(`{"traceId":"t2","spanId":"s2","body":{"stringValue":"two"},"attributes":[{"key":"http.method","value":{"stringValue":"GET"}}]}`), wantModified: 1, wantFields: []string{"attributes.http.method"}, wantID: "t2/s2"},
		{name: "attribute added to uncorrelated record", before: logRecordsJSON(`{"body":{"stringValue":"one"},"timeUnixNano":"1000"}`), after: logRecordsJSON(`{"body":{"stringValue":"one"},"timeUnixNano":"1000","attributes":[{"key":"x","value":{"stringValue":"y"}}]}`), wantModified: 1, wantFields: []string{"attributes.x"}},
		{name: "body changed on uncorrelated record", before: logRecordsJSON(`{"body":{"stringValue":"one"},"timeUnixNano":"1000"}`), after: logRecordsJSON(`{"body":{"stringValue":"two"},"timeUnixNano":"1000"}`), wantModified: 1, wantFields: []string{"body"}},
		{name: "record added between existing records", before: logRecordsJSON(`{"traceId":"t1","spanId":"s1","body":{"stringValue":"one"}}`, `{"traceId":"t3","spanId":"s3","body":{"stringValue":"three"}}`), after: logRecordsJSON(`{"traceId":"t1","spanId":"s1","body":{"stringValue":"one"}}`, `{"traceId":"t2","spanId":"s2","body":{"stringValue":"two"}}`, `{"traceId":"t3","spanId":"s3","body":{"stringValue":"three"}}`), wantAdded: 1, wantID: "t2/s2"},
		{name: "identical duplicate removed", before: logRecordsJSON(`{"traceId":"t1","spanId":"s1","body":{"stringValue":"one"}}`, `{"traceId":"t1","spanId":"s1","body":{"stringValue":"one"}}`), after: logRecordsJSON(`{"traceId":"t1","spanId":"s1","body":{"stringValue":"one"}}`), wantRemoved: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := computeStageDiff(tt.before, tt.after, 0, "logs")
			if len(d.Modified) != tt.wantModified || len(d.Added) != tt.wantAdded || len(d.Removed) != tt.wantRemoved {
				t.Fatalf("modified=%d added=%d removed=%d, want %d/%d/%d: %+v", len(d.Modified), len(d.Added), len(d.Removed), tt.wantModified, tt.wantAdded, tt.wantRemoved, d)
			}
			switch {
			case tt.wantModified == 1 && tt.wantID != "":
				if d.Modified[0].ID != tt.wantID {
					t.Fatalf("modified id = %q, want %q", d.Modified[0].ID, tt.wantID)
				}
			case tt.wantAdded == 1 && tt.wantID != "":
				if d.Added[0].ID != tt.wantID {
					t.Fatalf("added id = %q, want %q", d.Added[0].ID, tt.wantID)
				}
			case tt.wantRemoved == 1 && tt.wantID != "":
				if d.Removed[0].ID != tt.wantID {
					t.Fatalf("removed id = %q, want %q", d.Removed[0].ID, tt.wantID)
				}
			}
			if tt.wantModified == 1 {
				if got := changeFieldSet(d.Modified[0].Changes); !slices.Equal(got, tt.wantFields) {
					t.Fatalf("changed fields = %v, want %v", got, tt.wantFields)
				}
			}
		})
	}
}

func TestMetricDataPointDiffGranularity(t *testing.T) {
	tests := []metricDiffCase{
		{name: "gauge datapoint attribute changed", before: metricSeriesJSON(`{"name":"http.requests","gauge":{"dataPoints":[{"attributes":[{"key":"route","value":{"stringValue":"/a"}}],"asInt":"5"},{"attributes":[{"key":"status","value":{"stringValue":"ok"}}],"asInt":"9"}]}}`), after: metricSeriesJSON(`{"name":"http.requests","gauge":{"dataPoints":[{"attributes":[{"key":"route","value":{"stringValue":"/a"}}],"asInt":"5"},{"attributes":[{"key":"status","value":{"stringValue":"error"}}],"asInt":"9"}]}}`), wantModified: 1, wantFields: []string{"data_points[1].attributes.status"}, wantID: "http.requests"},
		{name: "histogram datapoint attribute added", before: metricSeriesJSON(`{"name":"latency","histogram":{"count":"2","sum":3.5,"dataPoints":[{"count":"2","sum":3.5}]}}`), after: metricSeriesJSON(`{"name":"latency","histogram":{"count":"2","sum":3.5,"dataPoints":[{"count":"2","sum":3.5,"attributes":[{"key":"zone","value":{"stringValue":"a"}}]}]}}`), wantModified: 1, wantFields: []string{"data_points[0].attributes.zone"}, wantID: "latency"},
		{name: "gauge datapoint value changed", before: metricSeriesJSON(`{"name":"queue.depth","gauge":{"dataPoints":[{"asInt":"1"}]}}`), after: metricSeriesJSON(`{"name":"queue.depth","gauge":{"dataPoints":[{"asInt":"2"}]}}`), wantModified: 1, wantFields: []string{"data_points[0].asInt"}},
		{name: "snake case gauge datapoint attribute changed", before: json.RawMessage(`{"resource_metrics":[{"scope_metrics":[{"metrics":[{"name":"temperature","gauge":{"data_points":[{"attributes":[{"key":"room","value":{"stringValue":"a"}}],"asDouble":1.5}]}}]}]}]}`), after: json.RawMessage(`{"resource_metrics":[{"scope_metrics":[{"metrics":[{"name":"temperature","gauge":{"data_points":[{"attributes":[{"key":"room","value":{"stringValue":"b"}}],"asDouble":1.5}]}}]}]}]}`), wantModified: 1, wantFields: []string{"data_points[0].attributes.room"}},
		{name: "new series added", before: metricSeriesJSON(`{"name":"existing","gauge":{"dataPoints":[{"asInt":"1"}]}}`), after: metricSeriesJSON(`{"name":"existing","gauge":{"dataPoints":[{"asInt":"1"}]}}`, `{"name":"fresh","gauge":{"dataPoints":[{"asInt":"7"}]}}`), wantAdded: 1, wantID: "fresh"},
		{name: "removed series removed", before: metricSeriesJSON(`{"name":"existing","gauge":{"dataPoints":[{"asInt":"1"}]}}`, `{"name":"stale","gauge":{"dataPoints":[{"asInt":"7"}]}}`), after: metricSeriesJSON(`{"name":"existing","gauge":{"dataPoints":[{"asInt":"1"}]}}`), wantRemoved: 1, wantID: "stale"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := computeStageDiff(tt.before, tt.after, 0, "metrics")
			if len(d.Modified) != tt.wantModified || len(d.Added) != tt.wantAdded || len(d.Removed) != tt.wantRemoved {
				t.Fatalf("modified=%d added=%d removed=%d, want %d/%d/%d: %+v", len(d.Modified), len(d.Added), len(d.Removed), tt.wantModified, tt.wantAdded, tt.wantRemoved, d)
			}
			switch {
			case tt.wantModified == 1 && tt.wantID != "":
				if d.Modified[0].ID != tt.wantID {
					t.Fatalf("modified id = %q, want %q", d.Modified[0].ID, tt.wantID)
				}
			case tt.wantAdded == 1 && tt.wantID != "":
				if d.Added[0].ID != tt.wantID {
					t.Fatalf("added id = %q, want %q", d.Added[0].ID, tt.wantID)
				}
			case tt.wantRemoved == 1 && tt.wantID != "":
				if d.Removed[0].ID != tt.wantID {
					t.Fatalf("removed id = %q, want %q", d.Removed[0].ID, tt.wantID)
				}
			}
			if tt.wantModified != 1 {
				return
			}
			got := changeFieldSet(d.Modified[0].Changes)
			if !slices.Equal(got, tt.wantFields) {
				t.Fatalf("changed fields = %v, want %v", got, tt.wantFields)
			}
			for _, f := range got {
				if !strings.HasPrefix(f, "data_points[") {
					t.Fatalf("coarse change %q reported instead of a data point change", f)
				}
			}
		})
	}
}

func TestTraceDiffIdentityUnchanged(t *testing.T) {
	before := json.RawMessage(`{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"t1","spanId":"s1","attributes":[{"key":"x","value":{"stringValue":"a"}}]},{"traceId":"t2","spanId":"s2"}]}]}]}`)
	after := json.RawMessage(`{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"t1","spanId":"s1","attributes":[{"key":"x","value":{"stringValue":"b"}}]}]}]}]}`)
	d := computeStageDiff(before, after, 0, "traces")
	if len(d.Modified) != 1 || d.Modified[0].ID != "t1/s1" {
		t.Fatalf("trace identity lost: %+v", d.Modified)
	}
	if got := changeFieldSet(d.Modified[0].Changes); !slices.Equal(got, []string{"attributes.x"}) {
		t.Fatalf("trace changed fields = %v, want [attributes.x]", got)
	}
	if len(d.Removed) != 1 || d.Removed[0].ID != "t2/s2" {
		t.Fatalf("trace removal lost: %+v", d.Removed)
	}
}
