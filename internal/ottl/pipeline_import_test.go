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
		want    PipelinesBySignal
	}{
		{
			name: "full collector config with traces",
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
			want: PipelinesBySignal{
				"traces": {
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
		},
		{
			name: "multiple signal pipelines",
			config: `
receivers:
  otlp:
processors:
  transform/add_env:
    trace_statements:
      - context: span
        statements:
          - set(attributes["env"], "prod")
  filter/drop_internal:
    traces:
      span:
        - kind != SPAN_KIND_INTERNAL
exporters:
  debug:
service:
  pipelines:
    traces:
      processors: [transform/add_env]
    metrics:
      processors: [filter/drop_internal]
`,
			want: PipelinesBySignal{
				"traces": {
					Stages: []Stage{
						{
							ID:           "add_env",
							Type:         StageTypeTransform,
							ErrorMode:    ErrorModePropagate,
							ProcessorRef: "transform/add_env",
						},
					},
					Processors: map[string]ProcessorConfig{
						"transform/add_env": {
							"trace_statements": []interface{}{
								map[string]interface{}{
									"context":    "span",
									"statements": []interface{}{`set(attributes["env"], "prod")`},
								},
							},
						},
					},
				},
				"metrics": {
					Stages: []Stage{
						{
							ID:           "drop_internal",
							Type:         StageTypeFilter,
							ErrorMode:    ErrorModePropagate,
							ProcessorRef: "filter/drop_internal",
						},
					},
					Processors: map[string]ProcessorConfig{
						"filter/drop_internal": {
							"traces": map[string]interface{}{
								"span": []interface{}{"kind != SPAN_KIND_INTERNAL"},
							},
						},
					},
				},
			},
		},
		{
			name: "pipeline with suffix",
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
    traces/with_suffix:
      processors: [transform/foo]
`,
			want: PipelinesBySignal{
				"traces": {
					Stages: []Stage{
						{
							ID:           "foo",
							Type:         StageTypeTransform,
							ErrorMode:    ErrorModePropagate,
							ProcessorRef: "transform/foo",
						},
					},
					Processors: map[string]ProcessorConfig{
						"transform/foo": {
							"trace_statements": []interface{}{
								map[string]interface{}{
									"context":    "span",
									"statements": []interface{}{`set(attributes["x"], "y")`},
								},
							},
						},
					},
				},
			},
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
			wantErr: `processor "transform/bar" referenced in pipeline "traces"`,
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
			want: PipelinesBySignal{
				"traces": {
					Stages:     []Stage{},
					Processors: map[string]ProcessorConfig{},
				},
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

			if len(got) != len(tt.want) {
				t.Fatalf("signal count: got %d, want %d", len(got), len(tt.want))
			}

			for signalName, wantPipeline := range tt.want {
				gotPipeline, ok := got[signalName]
				if !ok {
					t.Fatalf("missing signal %q in result", signalName)
				}

				if len(gotPipeline.Stages) != len(wantPipeline.Stages) {
					t.Fatalf("signal %q stage count: got %d, want %d", signalName, len(gotPipeline.Stages), len(wantPipeline.Stages))
				}
				for i, wantStage := range wantPipeline.Stages {
					gotStage := gotPipeline.Stages[i]
					if gotStage.ID != wantStage.ID {
						t.Errorf("signal %q stage[%d].ID: got %q, want %q", signalName, i, gotStage.ID, wantStage.ID)
					}
					if gotStage.Type != wantStage.Type {
						t.Errorf("signal %q stage[%d].Type: got %q, want %q", signalName, i, gotStage.Type, wantStage.Type)
					}
					if gotStage.ErrorMode != wantStage.ErrorMode {
						t.Errorf("signal %q stage[%d].ErrorMode: got %q, want %q", signalName, i, gotStage.ErrorMode, wantStage.ErrorMode)
					}
					if gotStage.ProcessorRef != wantStage.ProcessorRef {
						t.Errorf("signal %q stage[%d].ProcessorRef: got %q, want %q", signalName, i, gotStage.ProcessorRef, wantStage.ProcessorRef)
					}
					if gotStage.Signals != nil {
						t.Errorf("signal %q stage[%d].Signals: got %v, want nil", signalName, i, gotStage.Signals)
					}
				}

				if len(gotPipeline.Processors) != len(wantPipeline.Processors) {
					t.Fatalf("signal %q processor count: got %d, want %d", signalName, len(gotPipeline.Processors), len(wantPipeline.Processors))
				}
				for ref, wantProc := range wantPipeline.Processors {
					gotProc, ok := gotPipeline.Processors[ref]
					if !ok {
						t.Errorf("signal %q missing processor %q", signalName, ref)
						continue
					}
					// compare serialized JSON to handle nested interface{} equality
					gotStr := fmt.Sprintf("%v", gotProc)
					wantStr := fmt.Sprintf("%v", wantProc)
					if gotStr != wantStr {
						t.Errorf("signal %q processor %q:\ngot  %v\nwant %v", signalName, ref, gotProc, wantProc)
					}
				}
			}
		})
	}
}
