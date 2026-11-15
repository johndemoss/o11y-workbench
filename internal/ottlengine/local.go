package ottlengine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlmetric"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlresource"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlscope"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"
	xottlfuncs "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/xottl/ottlfuncs"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"

	"gopkg.in/yaml.v3"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/ottl"
)

// LocalEngine runs OTTL against the in-process collector train linked into this binary.
type LocalEngine struct{}

func (e *LocalEngine) Info() VersionInfo {
	return VersionInfo{
		Contrib: DefaultContribVersion,
		Stable:  DefaultStableVersion,
		Source:  "in-process",
	}
}

func (e *LocalEngine) ValidateStatements(ctx context.Context, statements []string) ottl.ValidateResult {
	if err := ctx.Err(); err != nil {
		return ottl.ValidateResult{Errors: []string{err.Error()}}
	}
	return ottl.ValidateStatements(statements)
}

func (e *LocalEngine) ValidateFilterConditions(ctx context.Context, conditions []string, signals gen.Signals) ottl.ValidateResult {
	if err := ctx.Err(); err != nil {
		return ottl.ValidateResult{Errors: []string{err.Error()}}
	}
	return ottl.ValidateFilterConditionsParse(conditions, signals)
}

func (e *LocalEngine) TransformTraces(ctx context.Context, data json.RawMessage, statements []string) (json.RawMessage, int, error) {
	return ottl.TransformRunData(ctx, data, statements)
}

func (e *LocalEngine) TransformLogs(ctx context.Context, data json.RawMessage, statements []string) (json.RawMessage, error) {
	return ottl.TransformLogRunData(ctx, data, statements)
}

func (e *LocalEngine) TransformMetrics(ctx context.Context, data json.RawMessage, statements []string) (json.RawMessage, error) {
	return ottl.TransformMetricRunData(ctx, data, statements)
}

func (e *LocalEngine) FilterTraces(ctx context.Context, data json.RawMessage, conditions []string, mode ottl.FilterMode) (json.RawMessage, int, int, error) {
	return ottl.FilterRunData(ctx, data, conditions, mode)
}

func (e *LocalEngine) FilterLogs(ctx context.Context, data json.RawMessage, conditions []string, mode ottl.FilterMode) (json.RawMessage, int, int, error) {
	return ottl.FilterLogRunData(ctx, data, conditions, mode)
}

func (e *LocalEngine) FilterMetrics(ctx context.Context, data json.RawMessage, conditions []string, mode ottl.FilterMode) (json.RawMessage, int, int, error) {
	return ottl.FilterMetricRunData(ctx, data, conditions, mode)
}

func (e *LocalEngine) ExecutePipeline(ctx context.Context, data json.RawMessage, pipeline ottl.Pipeline) (*ottl.PipelineResult, error) {
	return ottl.ExecutePipeline(ctx, data, pipeline)
}

func (e *LocalEngine) ExecutePipelineForSignal(ctx context.Context, data json.RawMessage, pipeline ottl.Pipeline, signal string) (*ottl.PipelineResult, error) {
	return ottl.ExecutePipelineForSignal(ctx, data, pipeline, signal)
}

func (e *LocalEngine) ImportCollectorConfig(ctx context.Context, configYAML string) (ottl.PipelinesBySignal, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ottl.ImportCollectorConfig(configYAML)
}

