package ottl

import (
	"fmt"
	"strings"
	"testing"
)

func TestImportCollectorConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr string
		want    *Pipeline
	}{
		{
			name: "full collector config",
			config: `
receivers:
  otlp:
    protocols:
      grpc:
processors:
  transform/add_env:
    error_mode: ignore
    trace_statements:
      - context: span
        statements:
          - set(attributes["env"], "prod")
  filter/drop_internal:
    error_mode: propagate
    traces:
      span:
        - kind != SPAN_KIND_INTERNAL
exporters:
  debug:
    verbosity: detailed
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [transform/add_env, filter/drop_internal]
      exporters: [debug]
`,
			want: &Pipeline{
				Stages: []Stage{
					{
						ID:           "add_env",
						Type:         StageTypeTransform,
						ErrorMode:    ErrorModeIgnore,
						ProcessorRef: "transform/add_env",
					},
					{
						ID:           "drop_internal",
						Type:         StageTypeFilter,
						ErrorMode:    ErrorModePropagate,
						ProcessorRef: "filter/drop_internal",
					},
				},
				Processors: map[string]ProcessorConfig{
					"transform/add_env": {
						"error_mode": "ignore",
						"trace_statements": []interface{}{
							map[string]interface{}{
								"context":    "span",
								"statements": []interface{}{`set(attributes["env"], "prod")`},
							},
						},
					},
					"filter/drop_internal": {
						"error_mode": "propagate",
						"traces": map[string]interface{}{
							"span": []interface{}{"kind != SPAN_KIND_INTERNAL"},
						},
					},
				},
			},
		},
		{
			name: "missing service.pipelines.traces",
			config: `
receivers:
  otlp:
    protocols:
      grpc:
processors:
  transform/foo:
    trace_statements:
      - context: span
        statements:
          - set(attributes["x"], "y")
exporters:
  debug:
service:
  pipelines:
    metrics:
      processors: [transform/foo]
`,
			wantErr: "config missing 'service.pipelines.traces' section",
		},
		{
			name: "unknown processor reference",
			config: `
receivers:
  otlp:
processors:
  transform/foo:
    trace_statements:
      - context: span
        statements:
          - set(attributes["x"], "y")
exporters:
  debug:
service:
  pipelines:
    traces:
      processors: [transform/foo, transform/bar]
      exporters: [debug]
`,
			wantErr: `processor "transform/bar" referenced in pipeline but not defined in 'processors' section`,
		},
		{
			name: "empty processors list",
			config: `
receivers:
  otlp:
processors:
  transform/foo:
    trace_statements:
      - context: span
        statements:
          - set(attributes["x"], "y")
exporters:
  debug:
service:
  pipelines:
    traces:
      processors: []
      exporters: [debug]
`,
			want: &Pipeline{
				Stages:     []Stage{},
				Processors: map[string]ProcessorConfig{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ImportCollectorConfig(tt.config)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got.Stages) != len(tt.want.Stages) {
				t.Fatalf("stage count: got %d, want %d", len(got.Stages), len(tt.want.Stages))
			}
			for i, wantStage := range tt.want.Stages {
				gotStage := got.Stages[i]
				if gotStage.ID != wantStage.ID {
					t.Errorf("stage[%d].ID: got %q, want %q", i, gotStage.ID, wantStage.ID)
				}
				if gotStage.Type != wantStage.Type {
					t.Errorf("stage[%d].Type: got %q, want %q", i, gotStage.Type, wantStage.Type)
				}
				if gotStage.ErrorMode != wantStage.ErrorMode {
					t.Errorf("stage[%d].ErrorMode: got %q, want %q", i, gotStage.ErrorMode, wantStage.ErrorMode)
				}
				if gotStage.ProcessorRef != wantStage.ProcessorRef {
					t.Errorf("stage[%d].ProcessorRef: got %q, want %q", i, gotStage.ProcessorRef, wantStage.ProcessorRef)
				}
				if gotStage.Signals != nil {
					t.Errorf("stage[%d].Signals: got %v, want nil", i, gotStage.Signals)
				}
			}

			if len(got.Processors) != len(tt.want.Processors) {
				t.Fatalf("processor count: got %d, want %d", len(got.Processors), len(tt.want.Processors))
			}
			for ref, wantProc := range tt.want.Processors {
				gotProc, ok := got.Processors[ref]
				if !ok {
					t.Errorf("missing processor %q", ref)
					continue
				}
				if len(gotProc) != len(wantProc) {
					t.Errorf("processor %q: got %d keys, want %d keys", ref, len(gotProc), len(wantProc))
				}
				for k, wantVal := range wantProc {
					gotVal, ok := gotProc[k]
					if !ok {
						t.Errorf("processor %q missing key %q", ref, k)
						continue
					}
					gotStr := fmt.Sprintf("%v", gotVal)
					wantStr := fmt.Sprintf("%v", wantVal)
					if gotStr != wantStr {
						t.Errorf("processor %q key %q:\ngot  %v\nwant %v", ref, k, gotVal, wantVal)
					}
				}
			}
		})
	}
}
