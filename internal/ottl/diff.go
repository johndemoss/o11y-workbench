package ottl

import (
	"encoding/json"
	"fmt"
	"strings"
)

type DiffRecord struct {
	ID      string          `json:"id"`
	Name    string          `json:"name,omitempty"`
	Status  string          `json:"status"`
	Before  json.RawMessage `json:"before,omitempty"`
	After   json.RawMessage `json:"after,omitempty"`
	Changes []RecordChange  `json:"changes,omitempty"`
}

type RecordChange struct {
	Field  string      `json:"field"`
	Type   string      `json:"type"`
	Before interface{} `json:"before,omitempty"`
	After  interface{} `json:"after,omitempty"`
}

type StageDiff struct {
	StageIndex  int          `json:"stage_index"`
	Signal      string       `json:"signal"`
	InputCount  int          `json:"input_count"`
	OutputCount int          `json:"output_count"`
	Added       []DiffRecord `json:"added"`
	Removed     []DiffRecord `json:"removed"`
	Modified    []DiffRecord `json:"modified"`
}

type UnifiedDiffLine struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type diffItem struct {
	id       string
	name     string
	matchKey string
	value    map[string]interface{}
}

func field(m map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return nil
}

func text(m map[string]interface{}, keys ...string) string {
	s, _ := field(m, keys...).(string)
	return s
}

func extractRecords(data json.RawMessage, signal string) []diffItem {
	if signal != "logs" && signal != "metrics" {
		signal = "traces"
	}
	var raw interface{}
	if len(data) == 0 || json.Unmarshal(data, &raw) != nil {
		return nil
	}
	root, _ := raw.(map[string]interface{})
	if nested, ok := root[signal[:len(signal)-1]+"_data"].(map[string]interface{}); ok {
		root = nested
	}
	var resourceKeys, scopeKeys []string
	var itemKey string
	switch signal {
	case "logs":
		resourceKeys, scopeKeys, itemKey = []string{"resourceLogs", "resource_logs"}, []string{"scopeLogs", "scope_logs"}, "logRecords"
	case "metrics":
		resourceKeys, scopeKeys, itemKey = []string{"resourceMetrics", "resource_metrics"}, []string{"scopeMetrics", "scope_metrics"}, "metrics"
	default:
		resourceKeys, scopeKeys, itemKey = []string{"resourceSpans", "resource_spans"}, []string{"scopeSpans", "scope_spans"}, "spans"
	}
	var records []diffItem
	add := func(v interface{}, resource, scope interface{}) {
		m, ok := v.(map[string]interface{})
		if !ok {
			return
		}
		id := ""
		matchKey := ""
		switch signal {
		case "traces":
			id = text(m, "traceId", "trace_id") + "/" + text(m, "spanId", "span_id")
		case "metrics":
			id = text(m, "name")
		case "logs":
			id = logRecordID(m)
			matchKey = logRecordMatchKey(m)
		}
		if id == "" {
			id = "record"
		}
		value := make(map[string]interface{}, len(m)+2)
		for k, v := range m {
			value[k] = v
		}
		if resource != nil {
			value["resource"] = resource
		}
		if scope != nil {
			value["scope"] = scope
		}
		records = append(records, diffItem{id: id, name: text(m, "name"), matchKey: matchKey, value: value})
	}
	if list, ok := raw.([]interface{}); ok {
		for _, v := range list {
			add(v, nil, nil)
		}
		return records
	}
	if root == nil {
		return records
	}
	resources, _ := field(root, resourceKeys...).([]interface{})
	for _, rv := range resources {
		r, _ := rv.(map[string]interface{})
		scopes, _ := field(r, scopeKeys...).([]interface{})
		for _, sv := range scopes {
			s, _ := sv.(map[string]interface{})
			items, _ := field(s, itemKey, "log_records").([]interface{})
			for _, item := range items {
				add(item, r["resource"], s["scope"])
			}
		}
	}
	if len(resources) == 0 {
		items, _ := field(root, itemKey, "log_records").([]interface{})
		for _, item := range items {
			add(item, nil, nil)
		}
	}
	return records
}

func marshalDiff(v interface{}) json.RawMessage { b, _ := json.Marshal(v); return b }

// logRecordID prefers the trace/span correlation identity of a log record so
// correlated records pair with their counterpart instead of by position.
func logRecordID(m map[string]interface{}) string {
	traceID, spanID := text(m, "traceId", "trace_id"), text(m, "spanId", "span_id")
	switch {
	case traceID != "" && spanID != "":
		return traceID + "/" + spanID
	case traceID != "":
		return traceID
	case spanID != "":
		return spanID
	}
	return ""
}

