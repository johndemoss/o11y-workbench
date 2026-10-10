package gen

import (
	"context"

	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/johndemoss/o11y-workbench/internal/export"
)

// metricReceiptExporter is an optional capability for transport acceptance on metrics.
type metricReceiptExporter interface {
	ExportWithReceipt(ctx context.Context, metrics []*metricspb.ResourceMetrics) (export.ExportReceipt, error)
}

// exportAccountMetrics attempts one metrics export and records transport-acceptance stats.
// Receipt-aware exporters may split accepted vs rejected items; Export-only
// exporters keep response-success accounting. Neither path claims destination delivery.
func (g *Generator) exportAccountMetrics(ctx context.Context, exp export.MetricExporter, batch []*metricspb.ResourceMetrics, items int) error {
	g.stats.recordAttempt(items)
	if re, ok := exp.(metricReceiptExporter); ok {
		receipt, err := re.ExportWithReceipt(ctx, batch)
		g.stats.recordReceipt(receipt)
		if err != nil {
			return g.exportErr(ctx, err)
		}
		return nil
	}
	err := exp.Export(ctx, batch)
	if err != nil {
		g.stats.recordFail(items)
		return g.exportErr(ctx, err)
	}
	g.stats.recordAck(items)
	return nil
}
