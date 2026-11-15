package ottl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	collectorottl "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottldatapoint"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlmetric"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlresource"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlscope"
)

// TransformMetricRunData parses and executes OTTL transform statements against
// metric data stored in a completed run. It returns the transformed metric data
// as OTLP JSON. Statements are executed in the datapoint context against every
// metric data point.
func TransformMetricRunData(ctx context.Context, metricData json.RawMessage, statements []string) (json.RawMessage, error) {
	out, _, err := TransformMetricRunDataWithErrorMode(ctx, metricData, MetricStatementGroups(statements), ErrorModePropagate)
	return out, err
}

// TransformMetricRunDataWithErrorMode runs transform statement groups against
// metric OTLP JSON with the given per-datapoint / per-metric error handling.
// It returns transformed JSON and the number of statements parsed from the groups.
func TransformMetricRunDataWithErrorMode(ctx context.Context, metricData json.RawMessage, groups []ContextStatements, errorMode ErrorMode) (json.RawMessage, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if len(groups) == 0 || totalMetricStatements(groups) == 0 {
		return nil, 0, fmt.Errorf("no statements provided")
	}

	metrics, err := UnmarshalMetricsFromJSON(metricData)
	if err != nil {
		return nil, 0, err
	}

	stmtsParsed := totalMetricStatements(groups)
	if _, err := TransformMetricGroups(ctx, metrics, groups, errorMode); err != nil {
		return nil, stmtsParsed, err
	}

	outJSON, err := MarshalMetricsToJSON(metrics)
	if err != nil {
		return nil, stmtsParsed, err
	}
	return outJSON, stmtsParsed, nil
}

// TransformMetricGroups executes OTTL statement groups against metrics in order.
// Supported contexts are resource, scope, metric, and datapoint (default).
// Group conditions are ORed; execution runs only when any condition matches.
func TransformMetricGroups(ctx context.Context, metrics pmetric.Metrics, groups []ContextStatements, errorMode ErrorMode) (int, error) {
	if len(groups) == 0 || totalMetricStatements(groups) == 0 {
		return 0, fmt.Errorf("no statements provided")
	}

	mode := collectorErrorMode(errorMode)
	for _, group := range groups {
		if len(group.Statements) == 0 {
			continue
		}
		if err := executeMetricStatementGroup(ctx, metrics, group, errorMode, mode); err != nil {
			return 0, err
		}
	}

	return countMetricDataPointsPdata(metrics), nil
}

// MetricStatementGroups wraps a flat statement list as a single datapoint-context group.
func MetricStatementGroups(statements []string) []ContextStatements {
	if len(statements) == 0 {
		return nil
	}
	return []ContextStatements{{Context: "datapoint", Statements: statements}}
}

func totalMetricStatements(groups []ContextStatements) int {
	n := 0
	for _, g := range groups {
		n += len(g.Statements)
	}
	return n
}

type metricGroupKind int

const (
	metricGroupDatapoint metricGroupKind = iota
	metricGroupMetric
	metricGroupScope
	metricGroupResource
)

func metricGroupExecutionKind(contextName string) metricGroupKind {
	switch strings.ToLower(strings.TrimSpace(contextName)) {
	case "resource":
		return metricGroupResource
	case "scope", "instrumentation_scope", "instrumentationscope":
		return metricGroupScope
	case "metric":
		return metricGroupMetric
	default:
		return metricGroupDatapoint
	}
}

func executeMetricStatementGroup(ctx context.Context, metrics pmetric.Metrics, group ContextStatements, errorMode ErrorMode, mode collectorottl.ErrorMode) error {
	switch metricGroupExecutionKind(group.Context) {
	case metricGroupResource:
		return executeMetricGroupResource(ctx, metrics, group, errorMode, mode)
	case metricGroupScope:
		return executeMetricGroupScope(ctx, metrics, group, errorMode, mode)
	case metricGroupMetric:
		return executeMetricGroupMetric(ctx, metrics, group, errorMode, mode)
	default:
		return executeMetricGroupDatapoint(ctx, metrics, group, errorMode, mode)
	}
}

