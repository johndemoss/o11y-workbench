package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/johndemoss/o11y-workbench/internal/export"
	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/region"
)

var cfgFile string

func main() {
	cmd := newRootCmd()
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "o11y-workbench",
		Short: "Deterministic mock OTLP trace generator",
		Long: `o11y-workbench generates fake but structurally valid OpenTelemetry traces
for testing distributed system scenarios, specifically trace-ID-aware
load balancing across endpoints.

Traces follow OpenTelemetry's protobuf wire format and can be fed into
any trace collector (Jaeger, Datadog, etc.) or inspected as JSON/protobuf files.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			scenario := viper.GetString("scenario")

			// Load user-defined scenarios from file if specified.
			if sf := viper.GetString("scenario-file"); sf != "" {
				loaded, err := gen.LoadScenarioFile(sf)
				if err != nil {
					return fmt.Errorf("load scenarios: %w", err)
				}
				names := make([]string, 0, len(loaded))
				for n := range loaded {
					names = append(names, n)
				}
				log.Printf("Loaded %d user scenario(s) from %s: %v", len(loaded), sf, names)
			}

			region.SetEndpoints(viper.GetStringSlice("endpoint"))

			var topoType gen.TopologyType
			if scenario == "" {
				var err error
				topoType, err = gen.ParseTopology(viper.GetString("topology"))
				if err != nil {
					return err
				}
			}

			traces := viper.GetInt("traces")
			duration := viper.GetInt("duration")

			switch {
			case traces > 0:
				// Fixed-count mode: timed mode not useful
				if duration > 0 {
					log.Printf("Note: --traces=%d takes precedence over --duration=%d (mutually exclusive); using fixed-count mode", traces, duration)
				}
				duration = 0
			case duration > 0:
				// Timed mode: trace count not useful
				traces = 0
			}

			cfg := gen.Config{
				Seed:              viper.GetInt64("seed"),
				TraceCount:        traces,
				Rate:              viper.GetInt("rate"),
				Duration:          time.Duration(duration) * time.Second,
				Topology:          topoType,
				SpanCount:         viper.GetInt("span-count"),
				MaxDepth:          viper.GetInt("max-depth"),
				ServiceName:       viper.GetString("service-name"),
				Scenario:          scenario,
				ScenarioSpanCount: viper.GetInt("scenario-span-count"),
			}

			var exporter export.TraceExporter

			switch viper.GetString("output") {
			case "file":
				fmtFmt, err := export.ParseFormat(viper.GetString("format"))
				if err != nil {
					return fmt.Errorf("parse format: %w", err)
				}
				exporter = export.NewTraceFileExporter(viper.GetString("output-dir"), fmtFmt)

			case "otlp":
				exporters := make(map[region.Name]export.TraceExporter)
				for _, reg := range region.All() {
					ep := region.GetEndpoint(reg)
					exp, err := export.NewOTLPExporter(ep, viper.GetBool("insecure"), nil)
					if err != nil {
						return fmt.Errorf("create OTLP exporter for %s: %w", reg, err)
					}
					exporters[reg] = exp
				}
				exporter = export.NewEndpointRouter(exporters)

			default:
				return fmt.Errorf("unknown output mode %q, expected: file, otlp", viper.GetString("output"))
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

	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path (YAML, JSON, or TOML)")

	cmd.Flags().Int64("seed", 42, "RNG seed for deterministic trace generation")
	cmd.Flags().Int("traces", 0, "number of traces to generate (0 = unlimited; mutually exclusive with --duration)")
	cmd.Flags().Int("rate", 1, "target traces per second (0 = unlimited)")
	cmd.Flags().Int("duration", 10, "max run duration in seconds (0 = unlimited; mutually exclusive with --traces)")
	cmd.Flags().String("topology", "flat", "trace topology shape (flat, branching, split)")
	cmd.Flags().String("scenario", "", "trace scenario: microservice, monolith, 3-tier-web-app, event-driven, or a user-defined scenario")
	cmd.Flags().String("scenario-file", "", "path to YAML/JSON file with user-defined scenario definitions")
	cmd.Flags().Int("scenario-span-count", 0, "per-service span budget for scenarios; 0 uses --span-count")
	cmd.Flags().Int("span-count", 10, "target number of spans per trace")
	cmd.Flags().Int("max-depth", 5, "maximum span depth in branching/split topologies")
	cmd.Flags().String("service-name", "mock-service", "service name for resource attributes")

	cmd.Flags().String("output", "file", "output mode (file, otlp)")
	cmd.Flags().String("format", "json", "output format for file mode (json, protobuf)")
	cmd.Flags().String("output-dir", "./traces", "output directory for file mode")
	cmd.Flags().StringSlice("endpoint", nil, "collector endpoint address (repeatable, e.g. --endpoint localhost:4317 --endpoint collector:4317)")
	cmd.Flags().Bool("insecure", true, "use insecure gRPC connection")

	// Precedence: CLI flag > env var > config file > built-in default.
	for _, name := range []string{
		"seed", "traces", "rate", "duration", "topology", "scenario", "scenario-file", "scenario-span-count", "span-count", "max-depth",
		"service-name", "output", "format", "output-dir", "endpoint", "insecure",
	} {
		if err := viper.BindPFlag(name, cmd.Flags().Lookup(name)); err != nil {
			panic(fmt.Sprintf("bind flag %s: %v", name, err))
		}
	}

	cmd.AddCommand(regionsCmd())

	return cmd
}

func initConfig() error {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.SetConfigName("o11y-workbench")
	}

	viper.SetEnvPrefix("O11Y_WORKBENCH")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("read config file: %w", err)
		}
	}

	return nil
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
