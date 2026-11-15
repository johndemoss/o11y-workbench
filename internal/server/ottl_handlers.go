package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/ottl"
)

// structuredProcessorConfig reports whether config describes a structured
// processor body (grouped statements). Those execute through the pipeline path
// so contexts and group conditions match Collector semantics.
func structuredProcessorConfig(config string) (map[string]interface{}, bool) {
	if strings.TrimSpace(config) == "" {
		return nil, false
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(config), &raw); err != nil {
		return nil, false // flat line/statement configs keep the existing paths
	}
	for _, key := range []string{"trace_statements", "log_statements", "metric_statements"} {
		if _, ok := raw[key]; ok {
			return raw, true
		}
	}
	return nil, false
}

func singleTransformPipeline(proc map[string]interface{}) *ottl.Pipeline {
	return &ottl.Pipeline{
		Stages:     []ottl.Stage{{ID: "transform", Type: ottl.StageTypeTransform, ProcessorRef: "transform"}},
		Processors: map[string]ottl.ProcessorConfig{"transform": proc},
	}
}

func pipelineStatementsParsed(res *ottl.PipelineResult) int {
	parsed := 0
	for _, st := range res.Stages {
		parsed += st.StatementsParsed
	}
	return parsed
}

func (s *Server) handleOTTLValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Config           string   `json:"config"`
		Statements       []string `json:"statements"` // backward compat
		CollectorVersion string   `json:"collector_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	eng, err := s.resolveEngine(r, req.CollectorVersion)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if proc, structured := structuredProcessorConfig(req.Config); structured {
		result := eng.ValidatePipelineOTTL(r.Context(), *singleTransformPipeline(proc))
		status := http.StatusOK
		if !result.Valid {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, result)
		return
	}

	var statements []string
	if req.Config != "" {
		var err error
		statements, err = ottl.ParseTransformStatements(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.ValidateResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
	} else {
		statements = req.Statements
	}

	result := eng.ValidateStatements(r.Context(), statements)
	status := http.StatusOK
	if !result.Valid {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, result)
}

func (s *Server) handleOTTLTransform(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RunID            string       `json:"run_id"`
		Config           string       `json:"config"`
		Statements       []string     `json:"statements"`  // backward compat
		SignalType       string       `json:"signal_type"` // DEPRECATED: use Signals
		Signals          *gen.Signals `json:"signals"`
		CollectorVersion string       `json:"collector_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.RunID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id is required"})
		return
	}

	eng, err := s.resolveEngine(r, req.CollectorVersion)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	run := s.store.Get(req.RunID)
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found: " + req.RunID})
		return
	}

	// Normalize signal selection: prefer new Signals format, fall back to SignalType
	signals := req.Signals
	if signals == nil {
		signalType := req.SignalType
		if signalType == "" {
			signalType = "traces"
		}
		s := gen.NewSignalsFromType(signalType)
		signals = &s
	}
	if !signals.Traces && !signals.Logs && !signals.Metrics {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one signal is required"})
		return
	}

	if (signals.Traces && signals.Logs) || (signals.Traces && signals.Metrics) || (signals.Logs && signals.Metrics) {
		if proc, structured := structuredProcessorConfig(req.Config); structured {
			p := singleTransformPipeline(proc)
			result := ottl.TransformResult{Valid: true}
			if signals.Traces && run.TraceData != nil {
				res, execErr := eng.ExecutePipelineForSignal(r.Context(), *run.TraceData, *p, "traces")
				if execErr != nil {
					writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{"trace: " + execErr.Error()}})
					return
				}
				result.TransformedData = &res.FinalData
				result.SpanCount = res.FinalSpanCount
				result.TransformedCount = res.FinalSpanCount
				result.StatementsParsed = pipelineStatementsParsed(res)
			}
			if signals.Logs && run.LogData != nil {
				res, execErr := eng.ExecutePipelineForSignal(r.Context(), *run.LogData, *p, "logs")
				if execErr != nil {
					writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{"log: " + execErr.Error()}})
					return
				}
				result.LogTransformedData = &res.FinalData
				result.LogStatementsParsed = pipelineStatementsParsed(res)
			}
			if signals.Metrics && run.MetricData != nil {
				res, execErr := eng.ExecutePipelineForSignal(r.Context(), *run.MetricData, *p, "metrics")
				if execErr != nil {
					writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{"metric: " + execErr.Error()}})
					return
				}
				result.MetricTransformedData = &res.FinalData
				result.MetricStatementsParsed = pipelineStatementsParsed(res)
			}
			writeJSON(w, http.StatusOK, result)
			return
		}
		traceStmts, logStmts, metricStmts, err := ottl.ParseCombinedTransformConfigAll(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
		result := ottl.TransformResult{Valid: true}
		if signals.Traces && len(traceStmts) > 0 && run.TraceData != nil {
			transformed, spanCount, err := eng.TransformTraces(r.Context(), *run.TraceData, traceStmts)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.TransformResult{
					Valid:  false,
					Errors: []string{"trace: " + err.Error()},
				})
				return
			}
			result.TransformedData = &transformed
			result.StatementsParsed = len(traceStmts)
			result.SpanCount = spanCount
			result.TransformedCount = spanCount
		}
		if signals.Logs && len(logStmts) > 0 && run.LogData != nil {
			transformed, err := eng.TransformLogs(r.Context(), *run.LogData, logStmts)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.TransformResult{
					Valid:  false,
					Errors: []string{"log: " + err.Error()},
				})
				return
			}
			result.LogTransformedData = &transformed
			result.LogStatementsParsed = len(logStmts)
		}
		if signals.Metrics && len(metricStmts) > 0 && run.MetricData != nil {
			transformed, err := eng.TransformMetrics(r.Context(), *run.MetricData, metricStmts)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.TransformResult{
					Valid:  false,
					Errors: []string{"metric: " + err.Error()},
				})
				return
			}
			result.MetricTransformedData = &transformed
			result.MetricStatementsParsed = len(metricStmts)
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	if signals.Logs && !signals.Traces && !signals.Metrics {
		if run.LogData == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no log data; only completed runs with log data can be transformed"})
			return
		}
		if proc, structured := structuredProcessorConfig(req.Config); structured {
			res, execErr := eng.ExecutePipelineForSignal(r.Context(), *run.LogData, *singleTransformPipeline(proc), "logs")
			if execErr != nil {
				writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{execErr.Error()}})
				return
			}
			writeJSON(w, http.StatusOK, ottl.TransformResult{
				Valid:               true,
				LogTransformedData:  &res.FinalData,
				LogStatementsParsed: pipelineStatementsParsed(res),
			})
			return
		}
		var statements []string
		if req.Config != "" {
			var err error
			statements, err = ottl.ParseTransformStatements(req.Config)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{err.Error()}})
				return
			}
		} else {
			statements = req.Statements
		}
		transformed, err := eng.TransformLogs(r.Context(), *run.LogData, statements)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.TransformResult{
				Valid:  false,
				Errors: []string{err.Error()},
			})
			return
		}
		writeJSON(w, http.StatusOK, ottl.TransformResult{
			Valid:            true,
			TransformedData:  &transformed,
			StatementsParsed: len(statements),
		})
		return
	}

	if signals.Metrics && !signals.Traces && !signals.Logs {
		if run.MetricData == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no metric data; only completed runs with metric data can be transformed"})
			return
		}
		if proc, structured := structuredProcessorConfig(req.Config); structured {
			res, execErr := eng.ExecutePipelineForSignal(r.Context(), *run.MetricData, *singleTransformPipeline(proc), "metrics")
			if execErr != nil {
				writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{execErr.Error()}})
				return
			}
			writeJSON(w, http.StatusOK, ottl.TransformResult{
				Valid:                  true,
				MetricTransformedData:  &res.FinalData,
				MetricStatementsParsed: pipelineStatementsParsed(res),
			})
			return
		}
		var statements []string
		if req.Config != "" {
			var err error
			statements, err = ottl.ParseTransformStatements(req.Config)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{err.Error()}})
				return
			}
		} else {
			statements = req.Statements
		}
		transformed, err := eng.TransformMetrics(r.Context(), *run.MetricData, statements)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.TransformResult{
				Valid:  false,
				Errors: []string{err.Error()},
			})
			return
		}
		writeJSON(w, http.StatusOK, ottl.TransformResult{
			Valid:                  true,
			MetricTransformedData:  &transformed,
			MetricStatementsParsed: len(statements),
		})
		return
	}

	// Traces (default)
	if run.TraceData == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no trace data; only completed runs with trace data can be transformed"})
		return
	}
	if proc, structured := structuredProcessorConfig(req.Config); structured {
		res, execErr := eng.ExecutePipelineForSignal(r.Context(), *run.TraceData, *singleTransformPipeline(proc), "traces")
		if execErr != nil {
			writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{execErr.Error()}})
			return
		}
		writeJSON(w, http.StatusOK, ottl.TransformResult{
			Valid:            true,
			TransformedData:  &res.FinalData,
			StatementsParsed: pipelineStatementsParsed(res),
			SpanCount:        res.FinalSpanCount,
			TransformedCount: res.FinalSpanCount,
		})
		return
	}
	var statements []string
	if req.Config != "" {
		var err error
		statements, err = ottl.ParseTransformStatements(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
	} else {
		statements = req.Statements
	}
	transformed, spanCount, err := eng.TransformTraces(r.Context(), *run.TraceData, statements)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ottl.TransformResult{
			Valid:  false,
			Errors: []string{err.Error()},
		})
		return
	}
	writeJSON(w, http.StatusOK, ottl.TransformResult{
		Valid:            true,
		TransformedData:  &transformed,
		StatementsParsed: len(statements),
		SpanCount:        spanCount,
		TransformedCount: spanCount,
	})
}