func telemetrySettings() component.TelemetrySettings {
	return component.TelemetrySettings{
		Logger:         zap.NewNop(),
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  noop.NewMeterProvider(),
		Resource:       pcommon.NewResource(),
	}
}

func collectorErrorMode(errorMode ErrorMode) collectorottl.ErrorMode {
	switch errorMode {
	case ErrorModeIgnore:
		return collectorottl.IgnoreError
	case ErrorModeSilent:
		return collectorottl.SilentError
	default:
		return collectorottl.PropagateError
	}
}

func executeMetricGroupDatapoint(ctx context.Context, metrics pmetric.Metrics, group ContextStatements, errorMode ErrorMode, mode collectorottl.ErrorMode) error {
	settings := telemetrySettings()

	var parserOpts []collectorottl.Option[*ottldatapoint.TransformContext]
	if groupUsesQualifiedMetricPaths(group.Context) {
		parserOpts = append(parserOpts, ottldatapoint.EnablePathContextNames())
	}

	parser, err := ottldatapoint.NewParser(
		parserFuncs[*ottldatapoint.TransformContext](),
		settings,
		parserOpts...,
	)
	if err != nil {
		return fmt.Errorf("create OTTL datapoint parser: %w", err)
	}

	parsedStmts, err := parser.ParseStatements(group.Statements)
	if err != nil {
		return fmt.Errorf("parse OTTL statements: %w", err)
	}
	stmtSeq := ottldatapoint.NewStatementSequence(parsedStmts, settings, ottldatapoint.WithStatementSequenceErrorMode(mode))

	hasConds := len(group.Conditions) > 0
	var condSeq collectorottl.ConditionSequence[*ottldatapoint.TransformContext]
	if hasConds {
		parsedConds, err := parser.ParseConditions(group.Conditions)
		if err != nil {
			return fmt.Errorf("parse OTTL conditions: %w", err)
		}
		condSeq = ottldatapoint.NewConditionSequence(parsedConds, settings, ottldatapoint.WithConditionSequenceErrorMode(mode))
	}

	logger := zap.NewExample()
	return forEachMetricDataPoint(ctx, metrics, func(i, j, k, dp int, rm pmetric.ResourceMetrics, sm pmetric.ScopeMetrics, metric pmetric.Metric, dataPoint any) error {
		tCtx := ottldatapoint.NewTransformContext(rm, sm, metric, dataPoint)
		if hasConds {
			match, evalErr := condSeq.Eval(ctx, tCtx)
			if evalErr != nil {
				tCtx.Close()
				switch errorMode {
				case ErrorModeIgnore:
					logger.Warn("OTTL condition error", zap.Error(evalErr))
					return nil
				case ErrorModeSilent:
					return nil
				default:
					return fmt.Errorf("evaluate condition on datapoint %d/%d/%d/%d: %w", i, j, k, dp, evalErr)
				}
			}
			if !match {
				tCtx.Close()
				return nil
			}
		}
		execErr := stmtSeq.Execute(ctx, tCtx)
		tCtx.Close()
		if execErr != nil {
			switch errorMode {
			case ErrorModeIgnore:
				logger.Warn("OTTL execution error", zap.Error(execErr), zap.Int("resource", i), zap.Int("scope", j), zap.Int("metric", k), zap.Int("datapoint", dp))
			case ErrorModeSilent:
			default:
				return fmt.Errorf("execute on datapoint %d/%d/%d/%d: %w", i, j, k, dp, execErr)
			}
		}
		return nil
	})
}

