package gen

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"strings"
	"text/template"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"

	"github.com/johndemoss/o11y-workbench/internal/export"
	"github.com/johndemoss/o11y-workbench/internal/logging"
	"github.com/johndemoss/o11y-workbench/internal/region"
)

// SeverityDistribution holds percentage weights for each severity level.
// Values should sum to 100 for a meaningful distribution.
type SeverityDistribution struct {
	Info  int `json:"info"`
	Warn  int `json:"warn"`
	Error int `json:"error"`
	Fatal int `json:"fatal"`
}

// LogAttribute is a key-value pair attached to each log record.
type LogAttribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// LogConfig controls log generation behaviour.
type LogConfig struct {
	Enabled      bool                 `json:"enabled"`
	LogsPerSpan  int                  `json:"logs_per_span"`
	LogCount     int                  `json:"log_count"`
	SeverityDist SeverityDistribution `json:"severity_dist"`
	BodyTemplate string               `json:"body_template"`
	Attributes   []LogAttribute       `json:"attributes"`
}

// DefaultLogConfig returns sensible defaults for log generation.
func DefaultLogConfig() LogConfig {
	return LogConfig{
		Enabled:     false,
		LogsPerSpan: 3,
		LogCount:    10,
		SeverityDist: SeverityDistribution{
			Info:  60,
			Warn:  25,
			Error: 12,
			Fatal: 3,
		},
		BodyTemplate: `{"message":"{{.Message}}","service":"{{.ServiceName}}","severity":"{{.Severity}}","component":"{{.Component}}"}`,
	}
}

// LogTemplateVars is the data available to the log body Go template.
type LogTemplateVars struct {
	Message     string
	ServiceName string
	Severity    string
	TraceID     string
	SpanID      string
	Component   string
	DurationMs  float64
}

var defaultLogMessages = []string{
	"Request processed successfully",
	"Connection established",
	"Cache hit for key",
	"Health check passed",
	"Configuration reloaded",
	"User session created",
	"Background job completed",
	"Rate limit approaching",
	"Response time exceeded threshold",
	"Resource utilization spike detected",
	"Authentication challenge issued",
	"Data replication lag detected",
	"Connection pool exhausted",
	"Memory allocation warning",
	"Garbage collection cycle completed",
	"DNS resolution failed, retrying",
	"TLS handshake initiated",
	"Payload validation error",
	"Query execution plan updated",
	"Circuit breaker opened",
}

// LogGenerator produces OTLP log records.
type LogGenerator struct {
	Config     LogConfig
	OnProgress func(completed, total int)
	// Regions is immutable while Run is active; nil uses CLI process defaults.
	Regions []region.Name
}

// NewLogGenerator creates a LogGenerator with the given config.
func NewLogGenerator(config LogConfig) *LogGenerator {
	return &LogGenerator{Config: config}
}

// logEmitter carries per-run batching and RNG state for log generation.
type logEmitter struct {
	gen       *LogGenerator
	tmpl      *template.Template
	rng       *rand.Rand
	idRng     *SeededRNG
	endpoints []region.Name
	batch     []*logsv1.LogRecord
	service   string
	completed int
}

func newLogEmitter(g *LogGenerator) (*logEmitter, error) {
	tmpl, err := template.New("body").Parse(g.Config.BodyTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse body template: %w", err)
	}
	endpoints := g.Regions
	if len(endpoints) == 0 {
		endpoints = region.All()
	}
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints configured")
	}
	// Deterministic severity distribution, fresh timestamps and IDs.
	seed := int64(g.Config.SeverityDist.Info + g.Config.SeverityDist.Warn + g.Config.SeverityDist.Error + g.Config.SeverityDist.Fatal)
	return &logEmitter{
		gen:       g,
		tmpl:      tmpl,
		rng:       rand.New(rand.NewSource(seed)),
		idRng:     NewIDRng(),
		endpoints: endpoints,
		batch:     make([]*logsv1.LogRecord, 0, 100),
	}, nil
}

func (e *logEmitter) flush(ctx context.Context, exp export.LogExporter) error {
	if len(e.batch) == 0 {
		return nil
	}
	ep := e.endpoints[0] // use the first endpoint resource for all logs
	sl := &logsv1.ScopeLogs{
		Scope: &commonv1.InstrumentationScope{
			Name:    "o11y-workbench",
			Version: "0.1.0",
		},
		LogRecords: e.batch,
	}
	rl := &logsv1.ResourceLogs{
		Resource: &resourcev1.Resource{
			Attributes: region.ResourceAttributes(ep, e.service),
		},
		ScopeLogs: []*logsv1.ScopeLogs{sl},
		SchemaUrl: "https://opentelemetry.io/schemas/1.30.0",
	}
	if err := exp.Export(ctx, []*logsv1.ResourceLogs{rl}); err != nil {
		return err
	}
	e.batch = make([]*logsv1.LogRecord, 0, 100)
	return nil
}

