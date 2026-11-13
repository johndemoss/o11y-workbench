package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/server"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "HTTP listen address (loopback only by default)")
	jobsPath := flag.String("jobs", "", "Path to JSON file with startup job definitions")
	scenarioFile := flag.String("scenario-file", "", "Path to YAML/JSON file with user-defined scenario definitions")
	pipelinePrototyper := flag.Bool("pipeline-prototyper", false, "Enable the pipeline prototyper UI and API")
	flag.Parse()

	pipelineEnabled := *pipelinePrototyper
	if v := os.Getenv("O11Y_WORKBENCH_PIPELINE_PROTOTYPER"); v == "true" {
		pipelineEnabled = true
	}

	srv, err := server.New(*addr, pipelineEnabled)
	if err != nil {
		log.Fatalf("creating server: %v", err)
	}

	if *scenarioFile != "" {
		log.Printf("Loading user scenarios from %s", *scenarioFile)
		if _, err := gen.LoadScenarioFile(*scenarioFile); err != nil {
			log.Fatalf("loading scenario file: %v", err)
		}
	}

	if *jobsPath != "" {
		data, err := os.ReadFile(*jobsPath)
		if err != nil {
			log.Fatalf("reading jobs file: %v", err)
		}
		type jobDef struct {
			Seed              int64  `json:"seed"`
			TraceCount        int    `json:"trace_count"`
			Rate              int    `json:"rate"`
			Topology          string `json:"topology"`
			SpanCount         int    `json:"span_count"`
			MaxDepth          int    `json:"max_depth"`
			ServiceName       string `json:"service_name"`
			Scenario          string `json:"scenario"`
			ScenarioSpanCount int    `json:"scenario_span_count"`
		}
		var jobs []jobDef
		if err := json.Unmarshal(data, &jobs); err != nil {
			log.Fatalf("parsing jobs file: %v", err)
		}
		for _, jd := range jobs {
			topoType := gen.TopologyFlat
			if jd.Topology != "" {
				var err error
				topoType, err = gen.ParseTopology(jd.Topology)
				if err != nil {
					log.Printf("parsing topology %q in job: %v", jd.Topology, err)
					continue
				}
			}
			cfg := gen.Config{
				Seed: jd.Seed, TraceCount: jd.TraceCount, Rate: jd.Rate,
				Topology: topoType, SpanCount: jd.SpanCount, MaxDepth: jd.MaxDepth,
				ServiceName: jd.ServiceName, Scenario: jd.Scenario,
				ScenarioSpanCount: jd.ScenarioSpanCount,
			}
			run, err := server.StartRun(context.Background(), srv.Store(), cfg, nil, true, "otlp")
			if err != nil {
				log.Printf("starting job run: %v", err)
				continue
			}
			log.Printf("started job run %s: scenario=%q trace_count=%d", run.ID, cfg.Scenario, cfg.TraceCount)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.ListenAndServeContext(ctx); err != nil {
		log.Fatalf("server: %v", err)
	}
}
