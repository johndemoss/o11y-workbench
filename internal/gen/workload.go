package gen

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/johndemoss/o11y-workbench/internal/export"
)

// BurstConfig schedules on/off peak ingress for evidence runs.
type BurstConfig struct {
	On                  time.Duration `json:"on,omitempty"`
	Off                 time.Duration `json:"off,omitempty"`
	PeakRateItemsPerSec float64       `json:"peak_rate_items_per_sec,omitempty"`
}

// WorkloadProfile selects evidence-grade synthetic ingress knobs.
// Zero value preserves existing topology/scenario generation behavior.
//
// One generator (internal/gen) owns synthetic ingress. Remote TAP remains
// the captured-data ingress path.
type WorkloadProfile struct {
	ExperimentID        string      `json:"experiment_id,omitempty"`
	ItemsPerRequest     int         `json:"items_per_request,omitempty"`
	AttributeCount      int         `json:"attribute_count,omitempty"`
	AttributeValueBytes int         `json:"attribute_value_bytes,omitempty"`
	Cardinality         int         `json:"cardinality,omitempty"`
	Mode                string      `json:"mode,omitempty"` // constant|burst; empty = constant
	Burst               BurstConfig `json:"burst,omitempty"`
	RateItemsPerSec     float64     `json:"rate_items_per_sec,omitempty"`
	RequestsPerSec      float64     `json:"requests_per_sec,omitempty"`
	ContinueOnExportErr bool        `json:"continue_on_export_err,omitempty"`
	// SingleResource disables region fan-out: all spans use the first endpoint.
	SingleResource bool `json:"single_resource,omitempty"`
	// Shape selects span grammar. Empty keeps Config topology/scenario.
	// "flat" uses the explicit workload.span helper.
	Shape string `json:"shape,omitempty"`
}

// Active reports whether any WorkloadProfile overlay knobs are set.
// Active alone must not change span grammar — only Shape=="flat" does.
func (p WorkloadProfile) Active() bool {
	if p.ExperimentID != "" || p.ItemsPerRequest > 0 || p.AttributeCount > 0 ||
		p.AttributeValueBytes > 0 || p.Cardinality > 0 || p.RateItemsPerSec > 0 ||
		p.RequestsPerSec > 0 || p.ContinueOnExportErr || p.SingleResource ||
		strings.EqualFold(p.Shape, "flat") {
		return true
	}
	return strings.EqualFold(p.Mode, "burst")
}

func (p WorkloadProfile) flatShape() bool {
	return strings.EqualFold(strings.TrimSpace(p.Shape), "flat")
}

func (p WorkloadProfile) pacingActive() bool {
	return p.RateItemsPerSec > 0 || p.RequestsPerSec > 0 || strings.EqualFold(p.Mode, "burst")
}

func (p WorkloadProfile) evidenceAttrsActive() bool {
	return p.ExperimentID != "" || p.AttributeCount > 0 || p.AttributeValueBytes > 0 || p.Cardinality > 0
}

func (p WorkloadProfile) itemsPerRequest() int {
	if p.ItemsPerRequest > 0 {
		return p.ItemsPerRequest
	}
	return 1
}

func (p WorkloadProfile) experimentID(seed int64) string {
	if p.ExperimentID != "" {
		return p.ExperimentID
	}
	return fmt.Sprintf("gen-%d", seed)
}

func (p WorkloadProfile) requestRate(now time.Time) float64 {
	items := float64(p.itemsPerRequest())
	if items <= 0 {
		items = 1
	}
	if strings.EqualFold(p.Mode, "burst") {
		cycle := p.Burst.On + p.Burst.Off
		peak := p.Burst.PeakRateItemsPerSec / items
		if cycle <= 0 {
			return peak
		}
		mod := time.Duration(now.UnixNano()) % cycle
		if mod < p.Burst.On {
			return peak
		}
		return 0
	}
	if p.RequestsPerSec > 0 {
		return p.RequestsPerSec
	}
	if p.RateItemsPerSec > 0 {
		return p.RateItemsPerSec / items
	}
	return 0
}