// newRecord builds one log record. A nil ref produces an independent record with
// fresh trace/span IDs; a ref (and its service) produces a correlated record.
func (e *logEmitter) newRecord(ref *SpanRef) (*logsv1.LogRecord, string, error) {
	severNum, severText := pickSeverity(e.gen.Config.SeverityDist, e.rng)
	msg := defaultLogMessages[e.rng.Intn(len(defaultLogMessages))]

	vars := LogTemplateVars{
		Message:     msg,
		ServiceName: "mock-service",
		Severity:    severText,
		Component:   "workbench",
	}

	var traceID [16]byte
	var spanID [8]byte
	if ref != nil {
		traceID = ref.TraceID
		spanID = ref.SpanID
		if ref.Service != "" {
			vars.ServiceName = ref.Service
		}
	} else {
		traceID = e.idRng.TraceID()
		spanID = e.idRng.SpanID()
	}
	vars.TraceID = fmt.Sprintf("%016x%016x", binary.BigEndian.Uint64(traceID[:8]), binary.BigEndian.Uint64(traceID[8:]))
	vars.SpanID = fmt.Sprintf("%016x", binary.BigEndian.Uint64(spanID[:]))

	var bodyBuf strings.Builder
	if err := e.tmpl.Execute(&bodyBuf, vars); err != nil {
		return nil, "", fmt.Errorf("render body template: %w", err)
	}

	attrs := make([]*commonv1.KeyValue, 0, len(e.gen.Config.Attributes)+2)
	for _, attr := range e.gen.Config.Attributes {
		attrs = append(attrs, &commonv1.KeyValue{
			Key:   attr.Key,
			Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: attr.Value}},
		})
	}
	// Add the message as an attribute too so it's queryable.
	attrs = append(attrs, &commonv1.KeyValue{
		Key:   "log.message",
		Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: msg}},
	})

	now := time.Now()
	record := &logsv1.LogRecord{
		TimeUnixNano:         uint64(now.UnixNano()),
		ObservedTimeUnixNano: uint64(now.UnixNano()),
		SeverityNumber:       severNum,
		SeverityText:         severText,
		Body:                 &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: bodyBuf.String()}},
		Attributes:           attrs,
		TraceId:              traceID[:],
		SpanId:               spanID[:],
	}
	return record, vars.ServiceName, nil
}

func (e *logEmitter) appendRecord(ctx context.Context, exp export.LogExporter, ref *SpanRef, progressTotal int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	record, service, err := e.newRecord(ref)
	if err != nil {
		return err
	}
	if len(e.batch) > 0 && e.service != service {
		if err := e.flush(ctx, exp); err != nil {
			return err
		}
	}
	e.service = service
	e.batch = append(e.batch, record)
	e.completed++
	if e.gen.OnProgress != nil {
		e.gen.OnProgress(e.completed, progressTotal)
	}
	if len(e.batch) >= 100 {
		return e.flush(ctx, exp)
	}
	return nil
}

// Run generates LogCount independent log records and exports them via exp.
func (g *LogGenerator) Run(ctx context.Context, exp export.LogExporter) error {
	logging.Debugf("LogGen: Starting uncorrelated. LogCount=%d", g.Config.LogCount)
	total := g.Config.LogCount
	if total <= 0 {
		return nil
	}
	em, err := newLogEmitter(g)
	if err != nil {
		return err
	}
	for range total {
		if err := em.appendRecord(ctx, exp, nil, total); err != nil {
			_ = em.flush(context.Background(), exp)
			return err
		}
	}
	return em.flush(ctx, exp)
}

// RunCorrelated generates LogsPerSpan records per streamed span reference,
// grouping records under the referenced span's service resource. It consumes
// until refs closes or ctx ends. The reported total grows as references arrive;
// blocking in this consumer backpressures the trace producer.
func (g *LogGenerator) RunCorrelated(ctx context.Context, refs <-chan []SpanRef, exp export.LogExporter) error {
	if g.Config.LogsPerSpan <= 0 {
		return fmt.Errorf("log correlation requires logs_per_span > 0")
	}
	em, err := newLogEmitter(g)
	if err != nil {
		return err
	}
	total := 0
	for {
		select {
		case <-ctx.Done():
			_ = em.flush(context.Background(), exp)
			return ctx.Err()
		case batch, ok := <-refs:
			if !ok {
				return em.flush(ctx, exp)
			}
			for i := range batch {
				for range g.Config.LogsPerSpan {
					total++
					if err := em.appendRecord(ctx, exp, &batch[i], total); err != nil {
						_ = em.flush(context.Background(), exp)
						return err
					}
				}
			}
		}
	}
}

// pickSeverity returns (SeverityNumber, SeverityText) chosen by weighted random
// sampling from the distribution.
func pickSeverity(dist SeverityDistribution, rng *rand.Rand) (logsv1.SeverityNumber, string) {
	total := dist.Info + dist.Warn + dist.Error + dist.Fatal
	if total == 0 {
		// No distribution specified — use the default distribution.
		def := DefaultLogConfig().SeverityDist
		return pickSeverity(def, rng)
	}
	roll := rng.Intn(total)
	cutoff := 0

	cutoff += dist.Info
	if roll < cutoff {
		return logsv1.SeverityNumber_SEVERITY_NUMBER_INFO, "INFO"
	}
	cutoff += dist.Warn
	if roll < cutoff {
		return logsv1.SeverityNumber_SEVERITY_NUMBER_WARN, "WARN"
	}
	cutoff += dist.Error
	if roll < cutoff {
		return logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR, "ERROR"
	}
	return logsv1.SeverityNumber_SEVERITY_NUMBER_FATAL, "FATAL"
}
