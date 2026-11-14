package ottlengine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/ottl"
)

// WorkerClient implements Engine over HTTP against an ottl worker process.
type WorkerClient struct {
	version string
	stable  string
	baseURL string
	client  *http.Client
}

// NewWorkerClient constructs a WorkerClient for the given contrib train.
func NewWorkerClient(version, stable, baseURL string) *WorkerClient {
	return &WorkerClient{
		version: version,
		stable:  stable,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *WorkerClient) Info() VersionInfo {
	return VersionInfo{
		Contrib: c.version,
		Stable:  c.stable,
		Source:  "worker",
	}
}

func (c *WorkerClient) post(path string, reqBody any, respBody any) error {
	var body io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	resp, err := c.client.Post(c.baseURL+path, "application/json", body)
	if err != nil {
		return fmt.Errorf("ottl worker %s: %w", c.version, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ottl worker %s: read body: %w", c.version, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ottl worker %s: %s", c.version, string(data))
	}
	if respBody == nil {
		return nil
	}
	if err := json.Unmarshal(data, respBody); err != nil {
		return fmt.Errorf("ottl worker %s: decode: %w", c.version, err)
	}
	return nil
}

func (c *WorkerClient) get(path string, respBody any) error {
	resp, err := c.client.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("ottl worker %s: %w", c.version, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ottl worker %s: read body: %w", c.version, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ottl worker %s: %s", c.version, string(data))
	}
	if respBody == nil {
		return nil
	}
	if err := json.Unmarshal(data, respBody); err != nil {
		return fmt.Errorf("ottl worker %s: decode: %w", c.version, err)
	}
	return nil
}

func (c *WorkerClient) ValidateStatements(statements []string) ottl.ValidateResult {
	var res ottl.ValidateResult
	err := c.post("/v1/validate", map[string]any{"statements": statements}, &res)
	if err != nil {
		return ottl.ValidateResult{Valid: false, Errors: []string{err.Error()}}
	}
	return res
}

func (c *WorkerClient) ValidateFilterConditions(conditions []string, signals gen.Signals) ottl.ValidateResult {
	var res ottl.ValidateResult
	err := c.post("/v1/filter-validate", map[string]any{
		"conditions": conditions,
		"signals":    signals,
	}, &res)
	if err != nil {
		return ottl.ValidateResult{Valid: false, Errors: []string{err.Error()}}
	}
	return res
}

type transformRequest struct {
	Signal     string          `json:"signal"`
	Data       json.RawMessage `json:"data"`
	Statements []string        `json:"statements"`
}

type transformResponse struct {
	Data      json.RawMessage `json:"data"`
	SpanCount int             `json:"span_count"`
}

func (c *WorkerClient) TransformTraces(data json.RawMessage, statements []string) (json.RawMessage, int, error) {
	var res transformResponse
	err := c.post("/v1/transform", transformRequest{
		Signal: "traces", Data: data, Statements: statements,
	}, &res)
	if err != nil {
		return nil, 0, err
	}
	return res.Data, res.SpanCount, nil
}

func (c *WorkerClient) TransformLogs(data json.RawMessage, statements []string) (json.RawMessage, error) {
	var res transformResponse
	err := c.post("/v1/transform", transformRequest{
		Signal: "logs", Data: data, Statements: statements,
	}, &res)
	if err != nil {
		return nil, err
	}
	return res.Data, nil
}

func (c *WorkerClient) TransformMetrics(data json.RawMessage, statements []string) (json.RawMessage, error) {
	var res transformResponse
	err := c.post("/v1/transform", transformRequest{
		Signal: "metrics", Data: data, Statements: statements,
	}, &res)
	if err != nil {
		return nil, err
	}
	return res.Data, nil
}

type filterRequest struct {
	Signal     string          `json:"signal"`
	Data       json.RawMessage `json:"data"`
	Conditions []string        `json:"conditions"`
	Mode       ottl.FilterMode `json:"mode"`
}

type filterResponse struct {
	Data  json.RawMessage `json:"data"`
	Total int             `json:"total"`
	Kept  int             `json:"kept"`
}

func (c *WorkerClient) FilterTraces(data json.RawMessage, conditions []string, mode ottl.FilterMode) (json.RawMessage, int, int, error) {
	var res filterResponse
	err := c.post("/v1/filter", filterRequest{
		Signal: "traces", Data: data, Conditions: conditions, Mode: mode,
	}, &res)
	if err != nil {
		return nil, 0, 0, err
	}
	return res.Data, res.Total, res.Kept, nil
}

func (c *WorkerClient) FilterLogs(data json.RawMessage, conditions []string, mode ottl.FilterMode) (json.RawMessage, int, int, error) {
	var res filterResponse
	err := c.post("/v1/filter", filterRequest{
		Signal: "logs", Data: data, Conditions: conditions, Mode: mode,
	}, &res)
	if err != nil {
		return nil, 0, 0, err
	}
	return res.Data, res.Total, res.Kept, nil
}

func (c *WorkerClient) FilterMetrics(data json.RawMessage, conditions []string, mode ottl.FilterMode) (json.RawMessage, int, int, error) {
	var res filterResponse
	err := c.post("/v1/filter", filterRequest{
		Signal: "metrics", Data: data, Conditions: conditions, Mode: mode,
	}, &res)
	if err != nil {
		return nil, 0, 0, err
	}
	return res.Data, res.Total, res.Kept, nil
}

func (c *WorkerClient) ExecutePipeline(data json.RawMessage, pipeline ottl.Pipeline) (*ottl.PipelineResult, error) {
	var res ottl.PipelineResult
	err := c.post("/v1/pipeline/execute", map[string]any{
		"data":     data,
		"pipeline": pipeline,
	}, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *WorkerClient) ExecutePipelineForSignal(data json.RawMessage, pipeline ottl.Pipeline, signal string) (*ottl.PipelineResult, error) {
	var res ottl.PipelineResult
	body := map[string]any{
		"data":     data,
		"pipeline": pipeline,
	}
	if signal != "" {
		body["signal"] = signal
	}
	err := c.post("/v1/pipeline/execute", body, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *WorkerClient) ImportCollectorConfig(configYAML string) (ottl.PipelinesBySignal, error) {
	var res ottl.PipelinesBySignal
	err := c.post("/v1/pipeline/import-config", map[string]any{
		"config_yaml": configYAML,
	}, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (c *WorkerClient) ValidatePipelineOTTL(pipeline ottl.Pipeline) ottl.ValidateResult {
	var res ottl.ValidateResult
	err := c.post("/v1/pipeline/validate-ottl", map[string]any{
		"pipeline": pipeline,
	}, &res)
	if err != nil {
		return ottl.ValidateResult{Valid: false, Errors: []string{err.Error()}}
	}
	return res
}

// Health probes GET /v1/health and returns contrib/stable reported by the worker.
func (c *WorkerClient) Health() (contrib, stable string, err error) {
	var res struct {
		Contrib string `json:"contrib"`
		Stable  string `json:"stable"`
	}
	if err := c.get("/v1/health", &res); err != nil {
		return "", "", err
	}
	return res.Contrib, res.Stable, nil
}
