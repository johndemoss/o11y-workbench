package engine

import (
	"context"
	"encoding/json"
	"testing"

	"go.opentelemetry.io/collector/pdata/pmetric"
	"google.golang.org/protobuf/encoding/protojson"

	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

// makeAllMetricTypesPdata builds one datapoint for each OTLP metric type (gauge, sum,
// histogram, exponential histogram, summary).
func makeAllMetricTypesPdata() pmetric.Metrics {
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "metric-svc")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")

	gauge := sm.Metrics().AppendEmpty()
	gauge.SetName("demo.gauge")
	gDp := gauge.SetEmptyGauge().DataPoints().AppendEmpty()
	gDp.Attributes().PutStr("kind", "gauge")

	sumMetric := sm.Metrics().AppendEmpty()
	sumMetric.SetName("demo.sum")
	sDp := sumMetric.SetEmptySum().DataPoints().AppendEmpty()
	sDp.Attributes().PutStr("kind", "sum")

	hist := sm.Metrics().AppendEmpty()
	hist.SetName("demo.histogram")
	hDp := hist.SetEmptyHistogram().DataPoints().AppendEmpty()
	hDp.Attributes().PutStr("kind", "histogram")

	ehMetric := sm.Metrics().AppendEmpty()
	ehMetric.SetName("demo.exp_histogram")
	ehDp := ehMetric.SetEmptyExponentialHistogram().DataPoints().AppendEmpty()
	ehDp.Attributes().PutStr("kind", "exponential_histogram")

	summaryMetric := sm.Metrics().AppendEmpty()
	summaryMetric.SetName("demo.summary")
	sumDp := summaryMetric.SetEmptySummary().DataPoints().AppendEmpty()
	sumDp.Attributes().PutStr("kind", "summary")

	return metrics
}

// makeTwoScopesOneResource builds two scopes under one resource for once-only group tests.
func makeTwoScopesOneResource() pmetric.Metrics {
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "dual-scope")

	smA := rm.ScopeMetrics().AppendEmpty()
	smA.Scope().SetName("scope-a")
	mA := smA.Metrics().AppendEmpty()
	mA.SetName("a.counter")
	mA.SetEmptySum().DataPoints().AppendEmpty().Attributes().PutStr("scope", "a")

	smB := rm.ScopeMetrics().AppendEmpty()
	smB.Scope().SetName("scope-b")
	mB := smB.Metrics().AppendEmpty()
	mB.SetName("b.counter")
	mB.SetEmptySum().DataPoints().AppendEmpty().Attributes().PutStr("scope", "b")

	return metrics
}

// makeConditionalDatapointMetrics has two sum datapoints with different env tags.
func makeConditionalDatapointMetrics() pmetric.Metrics {
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	m := sm.Metrics().AppendEmpty()
	m.SetName("split.counter")
	sum := m.SetEmptySum()
	dpDev := sum.DataPoints().AppendEmpty()
	dpDev.Attributes().PutStr("env", "dev")
	dpProd := sum.DataPoints().AppendEmpty()
	dpProd.Attributes().PutStr("env", "prod")
	return metrics
}

func datapointHasBoolAttr(metrics pmetric.Metrics, key string, want bool) map[string]bool {
	found := map[string]bool{}
	_ = forEachMetricDataPoint(context.Background(), metrics, func(_, _, _, _ int, _ pmetric.ResourceMetrics, _ pmetric.ScopeMetrics, _ pmetric.Metric, dataPoint any) error {
		switch dp := dataPoint.(type) {
		case pmetric.NumberDataPoint:
			if k, ok := dp.Attributes().Get("kind"); ok {
				if v, ok := dp.Attributes().Get(key); ok && v.Bool() == want {
					found[k.Str()] = true
				}
			}
		case pmetric.HistogramDataPoint:
			if k, ok := dp.Attributes().Get("kind"); ok {
				if v, ok := dp.Attributes().Get(key); ok && v.Bool() == want {
					found[k.Str()] = true
				}
			}
		case pmetric.ExponentialHistogramDataPoint:
			if k, ok := dp.Attributes().Get("kind"); ok {
				if v, ok := dp.Attributes().Get(key); ok && v.Bool() == want {
					found[k.Str()] = true
				}
			}
		case pmetric.SummaryDataPoint:
			if k, ok := dp.Attributes().Get("kind"); ok {
				if v, ok := dp.Attributes().Get(key); ok && v.Bool() == want {
					found[k.Str()] = true
				}
			}
		}
		return nil
	})
	return found
}