// logRecordMatchKey is the stable fallback identity of a log record: the parts
// that identify it independently of its position. Attributes are excluded so
// attribute-only edits still pair as modifications rather than as an add plus a
// remove.
func logRecordMatchKey(m map[string]interface{}) string {
	key, _ := json.Marshal(map[string]interface{}{
		"body":                 field(m, "body"),
		"severityNumber":       field(m, "severityNumber", "severity_number"),
		"severityText":         field(m, "severityText", "severity_text"),
		"timeUnixNano":         field(m, "timeUnixNano", "time_unix_nano"),
		"observedTimeUnixNano": field(m, "observedTimeUnixNano", "observed_time_unix_nano"),
	})
	return string(key)
}

func recordChanges(before, after map[string]interface{}) []RecordChange {
	changes := []RecordChange{}
	keys := make(map[string]bool, len(before)+len(after))
	for k := range before {
		keys[k] = true
	}
	for k := range after {
		keys[k] = true
	}
	for k := range keys {
		b, bok := before[k]
		a, aok := after[k]
		if k == "attributes" {
			bm, am := attributeMap(b), attributeMap(a)
			for _, c := range recordChanges(bm, am) {
				c.Field = "attributes." + c.Field
				changes = append(changes, c)
			}
			continue
		}
		if string(marshalDiff(b)) == string(marshalDiff(a)) && bok == aok {
			continue
		}
		kind := "changed"
		if !bok {
			kind = "added"
		}
		if !aok {
			kind = "removed"
		}
		changes = append(changes, RecordChange{Field: k, Type: kind, Before: b, After: a})
	}
	return changes
}

func attributeMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	m := map[string]interface{}{}
	if list, ok := v.([]interface{}); ok {
		for _, item := range list {
			kv, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if key, ok := kv["key"].(string); ok {
				m[key] = kv["value"]
			}
		}
	}
	return m
}

// metricDataField reports whether a metric field is an OTLP data body whose
// data points are diffed individually instead of reported as one coarse block.
func metricDataField(name string) bool {
	switch name {
	case "gauge", "sum", "histogram", "exponentialHistogram", "exponential_histogram", "summary":
		return true
	}
	return false
}

// metricChanges expands a coarse metric data-body change (for example
// "histogram") into per-data-point changes such as
// "data_points[0].attributes.http.method".
func metricChanges(changes []RecordChange) []RecordChange {
	expanded := make([]RecordChange, 0, len(changes))
	for _, c := range changes {
		before, bok := c.Before.(map[string]interface{})
		after, aok := c.After.(map[string]interface{})
		if c.Type != "changed" || !metricDataField(c.Field) || !bok || !aok {
			expanded = append(expanded, c)
			continue
		}
		detailed := dataPointChanges(before, after)
		for _, rest := range recordChanges(dataPointSkeleton(before), dataPointSkeleton(after)) {
			rest.Field = c.Field + "." + rest.Field
			detailed = append(detailed, rest)
		}
		if len(detailed) == 0 {
			expanded = append(expanded, c)
			continue
		}
		expanded = append(expanded, detailed...)
	}
	return expanded
}

// dataPointChanges pairs a series' data points by index and reports their field
// changes relative to the unchanged series skeleton.
func dataPointChanges(before, after map[string]interface{}) []RecordChange {
	beforePoints, afterPoints := dataPointList(before), dataPointList(after)
	pairs := len(beforePoints)
	if len(afterPoints) < pairs {
		pairs = len(afterPoints)
	}
	var changes []RecordChange
	for i := range pairs {
		bm, _ := beforePoints[i].(map[string]interface{})
		am, _ := afterPoints[i].(map[string]interface{})
		if bm == nil || am == nil {
			continue
		}
		for _, c := range recordChanges(bm, am) {
			c.Field = fmt.Sprintf("data_points[%d].%s", i, c.Field)
			changes = append(changes, c)
		}
	}
	for i := pairs; i < len(beforePoints); i++ {
		changes = append(changes, RecordChange{Field: fmt.Sprintf("data_points[%d]", i), Type: "removed", Before: beforePoints[i]})
	}
	for i := pairs; i < len(afterPoints); i++ {
		changes = append(changes, RecordChange{Field: fmt.Sprintf("data_points[%d]", i), Type: "added", After: afterPoints[i]})
	}
	return changes
}

func dataPointList(m map[string]interface{}) []interface{} {
	list, _ := field(m, "dataPoints", "data_points").([]interface{})
	return list
}

