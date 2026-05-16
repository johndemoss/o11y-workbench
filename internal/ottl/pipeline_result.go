package ottl

import "encoding/json"

// PipelineResult holds the overall result of a pipeline execution.
type PipelineResult struct {
	Stages         []StageResult   `json:"stages"`
	FinalData      json.RawMessage `json:"final_data,omitempty"`
	FinalSpanCount int             `json:"final_span_count"`
}

// StageResult holds the result of a single pipeline stage.
type StageResult struct {
	StageIndex       int             `json:"stage_index"`
	StageID          string          `json:"stage_id"`
	InputSpanCount   int             `json:"input_span_count"`
	OutputSpanCount  int             `json:"output_span_count"`
	StatementsParsed int             `json:"statements_parsed"`
	Errors           []string        `json:"errors,omitempty"`
	IntermediateData json.RawMessage `json:"intermediate_data,omitempty"`
	DurationNs       int64           `json:"duration_ns"`
}

// PipelineErrorResult is returned when a pipeline execution fails entirely.
type PipelineErrorResult struct {
	Error string `json:"error"`
}
