package gen

import (
	"context"
	"fmt"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/johndemoss/mocktracegen/internal/export"
	"github.com/johndemoss/mocktracegen/internal/region"
)

type Config struct {
	Seed          int64
	TraceCount    int
	Rate          int // target traces/sec; 0 = unlimited
	Topology      TopologyType
	SpanCount     int
	MaxDepth      int
	ServiceName   string
	BaseTime      time.Time
	TraceDuration time.Duration
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
	config Config
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

	for i := 0; i < g.config.TraceCount; i++ {
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

		topology := g.config.Topology
		root := GenerateTopology(topology, topoRng, g.config.SpanCount, g.config.MaxDepth)

		traceID := idRng.TraceID()

		spans, _, _ := g.buildSpans(idRng, root, traceID, [8]byte{}, g.config.BaseTime)

		endpoints := region.All()
		if len(endpoints) == 0 {
			continue
		}

		// Distribute spans across all endpoints so the trace is scattered.
		groups := make(map[region.Name][]*tracev1.Span, len(endpoints))
		for _, span := range spans {
			ep := endpoints[topoRng.Int63n(int64(len(endpoints)))]
			groups[ep] = append(groups[ep], span)
		}

		// Iterate in region.All() order so output is deterministic.
		for _, ep := range endpoints {
			groupSpans := groups[ep]
			rs := &tracev1.ResourceSpans{
				Resource: &resourcev1.Resource{
					Attributes: region.ResourceAttributes(ep, g.config.ServiceName),
				},
				ScopeSpans: []*tracev1.ScopeSpans{
					{
						Scope: &commonv1.InstrumentationScope{
							Name:    "mocktracegen",
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
