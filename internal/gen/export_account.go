package gen

import (
	"context"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/johndemoss/o11y-workbench/internal/export"
)

// traceReceiptExporter is an optional capability. Exporters that can report
// transport acceptance implement ExportWithReceipt; others keep Export-only.
// Defined where consumed (gen), not forced onto export.TraceExporter.
type traceReceiptExporter interface {
	ExportWithReceipt(ctx context.Context, spans []*tracev1.ResourceSpans) (export.ExportReceipt, error)
}

// exportAccount attempts one export and records transport-acceptance stats.
// Receipt-aware exporters may split accepted vs rejected items; Export-only
// exporters keep prior response-success accounting. Neither path claims
// destination delivery.
func (g *Generator) exportAccount(ctx context.Context, exp export.TraceExporter, batch []*tracev1.ResourceSpans, items int) error {
	g.stats.recordAttempt(items)
	if re, ok := exp.(traceReceiptExporter); ok {
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
	// Response success for exporters without receipts — not destination delivery.
	g.stats.recordAck(items)
	return nil
}

func (g *Generator) exportErr(ctx context.Context, err error) error {
	if g.config.Workload.ContinueOnExportErr {
		if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return nil
			}
			return ctx.Err()
		}
		return nil
	}
	return err
}
