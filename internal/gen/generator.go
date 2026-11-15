// Package gen provides deterministic OTLP telemetry generation for traces, logs, and metrics.
//
// # Architecture: Signal Generators
//
// This package contains three independent generator types, one per signal domain:
//
//   - Generator   (generator.go)  — produces tracev1.Span records
//   - LogGenerator (log_generator.go) — produces logsv1.LogRecord records
//   - MetricGenerator (metric_generator.go) — produces metricspb.Metric records
//
// Each generator is a plain Go struct with a typed Export method.  They share
// structural patterns (deterministic RNG, batch flush, OTLP resource/scope
// wrapping, progress callbacks) but do NOT share a common interface.
//
// # Signal Branching
//
// Signal branching (which generator to run for a given request) happens in
// internal/server/run.go via the executeRun function, using Config.SignalType
// as a string discriminant:
//
//	if doTraces  { Generator.Run(ctx, exp) }
//	if doLogs    { LogGenerator.Run(ctx, logExp) or LogGenerator.RunCorrelated(ctx, refs, logExp) }
//	if doMetrics { MetricGenerator.Run(ctx, metricExp) }
//
// Correlated logs stream from Generator.OnTraceSpans into RunCorrelated;
// neither side reads exported or captured trace data.
//
// SignalType is never inspected inside any generator — each generator is
// purely self-contained and signal-agnostic beyond its own typed Export call.
//
// # Strategy Pattern Evaluation (ADR)
//
// Evaluating whether a SignalGenerator interface + strategy pattern would improve
// the architecture:
//
// PROPOSED INTERFACE (hypothetical):
//
//	type SignalGenerator[T any] interface {
//	  Run(ctx context.Context, exp Exporter[T]) error
//	}
//
// ANALYSIS:
//
// ✅ PRO: Type-safe dispatch without string branching in run.go
// ✅ PRO: Adding a new signal type would be self-contained (no run.go changes)
// ✅ PRO: Shared SignalGenerator base could consolidate flush/batch patterns
// ✅ PRO: Easier to unit-test generators in isolation with mock exporters
//
// CONCERNS:
// ⚠️  Generators are already independent — no shared base class or parent interface
// ⚠️  MetricGenerator.Run and LogGenerator.RunCorrelated have different signatures
//
//	(LogGenerator.RunCorrelated consumes span references; MetricGenerator.Run does not)
//
// ⚠️  Run.go would still need to know which signals are enabled (doTraces/doLogs/doMetrics)
//
//	— a strategy interface doesn't eliminate that branching, it just moves it
//
// ⚠️  Run.go signal routing is already clean and unlikely to grow — adding a new
//
//	signal would only require adding one more if-block
//
// VERDICT: DO NOT IMPLEMENT a strategy pattern.
//
// The current design achieves separation through composition.  The "branching"
// lives at exactly one place (run.go) and is unlikely to grow.  A strategy
// interface would add indirection without meaningful benefit, and the signature
// mismatch between LogGenerator and MetricGenerator would require either a
// variant interface or workarounds that reduce the clarity of the current design.
//
// Instead, if reduced duplication is desired, consider extracting shared helpers:
//
//   - flushBatch() utility for the common batch→export→reset pattern
//   - Progress callback wiring as a utility function
//   - region.All() iteration as a utility
//
// These can be introduced without changing the generator interfaces.
//
// ---
//
// See also: internal/server/run.go (executeRun) for the signal dispatch layer.
package gen

import (
	"context"
	"fmt"
	"sort"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/johndemoss/o11y-workbench/internal/export"
	"github.com/johndemoss/o11y-workbench/internal/region"
)

// Signals represents which telemetry signal types to generate.
// This replaces the magic string SignalType with explicit boolean flags.
type Signals struct {
	Traces  bool `json:"traces"`
	Logs    bool `json:"logs"`
	Metrics bool `json:"metrics"`
}

// NewSignalsFromType converts legacy signal type strings to the new Signals struct.
// Used for backwards compatibility during migration.
func NewSignalsFromType(signalType string) Signals {
	if signalType == "" {
		signalType = "traces"
	}
	return Signals{
		Traces:  signalType == "traces" || signalType == "both" || signalType == "all" || signalType == "traces+metrics",
		Logs:    signalType == "logs" || signalType == "both" || signalType == "all" || signalType == "logs+metrics",
		Metrics: signalType == "metrics" || signalType == "all" || signalType == "traces+metrics" || signalType == "logs+metrics",
	}
}

