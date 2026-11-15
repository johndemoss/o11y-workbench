package ottlengine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"github.com/johndemoss/o11y-workbench/internal/ottl"
)

// Worker protocol bounds. MaxWorkerRequestBytes must equal maxRequestBytes in
// workers/*/main.go; it stays above the largest payload the server can store
// (32 MiB HTTP bodies plus the JSON envelope). Responses carry per-stage
// intermediate data, so they get a larger, still explicit, ceiling.
const (
	MaxWorkerRequestBytes  = 128 << 20
	MaxWorkerResponseBytes = 512 << 20
)

// WorkerClient implements Engine over HTTP against an ottl worker process.
// Requests are bounded only by the caller's context.
type WorkerClient struct {
	version string
	stable  string
	baseURL string
	client  *http.Client
	done    <-chan struct{}
}

// NewWorkerClient constructs a WorkerClient for the given contrib train.
func NewWorkerClient(version, stable, baseURL string) *WorkerClient {
	return &WorkerClient{
		version: version,
		stable:  stable,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{},
	}
}

func (c *WorkerClient) Info() VersionInfo {
	return VersionInfo{
		Contrib: c.version,
		Stable:  c.stable,
		Source:  "worker",
	}
}

func (c *WorkerClient) post(ctx context.Context, path string, reqBody any, respBody any) error {
	b, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	if len(b) > MaxWorkerRequestBytes {
		return fmt.Errorf("ottl worker %s: request is %d bytes, exceeds the %d MiB worker limit", c.version, len(b), MaxWorkerRequestBytes>>20)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("create worker request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(ctx, req, respBody)
}

func (c *WorkerClient) get(ctx context.Context, path string, respBody any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create worker request: %w", err)
	}
	return c.do(ctx, req, respBody)
}

func (c *WorkerClient) do(ctx context.Context, req *http.Request, respBody any) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return c.transportErr(ctx, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxWorkerResponseBytes+1))
	if err != nil {
		return c.transportErr(ctx, fmt.Errorf("read body: %w", err))
	}
	if len(data) > MaxWorkerResponseBytes {
		return fmt.Errorf("ottl worker %s: response exceeds the %d MiB limit", c.version, MaxWorkerResponseBytes>>20)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ottl worker %s: %s", c.version, strings.TrimSpace(string(data)))
	}
	if respBody == nil {
		return nil
	}
	if err := json.Unmarshal(data, respBody); err != nil {
		return fmt.Errorf("ottl worker %s: decode: %w", c.version, err)
	}
	return nil
}

// transportErr reports the caller's cancellation as such, so errors.Is(err,
// context.Canceled) holds even when the transport surfaces a connection error.
func (c *WorkerClient) transportErr(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("ottl worker %s: %w", c.version, ctxErr)
	}
	return fmt.Errorf("ottl worker %s: %w", c.version, err)
}

func (c *WorkerClient) ValidateStatements(ctx context.Context, statements []string) ottl.ValidateResult {
	var res ottl.ValidateResult
	err := c.post(ctx, "/v1/validate", map[string]any{"statements": statements}, &res)
	if err != nil {
		return ottl.ValidateResult{Valid: false, Errors: []string{err.Error()}}
	}
	return res
}

