package gen

import (
	"context"

	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"

	"github.com/johndemoss/o11y-workbench/internal/export"
)

// logReceiptExporter is an optional capability for transport acceptance on logs.
type logReceiptExporter interface {
	ExportWithReceipt(ctx context.Context, logs []*logsv1.ResourceLogs) (export.ExportReceipt, error)
}

// exportAccountLogs attempts one logs export and records transport-acceptance stats.
// Receipt-aware exporters may split accepted vs rejected items; Export-only
// exporters keep response-success accounting. Neither path claims destination delivery.
func (g *Generator) exportAccountLogs(ctx context.Context, exp export.LogExporter, batch []*logsv1.ResourceLogs, items int) error {
	g.stats.recordAttempt(items)
	if re, ok := exp.(logReceiptExporter); ok {
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
