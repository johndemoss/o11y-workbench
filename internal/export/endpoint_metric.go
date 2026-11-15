package export

import (
	"context"
	"fmt"

	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/johndemoss/o11y-workbench/internal/region"
)

// MetricEndpointRouter dispatches ResourceMetrics to per-endpoint metric exporters
// based on each metric's cloud.region resource attribute.
type MetricEndpointRouter struct {
	exporters map[region.Name]MetricExporter
}

// NewMetricEndpointRouter creates a MetricEndpointRouter that routes ResourceMetrics
// to the exporter for the matching region.
func NewMetricEndpointRouter(exporters map[region.Name]MetricExporter) *MetricEndpointRouter {
	return &MetricEndpointRouter{exporters: exporters}
}

func (r *MetricEndpointRouter) Export(ctx context.Context, metrics []*metricspb.ResourceMetrics) error {
	_, err := r.ExportWithReceipt(ctx, metrics)
	return err
}

func (r *MetricEndpointRouter) ExportWithReceipt(ctx context.Context, metrics []*metricspb.ResourceMetrics) (ExportReceipt, error) {
	total := ReceiptFullSuccess(0)
	groups := make(map[region.Name][]*metricspb.ResourceMetrics)

	for _, rm := range metrics {
		reg := regionFromMetricResource(rm)
		groups[reg] = append(groups[reg], rm)
	}

	for reg, batch := range groups {
		exp, ok := r.exporters[reg]
		if !ok {
			err := fmt.Errorf("no exporter for region %s", reg)
			return ReceiptRPCFailure(CountDataPoints(metrics), err), err
		}
		var receipt ExportReceipt
		var err error
		if re, ok := exp.(metricReceiptExporter); ok {
			receipt, err = re.ExportWithReceipt(ctx, batch)
		} else {
			err = exp.Export(ctx, batch)
			receipt = ReceiptFullSuccess(CountDataPoints(batch))
		}
		if err != nil {
			err = fmt.Errorf("region %s: %w", reg, err)
			return ReceiptRPCFailure(CountDataPoints(metrics), err), err
		}
		mergeReceipts(&total, receipt)
	}
	return total, nil
}

func (r *MetricEndpointRouter) Shutdown(ctx context.Context) error {
	var lastErr error
	for reg, exp := range r.exporters {
		if err := exp.Shutdown(ctx); err != nil {
			lastErr = fmt.Errorf("region %s shutdown: %w", reg, err)
		}
	}
	return lastErr
}

func regionFromMetricResource(rm *metricspb.ResourceMetrics) region.Name {
	if rm.Resource == nil {
		return region.RegionLocal
	}
	for _, attr := range rm.Resource.Attributes {
		if attr.Key == "cloud.region" {
			if v := attr.GetValue(); v != nil {
				if s := v.GetStringValue(); s != "" {
					return region.Name(s)
				}
			}
		}
	}
	return region.RegionLocal
}