// runWorkloadProfile is the explicit Shape=="flat" helper: paced OTLP traces
// with one ResourceSpans per export request (no region fan-out).
// Topology and scenario use the shared Run path with overlays.
func (g *Generator) runWorkloadProfile(ctx context.Context, exp export.TraceExporter) error {
	p := g.config.Workload
	if g.config.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.config.Duration)
		defer cancel()
	}

	items := p.itemsPerRequest()
	expID := p.experimentID(g.config.Seed)
	svc := g.config.ServiceName
	if svc == "" {
		svc = "o11y-workbench-workload"
	}

	var seq uint64
	requestBudget := 0
	if g.config.TraceCount > 0 {
		requestBudget = g.config.TraceCount
	}

	for {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return nil
			}
			return ctx.Err()
		default:
		}

		if requestBudget > 0 && int(seq) >= requestBudget {
			return nil
		}

		rate := p.requestRate(time.Now())
		if rate <= 0 && g.config.Rate > 0 && !strings.EqualFold(p.Mode, "burst") {
			rate = float64(g.config.Rate)
		}
		if rate <= 0 {
			// Burst off-window or unset rate: wait briefly.
			select {
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					return nil
				}
				return ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
			continue
		}

		interval := time.Duration(float64(time.Second) / rate)
		if interval <= 0 {
			interval = time.Millisecond
		}

		seq++
		rs := g.workloadResourceSpans(expID, svc, seq, items)
		if err := g.exportAccount(ctx, exp, []*tracev1.ResourceSpans{rs}, items); err != nil {
			return err
		}

		if g.OnProgress != nil {
			total := requestBudget
			g.OnProgress(int(seq), total)
		}

		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return nil
			}
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func (g *Generator) workloadResourceSpans(expID, svc string, seq uint64, items int) *tracev1.ResourceSpans {
	spans := make([]*tracev1.Span, 0, items)
	now := uint64(time.Now().UnixNano())
	card := g.config.Workload.Cardinality
	if card <= 0 {
		card = 1
	}
	val := strings.Repeat("x", g.config.Workload.AttributeValueBytes)
	for i := 0; i < items; i++ {
		tid := make([]byte, 16)
		sid := make([]byte, 8)
		binary.BigEndian.PutUint64(tid[0:8], uint64(g.config.Seed))
		binary.BigEndian.PutUint64(tid[8:16], seq*1000+uint64(i))
		binary.BigEndian.PutUint64(sid, seq*1000+uint64(i)+1)

		attrs := []*commonv1.KeyValue{
			stringAttr("experiment.id", expID),
			stringAttr("payload.id", fmt.Sprintf("%s-%d-%d", expID, seq, i)),
			stringAttr("cardinality.key", fmt.Sprintf("c-%d", int(seq)%card)),
		}
		for a := 0; a < g.config.Workload.AttributeCount; a++ {
			attrs = append(attrs, stringAttr(fmt.Sprintf("attr.%d", a), val))
		}

		spans = append(spans, &tracev1.Span{
			TraceId:           tid,
			SpanId:            sid,
			Name:              "workload.span",
			Kind:              tracev1.Span_SPAN_KIND_INTERNAL,
			StartTimeUnixNano: now,
			EndTimeUnixNano:   now + 1000,
			Attributes:        attrs,
			Status:            &tracev1.Status{Code: tracev1.Status_STATUS_CODE_OK},
			Flags:             1,
		})
	}
	return &tracev1.ResourceSpans{
		Resource: &resourcev1.Resource{
			Attributes: []*commonv1.KeyValue{stringAttr("service.name", svc)},
		},
		ScopeSpans: []*tracev1.ScopeSpans{{
			Scope: &commonv1.InstrumentationScope{Name: "o11y-workbench", Version: "0.1.0"},
			Spans: spans,
		}},
		SchemaUrl: "https://opentelemetry.io/schemas/1.30.0",
	}
}

func (g *Generator) stampEvidenceAttrs(spans []*tracev1.Span, expID string, seq uint64) {
	p := g.config.Workload
	if !p.evidenceAttrsActive() {
		return
	}
	card := p.Cardinality
	if card <= 0 {
		card = 1
	}
	val := strings.Repeat("x", p.AttributeValueBytes)
	for i, sp := range spans {
		if sp == nil {
			continue
		}
		attrs := []*commonv1.KeyValue{
			stringAttr("experiment.id", expID),
			stringAttr("payload.id", fmt.Sprintf("%s-%d-%d", expID, seq, i)),
			stringAttr("cardinality.key", fmt.Sprintf("c-%d", int(seq)%card)),
		}
		for a := 0; a < p.AttributeCount; a++ {
			attrs = append(attrs, stringAttr(fmt.Sprintf("attr.%d", a), val))
		}
		sp.Attributes = append(attrs, sp.Attributes...)
	}
}

func (g *Generator) exportSpanBatches(ctx context.Context, exp export.TraceExporter, rs *tracev1.ResourceSpans, seq *uint64) error {
	if rs == nil {
		return nil
	}
	var spans []*tracev1.Span
	for _, ss := range rs.ScopeSpans {
		spans = append(spans, ss.Spans...)
	}
	if len(spans) == 0 {
		return nil
	}
	p := g.config.Workload
	batch := 0
	if p.ItemsPerRequest > 0 {
		batch = p.ItemsPerRequest
	}
	if batch <= 0 || batch >= len(spans) {
		return g.exportOne(ctx, exp, rs, spans, seq)
	}
	for start := 0; start < len(spans); start += batch {
		end := start + batch
		if end > len(spans) {
			end = len(spans)
		}
		chunk := spans[start:end]
		part := &tracev1.ResourceSpans{
			Resource:  rs.Resource,
			SchemaUrl: rs.SchemaUrl,
			ScopeSpans: []*tracev1.ScopeSpans{{
				Scope: &commonv1.InstrumentationScope{Name: "o11y-workbench", Version: "0.1.0"},
				Spans: chunk,
			}},
		}
		if err := g.exportOne(ctx, exp, part, chunk, seq); err != nil {
			return err
		}
	}
	return nil
}

func (g *Generator) exportOne(ctx context.Context, exp export.TraceExporter, rs *tracev1.ResourceSpans, spans []*tracev1.Span, seq *uint64) error {
	*seq++
	expID := g.config.Workload.experimentID(g.config.Seed)
	if g.config.Workload.evidenceAttrsActive() || g.config.Workload.ExperimentID != "" {
		g.stampEvidenceAttrs(spans, expID, *seq)
	}
	items := len(spans)
	return g.exportAccount(ctx, exp, []*tracev1.ResourceSpans{rs}, items)
}
