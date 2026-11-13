package ottl

import (
	"encoding/json"
	"strings"
)

type DiffSpan struct {
	SpanID  string          `json:"spanId"`
	Name    string          `json:"name,omitempty"`
	Status  string          `json:"status"`
	Before  json.RawMessage `json:"before,omitempty"`
	After   json.RawMessage `json:"after,omitempty"`
	Changes []SpanChange    `json:"changes,omitempty"`
}

type SpanChange struct {
	Field  string      `json:"field"`
	Type   string      `json:"type"`
	Before interface{} `json:"before,omitempty"`
	After  interface{} `json:"after,omitempty"`
}

type StageDiff struct {
	StageIndex      int        `json:"stage_index"`
	InputSpanCount  int        `json:"input_span_count"`
	OutputSpanCount int        `json:"output_span_count"`
	Added           []DiffSpan `json:"added"`
	Removed         []DiffSpan `json:"removed"`
	Modified        []DiffSpan `json:"modified"`
}

type UnifiedDiffLine struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func extractSpansFromData(data json.RawMessage) ([]map[string]interface{}, error) {
	var result []map[string]interface{}
	if len(data) == 0 || string(data) == "null" {
		return result, nil
	}

	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	spans := make([]map[string]interface{}, 0)

	extractFromSlice := func(arr []interface{}) bool {
		for _, item := range arr {
			span, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			spanID := getStringField(span, "spanId", "span_id")
			if spanID != "" {
				spans = append(spans, span)
			}
		}
		return len(spans) > 0
	}

	extractFromMap := func(m map[string]interface{}) bool {
		resourceSpans, ok := m["resourceSpans"].([]interface{})
		if !ok {
			resourceSpans, ok = m["resource_spans"].([]interface{})
		}
		if !ok {
			return false
		}
		for _, rs := range resourceSpans {
			rsMap, ok := rs.(map[string]interface{})
			if !ok {
				continue
			}
			scopeSpans, ok := rsMap["scopeSpans"].([]interface{})
			if !ok {
				scopeSpans, ok = rsMap["scope_spans"].([]interface{})
			}
			if ok {
				for _, ss := range scopeSpans {
					ssMap, ok := ss.(map[string]interface{})
					if !ok {
						continue
					}
					items, ok := ssMap["spans"].([]interface{})
					if ok {
						for _, item := range items {
							span, ok := item.(map[string]interface{})
							if !ok {
								continue
							}
							spanID := getStringField(span, "spanId", "span_id")
							if spanID != "" {
								spans = append(spans, span)
							}
						}
					}
				}
			}
		}
		return len(spans) > 0
	}

	switch r := raw.(type) {
	case []interface{}:
		extractFromSlice(r)
	case map[string]interface{}:
		if extractFromMap(r) {
			return spans, nil
		}
		if traceData, ok := r["trace_data"].(map[string]interface{}); ok {
			if extractFromMap(traceData) {
				return spans, nil
			}
		}
		if flatSpans, ok := r["spans"].([]interface{}); ok {
			extractFromSlice(flatSpans)
		}
	}

	return spans, nil
}

func getStringField(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key].(string); ok {
			return v
		}
	}
	return ""
}

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func computeStageDiff(before, after json.RawMessage, stageIndex int) StageDiff {
	var beforeSpans []map[string]interface{}
	var afterSpans []map[string]interface{}
	var err error

	if len(before) > 0 && string(before) != "null" {
		beforeSpans, err = extractSpansFromData(before)
		if err != nil {
			beforeSpans = nil
		}
	}

	if len(after) > 0 && string(after) != "null" {
		afterSpans, err = extractSpansFromData(after)
		if err != nil {
			afterSpans = nil
		}
	}

	beforeCount := len(beforeSpans)
	afterCount := len(afterSpans)

	diff := StageDiff{
		StageIndex:      stageIndex,
		InputSpanCount:  beforeCount,
		OutputSpanCount: afterCount,
		Added:           []DiffSpan{},
		Removed:         []DiffSpan{},
		Modified:        []DiffSpan{},
	}

	beforeMap := make(map[string]map[string]interface{})
	for _, s := range beforeSpans {
		if id := getStringField(s, "spanId", "span_id"); id != "" {
			beforeMap[id] = s
		}
	}

	afterMap := make(map[string]map[string]interface{})
	for _, s := range afterSpans {
		if id := getStringField(s, "spanId", "span_id"); id != "" {
			afterMap[id] = s
		}
	}

	for id, afterSpan := range afterMap {
		beforeSpan, exists := beforeMap[id]
		if !exists {
			diff.Added = append(diff.Added, DiffSpan{
				SpanID: id,
				Name:   getStringField(afterSpan, "name"),
				Status: "added",
				After:  mustMarshal(afterSpan),
			})
		} else {
			beforeJSON := mustMarshal(beforeSpan)
			afterJSON := mustMarshal(afterSpan)
			if string(beforeJSON) != string(afterJSON) {
				changes := computeSpanChanges(beforeSpan, afterSpan)
				diff.Modified = append(diff.Modified, DiffSpan{
					SpanID:  id,
					Name:    getStringField(afterSpan, "name"),
					Status:  "modified",
					Before:  beforeJSON,
					After:   afterJSON,
					Changes: changes,
				})
			}
		}
	}

	for id, beforeSpan := range beforeMap {
		if _, exists := afterMap[id]; !exists {
			diff.Removed = append(diff.Removed, DiffSpan{
				SpanID: id,
				Name:   getStringField(beforeSpan, "name"),
				Status: "removed",
				Before: mustMarshal(beforeSpan),
			})
		}
	}

	return diff
}

