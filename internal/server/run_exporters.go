package server

import (
	"context"
	"fmt"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/export"
	"github.com/johndemoss/o11y-workbench/internal/region"
)

// buildRunLogExporter wraps a buffered log sink with OTLP routing when the run
// targets OTLP. Returned closers shut down every successfully created client.
func buildRunLogExporter(run *Run, routes []region.Region, sink export.LogExporter, ctx context.Context) (export.LogExporter, []func(), error) {
	var closers []func()
	if run.Output != "otlp" {
		return sink, closers, nil
	}
	exporters := make(map[region.Name]export.LogExporter, len(routes))
	for _, route := range routes {
		exp, err := export.NewOTLPLogExporter(route.Endpoint, run.Insecure, nil)
		if err != nil {
			return nil, closers, fmt.Errorf("create OTLP log exporter for %s: %w", route.Name, err)
		}
		closers = append(closers, func() { _ = exp.Shutdown(context.Background()) })
		exporters[route.Name] = export.WithLogExportTimeoutContext(ctx, exp, 30*time.Second)
	}
	return multiLogExporter{sink, export.NewLogEndpointRouter(exporters)}, closers, nil
}

// buildRunMetricExporter wraps a buffered metric sink with OTLP routing when the
// run targets OTLP. Returned closers shut down every successfully created client.
func buildRunMetricExporter(run *Run, routes []region.Region, sink export.MetricExporter, ctx context.Context) (export.MetricExporter, []func(), error) {
	var closers []func()
	if run.Output != "otlp" {
		return sink, closers, nil
	}
	exporters := make(map[region.Name]export.MetricExporter, len(routes))
	for _, route := range routes {
		exp, err := export.NewOTLPMetricExporter(route.Endpoint, run.Insecure, nil)
		if err != nil {
			return nil, closers, fmt.Errorf("create OTLP metric exporter for %s: %w", route.Name, err)
		}
		closers = append(closers, func() { _ = exp.Shutdown(context.Background()) })
		exporters[route.Name] = export.WithMetricExportTimeoutContext(ctx, exp, 30*time.Second)
	}
	return multiMetricExporter{sink, export.NewMetricEndpointRouter(exporters)}, closers, nil
}