// ValidatePipelineOTTL walks processor configs and validates transform statements
// and filter conditions against this train.
func (e *LocalEngine) ValidatePipelineOTTL(ctx context.Context, pipeline ottl.Pipeline) ottl.ValidateResult {
	if len(pipeline.Processors) == 0 {
		return ottl.ValidateResult{Valid: true, Parsed: 0}
	}

	var (
		errors []string
		parsed int
	)

	for name, cfg := range pipeline.Processors {
		if err := ctx.Err(); err != nil {
			return ottl.ValidateResult{Errors: []string{err.Error()}}
		}
		kind := classifyProcessor(cfg)
		switch kind {
		case processorTransform:
			cfgYAML, err := marshalProcessorYAML(cfg)
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			meta, err := ottl.ParseTransformConfigMeta(cfgYAML)
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			for _, section := range []struct {
				groups  []ottl.ContextStatements
				signals gen.Signals
			}{
				{meta.TraceGroups, gen.Signals{Traces: true}},
				{meta.LogGroups, gen.Signals{Logs: true}},
				{meta.MetricGroups, gen.Signals{Metrics: true}},
			} {
				for _, group := range section.groups {
					if len(group.Conditions) == 0 {
						continue
					}
					res := validateGroupConditions(group, section.signals)
					for _, msg := range res.Errors {
						errors = append(errors, fmt.Sprintf("%s: %s", name, msg))
					}
				}
				stmts := ottl.ExtractStatements(section.groups)
				if len(stmts) == 0 {
					continue
				}
				res := ottl.ValidateStatements(stmts)
				parsed += res.Parsed
				for _, msg := range res.Errors {
					errors = append(errors, fmt.Sprintf("%s: %s", name, msg))
				}
			}
		case processorFilter:
			cfgYAML, err := marshalProcessorYAML(cfg)
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			conds, _, err := ottl.ParseFilterConditionsBySignal(cfgYAML)
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			for _, section := range []struct {
				conditions []string
				signals    gen.Signals
			}{
				{conds.Traces, gen.Signals{Traces: true}},
				{conds.Logs, gen.Signals{Logs: true}},
				{conds.Metrics, gen.Signals{Metrics: true}},
			} {
				if len(section.conditions) == 0 {
					continue
				}
				res := ottl.ValidateFilterConditionsParse(section.conditions, section.signals)
				parsed += res.Parsed
				for _, msg := range res.Errors {
					errors = append(errors, fmt.Sprintf("%s: %s", name, msg))
				}
			}
		}
	}

	if len(errors) > 0 {
		return ottl.ValidateResult{Valid: false, Errors: errors, Parsed: parsed}
	}
	return ottl.ValidateResult{Valid: true, Parsed: parsed}
}

func validateGroupConditions(group ottl.ContextStatements, signals gen.Signals) ottl.ValidateResult {
	contextName := strings.ToLower(strings.TrimSpace(group.Context))
	if signals.Traces && contextName != "" && contextName != "span" && contextName != "resource" && contextName != "scope" && contextName != "instrumentation_scope" && contextName != "instrumentationscope" {
		return ottl.ValidateResult{Errors: []string{fmt.Sprintf("unsupported trace statement context: %q", group.Context)}}
	}
	settings := component.TelemetrySettings{Logger: zap.NewNop()}
	var err error
	switch contextName {
	case "resource":
		parser, parseErr := ottlresource.NewParser(xottlfuncs.WithExperimentalConverters(ottlfuncs.StandardFuncs[*ottlresource.TransformContext]()), settings)
		err = parseErr
		if err == nil {
			_, err = parser.ParseConditions(group.Conditions)
		}
	case "scope", "instrumentation_scope", "instrumentationscope":
		parser, parseErr := ottlscope.NewParser(xottlfuncs.WithExperimentalConverters(ottlfuncs.StandardFuncs[*ottlscope.TransformContext]()), settings)
		err = parseErr
		if err == nil {
			_, err = parser.ParseConditions(group.Conditions)
		}
	case "metric":
		if !signals.Metrics {
			return ottl.ValidateTransformConditionsParse(group.Conditions, signals)
		}
		parser, parseErr := ottlmetric.NewParser(xottlfuncs.WithExperimentalConverters(ottlfuncs.StandardFuncs[*ottlmetric.TransformContext]()), settings)
		err = parseErr
		if err == nil {
			_, err = parser.ParseConditions(group.Conditions)
		}
	default:
		return ottl.ValidateTransformConditionsParse(group.Conditions, signals)
	}
	if err != nil {
		return ottl.ValidateResult{Errors: []string{fmt.Sprintf("%s context: %v", contextName, err)}}
	}
	return ottl.ValidateResult{Valid: true}
}

type processorKind int

const (
	processorUnknown processorKind = iota
	processorTransform
	processorFilter
)

func classifyProcessor(cfg ottl.ProcessorConfig) processorKind {
	hasTransform := false
	hasFilter := false
	for key := range cfg {
		switch key {
		case "statements", "trace_statements", "log_statements", "metric_statements":
			hasTransform = true
		case "traces", "logs", "metrics":
			hasFilter = true
		}
	}
	if hasTransform {
		return processorTransform
	}
	if hasFilter {
		return processorFilter
	}
	return processorUnknown
}

func marshalProcessorYAML(cfg ottl.ProcessorConfig) (string, error) {
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal processor config: %w", err)
	}
	return string(out), nil
}
