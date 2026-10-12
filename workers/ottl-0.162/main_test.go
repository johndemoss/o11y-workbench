package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlePipelineImportConfig_ConfigYAML(t *testing.T) {
	body := map[string]string{
		"config_yaml": `service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: []
      exporters: [debug]
`,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/pipeline/import-config", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handlePipelineImportConfig(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"traces"`) {
		t.Fatalf("response missing traces: %s", rr.Body.String())
	}
}

func TestHandlePipelineImportConfig_LegacyConfigAlias(t *testing.T) {
	body := map[string]string{
		"config": `service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: []
      exporters: [debug]
`,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/pipeline/import-config", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handlePipelineImportConfig(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}
