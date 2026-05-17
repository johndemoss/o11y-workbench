package gen

import (
	"context"
	"fmt"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"

	"github.com/johndemoss/o11y-workbench/internal/export"
	"github.com/johndemoss/o11y-workbench/internal/region"
)

// MetricAttribute is a key-value pair attached to each metric data point.
type MetricAttribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// MetricConfig controls metric generation behaviour.
type MetricConfig struct {
	Enabled     bool              `json:"enabled"`
	MetricCount int               `json:"metric_count"`
	GaugeRatio  float64           `json:"gauge_ratio"`
	SumRatio    float64           `json:"sum_ratio"`
	ValueMin    float64           `json:"value_min"`
	ValueMax    float64           `json:"value_max"`
	Attributes  []MetricAttribute `json:"attributes"`
}

// DefaultMetricConfig returns sensible defaults for metric generation.
func DefaultMetricConfig() MetricConfig {
	return MetricConfig{
		Enabled:     false,
		MetricCount: 20,
		GaugeRatio:  0.4,
		SumRatio:    0.4,
		ValueMin:    0,
		ValueMax:    100,
	}
}

var defaultMetricNames = []string{
	"http.requests",
	"cpu.utilization",
	"memory.usage",
	"latency.ms",
	"error.rate",
	"throughput",
}

// MetricGenerator produces OTLP metric data points (Gauge, Sum, Histogram).
type MetricGenerator struct {
	Config     MetricConfig
	OnProgress func(completed, total int)
}

// NewMetricGenerator creates a MetricGenerator with the given config.
func NewMetricGenerator(config MetricConfig) *MetricGenerator {
	return &MetricGenerator{Config: config}
}

// Run generates metric data points and exports them via exp.
func (g *MetricGenerator) Run(ctx context.Context, exp export.MetricExporter) error {
	if !g.Config.Enabled || g.Config.MetricCount == 0 {
		return nil
	}

	// Use a deterministic seed for reproducible metric type decisions.
	rng := NewSeededRNG(42)
	idRng := NewIDRng()
	_ = idRng // available for future ID generation needs

	metricsGenerated := 0
	exportBatch := make([]*metricspb.Metric, 0, 100)

	endpoints := region.All()
	if len(endpoints) == 0 {
		return fmt.Errorf("no endpoints configured")
	}

	flush := func() error {
		if len(exportBatch) == 0 {
			return nil
		}
		ep := endpoints[0] // use the first endpoint resource for all metrics
		svcName := "mock-service"

		sm := &metricspb.ScopeMetrics{
			Scope: &commonv1.InstrumentationScope{
				Name:    "o11y-workbench",
				Version: "0.1.0",
			},
			Metrics: exportBatch,
		}
		rm := &metricspb.ResourceMetrics{
			Resource: &resourcev1.Resource{
				Attributes: region.ResourceAttributes(ep, svcName),
			},
			ScopeMetrics: []*metricspb.ScopeMetrics{sm},
			SchemaUrl:    "https://opentelemetry.io/schemas/1.30.0",
		}
		if err := exp.Export(ctx, []*metricspb.ResourceMetrics{rm}); err != nil {
			return err
		}
		exportBatch = make([]*metricspb.Metric, 0, 100)
		return nil
	}

	for metricsGenerated < g.Config.MetricCount {
		select {
		case <-ctx.Done():
			flush()
			return ctx.Err()
		default:
		}

		// Determine metric type by weighted random selection.
		roll := rng.Float64()

		// Assign a name from the default pool.
		metricName := defaultMetricNames[rng.Int63n(int64(len(defaultMetricNames)))]

		// Generate a random value within the configured range.
		valueRange := g.Config.ValueMax - g.Config.ValueMin
		if valueRange < 0 {
			valueRange = 0
		}
		value := g.Config.ValueMin + rng.Float64()*valueRange

		now := time.Now()
		startTime := now.Add(-time.Minute) // simulate a 1-minute collection interval

		// Build per-data-point attributes from user configuration.
		attrs := make([]*commonv1.KeyValue, 0, len(g.Config.Attributes))
		for _, attr := range g.Config.Attributes {
			attrs = append(attrs, &commonv1.KeyValue{
				Key: attr.Key,
				Value: &commonv1.AnyValue{
					Value: &commonv1.AnyValue_StringValue{StringValue: attr.Value},
				},
			})
		}

		var metric *metricspb.Metric

		if roll < g.Config.GaugeRatio {
			metric = &metricspb.Metric{
				Name:        metricName,
				Description: "A gauge metric",
				Unit:        "1",
				Data: &metricspb.Metric_Gauge{
					Gauge: &metricspb.Gauge{
						DataPoints: []*metricspb.NumberDataPoint{
							{
								Attributes:  attrs,
								TimeUnixNano: uint64(now.UnixNano()),
								Value: &metricspb.NumberDataPoint_AsDouble{
									AsDouble: value,
								},
							},
						},
					},
				},
			}
		} else if roll < g.Config.GaugeRatio+g.Config.SumRatio {
			metric = &metricspb.Metric{
				Name:        metricName,
				Description: "A sum metric",
				Unit:        "1",
				Data: &metricspb.Metric_Sum{
					Sum: &metricspb.Sum{
						DataPoints: []*metricspb.NumberDataPoint{
							{
								Attributes:       attrs,
								TimeUnixNano:     uint64(now.UnixNano()),
								StartTimeUnixNano: uint64(startTime.UnixNano()),
								Value: &metricspb.NumberDataPoint_AsDouble{
									AsDouble: value,
								},
							},
						},
						AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
						IsMonotonic:            true,
					},
				},
			}
		} else {
			// Histogram — distribute a random count across reasonable bucket bounds.
			explicitBounds := []float64{5, 10, 25, 50, 75, 100}
			bucketCounts := make([]uint64, len(explicitBounds)+1)
			var totalCount uint64
			remaining := uint64(rng.Int63n(100) + 1) // random count in [1, 100]
			for i := range bucketCounts {
				if remaining == 0 {
					break
				}
				if i == len(bucketCounts)-1 {
					bucketCounts[i] = remaining
				} else {
					bucketCounts[i] = uint64(rng.Int63n(int64(remaining) + 1))
				}
				totalCount += bucketCounts[i]
				remaining -= bucketCounts[i]
			}
			if totalCount == 0 {
				bucketCounts[0] = 1
				totalCount = 1
			}
			histSum := value * float64(totalCount)

			metric = &metricspb.Metric{
				Name:        metricName,
				Description: "A histogram metric",
				Unit:        "1",
				Data: &metricspb.Metric_Histogram{
					Histogram: &metricspb.Histogram{
						DataPoints: []*metricspb.HistogramDataPoint{
							{
								Attributes:       attrs,
								TimeUnixNano:     uint64(now.UnixNano()),
								StartTimeUnixNano: uint64(startTime.UnixNano()),
								Count:            totalCount,
								Sum:              &histSum,
								BucketCounts:     bucketCounts,
								ExplicitBounds:   explicitBounds,
							},
						},
						AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
					},
				},
			}
		}

		exportBatch = append(exportBatch, metric)
		metricsGenerated++

		if g.OnProgress != nil {
			g.OnProgress(metricsGenerated, g.Config.MetricCount)
		}

		// Flush every 100 data points to avoid huge batches.
		if len(exportBatch) >= 100 {
			if err := flush(); err != nil {
				return err
			}
		}
	}

	return flush()
}
