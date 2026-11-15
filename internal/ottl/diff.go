package ottl

import (
	"encoding/json"
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
	id    string
	name  string
	value map[string]interface{}
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
		if signal == "traces" {
			id = text(m, "traceId", "trace_id") + "/" + text(m, "spanId", "span_id")
		} else if signal == "metrics" {
			id = text(m, "name")
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
		records = append(records, diffItem{id: id, name: text(m, "name"), value: value})
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

func computeStageDiff(before, after json.RawMessage, stageIndex int, signal string) StageDiff {
	b, a := extractRecords(before, signal), extractRecords(after, signal)
	d := StageDiff{StageIndex: stageIndex, Signal: signal, InputCount: len(b), OutputCount: len(a), Added: []DiffRecord{}, Removed: []DiffRecord{}, Modified: []DiffRecord{}}
	byID := make(map[string][]int, len(b))
	for i, item := range b {
		byID[item.id] = append(byID[item.id], i)
	}
	used := make([]bool, len(b))
	next := make(map[string]int, len(byID))
	for _, item := range a {
		indices := byID[item.id]
		n := next[item.id]
		if n >= len(indices) {
			d.Added = append(d.Added, DiffRecord{ID: item.id, Name: item.name, Status: "added", After: marshalDiff(item.value)})
			continue
		}
		i := indices[n]
		next[item.id] = n + 1
		used[i] = true
		old, current := marshalDiff(b[i].value), marshalDiff(item.value)
		if string(old) != string(current) {
			d.Modified = append(d.Modified, DiffRecord{ID: item.id, Name: item.name, Status: "modified", Before: old, After: current, Changes: recordChanges(b[i].value, item.value)})
		}
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
