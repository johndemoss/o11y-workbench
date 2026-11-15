package ottl

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottldatapoint"
)

// FilterMetricRunData evaluates OTTL conditions against each metric data point
// in the metric data and returns a filtered result containing only the data
// points that match (include mode) or only the data points that do NOT match
// (exclude mode).
//
// Conditions are ORed together — if ANY condition evaluates to true for a data
// point, the data point is considered a match.
func FilterMetricRunData(ctx context.Context, metricData json.RawMessage, conditions []string, mode FilterMode) (json.RawMessage, int, int, error) {
	return FilterMetricRunDataWithErrorMode(ctx, metricData, conditions, mode, ErrorModePropagate)
}

// FilterMetricRunDataWithErrorMode is the pipeline entry point; use FilterModeExclude
// for collector-aligned filter processors (drop matching telemetry).
func FilterMetricRunDataWithErrorMode(ctx context.Context, metricData json.RawMessage, conditions []string, mode FilterMode, errorMode ErrorMode) (json.RawMessage, int, int, error) {
	if err := checkFilterCtx(ctx); err != nil {
		return nil, 0, 0, err
	}
	if len(conditions) == 0 {
		return nil, 0, 0, fmt.Errorf("no conditions provided")
	}

	metrics, err := UnmarshalMetricsFromJSON(metricData)
	if err != nil {
		return nil, 0, 0, err
	}

	filtered, total, kept, err := FilterMetricDataPoints(ctx, metrics, conditions, mode, errorMode)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("filter metric data points: %w", err)
	}

	outJSON, err := MarshalMetricsToJSON(filtered)
	if err != nil {
		return nil, 0, 0, err
	}
	return outJSON, total, kept, nil
}

type metricDataPointKey struct {
	ri, si, mi, dpi int
}