func (c *WorkerClient) ValidateFilterConditions(ctx context.Context, conditions []string, signals gen.Signals) ottl.ValidateResult {
	var res ottl.ValidateResult
	err := c.post(ctx, "/v1/filter-validate", map[string]any{
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

func (c *WorkerClient) TransformTraces(ctx context.Context, data json.RawMessage, statements []string) (json.RawMessage, int, error) {
	var res transformResponse
	err := c.post(ctx, "/v1/transform", transformRequest{
		Signal: "traces", Data: data, Statements: statements,
	}, &res)
	if err != nil {
		return nil, 0, err
	}
	return res.Data, res.SpanCount, nil
}

func (c *WorkerClient) TransformLogs(ctx context.Context, data json.RawMessage, statements []string) (json.RawMessage, error) {
	var res transformResponse
	err := c.post(ctx, "/v1/transform", transformRequest{
		Signal: "logs", Data: data, Statements: statements,
	}, &res)
	if err != nil {
		return nil, err
	}
	return res.Data, nil
}

func (c *WorkerClient) TransformMetrics(ctx context.Context, data json.RawMessage, statements []string) (json.RawMessage, error) {
	var res transformResponse
	err := c.post(ctx, "/v1/transform", transformRequest{
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

func (c *WorkerClient) FilterTraces(ctx context.Context, data json.RawMessage, conditions []string, mode ottl.FilterMode) (json.RawMessage, int, int, error) {
	var res filterResponse
	err := c.post(ctx, "/v1/filter", filterRequest{
		Signal: "traces", Data: data, Conditions: conditions, Mode: mode,
	}, &res)
	if err != nil {
		return nil, 0, 0, err
	}
	return res.Data, res.Total, res.Kept, nil
}

func (c *WorkerClient) FilterLogs(ctx context.Context, data json.RawMessage, conditions []string, mode ottl.FilterMode) (json.RawMessage, int, int, error) {
	var res filterResponse
	err := c.post(ctx, "/v1/filter", filterRequest{
		Signal: "logs", Data: data, Conditions: conditions, Mode: mode,
	}, &res)
	if err != nil {
		return nil, 0, 0, err
	}
	return res.Data, res.Total, res.Kept, nil
}

func (c *WorkerClient) FilterMetrics(ctx context.Context, data json.RawMessage, conditions []string, mode ottl.FilterMode) (json.RawMessage, int, int, error) {
	var res filterResponse
	err := c.post(ctx, "/v1/filter", filterRequest{
		Signal: "metrics", Data: data, Conditions: conditions, Mode: mode,
	}, &res)
	if err != nil {
		return nil, 0, 0, err
	}
	return res.Data, res.Total, res.Kept, nil
}

func (c *WorkerClient) ExecutePipeline(ctx context.Context, data json.RawMessage, pipeline ottl.Pipeline) (*ottl.PipelineResult, error) {
	var res ottl.PipelineResult
	err := c.post(ctx, "/v1/pipeline/execute", map[string]any{
		"data":     data,
		"pipeline": pipeline,
	}, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *WorkerClient) ExecutePipelineForSignal(ctx context.Context, data json.RawMessage, pipeline ottl.Pipeline, signal string) (*ottl.PipelineResult, error) {
	var res ottl.PipelineResult
	body := map[string]any{
		"data":     data,
		"pipeline": pipeline,
	}
	if signal != "" {
		body["signal"] = signal
	}
	err := c.post(ctx, "/v1/pipeline/execute", body, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *WorkerClient) ImportCollectorConfig(ctx context.Context, configYAML string) (ottl.PipelinesBySignal, error) {
	var res ottl.PipelinesBySignal
	err := c.post(ctx, "/v1/pipeline/import-config", map[string]any{
		"config_yaml": configYAML,
	}, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (c *WorkerClient) ValidatePipelineOTTL(ctx context.Context, pipeline ottl.Pipeline) ottl.ValidateResult {
	var res ottl.ValidateResult
	err := c.post(ctx, "/v1/pipeline/validate-ottl", map[string]any{
		"pipeline": pipeline,
	}, &res)
	if err != nil {
		return ottl.ValidateResult{Valid: false, Errors: []string{err.Error()}}
	}
	return res
}

// exited reports whether the supervised worker process behind this client has
// been reaped. Clients without a supervisor never report exit.
func (c *WorkerClient) exited() bool {
	if c.done == nil {
		return false
	}
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// Health probes GET /v1/health and returns contrib/stable reported by the worker.
func (c *WorkerClient) Health(ctx context.Context) (contrib, stable string, err error) {
	var res struct {
		Contrib string `json:"contrib"`
		Stable  string `json:"stable"`
	}
	if err := c.get(ctx, "/v1/health", &res); err != nil {
		return "", "", err
	}
	return res.Contrib, res.Stable, nil
}
