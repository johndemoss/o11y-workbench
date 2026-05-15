package gen

import (
	"context"
	"fmt"
	"sort"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/johndemoss/otel-workbench/internal/export"
	"github.com/johndemoss/otel-workbench/internal/region"
)

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
	Scenario          string // named scenario; "" means use Topology
	ScenarioSpanCount int    // per-service span budget for scenarios; 0 means use SpanCount
	SignalType        string       `json:"signal_type,omitempty"` // "traces" (default), "logs", or "both"
	LogConfig         LogConfig    `json:"log_config,omitempty"`
	MetricConfig      MetricConfig `json:"metric_config,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		Seed:          42,
		TraceCount:    1,
		Topology:      TopologyFlat,
		SpanCount:     10,
		MaxDepth:      5,
		ServiceName:   "mock-service",
		BaseTime:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		TraceDuration: 5 * time.Second,
	}
}

type Generator struct {
	config     Config
	OnProgress func(completed, total int)
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

// Run generates traces and exports them via exporter.
// Topology (span names, depth, parent-child) is deterministic from the
// config seed, while trace/span IDs use a fresh random seed per run so
// each execution produces globally unique identifiers.
// When Config.Rate > 0, trace starts are paced to approximately that many
// traces per second. The actual rate may be lower if generation + export
// takes longer than the tick interval.
func (g *Generator) Run(ctx context.Context, exp export.Exporter) error {
	var ticker *time.Ticker
	if g.config.Rate > 0 {
		interval := time.Second / time.Duration(g.config.Rate)
		if interval == 0 {
			interval = time.Nanosecond
		}
		ticker = time.NewTicker(interval)
		defer ticker.Stop()
	}

	for i := 0; g.config.TraceCount == 0 || i < g.config.TraceCount; i++ {
		if ticker != nil {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		select {
		case <-ctx.Done():
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
			root := GenerateTopology(topology, topoRng, spanCount, g.config.MaxDepth)
			spans, _, _ := g.buildSpans(idRng, root, traceID, [8]byte{}, g.config.BaseTime)
			spansByService = map[string][]*tracev1.Span{g.config.ServiceName: spans}
		}

		endpoints := region.All()
		if len(endpoints) == 0 {
			continue
		}

		// Distribute spans by (endpoint, serviceName) so each ResourceSpans
		// gets the correct resource-level service.name.
		type epSvcKey struct {
			ep  region.Name
			svc string
		}

		// Collect deterministic service name ordering before distribution
		// (map iteration is non-deterministic and would break the RNG sequence).
		svcNames := make([]string, 0, len(spansByService))
		for svcName := range spansByService {
			svcNames = append(svcNames, svcName)
		}
		sort.Strings(svcNames)

		groups := make(map[epSvcKey][]*tracev1.Span)
		for _, svcName := range svcNames {
			for _, sp := range spansByService[svcName] {
				ep := endpoints[topoRng.Int63n(int64(len(endpoints)))]
				key := epSvcKey{ep, svcName}
				groups[key] = append(groups[key], sp)
			}
		}

		// Iterate in region.All() × sorted service name order.
		for _, ep := range endpoints {
			for _, svcName := range svcNames {
				key := epSvcKey{ep, svcName}
				if groupSpans := groups[key]; len(groupSpans) > 0 {
					rs := &tracev1.ResourceSpans{
						Resource: &resourcev1.Resource{
							Attributes: region.ResourceAttributes(ep, svcName),
						},
						ScopeSpans: []*tracev1.ScopeSpans{
							{
								Scope: &commonv1.InstrumentationScope{
									Name:    "otel-workbench",
									Version: "0.1.0",
								},
								Spans: groupSpans,
							},
						},
						SchemaUrl: "https://opentelemetry.io/schemas/1.30.0",
					}

					if err := exp.Export(ctx, []*tracev1.ResourceSpans{rs}); err != nil {
						return err
					}
				}
			}
		}
		if g.OnProgress != nil {
			g.OnProgress(i+1, g.config.TraceCount)
		}
	}

	return nil
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
		childList    []*tracev1.Span
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
		TraceId:                traceID[:],
		SpanId:                 spanID[:],
		ParentSpanId:           parentSpanIDBytes,
		Name:                   node.Name,
		Kind:                   tracev1.Span_SPAN_KIND_INTERNAL,
		Attributes:             attrs,
		StartTimeUnixNano:      uint64(startTime.UnixNano()),
		EndTimeUnixNano:        uint64(endTime.UnixNano()),
		Flags:                  1, // sampled
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
