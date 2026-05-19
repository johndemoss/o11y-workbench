package server

import (
	"encoding/json"
	"net/http"

	"github.com/johndemoss/o11y-workbench/internal/ottl"
)

func (s *Server) handleOTTLValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Config     string   `json:"config"`
		Statements []string `json:"statements"` // backward compat
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
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

	result := ottl.ValidateStatements(statements)
	status := http.StatusOK
	if !result.Valid {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, result)
}

func (s *Server) handleOTTLTransform(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RunID      string   `json:"run_id"`
		Config     string   `json:"config"`
		Statements []string `json:"statements"` // backward compat
		SignalType string   `json:"signal_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.RunID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id is required"})
		return
	}

	run := s.store.Get(req.RunID)
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found: " + req.RunID})
		return
	}

	signalType := req.SignalType
	if signalType == "" {
		signalType = "traces"
	}

	if signalType == "both" {
		traceStmts, logStmts, err := ottl.ParseCombinedTransformConfig(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
		result := ottl.TransformResult{Valid: true}
		if len(traceStmts) > 0 && run.TraceData != nil {
			transformed, spanCount, err := ottl.TransformRunData(*run.TraceData, traceStmts)
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
		if len(logStmts) > 0 && run.LogData != nil {
			transformed, err := ottl.TransformLogRunData(*run.LogData, logStmts)
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
		writeJSON(w, http.StatusOK, result)
		return
	}

	if signalType == "all" {
		traceStmts, logStmts, metricStmts, err := ottl.ParseCombinedTransformConfigAll(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.TransformResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
		result := ottl.TransformResult{Valid: true}
		if len(traceStmts) > 0 && run.TraceData != nil {
			transformed, spanCount, err := ottl.TransformRunData(*run.TraceData, traceStmts)
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
		if len(logStmts) > 0 && run.LogData != nil {
			transformed, err := ottl.TransformLogRunData(*run.LogData, logStmts)
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
		if len(metricStmts) > 0 && run.MetricData != nil {
			transformed, err := ottl.TransformMetricRunData(*run.MetricData, metricStmts)
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

	if signalType == "logs" {
		if run.LogData == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no log data; only completed runs with log data can be transformed"})
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
		transformed, err := ottl.TransformLogRunData(*run.LogData, statements)
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

	if signalType == "metrics" {
		if run.MetricData == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no metric data; only completed runs with metric data can be transformed"})
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
		transformed, err := ottl.TransformMetricRunData(*run.MetricData, statements)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.TransformResult{
				Valid:  false,
				Errors: []string{err.Error()},
			})
			return
		}
		writeJSON(w, http.StatusOK, ottl.TransformResult{
			Valid:                 true,
			MetricTransformedData: &transformed,
			MetricStatementsParsed: len(statements),
		})
		return
	}

	// Traces (default)
	if run.TraceData == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run has no trace data; only completed runs with trace data can be transformed"})
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
	transformed, spanCount, err := ottl.TransformRunData(*run.TraceData, statements)
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
		RunID      string         `json:"run_id"`
		Config     string         `json:"config"`
		Mode       ottl.FilterMode `json:"mode"`
		Conditions []string       `json:"conditions"` // backward compat
		SignalType string         `json:"signal_type"`
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

	run := s.store.Get(req.RunID)
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found: " + req.RunID})
		return
	}

	signalType := req.SignalType
	if signalType == "" {
		signalType = "traces"
	}

	if signalType == "both" {
		traceConds, logConds, err := ottl.ParseCombinedFilterConfig(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
		result := ottl.FilterResult{Valid: true}
		if len(traceConds) > 0 && run.TraceData != nil {
			filtered, total, kept, err := ottl.FilterRunData(*run.TraceData, traceConds, req.Mode)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{"trace: " + err.Error()}})
				return
			}
			result.FilteredData = &filtered
			result.ConditionsParsed = len(traceConds)
			result.TotalSpanCount = total
			result.FilteredSpanCount = kept
		}
		if len(logConds) > 0 && run.LogData != nil {
			filtered, total, kept, err := ottl.FilterLogRunData(*run.LogData, logConds, req.Mode)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{"log: " + err.Error()}})
				return
			}
			result.LogFilteredData = &filtered
			result.LogConditionsParsed = len(logConds)
			result.TotalLogCount = total
			result.FilteredLogCount = kept
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	if signalType == "all" {
		traceConds, logConds, metricConds, err := ottl.ParseCombinedFilterConfigAll(req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{err.Error()}})
			return
		}
		result := ottl.FilterResult{Valid: true}
		if len(traceConds) > 0 && run.TraceData != nil {
			filtered, total, kept, err := ottl.FilterRunData(*run.TraceData, traceConds, req.Mode)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{"trace: " + err.Error()}})
				return
			}
			result.FilteredData = &filtered
			result.ConditionsParsed = len(traceConds)
			result.TotalSpanCount = total
			result.FilteredSpanCount = kept
		}
		if len(logConds) > 0 && run.LogData != nil {
			filtered, total, kept, err := ottl.FilterLogRunData(*run.LogData, logConds, req.Mode)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, ottl.FilterResult{Valid: false, Errors: []string{"log: " + err.Error()}})
				return
			}
			result.LogFilteredData = &filtered
			result.LogConditionsParsed = len(logConds)
			result.TotalLogCount = total
			result.FilteredLogCount = kept
		}
		if len(metricConds) > 0 && run.MetricData != nil {
			metricFiltered, metricTotal, metricKept, err := ottl.FilterMetricRunData(*run.MetricData, metricConds, req.Mode)
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

	if signalType == "logs" {
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
		filtered, total, kept, err := ottl.FilterLogRunData(*run.LogData, conditions, req.Mode)
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

	if signalType == "metrics" {
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
		filtered, total, kept, err := ottl.FilterMetricRunData(*run.MetricData, conditions, req.Mode)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ottl.FilterResult{
				Valid:  false,
				Errors: []string{err.Error()},
			})
			return
		}
		writeJSON(w, http.StatusOK, ottl.FilterResult{
			Valid:                true,
			MetricFilteredData:   &filtered,
			MetricConditionsParsed: len(conditions),
			TotalMetricCount:     total,
			FilteredMetricCount:  kept,
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
	filtered, total, kept, err := ottl.FilterRunData(*run.TraceData, conditions, req.Mode)
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
		Config     string `json:"config"`
		Conditions []string `json:"conditions"` // backward compat
		SignalType string `json:"signal_type"`  // OTTL context (traces/logs/metrics)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
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

	signalType := req.SignalType
	if signalType == "" {
		signalType = "traces"
	}

	result := ottl.ValidateFilterConditionsParse(conditions, signalType)
	status := http.StatusOK
	if !result.Valid {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, result)
}