func executeMetricGroupMetric(ctx context.Context, metrics pmetric.Metrics, group ContextStatements, errorMode ErrorMode, mode collectorottl.ErrorMode) error {
	settings := telemetrySettings()

	var parserOpts []collectorottl.Option[*ottlmetric.TransformContext]
	if groupUsesQualifiedMetricPaths(group.Context) {
		parserOpts = append(parserOpts, ottlmetric.EnablePathContextNames())
	}

	parser, err := ottlmetric.NewParser(
		parserFuncs[*ottlmetric.TransformContext](),
		settings,
		parserOpts...,
	)
	if err != nil {
		return fmt.Errorf("create OTTL metric parser: %w", err)
	}

	parsedStmts, err := parser.ParseStatements(group.Statements)
	if err != nil {
		return fmt.Errorf("parse OTTL statements: %w", err)
	}
	stmtSeq := ottlmetric.NewStatementSequence(parsedStmts, settings, ottlmetric.WithStatementSequenceErrorMode(mode))

	hasConds := len(group.Conditions) > 0
	var condSeq collectorottl.ConditionSequence[*ottlmetric.TransformContext]
	if hasConds {
		parsedConds, err := parser.ParseConditions(group.Conditions)
		if err != nil {
			return fmt.Errorf("parse OTTL conditions: %w", err)
		}
		condSeq = ottlmetric.NewConditionSequence(parsedConds, settings, ottlmetric.WithConditionSequenceErrorMode(mode))
	}

	logger := zap.NewExample()
	rms := metrics.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		rm := rms.At(i)
		sms := rm.ScopeMetrics()
		for j := 0; j < sms.Len(); j++ {
			sm := sms.At(j)
			mSlice := sm.Metrics()
			for k := 0; k < mSlice.Len(); k++ {
				metric := mSlice.At(k)
				tCtx := ottlmetric.NewTransformContext(rm, sm, metric)
				if hasConds {
					match, evalErr := condSeq.Eval(ctx, tCtx)
					if evalErr != nil {
						tCtx.Close()
						switch errorMode {
						case ErrorModeIgnore:
							logger.Warn("OTTL condition error", zap.Error(evalErr))
							continue
						case ErrorModeSilent:
							continue
						default:
							return fmt.Errorf("evaluate condition on metric %d/%d/%d: %w", i, j, k, evalErr)
						}
					}
					if !match {
						tCtx.Close()
						continue
					}
				}
				execErr := stmtSeq.Execute(ctx, tCtx)
				tCtx.Close()
				if execErr != nil {
					switch errorMode {
					case ErrorModeIgnore:
						logger.Warn("OTTL execution error", zap.Error(execErr), zap.Int("resource", i), zap.Int("scope", j), zap.Int("metric", k))
					case ErrorModeSilent:
					default:
						return fmt.Errorf("execute on metric %d/%d/%d: %w", i, j, k, execErr)
					}
				}
			}
		}
	}
	return nil
}

