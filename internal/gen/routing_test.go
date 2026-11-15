package gen_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/region"
)

func resourceValue(attrs []*commonv1.KeyValue, key string) string {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value.GetStringValue()
		}
	}
	return ""
}

func TestCorrelationRetainsSpanResource(t *testing.T) {
	for _, tt := range []struct {
		name     string
		workload gen.WorkloadProfile
	}{
		{name: "topology"},
		{name: "flat workload", workload: gen.WorkloadProfile{Shape: "flat", ItemsPerRequest: 4}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := gen.Config{Regions: testRegions, Signals: testSignals, TraceCount: 12, Scenario: "microservice", SpanCount: 6, MaxDepth: 3, Workload: tt.workload}
			traces := &mockExporter{}
			var refs []gen.SpanRef
			g := gen.NewGenerator(cfg)
			g.OnTraceSpans = func(batch []gen.SpanRef) { refs = append(refs, batch...) }
			if err := g.Run(context.Background(), traces); err != nil {
				t.Fatal(err)
			}
			type resource struct{ region, service string }
			spans := make(map[string]resource)
			seenRegions := make(map[string]bool)
			for _, rs := range traces.spans {
				r := resource{resourceValue(rs.Resource.Attributes, "cloud.region"), resourceValue(rs.Resource.Attributes, "service.name")}
				seenRegions[r.region] = true
				for _, scope := range rs.ScopeSpans {
					for _, sp := range scope.Spans {
						spans[fmt.Sprintf("%x/%x", sp.TraceId, sp.SpanId)] = r
					}
				}
			}
			if tt.name == "topology" && len(seenRegions) != 3 {
				t.Fatalf("trace regions = %v", seenRegions)
			}
			ch := make(chan []gen.SpanRef, 1)
			ch <- refs
			close(ch)
			logs := &logFailNExporter{}
			lc := gen.DefaultLogConfig()
			lc.LogsPerSpan = 1
			lg := gen.NewLogGenerator(lc)
			lg.Regions = testRegions
			if err := lg.RunCorrelated(context.Background(), ch, logs); err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, rl := range logs.logs {
				r := resource{resourceValue(rl.Resource.Attributes, "cloud.region"), resourceValue(rl.Resource.Attributes, "service.name")}
				for _, scope := range rl.ScopeLogs {
					for _, record := range scope.LogRecords {
						key := fmt.Sprintf("%x/%x", record.TraceId, record.SpanId)
						if want, ok := spans[key]; !ok || want != r {
							t.Fatalf("log %s resource = %+v, span = %+v (present %v)", key, r, want, ok)
						}
						count++
					}
				}
			}
			if count != len(spans) {
				t.Fatalf("got %d logs for %d spans", count, len(spans))
			}
		})
	}
}

func TestCorrelatedMixedResourceBatch(t *testing.T) {
	refs := []gen.SpanRef{
		{Region: testRegions[0], Service: "api"},
		{Region: testRegions[1], Service: "api"},
		{Region: testRegions[0], Service: "worker"},
		{Region: testRegions[0], Service: "api"},
	}
	for i := range refs {
		refs[i].SpanID[0] = byte(i + 1)
	}
	ch := make(chan []gen.SpanRef, 1)
	ch <- refs
	close(ch)
	lc := gen.DefaultLogConfig()
	lc.LogsPerSpan = 1
	lg := gen.NewLogGenerator(lc)
	lg.Regions = testRegions
	out := &logFailNExporter{}
	if err := lg.RunCorrelated(context.Background(), ch, out); err != nil {
		t.Fatal(err)
	}
	if out.exports != 1 || len(out.logs) != 3 {
		t.Fatalf("exports=%d resources=%d", out.exports, len(out.logs))
	}
	for _, rl := range out.logs {
		for _, scope := range rl.ScopeLogs {
			for _, record := range scope.LogRecords {
				ref := refs[int(record.SpanId[0])-1]
				if got := resourceValue(rl.Resource.Attributes, "cloud.region"); got != string(ref.Region) {
					t.Fatalf("region %q want %q", got, ref.Region)
				}
				if got := resourceValue(rl.Resource.Attributes, "service.name"); got != ref.Service {
					t.Fatalf("service %q want %q", got, ref.Service)
				}
			}
		}
	}
}

