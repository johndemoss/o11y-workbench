package gen

import (
	"context"
	"fmt"
	"strings"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"

	"github.com/johndemoss/o11y-workbench/internal/export"
)

// RunMetrics generates paced flat OTLP metrics.
// Mirrors runWorkloadProfile ledger semantics (items = metric data points).
func (g *Generator) RunMetrics(ctx context.Context, exp export.MetricExporter) error {
	if g == nil {
		return fmt.Errorf("generator is nil")
	}
	if err := ValidateConfig(g.config); err != nil {
		return err
	}
	return g.runMetricWorkloadProfile(ctx, exp)
}

// runMetricWorkloadProfile is the explicit Shape=="flat" helper for metrics:
// paced OTLP data points with one ResourceMetrics per export request.
func (g *Generator) runMetricWorkloadProfile(ctx context.Context, exp export.MetricExporter) error {
	p := g.config.Workload
	if g.config.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.config.Duration)
		defer cancel()
	}

	items := p.itemsPerRequest()
	expID := p.experimentID(g.config.Seed)
	svc := g.config.ServiceName
	if svc == "" {
		svc = "o11y-workbench-workload"
	}

	var seq uint64
	pacer := workloadPacer{profile: p, fallbackRate: g.config.Rate}
	requestBudget := 0
	if g.config.TraceCount > 0 {
		// Reuse TraceCount as request budget for flat evidence workloads.
		requestBudget = g.config.TraceCount
	} else if g.config.MetricConfig.MetricCount > 0 && !p.pacingActive() && g.config.Duration == 0 {
		requestBudget = (g.config.MetricConfig.MetricCount-1)/items + 1
	}

	for {
		if requestBudget > 0 && int(seq) >= requestBudget {
			return nil
		}
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return nil
			}
			return ctx.Err()
		default:
		}

		if err := pacer.start(ctx, items); err != nil {
			if err == context.DeadlineExceeded {
				return nil
			}
			return err
		}
		seq++
		rm := g.workloadResourceMetrics(expID, svc, seq, items)
		if err := g.exportAccountMetrics(ctx, exp, []*metricspb.ResourceMetrics{rm}, items); err != nil {
			return err
		}

		if g.OnProgress != nil {
			g.OnProgress(int(seq), requestBudget)
		}

	}
}

func (g *Generator) workloadResourceMetrics(expID, svc string, seq uint64, items int) *metricspb.ResourceMetrics {
	now := uint64(time.Now().UnixNano())
	card := g.config.Workload.Cardinality
	if card <= 0 {
		card = 1
	}
	val := strings.Repeat("x", g.config.Workload.AttributeValueBytes)
	points := make([]*metricspb.NumberDataPoint, 0, items)
	for i := 0; i < items; i++ {
		attrs := []*commonv1.KeyValue{
			stringAttr("experiment.id", expID),
			stringAttr("payload.id", fmt.Sprintf("%s-%d-%d", expID, seq, i)),
			stringAttr("cardinality.key", fmt.Sprintf("c-%d", int(seq)%card)),
		}
		for a := 0; a < g.config.Workload.AttributeCount; a++ {
			attrs = append(attrs, stringAttr(fmt.Sprintf("attr.%d", a), val))
		}
		points = append(points, &metricspb.NumberDataPoint{
			Attributes:   attrs,
			TimeUnixNano: now,
			Value: &metricspb.NumberDataPoint_AsDouble{
				AsDouble: float64(i + 1),
			},
		})
	}
	return &metricspb.ResourceMetrics{
		Resource: &resourcev1.Resource{
			Attributes: []*commonv1.KeyValue{stringAttr("service.name", svc)},
		},
		ScopeMetrics: []*metricspb.ScopeMetrics{{
			Scope: &commonv1.InstrumentationScope{Name: "o11y-workbench", Version: "0.1.0"},
			Metrics: []*metricspb.Metric{{
				Name:        "workload.gauge",
				Description: "flat evidence workload gauge",
				Unit:        "1",
				Data: &metricspb.Metric_Gauge{
					Gauge: &metricspb.Gauge{DataPoints: points},
				},
			}},
		}},
		SchemaUrl: "https://opentelemetry.io/schemas/1.30.0",
	}
}
