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
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

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
}

// NewLogGenerator creates a LogGenerator with the given config.
func NewLogGenerator(config LogConfig) *LogGenerator {
	return &LogGenerator{Config: config}
}

type tsPair struct {
	traceID [16]byte
	spanID  [8]byte
	svcName string
}

// Run generates log records and exports them via exp.
// When traceSpans is non-nil the log generator creates logs_per_span log records
// per span, with trace/span IDs correlated.  When traceSpans is nil the generator
// produces LogCount independent log records.
func (g *LogGenerator) Run(ctx context.Context, exp export.LogExporter, traceSpans []*tracev1.ResourceSpans) error {
	logging.Debugf("LogGen: Starting. len(traceSpans)=%d, LogsPerSpan=%d, LogCount=%d",
		len(traceSpans), g.Config.LogsPerSpan, g.Config.LogCount)

	tmpl, err := template.New("body").Parse(g.Config.BodyTemplate)
	if err != nil {
		return fmt.Errorf("parse body template: %w", err)
	}

	// Collect trace/span pairs and determine total log count.
	var pairs []tsPair
	totalLogs := 0

	if len(traceSpans) > 0 {
		for _, rs := range traceSpans {
			svcName := "unknown"
			if rs.Resource != nil {
				for _, attr := range rs.Resource.Attributes {
					if attr.Key == "service.name" {
						if s, ok := attr.Value.Value.(*commonv1.AnyValue_StringValue); ok {
							svcName = s.StringValue
						}
					}
				}
			}
			for _, ss := range rs.ScopeSpans {
				for _, sp := range ss.Spans {
					var p tsPair
					copy(p.traceID[:], sp.TraceId)
					copy(p.spanID[:], sp.SpanId)
					p.svcName = svcName
					pairs = append(pairs, p)
				}
			}
		}
		totalLogs = len(pairs) * g.Config.LogsPerSpan
		logging.Debugf("LogGen: Extracted %d trace/span pairs, totalLogs=%d", len(pairs), totalLogs)
	} else {
		totalLogs = g.Config.LogCount
		logging.Debugf("LogGen: No trace correlation, totalLogs=%d (from LogCount)", totalLogs)
	}

	if totalLogs == 0 {
		logging.Debugf("LogGen: EARLY RETURN - totalLogs is 0")
		logging.Warnf("LogGen: totalLogs is 0 (%d traceSpans, %d pairs, LogsPerSpan=%d, LogCount=%d)",
			len(traceSpans), len(pairs), g.Config.LogsPerSpan, g.Config.LogCount)
		return nil
	}

	logging.Debugf("LogGen: Proceeding to generate %d logs", totalLogs)

	// Use a deterministic seed for reproducible severity distributions but
	// fresh randomness for timestamps and IDs.
	rng := rand.New(rand.NewSource(int64(g.Config.SeverityDist.Info + g.Config.SeverityDist.Warn + g.Config.SeverityDist.Error + g.Config.SeverityDist.Fatal)))
	idRng := NewIDRng()

	logsGenerated := 0
	exportBatch := make([]*logsv1.LogRecord, 0, 100)

	endpoints := region.All()
	if len(endpoints) == 0 {
		return fmt.Errorf("no endpoints configured")
	}

	flush := func() error {
		if len(exportBatch) == 0 {
			return nil
		}
		ep := endpoints[0] // use the first endpoint resource for all logs
		svcName := "mock-service"
		if len(pairs) > 0 {
			svcName = pairs[0].svcName
		}

		sl := &logsv1.ScopeLogs{
			Scope: &commonv1.InstrumentationScope{
				Name:    "o11y-workbench",
				Version: "0.1.0",
			},
			LogRecords: exportBatch,
		}
		rl := &logsv1.ResourceLogs{
			Resource: &resourcev1.Resource{
				Attributes: region.ResourceAttributes(ep, svcName),
			},
			ScopeLogs: []*logsv1.ScopeLogs{sl},
			SchemaUrl: "https://opentelemetry.io/schemas/1.30.0",
		}
		if err := exp.Export(ctx, []*logsv1.ResourceLogs{rl}); err != nil {
			return err
		}
		exportBatch = make([]*logsv1.LogRecord, 0, 100)
		return nil
	}

	for logsGenerated < totalLogs {
		select {
		case <-ctx.Done():
			flush()
			return ctx.Err()
		default:
		}

		severNum, severText := pickSeverity(g.Config.SeverityDist, rng)

		now := time.Now()
		msg := defaultLogMessages[rng.Intn(len(defaultLogMessages))]

		vars := LogTemplateVars{
			Message:     msg,
			ServiceName: "mock-service",
			Severity:    severText,
			Component:   "workbench",
		}

		var traceID [16]byte
		var spanID [8]byte
		hasTrace := false

		if len(pairs) > 0 && g.Config.LogsPerSpan > 0 {
			pairIdx := logsGenerated / g.Config.LogsPerSpan
			if pairIdx < len(pairs) {
				traceID = pairs[pairIdx].traceID
				spanID = pairs[pairIdx].spanID
				vars.ServiceName = pairs[pairIdx].svcName
				hasTrace = true
			}
		}

		if hasTrace {
			vars.TraceID = fmt.Sprintf("%016x%016x", binary.BigEndian.Uint64(traceID[:8]), binary.BigEndian.Uint64(traceID[8:]))
			vars.SpanID = fmt.Sprintf("%016x", binary.BigEndian.Uint64(spanID[:]))
		} else {
			// Independent logs still get a unique trace/span pair to avoid empty IDs.
			traceID = idRng.TraceID()
			spanID = idRng.SpanID()
			vars.TraceID = fmt.Sprintf("%016x%016x", binary.BigEndian.Uint64(traceID[:8]), binary.BigEndian.Uint64(traceID[8:]))
			vars.SpanID = fmt.Sprintf("%016x", binary.BigEndian.Uint64(spanID[:]))
		}

		// Render body template.
		var bodyBuf strings.Builder
		if err := tmpl.Execute(&bodyBuf, vars); err != nil {
			return fmt.Errorf("render body template: %w", err)
		}

		attrs := make([]*commonv1.KeyValue, 0, len(g.Config.Attributes)+2)
		for _, attr := range g.Config.Attributes {
			attrs = append(attrs, &commonv1.KeyValue{
				Key: attr.Key,
				Value: &commonv1.AnyValue{
					Value: &commonv1.AnyValue_StringValue{StringValue: attr.Value},
				},
			})
		}
		// Add the message as an attribute too so it's queryable.
		attrs = append(attrs, &commonv1.KeyValue{
			Key: "log.message",
			Value: &commonv1.AnyValue{
				Value: &commonv1.AnyValue_StringValue{StringValue: msg},
			},
		})

		record := &logsv1.LogRecord{
			TimeUnixNano:         uint64(now.UnixNano()),
			ObservedTimeUnixNano: uint64(now.UnixNano()),
			SeverityNumber:       severNum,
			SeverityText:         severText,
			Body: &commonv1.AnyValue{
				Value: &commonv1.AnyValue_StringValue{StringValue: bodyBuf.String()},
			},
			Attributes: attrs,
			TraceId:    traceID[:],
			SpanId:     spanID[:],
		}

		exportBatch = append(exportBatch, record)

		logsGenerated++
		if g.OnProgress != nil {
			g.OnProgress(logsGenerated, totalLogs)
		}

		// Flush every 100 records to avoid huge batches.
		if len(exportBatch) >= 100 {
			if err := flush(); err != nil {
				return err
			}
		}
	}

	return flush()
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
