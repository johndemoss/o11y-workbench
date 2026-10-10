package gen

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"

	"github.com/johndemoss/o11y-workbench/internal/export"
)

// RunLogs generates paced flat OTLP logs.
// Mirrors runWorkloadProfile ledger semantics (items = log records).
func (g *Generator) RunLogs(ctx context.Context, exp export.LogExporter) error {
	if g == nil {
		return fmt.Errorf("generator is nil")
	}
	return g.runLogWorkloadProfile(ctx, exp)
}

// runLogWorkloadProfile is the explicit Shape=="flat" helper for logs:
// paced OTLP log records with one ResourceLogs per export request.
func (g *Generator) runLogWorkloadProfile(ctx context.Context, exp export.LogExporter) error {
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
		// Reuse TraceCount as request budget for flat evidence workloads.
		requestBudget = g.config.TraceCount
	} else if g.config.LogConfig.LogCount > 0 && !p.pacingActive() && g.config.Duration == 0 {
		requestBudget = (g.config.LogConfig.LogCount + items - 1) / items
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
		rl := g.workloadResourceLogs(expID, svc, seq, items)
		if err := g.exportAccountLogs(ctx, exp, []*logsv1.ResourceLogs{rl}, items); err != nil {
			return err
		}

		if g.OnProgress != nil {
			g.OnProgress(int(seq), requestBudget)
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

func (g *Generator) workloadResourceLogs(expID, svc string, seq uint64, items int) *logsv1.ResourceLogs {
	records := make([]*logsv1.LogRecord, 0, items)
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

		records = append(records, &logsv1.LogRecord{
			TimeUnixNano:         now,
			ObservedTimeUnixNano: now,
			SeverityNumber:       logsv1.SeverityNumber_SEVERITY_NUMBER_INFO,
			SeverityText:         "INFO",
			Body: &commonv1.AnyValue{
				Value: &commonv1.AnyValue_StringValue{StringValue: "workload.log"},
			},
			Attributes: attrs,
			TraceId:    tid,
			SpanId:     sid,
		})
	}
	return &logsv1.ResourceLogs{
		Resource: &resourcev1.Resource{
			Attributes: []*commonv1.KeyValue{stringAttr("service.name", svc)},
		},
		ScopeLogs: []*logsv1.ScopeLogs{{
			Scope:      &commonv1.InstrumentationScope{Name: "o11y-workbench", Version: "0.1.0"},
			LogRecords: records,
		}},
		SchemaUrl: "https://opentelemetry.io/schemas/1.30.0",
	}
}