// FilterMetricDataPoints evaluates OTTL conditions against all metric data points and
// returns filtered metrics. errorMode controls per-item evaluation errors (same as FilterSpans).
func FilterMetricDataPoints(ctx context.Context, metrics pmetric.Metrics, conditions []string, mode FilterMode, errorMode ErrorMode) (pmetric.Metrics, int, int, error) {
	if err := checkFilterCtx(ctx); err != nil {
		return metrics, 0, 0, err
	}
	if len(conditions) == 0 {
		return metrics, 0, 0, fmt.Errorf("no conditions provided")
	}

	settings := component.TelemetrySettings{
		Logger:         zap.NewNop(),
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  noop.NewMeterProvider(),
		Resource:       pcommon.NewResource(),
	}

	parser, err := ottldatapoint.NewParser(
		parserFuncs[*ottldatapoint.TransformContext](),
		settings,
	)
	if err != nil {
		return metrics, 0, 0, fmt.Errorf("create OTTL datapoint parser: %w", err)
	}

	parsedConditions, err := parser.ParseConditions(conditions)
	if err != nil {
		return metrics, 0, 0, fmt.Errorf("parse OTTL conditions: %w", err)
	}

	condSeq := ottldatapoint.NewConditionSequence(parsedConditions, settings)

	rms := metrics.ResourceMetrics()
	matched := make(map[metricDataPointKey]bool)

	totalDataPoints := 0

	var ignoreLogger *zap.Logger
	if errorMode == ErrorModeIgnore {
		ignoreLogger = zap.NewExample()
		defer ignoreLogger.Sync()
	}

	evalDataPoint := func(rm pmetric.ResourceMetrics, sm pmetric.ScopeMetrics, metric pmetric.Metric, dp any, i, j, k, dpIdx int) error {
		if err := checkFilterCtx(ctx); err != nil {
			return err
		}

		var tCtx *ottldatapoint.TransformContext
		switch v := dp.(type) {
		case pmetric.NumberDataPoint:
			tCtx = ottldatapoint.NewTransformContext(rm, sm, metric, v)
		case pmetric.HistogramDataPoint:
			tCtx = ottldatapoint.NewTransformContext(rm, sm, metric, v)
		case pmetric.ExponentialHistogramDataPoint:
			tCtx = ottldatapoint.NewTransformContext(rm, sm, metric, v)
		case pmetric.SummaryDataPoint:
			tCtx = ottldatapoint.NewTransformContext(rm, sm, metric, v)
		default:
			return nil
		}

		isMatch, evalErr := condSeq.Eval(ctx, tCtx)
		tCtx.Close()
		if evalErr != nil {
			switch errorMode {
			case ErrorModePropagate:
				return fmt.Errorf("evaluate condition on datapoint %d/%d/%d/%d: %w", i, j, k, dpIdx, evalErr)
			case ErrorModeIgnore:
				ignoreLogger.Warn("failed to evaluate condition, skipping metric datapoint",
					zap.Error(evalErr),
					zap.Int("resource", i),
					zap.Int("scope", j),
					zap.Int("metric", k),
					zap.Int("datapoint", dpIdx),
				)
				isMatch = false
			case ErrorModeSilent:
				isMatch = false
			default:
				return fmt.Errorf("unknown error mode: %s", errorMode)
			}
		}

		keep, err := filterKeep(isMatch, mode)
		if err != nil {
			return err
		}
		if keep {
			matched[metricDataPointKey{i, j, k, dpIdx}] = true
		}
		totalDataPoints++
		return nil
	}

	for i := 0; i < rms.Len(); i++ {
		if err := checkFilterCtx(ctx); err != nil {
			return metrics, 0, 0, err
		}
		rm := rms.At(i)
		sms := rm.ScopeMetrics()
		for j := 0; j < sms.Len(); j++ {
			sm := sms.At(j)
			mSlice := sm.Metrics()
			for k := 0; k < mSlice.Len(); k++ {
				metric := mSlice.At(k)
				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					dps := metric.Gauge().DataPoints()
					for dpIdx := 0; dpIdx < dps.Len(); dpIdx++ {
						if err := evalDataPoint(rm, sm, metric, dps.At(dpIdx), i, j, k, dpIdx); err != nil {
							return metrics, 0, 0, err
						}
					}
				case pmetric.MetricTypeSum:
					dps := metric.Sum().DataPoints()
					for dpIdx := 0; dpIdx < dps.Len(); dpIdx++ {
						if err := evalDataPoint(rm, sm, metric, dps.At(dpIdx), i, j, k, dpIdx); err != nil {
							return metrics, 0, 0, err
						}
					}
				case pmetric.MetricTypeHistogram:
					dps := metric.Histogram().DataPoints()
					for dpIdx := 0; dpIdx < dps.Len(); dpIdx++ {
						if err := evalDataPoint(rm, sm, metric, dps.At(dpIdx), i, j, k, dpIdx); err != nil {
							return metrics, 0, 0, err
						}
					}
				case pmetric.MetricTypeExponentialHistogram:
					dps := metric.ExponentialHistogram().DataPoints()
					for dpIdx := 0; dpIdx < dps.Len(); dpIdx++ {
						if err := evalDataPoint(rm, sm, metric, dps.At(dpIdx), i, j, k, dpIdx); err != nil {
							return metrics, 0, 0, err
						}
					}
				case pmetric.MetricTypeSummary:
					dps := metric.Summary().DataPoints()
					for dpIdx := 0; dpIdx < dps.Len(); dpIdx++ {
						if err := evalDataPoint(rm, sm, metric, dps.At(dpIdx), i, j, k, dpIdx); err != nil {
							return metrics, 0, 0, err
						}
					}
				}
			}
		}
	}

	filtered := pmetric.NewMetrics()
	outRms := filtered.ResourceMetrics()

	copyMetricDataPoints := func(src, dst pmetric.Metric, i, j, k int) bool {
		key := func(dpIdx int) metricDataPointKey { return metricDataPointKey{i, j, k, dpIdx} }

		switch src.Type() {
		case pmetric.MetricTypeGauge:
			srcDps := src.Gauge().DataPoints()
			dstDps := dst.SetEmptyGauge().DataPoints()
			for dpIdx := 0; dpIdx < srcDps.Len(); dpIdx++ {
				if matched[key(dpIdx)] {
					srcDps.At(dpIdx).CopyTo(dstDps.AppendEmpty())
				}
			}
			return dstDps.Len() > 0
		case pmetric.MetricTypeSum:
			srcDps := src.Sum().DataPoints()
			dstSum := dst.SetEmptySum()
			dstSum.SetAggregationTemporality(src.Sum().AggregationTemporality())
			dstSum.SetIsMonotonic(src.Sum().IsMonotonic())
			dstDps := dstSum.DataPoints()
			for dpIdx := 0; dpIdx < srcDps.Len(); dpIdx++ {
				if matched[key(dpIdx)] {
					srcDps.At(dpIdx).CopyTo(dstDps.AppendEmpty())
				}
			}
			return dstDps.Len() > 0
		case pmetric.MetricTypeHistogram:
			srcDps := src.Histogram().DataPoints()
			dstHist := dst.SetEmptyHistogram()
			dstHist.SetAggregationTemporality(src.Histogram().AggregationTemporality())
			dstDps := dstHist.DataPoints()
			for dpIdx := 0; dpIdx < srcDps.Len(); dpIdx++ {
				if matched[key(dpIdx)] {
					srcDps.At(dpIdx).CopyTo(dstDps.AppendEmpty())
				}
			}
			return dstDps.Len() > 0
		case pmetric.MetricTypeExponentialHistogram:
			srcDps := src.ExponentialHistogram().DataPoints()
			dstHist := dst.SetEmptyExponentialHistogram()
			dstHist.SetAggregationTemporality(src.ExponentialHistogram().AggregationTemporality())
			dstDps := dstHist.DataPoints()
			for dpIdx := 0; dpIdx < srcDps.Len(); dpIdx++ {
				if matched[key(dpIdx)] {
					srcDps.At(dpIdx).CopyTo(dstDps.AppendEmpty())
				}
			}
			return dstDps.Len() > 0
		case pmetric.MetricTypeSummary:
			srcDps := src.Summary().DataPoints()
			dstDps := dst.SetEmptySummary().DataPoints()
			for dpIdx := 0; dpIdx < srcDps.Len(); dpIdx++ {
				if matched[key(dpIdx)] {
					srcDps.At(dpIdx).CopyTo(dstDps.AppendEmpty())
				}
			}
			return dstDps.Len() > 0
		default:
			return false
		}
	}

	for i := 0; i < rms.Len(); i++ {
		rm := rms.At(i)
		sms := rm.ScopeMetrics()

		hasRmMatch := false
		for j := 0; j < sms.Len() && !hasRmMatch; j++ {
			sm := sms.At(j)
			mSlice := sm.Metrics()
			for k := 0; k < mSlice.Len() && !hasRmMatch; k++ {
				dpsLen := metricDataPointCount(mSlice.At(k))
				for dpIdx := 0; dpIdx < dpsLen && !hasRmMatch; dpIdx++ {
					if matched[metricDataPointKey{i, j, k, dpIdx}] {
						hasRmMatch = true
					}
				}
			}
		}
		if !hasRmMatch {
			continue
		}

		outRm := outRms.AppendEmpty()
		rm.Resource().CopyTo(outRm.Resource())
		outRm.SetSchemaUrl(rm.SchemaUrl())
		outSms := outRm.ScopeMetrics()

		for j := 0; j < sms.Len(); j++ {
			sm := sms.At(j)
			mSlice := sm.Metrics()

			hasSmMatch := false
			for k := 0; k < mSlice.Len() && !hasSmMatch; k++ {
				dpsLen := metricDataPointCount(mSlice.At(k))
				for dpIdx := 0; dpIdx < dpsLen && !hasSmMatch; dpIdx++ {
					if matched[metricDataPointKey{i, j, k, dpIdx}] {
						hasSmMatch = true
					}
				}
			}
			if !hasSmMatch {
				continue
			}

			outSm := outSms.AppendEmpty()
			sm.Scope().CopyTo(outSm.Scope())
			outSm.SetSchemaUrl(sm.SchemaUrl())
			outMetrics := outSm.Metrics()

			for k := 0; k < mSlice.Len(); k++ {
				metric := mSlice.At(k)
				dpsLen := metricDataPointCount(metric)
				hasMetricMatch := false
				for dpIdx := 0; dpIdx < dpsLen; dpIdx++ {
					if matched[metricDataPointKey{i, j, k, dpIdx}] {
						hasMetricMatch = true
						break
					}
				}
				if !hasMetricMatch {
					continue
				}

				scratch := pmetric.NewMetric()
				scratch.SetName(metric.Name())
				scratch.SetDescription(metric.Description())
				scratch.SetUnit(metric.Unit())
				metric.Metadata().CopyTo(scratch.Metadata())
				if copyMetricDataPoints(metric, scratch, i, j, k) {
					scratch.CopyTo(outMetrics.AppendEmpty())
				}
			}
		}
	}

	filteredCount := len(matched)
	return filtered, totalDataPoints, filteredCount, nil
}

func metricDataPointCount(metric pmetric.Metric) int {
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		return metric.Gauge().DataPoints().Len()
	case pmetric.MetricTypeSum:
		return metric.Sum().DataPoints().Len()
	case pmetric.MetricTypeHistogram:
		return metric.Histogram().DataPoints().Len()
	case pmetric.MetricTypeExponentialHistogram:
		return metric.ExponentialHistogram().DataPoints().Len()
	case pmetric.MetricTypeSummary:
		return metric.Summary().DataPoints().Len()
	default:
		return 0
	}
}
