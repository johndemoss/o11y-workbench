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

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottldatapoint"
)

// TransformMetricRunData parses and executes OTTL transform statements against
// metric data stored in a completed run. It returns the transformed metric data
// as OTLP JSON. Each OTTL statement is executed against every metric data point.
func TransformMetricRunData(metricData json.RawMessage, statements []string) (json.RawMessage, error) {
	if len(statements) == 0 {
		return nil, fmt.Errorf("no statements provided")
	}

	// Step 1: unmarshal JSON metric data into protobuf types.
	var srcReq collectormetricsv1.ExportMetricsServiceRequest
	if err := protojson.Unmarshal(metricData, &srcReq); err != nil {
		return nil, fmt.Errorf("unmarshal metric data: %w", err)
	}

	// Step 2: marshal protobuf to binary wire format.
	protoBytes, err := proto.Marshal(&srcReq)
	if err != nil {
		return nil, fmt.Errorf("marshal protobuf: %w", err)
	}

	// Step 3: unmarshal binary protobuf into pdata/pmetric types for OTTL.
	var unmarshaler pmetric.ProtoUnmarshaler
	metrics, err := unmarshaler.UnmarshalMetrics(protoBytes)
	if err != nil {
		return nil, fmt.Errorf("unmarshal to pdata: %w", err)
	}

	// Step 4: set up OTTL datapoint-context parser and statement sequence.
	settings := component.TelemetrySettings{
		Logger:         zap.NewNop(),
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  noop.NewMeterProvider(),
		Resource:       pcommon.NewResource(),
	}

	parser, err := ottldatapoint.NewParser(
		parserFunctions[*ottldatapoint.TransformContext](),
		settings,
	)
	if err != nil {
		return nil, fmt.Errorf("create OTTL datapoint parser: %w", err)
	}

	parsedStmts, err := parser.ParseStatements(statements)
	if err != nil {
		return nil, fmt.Errorf("parse OTTL statements: %w", err)
	}

	seq := ottldatapoint.NewStatementSequence(parsedStmts, settings)

	// Step 5: execute statements against all data points.
	ctx := context.Background()
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
					dps := metric.Gauge().DataPoints()
					for dp := 0; dp < dps.Len(); dp++ {
						tCtx := ottldatapoint.NewTransformContext(rm, sm, metric, dps.At(dp))
						if err := seq.Execute(ctx, tCtx); err != nil {
							tCtx.Close()
							return nil, fmt.Errorf("execute on gauge datapoint %d/%d/%d/%d: %w", i, j, k, dp, err)
						}
						tCtx.Close()
					}
				case pmetric.MetricTypeSum:
					dps := metric.Sum().DataPoints()
					for dp := 0; dp < dps.Len(); dp++ {
						tCtx := ottldatapoint.NewTransformContext(rm, sm, metric, dps.At(dp))
						if err := seq.Execute(ctx, tCtx); err != nil {
							tCtx.Close()
							return nil, fmt.Errorf("execute on sum datapoint %d/%d/%d/%d: %w", i, j, k, dp, err)
						}
						tCtx.Close()
					}
				case pmetric.MetricTypeHistogram:
					dps := metric.Histogram().DataPoints()
					for dp := 0; dp < dps.Len(); dp++ {
						tCtx := ottldatapoint.NewTransformContext(rm, sm, metric, dps.At(dp))
						if err := seq.Execute(ctx, tCtx); err != nil {
							tCtx.Close()
							return nil, fmt.Errorf("execute on histogram datapoint %d/%d/%d/%d: %w", i, j, k, dp, err)
						}
						tCtx.Close()
					}
				}
			}
		}
	}

	// Step 6: convert modified pdata Metrics back to OTLP protobuf JSON.
	var marshaler pmetric.ProtoMarshaler
	outBytes, err := marshaler.MarshalMetrics(metrics)
	if err != nil {
		return nil, fmt.Errorf("marshal pdata: %w", err)
	}

	var dstReq collectormetricsv1.ExportMetricsServiceRequest
	if err := proto.Unmarshal(outBytes, &dstReq); err != nil {
		return nil, fmt.Errorf("unmarshal pdata to protobuf: %w", err)
	}

	outJSON, err := protojson.MarshalOptions{
		UseProtoNames: false,
		Indent:        "",
	}.Marshal(&dstReq)
	if err != nil {
		return nil, fmt.Errorf("marshal result JSON: %w", err)
	}

	return outJSON, nil
}
