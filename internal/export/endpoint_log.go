package export

import (
	"context"
	"fmt"

	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"

	"github.com/johndemoss/o11y-workbench/internal/region"
)

// LogEndpointRouter dispatches ResourceLogs to per-endpoint log exporters
// based on each log's cloud.region resource attribute.
type LogEndpointRouter struct {
	exporters map[region.Name]LogExporter
}

// NewLogEndpointRouter creates a LogEndpointRouter that routes ResourceLogs
// to the exporter for the matching region.
func NewLogEndpointRouter(exporters map[region.Name]LogExporter) *LogEndpointRouter {
	return &LogEndpointRouter{exporters: exporters}
}

func (r *LogEndpointRouter) Export(ctx context.Context, logs []*logsv1.ResourceLogs) error {
	groups := make(map[region.Name][]*logsv1.ResourceLogs)

	for _, rl := range logs {
		reg := regionFromLogResource(rl)
		groups[reg] = append(groups[reg], rl)
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

func (r *LogEndpointRouter) Shutdown(ctx context.Context) error {
	var lastErr error
	for reg, exp := range r.exporters {
		if err := exp.Shutdown(ctx); err != nil {
			lastErr = fmt.Errorf("region %s shutdown: %w", reg, err)
		}
	}
	return lastErr
}

func regionFromLogResource(rl *logsv1.ResourceLogs) region.Name {
	if rl.Resource == nil {
		return region.RegionLocal
	}
	for _, attr := range rl.Resource.Attributes {
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
