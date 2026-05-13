package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/johndemoss/mocktracegen/internal/gen"
	"github.com/johndemoss/mocktracegen/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	jobsPath := flag.String("jobs", "", "Path to JSON file with startup job definitions")
	flag.Parse()

	srv := server.New(*addr)

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

	log.Fatal(srv.ListenAndServe())
}