func normalizeAttrs(attrs interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	if attrs == nil {
		return result
	}
	switch v := attrs.(type) {
	case map[string]interface{}:
		return v
	case map[string]string:
		for k, val := range v {
			result[k] = val
		}
	}
	return result
}

func getField(m map[string]interface{}, key string) interface{} {
	if m == nil {
		return nil
	}
	return m[key]
}

func jsonEqual(a, b interface{}) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

func computeSpanChanges(before, after map[string]interface{}) []SpanChange {
	changes := []SpanChange{}

	bAttrs := normalizeAttrs(getField(before, "attributes"))
	aAttrs := normalizeAttrs(getField(after, "attributes"))

	for key, aVal := range aAttrs {
		if _, exists := bAttrs[key]; !exists {
			changes = append(changes, SpanChange{
				Field: "attributes." + key,
				Type:  "added",
				After: aVal,
			})
		} else if !jsonEqual(bAttrs[key], aVal) {
			changes = append(changes, SpanChange{
				Field:  "attributes." + key,
				Type:   "changed",
				Before: bAttrs[key],
				After:  aVal,
			})
		}
	}
	for key, bVal := range bAttrs {
		if _, exists := aAttrs[key]; !exists {
			changes = append(changes, SpanChange{
				Field:  "attributes." + key,
				Type:   "removed",
				Before: bVal,
			})
		}
	}

	scalarFields := []string{"name", "kind", "trace_id", "traceId", "parent_span_id", "parentSpanId", "start_time_unix_nano", "end_time_unix_nano", "status_code", "statusCode"}
	for _, field := range scalarFields {
		bVal := getField(before, field)
		aVal := getField(after, field)
		if aVal != nil && !jsonEqual(bVal, aVal) {
			changes = append(changes, SpanChange{
				Field:  field,
				Type:   "changed",
				Before: bVal,
				After:  aVal,
			})
		}
	}

	return changes
}

func ComputeAllStageDiffs(stages []StageResult, originalData json.RawMessage) []StageDiff {
	if len(stages) == 0 {
		return nil
	}

	diffs := make([]StageDiff, 0, len(stages))

	for i := 0; i < len(stages); i++ {
		stage := stages[i]

		var before json.RawMessage
		if i == 0 {
			before = originalData
		} else {
			before = stages[i-1].IntermediateData
		}

		after := stage.IntermediateData

		stageDiff := computeStageDiff(before, after, stage.StageIndex)
		diffs = append(diffs, stageDiff)
	}

	return diffs
}

func formatUnifiedDiff(before, after []string, ctx int) []UnifiedDiffLine {
	var lines []UnifiedDiffLine
	minLen := len(before)
	if len(after) < minLen {
		minLen = len(after)
	}

	for i := 0; i < minLen; i++ {
		if before[i] != after[i] {
			lines = append(lines, UnifiedDiffLine{Type: "removed", Text: before[i]})
			lines = append(lines, UnifiedDiffLine{Type: "added", Text: after[i]})
		} else {
			lines = append(lines, UnifiedDiffLine{Type: "context", Text: before[i]})
		}
	}

	for i := minLen; i < len(before); i++ {
		lines = append(lines, UnifiedDiffLine{Type: "removed", Text: before[i]})
	}
	for i := minLen; i < len(after); i++ {
		lines = append(lines, UnifiedDiffLine{Type: "added", Text: after[i]})
	}

	return lines
}

func highlightJSON(data json.RawMessage) string {
	var m interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return string(data)
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return string(b)
}

func ComputeUnifiedDiff(before, after json.RawMessage, ctx int) []UnifiedDiffLine {
	var beforeLines, afterLines []string
	if len(before) > 0 && string(before) != "null" {
		formatted := highlightJSON(before)
		beforeLines = strings.Split(formatted, "\n")
	}
	if len(after) > 0 && string(after) != "null" {
		formatted := highlightJSON(after)
		afterLines = strings.Split(formatted, "\n")
	}
	return formatUnifiedDiff(beforeLines, afterLines, ctx)
}
