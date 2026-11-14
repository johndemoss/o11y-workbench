package engine

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
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottldatapoint"
)

// FilterMetricRunData evaluates OTTL conditions against each metric data point
// in the metric data and returns a filtered result containing only the data
// points that match (include mode) or only the data points that do NOT match
// (exclude mode).
//
// Conditions are ORed together — if ANY condition evaluates to true for a data
// point, the data point is considered a match.
func FilterMetricRunData(metricData json.RawMessage, conditions []string, mode FilterMode) (json.RawMessage, int, int, error) {
	if len(conditions) == 0 {
		return nil, 0, 0, fmt.Errorf("no conditions provided")
	}

	// Step 1: unmarshal JSON metric data into protobuf types.
	var srcReq collectormetricsv1.ExportMetricsServiceRequest
	if err := protojson.Unmarshal(metricData, &srcReq); err != nil {
		return nil, 0, 0, fmt.Errorf("unmarshal metric data: %w", err)
	}

	// Step 2: marshal protobuf to binary wire format, then to pdata for OTTL.
	protoBytes, err := proto.Marshal(&srcReq)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("marshal protobuf: %w", err)
	}

	var unmarshaler pmetric.ProtoUnmarshaler
	metrics, err := unmarshaler.UnmarshalMetrics(protoBytes)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("unmarshal to pdata: %w", err)
	}

	// Step 3: set up OTTL datapoint-context parser and condition sequence.
	settings := component.TelemetrySettings{
		Logger:         zap.NewNop(),
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  noop.NewMeterProvider(),
		Resource:       pcommon.NewResource(),
	}

	parser, err := ottldatapoint.NewParser(
		ParserFunctions[*ottldatapoint.TransformContext](),
		settings,
	)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("create OTTL datapoint parser: %w", err)
	}

	parsedConditions, err := parser.ParseConditions(conditions)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("parse OTTL conditions: %w", err)
	}

	condSeq := ottldatapoint.NewConditionSequence(parsedConditions, settings)

	// Step 4: evaluate conditions against every metric data point, recording matches.
	ctx := context.Background()
	rms := metrics.ResourceMetrics()

	type metricDataPointKey struct {
		ri, si, mi, dpi int
	}
	matched := make(map[metricDataPointKey]bool)

	totalDataPoints := 0
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
					dps := metric.Gauge().DataPoints()
					for dp := 0; dp < dps.Len(); dp++ {
						tCtx := ottldatapoint.NewTransformContext(rm, sm, metric, dps.At(dp))

						isMatch, evalErr := condSeq.Eval(ctx, tCtx)
						tCtx.Close()
						if evalErr != nil {
							return nil, 0, 0, fmt.Errorf("evaluate condition on gauge datapoint %d/%d/%d/%d: %w", i, j, k, dp, evalErr)
						}

						var keep bool
						switch mode {
						case FilterModeInclude:
							keep = isMatch
						case FilterModeExclude:
							keep = !isMatch
						default:
							return nil, 0, 0, fmt.Errorf("unknown filter mode: %s", mode)
						}

						if keep {
							matched[metricDataPointKey{i, j, k, dp}] = true
						}
						totalDataPoints++
					}
				case pmetric.MetricTypeSum:
					dps := metric.Sum().DataPoints()
					for dp := 0; dp < dps.Len(); dp++ {
						tCtx := ottldatapoint.NewTransformContext(rm, sm, metric, dps.At(dp))

						isMatch, evalErr := condSeq.Eval(ctx, tCtx)
						tCtx.Close()
						if evalErr != nil {
							return nil, 0, 0, fmt.Errorf("evaluate condition on sum datapoint %d/%d/%d/%d: %w", i, j, k, dp, evalErr)
						}

						var keep bool
						switch mode {
						case FilterModeInclude:
							keep = isMatch
						case FilterModeExclude:
							keep = !isMatch
						default:
							return nil, 0, 0, fmt.Errorf("unknown filter mode: %s", mode)
						}

						if keep {
							matched[metricDataPointKey{i, j, k, dp}] = true
						}
						totalDataPoints++
					}
				case pmetric.MetricTypeHistogram:
					dps := metric.Histogram().DataPoints()
					for dp := 0; dp < dps.Len(); dp++ {
						tCtx := ottldatapoint.NewTransformContext(rm, sm, metric, dps.At(dp))

						isMatch, evalErr := condSeq.Eval(ctx, tCtx)
						tCtx.Close()
						if evalErr != nil {
							return nil, 0, 0, fmt.Errorf("evaluate condition on histogram datapoint %d/%d/%d/%d: %w", i, j, k, dp, evalErr)
						}

						var keep bool
						switch mode {
						case FilterModeInclude:
							keep = isMatch
						case FilterModeExclude:
							keep = !isMatch
						default:
							return nil, 0, 0, fmt.Errorf("unknown filter mode: %s", mode)
						}

						if keep {
							matched[metricDataPointKey{i, j, k, dp}] = true
						}
						totalDataPoints++
					}
				}
			}
		}
	}

	// Step 5: build a new ExportMetricsServiceRequest containing only matched data points.
	var filteredReq collectormetricsv1.ExportMetricsServiceRequest
	for i, rms := range srcReq.ResourceMetrics {
		var outRms metricspb.ResourceMetrics
		outRms.Resource = rms.Resource
		outRms.SchemaUrl = rms.SchemaUrl

		for j, sms := range rms.ScopeMetrics {
			var outSms metricspb.ScopeMetrics
			outSms.Scope = sms.Scope
			outSms.SchemaUrl = sms.SchemaUrl

			for k, m := range sms.Metrics {
				var outMetric metricspb.Metric
				outMetric.Name = m.Name
				outMetric.Description = m.Description
				outMetric.Unit = m.Unit

				hasDataPoints := false

				switch data := m.Data.(type) {
				case *metricspb.Metric_Gauge:
					var filteredDps []*metricspb.NumberDataPoint
					for dp, dpProto := range data.Gauge.DataPoints {
						if matched[metricDataPointKey{i, j, k, dp}] {
							filteredDps = append(filteredDps, dpProto)
						}
					}
					if len(filteredDps) > 0 {
						outMetric.Data = &metricspb.Metric_Gauge{
							Gauge: &metricspb.Gauge{DataPoints: filteredDps},
						}
						hasDataPoints = true
					}
				case *metricspb.Metric_Sum:
					var filteredDps []*metricspb.NumberDataPoint
					for dp, dpProto := range data.Sum.DataPoints {
						if matched[metricDataPointKey{i, j, k, dp}] {
							filteredDps = append(filteredDps, dpProto)
						}
					}
					if len(filteredDps) > 0 {
						outMetric.Data = &metricspb.Metric_Sum{
							Sum: &metricspb.Sum{
								DataPoints:             filteredDps,
								AggregationTemporality: data.Sum.AggregationTemporality,
								IsMonotonic:            data.Sum.IsMonotonic,
							},
						}
						hasDataPoints = true
					}
				case *metricspb.Metric_Histogram:
					var filteredDps []*metricspb.HistogramDataPoint
					for dp, dpProto := range data.Histogram.DataPoints {
						if matched[metricDataPointKey{i, j, k, dp}] {
							filteredDps = append(filteredDps, dpProto)
						}
					}
					if len(filteredDps) > 0 {
						outMetric.Data = &metricspb.Metric_Histogram{
							Histogram: &metricspb.Histogram{
								DataPoints:             filteredDps,
								AggregationTemporality: data.Histogram.AggregationTemporality,
							},
						}
						hasDataPoints = true
					}
				default:
					// For unhandled metric types (ExponentialHistogram, Summary),
					// include the metric as-is if any of its data points matched.
					// Since we cannot reliably filter individual data points for
					// unhandled types, include the entire metric.
					hasDataPoints = true
					outMetric.Data = m.Data
				}

				if hasDataPoints {
					outSms.Metrics = append(outSms.Metrics, &outMetric)
				}
			}

			if len(outSms.Metrics) > 0 {
				outRms.ScopeMetrics = append(outRms.ScopeMetrics, &outSms)
			}
		}

		if len(outRms.ScopeMetrics) > 0 {
			filteredReq.ResourceMetrics = append(filteredReq.ResourceMetrics, &outRms)
		}
	}

	outJSON, err := protojson.MarshalOptions{
		UseProtoNames: false,
		Indent:        "",
	}.Marshal(&filteredReq)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("marshal result JSON: %w", err)
	}

	filteredCount := len(matched)
	return outJSON, totalDataPoints, filteredCount, nil
}