type Config struct {
	Seed              int64
	TraceCount        int
	Rate              int // target traces/sec; 0 = unlimited
	Topology          TopologyType
	SpanCount         int
	MaxDepth          int
	ServiceName       string
	BaseTime          time.Time
	TraceDuration     time.Duration
	Duration          time.Duration // max run duration; 0 = unlimited
	Scenario          string        // named scenario; "" means use Topology
	ScenarioSpanCount int           // per-service span budget for scenarios; 0 means use SpanCount
	SignalType        string        `json:"signal_type,omitempty"` // DEPRECATED: use Signals instead
	Signals           Signals       `json:"signals,omitempty"`
	LogConfig         LogConfig     `json:"log_config,omitempty"`
	MetricConfig      MetricConfig  `json:"metric_config,omitempty"`
	ErrorPercent      int           // percentage of spans to mark as errors; 0 = none
	// Workload enables optional paced synthetic-ingress knobs.
	// Zero value keeps topology/scenario generation unchanged.
	Workload WorkloadProfile `json:"workload,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		Seed:          42,
		TraceCount:    0,
		Topology:      TopologyFlat,
		SpanCount:     10,
		MaxDepth:      5,
		ServiceName:   "mock-service",
		BaseTime:      time.Now(),
		TraceDuration: 5 * time.Second,
		ErrorPercent:  10,
	}
}

// IncludesTraces returns true if traces should be generated.
// Prefers the Signals field if set, falls back to SignalType for backwards compat.
func (c Config) IncludesTraces() bool {
	// Prefer Signals if set
	if c.Signals != (Signals{}) {
		return c.Signals.Traces
	}
	// Fall back to SignalType for backwards compat
	st := c.SignalType
	if st == "" {
		st = "traces"
	}
	return st == "traces" || st == "both" || st == "all" || st == "traces+metrics"
}

// IncludesLogs returns true if logs should be generated.
// Prefers the Signals field if set, falls back to SignalType for backwards compat.
func (c Config) IncludesLogs() bool {
	if c.Signals != (Signals{}) {
		return c.Signals.Logs
	}
	st := c.SignalType
	return st == "logs" || st == "both" || st == "all" || st == "logs+metrics"
}

// IncludesMetrics returns true if metrics should be generated.
// Prefers the Signals field if set, falls back to SignalType for backwards compat.
func (c Config) IncludesMetrics() bool {
	if c.Signals != (Signals{}) {
		return c.Signals.Metrics
	}
	st := c.SignalType
	return st == "metrics" || st == "all" || st == "traces+metrics" || st == "logs+metrics"
}

// SpanRef identifies one exported span for trace/log correlation.
type SpanRef struct {
	TraceID [16]byte
	SpanID  [8]byte
	Service string
}

type Generator struct {
	config     Config
	stats      statsCounters
	OnProgress func(completed, total int)
	// OnTraceSpans receives the spans of each trace whose exports succeeded,
	// before OnProgress and before pacing waits for the next trace. The slice
	// is not reused by the generator. Blocking applies backpressure to Run.
	OnTraceSpans func([]SpanRef)
	// Regions pins routing labels for this instance. Nil uses CLI process defaults.
	// Set before Run and do not mutate while generation is active.
	Regions []region.Name
}

func NewGenerator(config Config) *Generator {
	defaults := DefaultConfig()
	if config.BaseTime.IsZero() {
		config.BaseTime = defaults.BaseTime
	}
	if config.TraceDuration == 0 {
		config.TraceDuration = defaults.TraceDuration
	}
	if config.ServiceName == "" {
		config.ServiceName = defaults.ServiceName
	}
	return &Generator{config: config}
}

// Stats returns attempted/acked/failed export counters for this generator.
func (g *Generator) Stats() Stats {
	return g.stats.snapshot()
}

// Run generates traces and exports them via exporter.
// Topology (span names, depth, parent-child) is deterministic from the
// config seed, while trace/span IDs use a fresh random seed per run so
// each execution produces globally unique identifiers.
// When Config.Rate > 0, trace starts are paced to approximately that many
// traces per second. The actual rate may be lower if generation + export
// takes longer than the tick interval.
func (g *Generator) Run(ctx context.Context, exp export.TraceExporter) error {
	if err := ValidateConfig(g.config); err != nil {
		return err
	}
	// Explicit flat evidence grammar: Workload.Shape=flat or Scenario=workload.
	// WorkloadProfile knobs alone still compose onto topology/tree scenarios.
	if g.config.Workload.flatShape() || IsWorkloadScenario(g.config.Scenario) {
		return g.runWorkloadProfile(ctx, exp)
	}

	if g.config.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.config.Duration)
		defer cancel()
	}

	wl := g.config.Workload
	useWorkloadPacing := wl.pacingActive()

	var ticker *time.Ticker
	if !useWorkloadPacing && g.config.Rate > 0 {
		interval := time.Second / time.Duration(g.config.Rate)
		if interval == 0 {
			interval = time.Nanosecond
		}
		ticker = time.NewTicker(interval)
		defer ticker.Stop()
	}

	traceCount := g.config.TraceCount
	if traceCount == 0 && !useWorkloadPacing && g.config.Rate > 0 && g.config.Duration > 0 {
		traceCount = int(float64(g.config.Rate) * g.config.Duration.Seconds())
		if traceCount == 0 {
			traceCount = 1
		}
	}

	var exportSeq uint64
	pacer := workloadPacer{profile: wl}
	if useWorkloadPacing {
		pacer.fallbackRate = g.config.Rate
	}
	for i := 0; traceCount == 0 || i < traceCount; i++ {
		if !useWorkloadPacing && i > 0 && ticker != nil {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					return nil
				}
				return ctx.Err()
			}
		}
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return nil
			}
			return ctx.Err()
		default:
		}

		topoRng := NewSeededRNG(g.config.Seed + int64(i))
		idRng := NewIDRng()

		traceID := idRng.TraceID()

		spanCount := g.config.SpanCount
		if g.config.Scenario != "" && g.config.ScenarioSpanCount > 0 {
			spanCount = g.config.ScenarioSpanCount
		}

		var spansByService map[string][]*tracev1.Span

		if g.config.Scenario != "" {
			scenarioRoots, err := getScenario(g.config.Scenario, topoRng, spanCount, g.config.MaxDepth)
			if err != nil {
				return err
			}
			spansByService = g.buildScenarioSpans(idRng, scenarioRoots, traceID, g.config.BaseTime)
		} else {
			topology := g.config.Topology
			root, err := GenerateTopologyWithErrors(topology, topoRng, spanCount, g.config.MaxDepth, g.config.ErrorPercent)
			if err != nil {
				return err
			}
			spans, _, _ := g.buildSpans(idRng, root, traceID, [8]byte{}, g.config.BaseTime)
			spansByService = map[string][]*tracev1.Span{g.config.ServiceName: spans}
		}

		endpoints := g.Regions
		if len(endpoints) == 0 {
			endpoints = region.All()
		}
		if len(endpoints) == 0 {
			continue
		}
		if wl.SingleResource {
			endpoints = endpoints[:1]
		}

		type epSvcKey struct {
			ep  region.Name
			svc string
		}

		svcNames := make([]string, 0, len(spansByService))
		for svcName := range spansByService {
			svcNames = append(svcNames, svcName)
		}
		sort.Strings(svcNames)

		groups := make(map[epSvcKey][]*tracev1.Span)
		for _, svcName := range svcNames {
			for _, sp := range spansByService[svcName] {
				var ep region.Name
				if wl.SingleResource {
					ep = endpoints[0]
				} else {
					ep = endpoints[topoRng.Int63n(int64(len(endpoints)))]
				}
				key := epSvcKey{ep, svcName}
				groups[key] = append(groups[key], sp)
			}
		}

		var refs []SpanRef
		if g.OnTraceSpans != nil {
			total := 0
			for _, spans := range spansByService {
				total += len(spans)
			}
			refs = make([]SpanRef, 0, total)
		}
		for _, ep := range endpoints {
			for _, svcName := range svcNames {
				key := epSvcKey{ep, svcName}
				groupSpans := groups[key]
				if len(groupSpans) == 0 {
					continue
				}
				rs := &tracev1.ResourceSpans{
					Resource: &resourcev1.Resource{
						Attributes: region.ResourceAttributes(ep, svcName),
					},
					ScopeSpans: []*tracev1.ScopeSpans{
						{
							Scope: &commonv1.InstrumentationScope{
								Name:    "o11y-workbench",
								Version: "0.1.0",
							},
							Spans: groupSpans,
						},
					},
					SchemaUrl: "https://opentelemetry.io/schemas/1.30.0",
				}
				exported := groupSpans
				if wl.Active() {
					var err error
					exported, err = g.exportSpanBatches(ctx, exp, rs, &exportSeq, &pacer)
					if err != nil {
						if err == context.DeadlineExceeded {
							return nil
						}
						return err
					}
				} else {
					ok, err := g.exportAccount(ctx, exp, []*tracev1.ResourceSpans{rs}, len(groupSpans))
					if err != nil {
						return err
					}
					if !ok {
						exported = nil
					}
				}
				if g.OnTraceSpans != nil {
					refs = appendSpanRefs(refs, svcName, exported)
				}
			}
		}
		if len(refs) > 0 {
			g.OnTraceSpans(refs)
		}
		if g.OnProgress != nil {
			g.OnProgress(i+1, g.config.TraceCount)
		}

	}

	return nil
}

func appendSpanRefs(refs []SpanRef, service string, spans []*tracev1.Span) []SpanRef {
	for _, sp := range spans {
		ref := SpanRef{Service: service}
		copy(ref.TraceID[:], sp.TraceId)
		copy(ref.SpanID[:], sp.SpanId)
		refs = append(refs, ref)
	}
	return refs
}

// buildSpans recursively converts a SpanNode tree into OTLP Span objects
// using a bottom-up (post-order) approach. Leaf spans are assigned random
// start times and durations within the trace window. Each parent span
// naturally encapsulates its children — its start is the earliest child
// start and its end is the latest child end. Returns the flattened span
// list together with the earliest start and latest end time in this subtree.
func (g *Generator) buildSpans(rng *SeededRNG, node *SpanNode, traceID [16]byte, parentSpanID [8]byte, traceStart time.Time) ([]*tracev1.Span, time.Time, time.Time) {
	if node == nil {
		return nil, time.Time{}, time.Time{}
	}

	spanID := rng.SpanID()

	// --- Process children first (post-order) ---
	var (
		childList     []*tracev1.Span
		earliestStart time.Time
		latestEnd     time.Time
	)

	for _, child := range node.Children {
		spans, cs, ce := g.buildSpans(rng, child, traceID, spanID, traceStart)
		childList = append(childList, spans...)
		if earliestStart.IsZero() || cs.Before(earliestStart) {
			earliestStart = cs
		}
		if ce.After(latestEnd) {
			latestEnd = ce
		}
	}

	// --- Determine this span's own timestamps ---
	var startTime, endTime time.Time

	if len(node.Children) == 0 {
		// Leaf span: place it at a random point within the trace window.
		offset := rng.Duration(g.config.TraceDuration / 2)
		startTime = traceStart.Add(offset)
		spanDuration := rng.Duration(g.config.TraceDuration / 2)
		if spanDuration < time.Millisecond {
			spanDuration = time.Millisecond
		}
		endTime = startTime.Add(spanDuration)
	} else {
		// Parent span: encapsulate all children.
		startTime = earliestStart
		endTime = latestEnd
	}

	// --- Build the OTLP span ---
	attrs := []*commonv1.KeyValue{
		stringAttr("span.depth", fmt.Sprintf("%d", node.Depth)),
	}

	var parentSpanIDBytes []byte
	if parentSpanID != [8]byte{} {
		parentSpanIDBytes = parentSpanID[:]
	}

	otlpSpan := &tracev1.Span{
		TraceId:           traceID[:],
		SpanId:            spanID[:],
		ParentSpanId:      parentSpanIDBytes,
		Name:              node.Name,
		Kind:              tracev1.Span_SPAN_KIND_INTERNAL,
		Attributes:        attrs,
		StartTimeUnixNano: uint64(startTime.UnixNano()),
		EndTimeUnixNano:   uint64(endTime.UnixNano()),
		Flags:             1, // sampled
	}

	if node.IsError {
		otlpSpan.Status = &tracev1.Status{
			Code:    tracev1.Status_STATUS_CODE_ERROR,
			Message: "simulated error",
		}
	} else {
		otlpSpan.Status = &tracev1.Status{
			Code: tracev1.Status_STATUS_CODE_OK,
		}
	}

	result := make([]*tracev1.Span, 0, 1+len(childList))
	result = append(result, otlpSpan)
	result = append(result, childList...)

	return result, startTime, endTime
}

func (g *Generator) buildScenarioSpans(rng *SeededRNG, roots []*ScenarioSpanNode, traceID [16]byte, traceStart time.Time) map[string][]*tracev1.Span {
	byService := make(map[string][]*tracev1.Span)
	for i, root := range roots {
		start := traceStart
		if i > 0 {
			start = traceStart.Add(g.config.TraceDuration / 2)
		}
		g.buildScenarioSubtree(rng, root, traceID, [8]byte{}, start, byService)
	}
	return byService
}

func (g *Generator) buildScenarioSubtree(rng *SeededRNG, node *ScenarioSpanNode, traceID [16]byte, parentSpanID [8]byte, traceStart time.Time, byService map[string][]*tracev1.Span) (time.Time, time.Time) {
	if node == nil {
		return time.Time{}, time.Time{}
	}

	spanID := rng.SpanID()

	var (
		earliestStart time.Time
		latestEnd     time.Time
		nextStart     = traceStart
	)

	for _, child := range node.Children {
		cs, ce := g.buildScenarioSubtree(rng, child, traceID, spanID, nextStart, byService)
		if earliestStart.IsZero() || cs.Before(earliestStart) {
			earliestStart = cs
		}
		if ce.After(latestEnd) {
			latestEnd = ce
		}
		if !ce.IsZero() {
			nextStart = ce
		}
	}

	var startTime, endTime time.Time

	if len(node.Children) == 0 {
		dur := g.config.TraceDuration / 2
		if node.MaxDuration > 0 {
			dur = rng.DurationRange(node.MinDuration, node.MaxDuration)
		}
		if dur < time.Millisecond {
			dur = time.Millisecond
		}
		jitter := dur / 10
		if jitter > 5*time.Millisecond {
			jitter = 5 * time.Millisecond
		}
		if jitter < time.Microsecond {
			jitter = time.Microsecond
		}
		startTime = traceStart.Add(rng.Duration(jitter))
		endTime = startTime.Add(dur)
	} else {
		startTime = earliestStart
		endTime = latestEnd
	}

	kind := node.Kind
	if kind == tracev1.Span_SPAN_KIND_UNSPECIFIED {
		kind = tracev1.Span_SPAN_KIND_INTERNAL
	}

	attrs := make([]*commonv1.KeyValue, 0, 1+len(node.Attributes))
	attrs = append(attrs, node.Attributes...)

	var parentSpanIDBytes []byte
	if parentSpanID != [8]byte{} {
		parentSpanIDBytes = parentSpanID[:]
	}

	otlpSpan := &tracev1.Span{
		TraceId:           traceID[:],
		SpanId:            spanID[:],
		ParentSpanId:      parentSpanIDBytes,
		Name:              node.Name,
		Kind:              kind,
		Attributes:        attrs,
		StartTimeUnixNano: uint64(startTime.UnixNano()),
		EndTimeUnixNano:   uint64(endTime.UnixNano()),
		Flags:             1,
	}

	if node.IsError {
		otlpSpan.Status = &tracev1.Status{
			Code:    tracev1.Status_STATUS_CODE_ERROR,
			Message: "simulated error",
		}
	} else {
		otlpSpan.Status = &tracev1.Status{
			Code: tracev1.Status_STATUS_CODE_OK,
		}
	}

	byService[node.ServiceName] = append(byService[node.ServiceName], otlpSpan)
	return startTime, endTime
}

func stringAttr(key, value string) *commonv1.KeyValue {
	return &commonv1.KeyValue{
		Key: key,
		Value: &commonv1.AnyValue{
			Value: &commonv1.AnyValue_StringValue{
				StringValue: value,
			},
		},
	}
}
