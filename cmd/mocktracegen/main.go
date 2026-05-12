package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/johndemoss/mocktracegen/internal/export"
	"github.com/johndemoss/mocktracegen/internal/gen"
	"github.com/johndemoss/mocktracegen/internal/region"
)

func main() {
	var (
		seed        int64
		traceCount  int
		rate        int
		topology    string
		spanCount   int
		maxDepth    int
		serviceName string

		outputMode string
		format     string
		outputDir  string
		endpoints  []string
		insecure   bool
	)

	rootCmd := &cobra.Command{
		Use:   "mocktracegen",
		Short: "Deterministic mock OTLP trace generator",
		Long: `mocktracegen generates fake but structurally valid OpenTelemetry traces
for testing distributed system scenarios, specifically trace-ID-aware
load balancing across endpoints.

Traces follow OpenTelemetry's protobuf wire format and can be fed into
any trace collector (Jaeger, Datadog, etc.) or inspected as JSON/protobuf files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			topoType, err := gen.ParseTopology(topology)
			if err != nil {
				return err
			}

			region.SetEndpoints(endpoints)

			cfg := gen.Config{
				Seed:          seed,
				TraceCount:    traceCount,
				Rate:          rate,
				Topology:      topoType,
				SpanCount:     spanCount,
				MaxDepth:      maxDepth,
				ServiceName:   serviceName,
			}

			var exporter export.Exporter

			switch outputMode {
			case "file":
				fmtFmt, err := export.ParseFormat(format)
				if err != nil {
					return fmt.Errorf("parse format: %w", err)
				}
				exporter = export.NewTraceFileExporter(outputDir, fmtFmt)

			case "otlp":
				exporters := make(map[region.Name]export.Exporter)
				for _, reg := range region.All() {
					ep := region.GetEndpoint(reg)
					exp, err := export.NewOTLPExporter(ep, insecure, nil)
					if err != nil {
						return fmt.Errorf("create OTLP exporter for %s: %w", reg, err)
					}
					exporters[reg] = exp
				}
				exporter = export.NewEndpointRouter(exporters)

			default:
				return fmt.Errorf("unknown output mode %q, expected: file, otlp", outputMode)
			}

			generator := gen.NewGenerator(cfg)

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			if err := generator.Run(ctx, exporter); err != nil {
				return fmt.Errorf("generate and export traces: %w", err)
			}

			if err := exporter.Shutdown(ctx); err != nil {
				return fmt.Errorf("shutdown exporter: %w", err)
			}

			return nil
		},
	}

	rootCmd.Flags().Int64Var(&seed, "seed", 42, "RNG seed for deterministic trace generation")
	rootCmd.Flags().IntVar(&traceCount, "traces", 1, "number of traces to generate")
	rootCmd.Flags().IntVar(&rate, "rate", 0, "target traces per second (0 = unlimited)")
	rootCmd.Flags().StringVar(&topology, "topology", "flat", "trace topology shape (flat, branching, split)")
	rootCmd.Flags().IntVar(&spanCount, "span-count", 10, "target number of spans per trace")
	rootCmd.Flags().IntVar(&maxDepth, "max-depth", 5, "maximum span depth in branching/split topologies")
	rootCmd.Flags().StringVar(&serviceName, "service-name", "mock-service", "service name for resource attributes")

	rootCmd.Flags().StringVar(&outputMode, "output", "file", "output mode (file, otlp)")
	rootCmd.Flags().StringVar(&format, "format", "json", "output format for file mode (json, protobuf)")
	rootCmd.Flags().StringVar(&outputDir, "output-dir", "./traces", "output directory for file mode")
	rootCmd.Flags().StringArrayVar(&endpoints, "endpoint", nil, "collector endpoint address (repeatable, e.g. --endpoint localhost:4317 --endpoint collector:4317)")
	rootCmd.Flags().BoolVar(&insecure, "insecure", true, "use insecure gRPC connection")

	rootCmd.AddCommand(regionsCmd())

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func regionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "regions",
		Short: "List available regions",
		Run: func(cmd *cobra.Command, args []string) {
			for _, r := range region.All() {
				fmt.Printf("%s  →  %s\n", r, region.GetEndpoint(r))
			}
		},
	}
}