func TestIndependentSignalsSpreadDeterministically(t *testing.T) {
	t.Run("logs", func(t *testing.T) {
		run := func(regions []region.Name) map[string][]string {
			lc := gen.DefaultLogConfig()
			lc.LogCount = 8
			lg := gen.NewLogGenerator(lc)
			lg.Regions = regions
			out := &logFailNExporter{}
			if err := lg.Run(context.Background(), out); err != nil {
				t.Fatal(err)
			}
			records := make(map[string][]string)
			for _, rl := range out.logs {
				ep := resourceValue(rl.Resource.Attributes, "cloud.region")
				for _, scope := range rl.ScopeLogs {
					for _, record := range scope.LogRecords {
						records[ep] = append(records[ep], record.Body.GetStringValue())
					}
				}
			}
			return records
		}
		all, routed := run(testRegions[:1])[string(testRegions[0])], run(testRegions)
		expected := make(map[string][]string)
		for i, record := range all {
			ep := string(testRegions[i%len(testRegions)])
			expected[ep] = append(expected[ep], record)
		}
		if !reflect.DeepEqual(routed, expected) {
			t.Fatalf("record routing = %v, want %v", routed, expected)
		}
	})
	t.Run("metrics", func(t *testing.T) {
		run := func(regions []region.Name) map[string][]string {
			mc := gen.DefaultMetricConfig()
			mc.Enabled = true
			mc.MetricCount = 8
			mg := gen.NewMetricGenerator(mc)
			mg.Regions = regions
			out := &metricFailNExporter{}
			if err := mg.Run(context.Background(), out); err != nil {
				t.Fatal(err)
			}
			series := make(map[string][]string)
			for _, rm := range out.metrics {
				ep := resourceValue(rm.Resource.Attributes, "cloud.region")
				for _, scope := range rm.ScopeMetrics {
					for _, m := range scope.Metrics {
						series[ep] = append(series[ep], m.Name)
					}
				}
			}
			return series
		}
		all, routed := run(testRegions[:1])[string(testRegions[0])], run(testRegions)
		expected := make(map[string][]string)
		for i, name := range all {
			ep := string(testRegions[i%len(testRegions)])
			expected[ep] = append(expected[ep], name)
		}
		if !reflect.DeepEqual(routed, expected) {
			t.Fatalf("series routing = %v, want %v", routed, expected)
		}
	})
}

func TestValidateConfigRequiresRegionsAndSignals(t *testing.T) {
	for _, tt := range []struct {
		name    string
		regions []region.Name
		signals gen.Signals
		want    string
	}{
		{"empty regions", nil, gen.Signals{Traces: true}, "no regions configured"},
		{"empty signals", testRegions, gen.Signals{}, "no signals selected"},
		{"valid", testRegions, gen.Signals{Traces: true}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := gen.ValidateConfig(gen.Config{Regions: tt.regions, Signals: tt.signals})
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %v, want %q", err, tt.want)
			}
		})
	}
}

func TestSignalGeneratorsRequireRegions(t *testing.T) {
	lc := gen.DefaultLogConfig()
	lc.LogCount = 1
	lg := gen.NewLogGenerator(lc)
	refs := make(chan []gen.SpanRef)
	close(refs)
	mc := gen.DefaultMetricConfig()
	mc.Enabled = true
	tests := []struct {
		name string
		run  func() error
	}{
		{"traces", func() error { return gen.NewGenerator(gen.Config{Signals: testSignals}).Run(context.Background(), nil) }},
		{"logs", func() error { return lg.Run(context.Background(), nil) }},
		{"correlated logs", func() error { return lg.RunCorrelated(context.Background(), refs, nil) }},
		{"metrics", func() error { return gen.NewMetricGenerator(mc).Run(context.Background(), nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil || !strings.Contains(err.Error(), "no regions configured") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
