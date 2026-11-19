// Package workload exposes the workbench generator's paced synthetic OTLP
// traces ingress through a supported public API. It is a facade only:
// scheduling, span grammar, and transport-acceptance accounting stay in
// internal/gen and internal/export.
package workload

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/export"
	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/region"
)

// exportTimeout bounds each wrapped export call and the final exporter
// shutdown.
const exportTimeout = 10 * time.Second

// Burst schedules on/off peak ingress for Mode "burst".
type Burst struct {
	On                  time.Duration
	Off                 time.Duration
	PeakRateItemsPerSec float64
}

// Config selects one paced synthetic traces workload. New validates every
// field before a generator is allocated or a connection is opened.
type Config struct {
	// Endpoint is the OTLP gRPC dial target (host:port, no scheme).
	Endpoint string
	// Insecure selects plaintext transport.
	Insecure bool
	Seed     int64
	Duration time.Duration
	// ServiceName labels the generated resource. Empty keeps the
	// generator's workload default.
	ServiceName string
	// ExperimentID stamps evidence attributes. Empty derives one from Seed.
	ExperimentID string
	// ItemsPerRequest is the span count per OTLP export request.
	ItemsPerRequest     int
	AttributeCount      int
	AttributeValueBytes int
	Cardinality         int
	// Mode is "constant" (empty normalizes to constant) or "burst".
	Mode  string
	Burst Burst
	// RateItemsPerSec and RequestsPerSec keep the generator's precedence:
	// RequestsPerSec wins when both are set; burst mode uses Burst only.
	RateItemsPerSec float64
	RequestsPerSec  float64
	// ContinueOnExportErr keeps generating after export failures.
	ContinueOnExportErr bool
}

// Stats tracks attempted versus transport-accepted export submissions.
// Acceptance semantics are OTLP response acceptance only, not destination
// delivery.
type Stats struct {
	AttemptedRequests uint64 `json:"attempted_requests"`
	AckedRequests     uint64 `json:"acked_requests"`
	FailedRequests    uint64 `json:"failed_requests"`
	AttemptedItems    uint64 `json:"attempted_items"`
	AckedItems        uint64 `json:"acked_items"`
	RejectedItems     uint64 `json:"rejected_items"`
	FailedItems       uint64 `json:"failed_items"`
	UncertainItems    uint64 `json:"uncertain_items"`
}

// Runner executes exactly one workload. New validates and constructs it;
// Run opens the exporter connection, generates, and closes the exporter.
type Runner struct {
	cfg     Config
	gen     *gen.Generator
	started atomic.Bool
}

// New validates cfg and constructs a Runner. It opens no connection and
// starts no goroutine; failures surface before any resource is held.
func New(cfg Config) (*Runner, error) {
	gc, err := genConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Runner{cfg: cfg, gen: gen.NewGenerator(gc)}, nil
}

// Run generates the configured workload to cfg.Endpoint. It is one-shot:
// repeat or concurrent calls return an error without touching the counters.
// The passed context is the owner lifetime: canceling it cancels blocked
// export calls. The exporter is closed before Run returns; a generation
// error is preserved even when shutdown also fails.
func (r *Runner) Run(ctx context.Context) error {
	if !r.started.CompareAndSwap(false, true) {
		return fmt.Errorf("workload runner already started")
	}
	exp, err := export.NewOTLPExporter(r.cfg.Endpoint, r.cfg.Insecure, nil)
	if err != nil {
		return fmt.Errorf("otlp exporter: %w", err)
	}
	wrapped := export.WithExportTimeoutContext(ctx, exp, exportTimeout)
	runErr := r.gen.Run(ctx, wrapped)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), exportTimeout)
	shutdownErr := exp.Shutdown(shutdownCtx)
	cancel()
	if runErr != nil {
		return runErr
	}
	if shutdownErr != nil {
		return fmt.Errorf("exporter shutdown: %w", shutdownErr)
	}
	return nil
}

// Stats returns a snapshot of the transport-acceptance counters. Read the
// final values after Run returns.
func (r *Runner) Stats() Stats {
	s := r.gen.Stats()
	return Stats{
		AttemptedRequests: s.AttemptedRequests,
		AckedRequests:     s.AckedRequests,
		FailedRequests:    s.FailedRequests,
		AttemptedItems:    s.AttemptedItems,
		AckedItems:        s.AckedItems,
		RejectedItems:     s.RejectedItems,
		FailedItems:       s.FailedItems,
		UncertainItems:    s.UncertainItems,
	}
}

// genConfig maps the public Config onto the generator's own dimensions and
// delegates dimension, rate, and allocation bounds to gen.ValidateConfig.
func genConfig(cfg Config) (gen.Config, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return gen.Config{}, fmt.Errorf("workload endpoint is required")
	}
	if cfg.Duration <= 0 {
		return gen.Config{}, fmt.Errorf("workload duration must be positive")
	}
	if cfg.ItemsPerRequest <= 0 {
		return gen.Config{}, fmt.Errorf("workload items per request must be positive")
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "constant"
	}
	if mode != "constant" && mode != "burst" {
		return gen.Config{}, fmt.Errorf("workload mode must be constant or burst (got %q)", cfg.Mode)
	}
	if !(effectiveRequestRate(mode, cfg) > 0) {
		return gen.Config{}, fmt.Errorf("workload requires a positive effective request rate")
	}
	gc := gen.Config{
		Seed:        cfg.Seed,
		Duration:    cfg.Duration,
		ServiceName: cfg.ServiceName,
		Signals:     gen.Signals{Traces: true},
		// RegionLocal is a resource label here, not routing authority: the
		// facade exporter dials Config.Endpoint directly, so global region
		// endpoints are never consulted.
		Regions: []region.Name{region.RegionLocal},
		Workload: gen.WorkloadProfile{
			ExperimentID:        cfg.ExperimentID,
			ItemsPerRequest:     cfg.ItemsPerRequest,
			AttributeCount:      cfg.AttributeCount,
			AttributeValueBytes: cfg.AttributeValueBytes,
			Cardinality:         cfg.Cardinality,
			Mode:                mode,
			Burst: gen.BurstConfig{
				On:                  cfg.Burst.On,
				Off:                 cfg.Burst.Off,
				PeakRateItemsPerSec: cfg.Burst.PeakRateItemsPerSec,
			},
			RateItemsPerSec:     cfg.RateItemsPerSec,
			RequestsPerSec:      cfg.RequestsPerSec,
			ContinueOnExportErr: cfg.ContinueOnExportErr,
			Shape:               "flat",
			SingleResource:      true,
		},
	}
	if err := gen.ValidateConfig(gc); err != nil {
		return gen.Config{}, err
	}
	return gc, nil
}

// effectiveRequestRate mirrors WorkloadProfile.requestRate precedence for
// the normalized mode: burst peak, then RequestsPerSec, then
// RateItemsPerSec converted to requests per second.
func effectiveRequestRate(mode string, cfg Config) float64 {
	items := float64(cfg.ItemsPerRequest)
	if items <= 0 {
		items = 1
	}
	if mode == "burst" {
		return cfg.Burst.PeakRateItemsPerSec / items
	}
	if cfg.RequestsPerSec > 0 {
		return cfg.RequestsPerSec
	}
	return cfg.RateItemsPerSec / items
}