func executeMetricGroupScope(ctx context.Context, metrics pmetric.Metrics, group ContextStatements, errorMode ErrorMode, mode collectorottl.ErrorMode) error {
	settings := telemetrySettings()

	var parserOpts []collectorottl.Option[*ottlscope.TransformContext]
	if groupUsesQualifiedMetricPaths(group.Context) {
		parserOpts = append(parserOpts, ottlscope.EnablePathContextNames())
	}

	parser, err := ottlscope.NewParser(
		parserFuncs[*ottlscope.TransformContext](),
		settings,
		parserOpts...,
	)
	if err != nil {
		return fmt.Errorf("create OTTL scope parser: %w", err)
	}

	parsedStmts, err := parser.ParseStatements(group.Statements)
	if err != nil {
		return fmt.Errorf("parse OTTL statements: %w", err)
	}
	stmtSeq := ottlscope.NewStatementSequence(parsedStmts, settings, ottlscope.WithStatementSequenceErrorMode(mode))

	hasConds := len(group.Conditions) > 0
	var condSeq collectorottl.ConditionSequence[*ottlscope.TransformContext]
	if hasConds {
		parsedConds, err := parser.ParseConditions(group.Conditions)
		if err != nil {
			return fmt.Errorf("parse OTTL conditions: %w", err)
		}
		condSeq = ottlscope.NewConditionSequence(parsedConds, settings, ottlscope.WithConditionSequenceErrorMode(mode))
	}

	logger := zap.NewExample()
	rms := metrics.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		rm := rms.At(i)
		sms := rm.ScopeMetrics()
		for j := 0; j < sms.Len(); j++ {
			sm := sms.At(j)
			mSlice := sm.Metrics()
			if mSlice.Len() == 0 {
				continue
			}
			tCtx := ottlscope.NewTransformContext(sm.Scope(), rm.Resource(), sm, rm)
			if hasConds {
				match, evalErr := condSeq.Eval(ctx, tCtx)
				if evalErr != nil {
					tCtx.Close()
					switch errorMode {
					case ErrorModeIgnore:
						logger.Warn("OTTL condition error", zap.Error(evalErr))
						continue
					case ErrorModeSilent:
						continue
					default:
						return fmt.Errorf("evaluate condition on scope %d/%d: %w", i, j, evalErr)
					}
				}
				if !match {
					tCtx.Close()
					continue
				}
			}
			execErr := stmtSeq.Execute(ctx, tCtx)
			tCtx.Close()
			if execErr != nil {
				switch errorMode {
				case ErrorModeIgnore:
					logger.Warn("OTTL execution error", zap.Error(execErr), zap.Int("resource", i), zap.Int("scope", j))
				case ErrorModeSilent:
				default:
					return fmt.Errorf("execute on scope %d/%d: %w", i, j, execErr)
				}
			}
		}
	}
	return nil
}

func executeMetricGroupResource(ctx context.Context, metrics pmetric.Metrics, group ContextStatements, errorMode ErrorMode, mode collectorottl.ErrorMode) error {
	settings := telemetrySettings()

	var parserOpts []collectorottl.Option[*ottlresource.TransformContext]
	if groupUsesQualifiedMetricPaths(group.Context) {
		parserOpts = append(parserOpts, ottlresource.EnablePathContextNames())
	}

	parser, err := ottlresource.NewParser(
		parserFuncs[*ottlresource.TransformContext](),
		settings,
		parserOpts...,
	)
	if err != nil {
		return fmt.Errorf("create OTTL resource parser: %w", err)
	}

	parsedStmts, err := parser.ParseStatements(group.Statements)
	if err != nil {
		return fmt.Errorf("parse OTTL statements: %w", err)
	}
	stmtSeq := ottlresource.NewStatementSequence(parsedStmts, settings, ottlresource.WithStatementSequenceErrorMode(mode))

	hasConds := len(group.Conditions) > 0
	var condSeq collectorottl.ConditionSequence[*ottlresource.TransformContext]
	if hasConds {
		parsedConds, err := parser.ParseConditions(group.Conditions)
		if err != nil {
			return fmt.Errorf("parse OTTL conditions: %w", err)
		}
		condSeq = ottlresource.NewConditionSequence(parsedConds, settings, ottlresource.WithConditionSequenceErrorMode(mode))
	}

	logger := zap.NewExample()
	rms := metrics.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		rm := rms.At(i)
		if rm.ScopeMetrics().Len() == 0 {
			continue
		}
		tCtx := ottlresource.NewTransformContext(rm.Resource(), rm)
		if hasConds {
			match, evalErr := condSeq.Eval(ctx, tCtx)
			if evalErr != nil {
				tCtx.Close()
				switch errorMode {
				case ErrorModeIgnore:
					logger.Warn("OTTL condition error", zap.Error(evalErr))
					continue
				case ErrorModeSilent:
					continue
				default:
					return fmt.Errorf("evaluate condition on resource %d: %w", i, evalErr)
				}
			}
			if !match {
				tCtx.Close()
				continue
			}
		}
		execErr := stmtSeq.Execute(ctx, tCtx)
		tCtx.Close()
		if execErr != nil {
			switch errorMode {
			case ErrorModeIgnore:
				logger.Warn("OTTL execution error", zap.Error(execErr), zap.Int("resource", i))
			case ErrorModeSilent:
			default:
				return fmt.Errorf("execute on resource %d: %w", i, execErr)
			}
		}
	}
	return nil
}

