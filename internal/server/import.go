package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/export"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func (s *Server) handleImportRun(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read body: " + err.Error()})
		return
	}

	spans, err := parseOTLPJSON(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	totalSpans := 0
	traceIDs := make(map[string]bool)
	for _, rs := range spans {
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				totalSpans++
				if sp.TraceId != nil {
					traceIDs[string(sp.TraceId)] = true
				}
			}
		}
	}

	req := &collectortracev1.ExportTraceServiceRequest{ResourceSpans: spans}
	data, err := protojson.MarshalOptions{UseProtoNames: false, Indent: ""}.Marshal(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "marshal error: " + err.Error()})
		return
	}

	raw := json.RawMessage(data)
	now := time.Now()
	run := &Run{
		ID:                fmt.Sprintf("run-%d", runIDCounter.Add(1)),
		Status:            RunStatusCompleted,
		StartedAt:         now,
		CompletedAt:       &now,
		SpanCount:         totalSpans,
		ResourceSpanCount: len(spans),
		TraceCount:        len(traceIDs),
		Scenario:          "imported",
		Output:            "import",
		TraceData:         &raw,
	}
	s.store.Add(run)

	writeJSON(w, http.StatusCreated, run)
}

// parseOTLPJSON parses OTLP JSON payload into ResourceSpans.
// Handles three formats:
//   - Single ExportTraceServiceRequest: {"resourceSpans":[...]}
//   - JSON array: [{"resourceSpans":[...]}, ...]
//   - NDJSON: one ExportTraceServiceRequest per line
//
// The OTLP JSON spec encodes trace_id, span_id, and parent_span_id as hex
// strings, but protojson expects base64. This function converts hex IDs to
// base64 before unmarshaling.
func parseOTLPJSON(data []byte) ([]*tracev1.ResourceSpans, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, errors.New("empty JSON body")
	}

	opts := protojson.UnmarshalOptions{DiscardUnknown: true}

	// Handle JSON array of ExportTraceServiceRequest objects
	if trimmed[0] == '[' {
		var rawItems []json.RawMessage
		if err := json.Unmarshal(trimmed, &rawItems); err != nil {
			return nil, fmt.Errorf("invalid JSON array: %w", err)
		}
		var all []*tracev1.ResourceSpans
		for i, item := range rawItems {
			fixed, err := export.FixTraceSpanIDsForUnmarshal(item)
			if err != nil {
				return nil, fmt.Errorf("array element %d: %w", i, err)
			}
			var req collectortracev1.ExportTraceServiceRequest
			if err := opts.Unmarshal(fixed, &req); err != nil {
				return nil, fmt.Errorf("array element %d: %w", i, err)
			}
			all = append(all, req.ResourceSpans...)
		}
		if len(all) == 0 {
			return nil, errors.New("no resourceSpans found in JSON array")
		}
		return all, nil
	}

	// Use streaming JSON decoder — handles single object and NDJSON.
	// Each complete JSON value is individually fixed (hex→base64) then parsed.
	var all []*tracev1.ResourceSpans
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("JSON decode error: %w", err)
		}
		fixed, err := export.FixTraceSpanIDsForUnmarshal(raw)
		if err != nil {
			return nil, fmt.Errorf("fixing trace IDs: %w", err)
		}
		var req collectortracev1.ExportTraceServiceRequest
		if err := opts.Unmarshal(fixed, &req); err != nil {
			return nil, fmt.Errorf("invalid OTLP JSON: %w", err)
		}
		all = append(all, req.ResourceSpans...)
	}

	if len(all) == 0 {
		return nil, errors.New("no resourceSpans found in JSON")
	}
	return all, nil
}
