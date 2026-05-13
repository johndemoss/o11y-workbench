package export

import (
	"context"
	"fmt"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/johndemoss/mocktelemetrygen/internal/region"
)

// EndpointRouter dispatches ResourceSpans to per-endpoint exporters
// based on each trace's cloud.region resource attribute.
type EndpointRouter struct {
	exporters map[region.Name]Exporter
}

func NewEndpointRouter(exporters map[region.Name]Exporter) *EndpointRouter {
	return &EndpointRouter{exporters: exporters}
}

func (r *EndpointRouter) Export(ctx context.Context, spans []*tracev1.ResourceSpans) error {
	groups := make(map[region.Name][]*tracev1.ResourceSpans)

	for _, rs := range spans {
		reg := regionFromResource(rs)
		groups[reg] = append(groups[reg], rs)
	}

	for reg, batch := range groups {
		exp, ok := r.exporters[reg]
		if !ok {
			continue
		}
		if err := exp.Export(ctx, batch); err != nil {
			return fmt.Errorf("region %s: %w", reg, err)
		}
	}
	return nil
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