// dataPointSkeleton strips data points so series-level fields can be compared
// separately from the per-data-point expansion.
func dataPointSkeleton(m map[string]interface{}) map[string]interface{} {
	skeleton := make(map[string]interface{}, len(m))
	for k, v := range m {
		if k == "dataPoints" || k == "data_points" {
			continue
		}
		skeleton[k] = v
	}
	return skeleton
}

func computeStageDiff(before, after json.RawMessage, stageIndex int, signal string) StageDiff {
	b, a := extractRecords(before, signal), extractRecords(after, signal)
	d := StageDiff{StageIndex: stageIndex, Signal: signal, InputCount: len(b), OutputCount: len(a), Added: []DiffRecord{}, Removed: []DiffRecord{}, Modified: []DiffRecord{}}
	byID := make(map[string][]int, len(b))
	byKey := make(map[string][]int, len(b))
	byValue := make(map[string][]int, len(b))
	for i, item := range b {
		byID[item.id] = append(byID[item.id], i)
		if item.matchKey == "" {
			continue
		}
		key := item.id + "\x00" + item.matchKey
		byKey[key] = append(byKey[key], i)
		valueKey := key + "\x00" + string(marshalDiff(item.value))
		byValue[valueKey] = append(byValue[valueKey], i)
	}
	used := make([]bool, len(b))
	partner := make([]int, len(a))
	for i := range partner {
		partner[i] = -1
	}
	// Prefer exact content matches so reordered records pair with their own
	// counterpart instead of being diffed against whatever holds the same index.
	nextValue := make(map[string]int, len(byValue))
	for ai, item := range a {
		if item.matchKey == "" {
			continue
		}
		valueKey := item.id + "\x00" + item.matchKey + "\x00" + string(marshalDiff(item.value))
		candidates := byValue[valueKey]
		for nextValue[valueKey] < len(candidates) && used[candidates[nextValue[valueKey]]] {
			nextValue[valueKey]++
		}
		if nextValue[valueKey] >= len(candidates) {
			continue
		}
		bi := candidates[nextValue[valueKey]]
		nextValue[valueKey]++
		partner[ai] = bi
		used[bi] = true
	}
	// Then pair by the stable fallback identity (trace/span correlation or body
	// fingerprint) before resorting to position.
	nextKey := make(map[string]int, len(byKey))
	for ai, item := range a {
		if item.matchKey == "" || partner[ai] >= 0 {
			continue
		}
		key := item.id + "\x00" + item.matchKey
		candidates := byKey[key]
		for nextKey[key] < len(candidates) && used[candidates[nextKey[key]]] {
			nextKey[key]++
		}
		if nextKey[key] >= len(candidates) {
			continue
		}
		bi := candidates[nextKey[key]]
		nextKey[key]++
		partner[ai] = bi
		used[bi] = true
	}
	// Fall back to positional pairing for the remaining records.
	next := make(map[string]int, len(byID))
	for ai, item := range a {
		if partner[ai] >= 0 {
			continue
		}
		indices := byID[item.id]
		for next[item.id] < len(indices) && used[indices[next[item.id]]] {
			next[item.id]++
		}
		if next[item.id] >= len(indices) {
			d.Added = append(d.Added, DiffRecord{ID: item.id, Name: item.name, Status: "added", After: marshalDiff(item.value)})
			continue
		}
		bi := indices[next[item.id]]
		next[item.id]++
		partner[ai] = bi
		used[bi] = true
	}
	for ai, item := range a {
		bi := partner[ai]
		if bi < 0 {
			continue
		}
		old, current := marshalDiff(b[bi].value), marshalDiff(item.value)
		if string(old) == string(current) {
			continue
		}
		changes := recordChanges(b[bi].value, item.value)
		if signal == "metrics" {
			changes = metricChanges(changes)
		}
		d.Modified = append(d.Modified, DiffRecord{ID: item.id, Name: item.name, Status: "modified", Before: old, After: current, Changes: changes})
	}
	for i, item := range b {
		if !used[i] {
			d.Removed = append(d.Removed, DiffRecord{ID: item.id, Name: item.name, Status: "removed", Before: marshalDiff(item.value)})
		}
	}
	return d
}

func ComputeAllStageDiffs(stages []StageResult, originalData json.RawMessage, signal string) []StageDiff {
	diffs := make([]StageDiff, 0, len(stages))
	before := originalData
	for _, stage := range stages {
		diffs = append(diffs, computeStageDiff(before, stage.IntermediateData, stage.StageIndex, signal))
		before = stage.IntermediateData
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