type metricDataPointFn func(i, j, k, dp int, rm pmetric.ResourceMetrics, sm pmetric.ScopeMetrics, metric pmetric.Metric, dataPoint any) error

func forEachMetricDataPoint(ctx context.Context, metrics pmetric.Metrics, fn metricDataPointFn) error {
	rms := metrics.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		rm := rms.At(i)
		sms := rm.ScopeMetrics()
		for j := 0; j < sms.Len(); j++ {
			sm := sms.At(j)
			mSlice := sm.Metrics()
			for k := 0; k < mSlice.Len(); k++ {
				metric := mSlice.At(k)
				if err := forEachDataPointInMetric(i, j, k, rm, sm, metric, fn); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func forEachDataPointInMetric(i, j, k int, rm pmetric.ResourceMetrics, sm pmetric.ScopeMetrics, metric pmetric.Metric, fn metricDataPointFn) error {
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		dps := metric.Gauge().DataPoints()
		for dp := range dps.Len() {
			if err := fn(i, j, k, dp, rm, sm, metric, dps.At(dp)); err != nil {
				return err
			}
		}
	case pmetric.MetricTypeSum:
		dps := metric.Sum().DataPoints()
		for dp := range dps.Len() {
			if err := fn(i, j, k, dp, rm, sm, metric, dps.At(dp)); err != nil {
				return err
			}
		}
	case pmetric.MetricTypeHistogram:
		dps := metric.Histogram().DataPoints()
		for dp := range dps.Len() {
			if err := fn(i, j, k, dp, rm, sm, metric, dps.At(dp)); err != nil {
				return err
			}
		}
	case pmetric.MetricTypeExponentialHistogram:
		dps := metric.ExponentialHistogram().DataPoints()
		for dp := range dps.Len() {
			if err := fn(i, j, k, dp, rm, sm, metric, dps.At(dp)); err != nil {
				return err
			}
		}
	case pmetric.MetricTypeSummary:
		dps := metric.Summary().DataPoints()
		for dp := range dps.Len() {
			if err := fn(i, j, k, dp, rm, sm, metric, dps.At(dp)); err != nil {
				return err
			}
		}
	}
	return nil
}

func countMetricDataPointsPdata(metrics pmetric.Metrics) int {
	count := 0
	rms := metrics.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		rm := rms.At(i)
		sms := rm.ScopeMetrics()
		for j := 0; j < sms.Len(); j++ {
			sm := sms.At(j)
			mSlice := sm.Metrics()
			for k := 0; k < mSlice.Len(); k++ {
				metric := mSlice.At(k)
				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					count += metric.Gauge().DataPoints().Len()
				case pmetric.MetricTypeSum:
					count += metric.Sum().DataPoints().Len()
				case pmetric.MetricTypeHistogram:
					count += metric.Histogram().DataPoints().Len()
				case pmetric.MetricTypeExponentialHistogram:
					count += metric.ExponentialHistogram().DataPoints().Len()
				case pmetric.MetricTypeSummary:
					count += metric.Summary().DataPoints().Len()
				}
			}
		}
	}
	return count
}

// groupUsesQualifiedMetricPaths reports whether metric group execution needs
// qualified path context names. Groups execute inside their declared context,
// so implicit paths must resolve normally; enabling qualified names would
// require metric./datapoint./resource. prefixes and break Collector configs.
func groupUsesQualifiedMetricPaths(contextName string) bool {
	_ = contextName
	return false
}
