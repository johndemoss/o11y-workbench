package export

import (
	"context"
	"fmt"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/johndemoss/o11y-workbench/internal/region"
)

// EndpointRouter dispatches ResourceSpans to per-endpoint exporters
// based on each trace's cloud.region resource attribute.
type EndpointRouter struct {
	exporters map[region.Name]TraceExporter
}

func NewEndpointRouter(exporters map[region.Name]TraceExporter) *EndpointRouter {
	return &EndpointRouter{exporters: exporters}
}

func (r *EndpointRouter) Export(ctx context.Context, spans []*tracev1.ResourceSpans) error {
	_, err := r.ExportWithReceipt(ctx, spans)
	return err
}

func (r *EndpointRouter) ExportWithReceipt(ctx context.Context, spans []*tracev1.ResourceSpans) (ExportReceipt, error) {
	total := ReceiptFullSuccess(0)
	groups := make(map[region.Name][]*tracev1.ResourceSpans)

	for _, rs := range spans {
		reg := regionFromResource(rs)
		groups[reg] = append(groups[reg], rs)
	}

	for reg, batch := range groups {
		exp, ok := r.exporters[reg]
		if !ok {
			err := fmt.Errorf("no exporter for region %s", reg)
			return ReceiptRPCFailure(CountSpans(spans), err), err
		}
		var receipt ExportReceipt
		var err error
		if re, ok := exp.(traceReceiptExporter); ok {
			receipt, err = re.ExportWithReceipt(ctx, batch)
		} else {
			err = exp.Export(ctx, batch)
			receipt = ReceiptFullSuccess(CountSpans(batch))
		}
		if err != nil {
			err = fmt.Errorf("region %s: %w", reg, err)
			return ReceiptRPCFailure(CountSpans(spans), err), err
		}
		mergeReceipts(&total, receipt)
	}
	return total, nil
}

func (r *EndpointRouter) Shutdown(ctx context.Context) error {
	var lastErr error
	for reg, exp := range r.exporters {
		if err := exp.Shutdown(ctx); err != nil {
			lastErr = fmt.Errorf("region %s shutdown: %w", reg, err)
		}
	}
	return lastErr
}

func regionFromResource(rs *tracev1.ResourceSpans) region.Name {
	if rs.Resource == nil {
		return region.RegionLocal
	}
	for _, attr := range rs.Resource.Attributes {
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