func TestTransformMetricGroups_AllFiveDataPointTypes(t *testing.T) {
	ctx := context.Background()
	metrics := makeAllMetricTypesPdata()
	_, err := TransformMetricGroups(ctx, metrics, []ContextStatements{
		{Context: "datapoint", Statements: []string{`set(attributes["transformed"], true)`}},
	}, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformMetricGroups: %v", err)
	}

	wantKinds := []string{"gauge", "sum", "histogram", "exponential_histogram", "summary"}
	found := datapointHasBoolAttr(metrics, "transformed", true)
	for _, kind := range wantKinds {
		if !found[kind] {
			t.Errorf("datapoint kind %q was not transformed", kind)
		}
	}
}

func TestTransformMetricGroups_ConditionalGroup(t *testing.T) {
	ctx := context.Background()
	metrics := makeConditionalDatapointMetrics()
	groups := []ContextStatements{
		{
			Context:    "datapoint",
			Conditions: []string{`attributes["env"] == "dev"`},
			Statements: []string{`set(attributes["matched"], true)`},
		},
	}
	_, err := TransformMetricGroups(ctx, metrics, groups, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformMetricGroups conditional: %v", err)
	}

	m := metrics.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Sum().DataPoints()
	dev := m.At(0)
	prod := m.At(1)
	if v, ok := dev.Attributes().Get("matched"); !ok || !v.Bool() {
		t.Error("dev datapoint should have matched=true")
	}
	if _, ok := prod.Attributes().Get("matched"); ok {
		t.Error("prod datapoint should not match condition")
	}
}

func TestTransformMetricGroups_ConditionalOr(t *testing.T) {
	ctx := context.Background()
	metrics := makeConditionalDatapointMetrics()
	// Add staging datapoint for OR coverage.
	sm := metrics.ResourceMetrics().At(0).ScopeMetrics().At(0)
	sum := sm.Metrics().At(0).Sum()
	dpStaging := sum.DataPoints().AppendEmpty()
	dpStaging.Attributes().PutStr("env", "staging")

	groups := []ContextStatements{
		{
			Context: "datapoint",
			Conditions: []string{
				`attributes["env"] == "dev"`,
				`attributes["env"] == "staging"`,
			},
			Statements: []string{`set(attributes["or_matched"], true)`},
		},
	}
	_, err := TransformMetricGroups(ctx, metrics, groups, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformMetricGroups OR: %v", err)
	}

	for i := range sum.DataPoints().Len() {
		dp := sum.DataPoints().At(i)
		env, _ := dp.Attributes().Get("env")
		matchedVal, matched := dp.Attributes().Get("or_matched")
		switch env.Str() {
		case "dev", "staging":
			if !matched || !matchedVal.Bool() {
				t.Errorf("env=%s should have or_matched", env.Str())
			}
		case "prod":
			if matched {
				t.Error("prod should not match OR conditions")
			}
		}
	}
}

func TestTransformMetricGroups_ResourceContextOnce(t *testing.T) {
	ctx := context.Background()
	metrics := makeTwoScopesOneResource()
	groups := []ContextStatements{
		{
			Context:    "resource",
			Statements: []string{`set(attributes["tier"], "gold")`},
		},
	}
	_, err := TransformMetricGroups(ctx, metrics, groups, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformMetricGroups resource: %v", err)
	}

	rm := metrics.ResourceMetrics().At(0)
	v, ok := rm.Resource().Attributes().Get("tier")
	if !ok || v.Str() != "gold" {
		t.Errorf("resource tier = %v, want gold", v)
	}
	if rm.ScopeMetrics().Len() != 2 {
		t.Fatalf("scope count = %d, want 2 (resource stmt must not drop scopes)", rm.ScopeMetrics().Len())
	}
}