func (s *Server) handleOTTLFilter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RunID            string          `json:"run_id"`
		Config           string          `json:"config"`
		Mode             ottl.FilterMode `json:"mode"`
		Conditions       []string        `json:"conditions"`  // backward compat
		SignalType       string          `json:"signal_type"` // DEPRECATED: use Signals
		Signals          *gen.Signals    `json:"signals"`
		CollectorVersion string          `json:"collector_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.RunID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id is required"})
		return
	}
	if req.Mode == "" {
		req.Mode = ottl.FilterModeInclude
	}

	eng, err := s.resolveEngine(r, req.CollectorVersion)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	run := s.store.Get(req.RunID)
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found: " + req.RunID})
		return
	}

	// Normalize signal selection: prefer new Signals format, fall back to SignalType
	signals := req.Signals
	if signals == nil {
		signalType := req.SignalType
		if signalType == "" {
			signalType = "traces"
		}
		s := gen.NewSignalsFromType(signalType)
		signals = &s
	}
	if !signals.Traces && !signals.Logs && !signals.Metrics {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one signal is required"})
		return
	}

	if (signals.Traces && signals.Logs) || (signals.Traces && signals.Metrics) || (signals.Logs && signals.Metrics) {
		traceConds, logConds, metricConds, err := ottl.ParseCombinedFilterConfigAll(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
		result := ottl.FilterResult{Valid: true}
		if signals.Traces && len(traceConds) > 0 && run.TraceData != nil {
			filtered, total, kept, err := eng.FilterTraces(r.Context(), *run.TraceData, traceConds, req.Mode)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{"trace: " + err.Error()}})
				return
			}
			result.FilteredData = &filtered
			result.ConditionsParsed = len(traceConds)
			result.TotalSpanCount = total
			result.FilteredSpanCount = kept
		}
		if signals.Logs && len(logConds) > 0 && run.LogData != nil {
			filtered, total, kept, err := eng.FilterLogs(r.Context(), *run.LogData, logConds, req.Mode)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{"log: " + err.Error()}})
				return
			}
			result.LogFilteredData = &filtered
			result.LogConditionsParsed = len(logConds)
			result.TotalLogCount = total
			result.FilteredLogCount = kept
		}
		if signals.Metrics && len(metricConds) > 0 && run.MetricData != nil {
			metricFiltered, metricTotal, metricKept, err := eng.FilterMetrics(r.Context(), *run.MetricData, metricConds, req.Mode)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{"metric: " + err.Error()}})
				return
			}
			result.MetricFilteredData = &metricFiltered
			result.MetricConditionsParsed = len(metricConds)
			result.TotalMetricCount = metricTotal
			result.FilteredMetricCount = metricKept
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	if signals.Logs && !signals.Traces && !signals.Metrics {
		if run.LogData == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no log data; only completed runs with log data can be filtered"})
			return
		}
		var conditions []string
		if req.Config != "" {
			var err error
			conditions, err = ottl.ParseFilterConditions(req.Config)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{err.Error()}})
				return
			}
		} else {
			conditions = req.Conditions
		}
		filtered, total, kept, err := eng.FilterLogs(r.Context(), *run.LogData, conditions, req.Mode)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.FilterResult{
				Valid:  false,
				Errors: []string{err.Error()},
			})
			return
		}
		writeJSON(w, http.StatusOK, ottl.FilterResult{
			Valid:             true,
			FilteredData:      &filtered,
			ConditionsParsed:  len(conditions),
			TotalSpanCount:    total,
			FilteredSpanCount: kept,
		})
		return
	}

	if signals.Metrics && !signals.Traces && !signals.Logs {
		if run.MetricData == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no metric data; only completed runs with metric data can be filtered"})
			return
		}
		var conditions []string
		if req.Config != "" {
			var err error
			conditions, err = ottl.ParseFilterConditions(req.Config)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{err.Error()}})
				return
			}
		} else {
			conditions = req.Conditions
		}
		filtered, total, kept, err := eng.FilterMetrics(r.Context(), *run.MetricData, conditions, req.Mode)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.FilterResult{
				Valid:  false,
				Errors: []string{err.Error()},
			})
			return
		}
		writeJSON(w, http.StatusOK, ottl.FilterResult{
			Valid:                  true,
			MetricFilteredData:     &filtered,
			MetricConditionsParsed: len(conditions),
			TotalMetricCount:       total,
			FilteredMetricCount:    kept,
		})
		return
	}

	// Traces (default)
	if run.TraceData == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no trace data; only completed runs with trace data can be filtered"})
		return
	}
	var conditions []string
	if req.Config != "" {
		var err error
		conditions, err = ottl.ParseFilterConditions(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
	} else {
		conditions = req.Conditions
	}
	filtered, total, kept, err := eng.FilterTraces(r.Context(), *run.TraceData, conditions, req.Mode)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ottl.FilterResult{
			Valid:  false,
			Errors: []string{err.Error()},
		})
		return
	}
	writeJSON(w, http.StatusOK, ottl.FilterResult{
		Valid:             true,
		FilteredData:      &filtered,
		ConditionsParsed:  len(conditions),
		TotalSpanCount:    total,
		FilteredSpanCount: kept,
	})
}

func (s *Server) handleOTTLFilterValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Config           string       `json:"config"`
		Conditions       []string     `json:"conditions"`  // backward compat
		SignalType       string       `json:"signal_type"` // DEPRECATED: use Signals
		Signals          *gen.Signals `json:"signals"`
		CollectorVersion string       `json:"collector_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	eng, err := s.resolveEngine(r, req.CollectorVersion)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var conditions []string
	if req.Config != "" {
		var err error
		conditions, err = ottl.ParseFilterConditions(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.ValidateResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
	} else {
		conditions = req.Conditions
	}

	// Normalize signal selection: prefer new Signals format, fall back to SignalType
	signals := req.Signals
	if signals == nil {
		signalType := req.SignalType
		if signalType == "" {
			signalType = "traces"
		}
		s := gen.NewSignalsFromType(signalType)
		signals = &s
	}

	result := eng.ValidateFilterConditions(r.Context(), conditions, *signals)
	status := http.StatusOK
	if !result.Valid {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, result)
}