func TestTransformMetricGroups_ScopeContextOnce(t *testing.T) {
	ctx := context.Background()
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("primary")

	m1 := sm.Metrics().AppendEmpty()
	m1.SetName("m1")
	m1.SetEmptySum().DataPoints().AppendEmpty()
	m2 := sm.Metrics().AppendEmpty()
	m2.SetName("m2")
	m2.SetEmptySum().DataPoints().AppendEmpty()

	groups := []ContextStatements{
		{
			Context:    "scope",
			Statements: []string{`set(attributes["scope_lane"], "primary")`},
		},
	}
	_, err := TransformMetricGroups(ctx, metrics, groups, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformMetricGroups scope: %v", err)
	}

	v, ok := sm.Scope().Attributes().Get("scope_lane")
	if !ok || v.Str() != "primary" {
		t.Errorf("scope_lane = %v, want primary", v)
	}
	if m1.Name() != "m1" || m2.Name() != "m2" {
		t.Error("scope statement should not rename individual metrics")
	}
}

func TestTransformMetricGroups_MetricContext(t *testing.T) {
	ctx := context.Background()
	metrics := makeAllMetricTypesPdata()
	_, err := TransformMetricGroups(ctx, metrics, []ContextStatements{
		{Context: "metric", Statements: []string{`set(name, Concat([name, ".v2"], ""))`}},
	}, ErrorModePropagate)
	if err != nil {
		t.Fatalf("TransformMetricGroups metric context: %v", err)
	}

	sm := metrics.ResourceMetrics().At(0).ScopeMetrics().At(0)
	if !stringsHasSuffix(sm.Metrics().At(0).Name(), ".v2") {
		t.Errorf("metric name = %q, want *.v2 suffix", sm.Metrics().At(0).Name())
	}
}

func TestTransformMetricGroups_MixedStatementsErrorModePropagate(t *testing.T) {
	ctx := context.Background()
	metrics := makeAllMetricTypesPdata()
	statements := []string{
		`set(attributes["first"], true)`,
		`set(attributes["second"], Substring("hello", -1, 10))`,
	}
	_, err := TransformMetricGroups(ctx, metrics, []ContextStatements{
		{Context: "datapoint", Statements: statements},
	}, ErrorModePropagate)
	if err == nil {
		t.Fatal("expected error for ErrorModePropagate, got nil")
	}
}

func TestTransformMetricGroups_MixedStatementsErrorModeIgnore(t *testing.T) {
	ctx := context.Background()
	metrics := makeAllMetricTypesPdata()
	statements := []string{
		`set(attributes["first"], true)`,
		`set(attributes["second"], Substring("hello", -1, 10))`,
	}
	_, err := TransformMetricGroups(ctx, metrics, []ContextStatements{
		{Context: "datapoint", Statements: statements},
	}, ErrorModeIgnore)
	if err != nil {
		t.Fatalf("TransformMetricGroups with ErrorModeIgnore: %v", err)
	}

	dp := metrics.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(1).Sum().DataPoints().At(0)
	if v, ok := dp.Attributes().Get("first"); !ok || !v.Bool() {
		t.Error("expected first=true on datapoint after ignore on failing statement")
	}
}

func TestTransformMetricGroups_MixedStatementsErrorModeSilent(t *testing.T) {
	ctx := context.Background()
	metrics := makeAllMetricTypesPdata()
	statements := []string{
		`set(attributes["first"], true)`,
		`set(attributes["second"], Substring("hello", -1, 10))`,
	}
	_, err := TransformMetricGroups(ctx, metrics, []ContextStatements{
		{Context: "datapoint", Statements: statements},
	}, ErrorModeSilent)
	if err != nil {
		t.Fatalf("TransformMetricGroups with ErrorModeSilent: %v", err)
	}

	dp := metrics.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(1).Sum().DataPoints().At(0)
	if v, ok := dp.Attributes().Get("first"); !ok || !v.Bool() {
		t.Error("expected first=true on datapoint after silent on failing statement")
	}
}

func TestTransformMetricRunData_Cancellation(t *testing.T) {
	req := &collectormetricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{
			{
				ScopeMetrics: []*metricspb.ScopeMetrics{
					{
						Metrics: []*metricspb.Metric{
							{
								Name: "demo.counter",
								Data: &metricspb.Metric_Sum{
									Sum: &metricspb.Sum{
										DataPoints: []*metricspb.NumberDataPoint{
											{
												Attributes: []*commonv1.KeyValue{
													{Key: "env", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "dev"}}},
												},
												Value: &metricspb.NumberDataPoint_AsInt{AsInt: 1},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	data, err := protojson.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = TransformMetricRunData(ctx, json.RawMessage(data), []string{`set(attributes["env"], "cancelled")`})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func stringsHasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
