package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/johndemoss/o11y-workbench/internal/gen"
)

func TestSignalsFromLegacyType(t *testing.T) {
	tests := []struct {
		in   string
		want gen.Signals
	}{
		{"", gen.Signals{Traces: true}},
		{"traces", gen.Signals{Traces: true}},
		{"logs", gen.Signals{Logs: true}},
		{"metrics", gen.Signals{Metrics: true}},
		{"both", gen.Signals{Traces: true, Logs: true}},
		{"all", gen.Signals{Traces: true, Logs: true, Metrics: true}},
		{"traces+metrics", gen.Signals{Traces: true, Metrics: true}},
		{"logs+metrics", gen.Signals{Logs: true, Metrics: true}},
		{"unknown", gen.Signals{Traces: true}},
	}
	for _, tt := range tests {
		if got := signalsFromLegacyType(tt.in); got != tt.want {
			t.Errorf("signalsFromLegacyType(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestHandleStartRunSignalSelection(t *testing.T) {
	tests := []struct {
		name   string
		body   map[string]any
		status int
		want   gen.Signals
	}{
		{
			name:   "legacy signal_type is translated",
			body:   map[string]any{"signal_type": "both"},
			status: http.StatusAccepted,
			want:   gen.Signals{Traces: true, Logs: true},
		},
		{
			name:   "signals win over legacy signal_type",
			body:   map[string]any{"signal_type": "metrics", "signals": map[string]bool{"logs": true}},
			status: http.StatusAccepted,
			want:   gen.Signals{Logs: true},
		},
		{
			name:   "absent selection defaults to traces",
			body:   map[string]any{},
			status: http.StatusAccepted,
			want:   gen.Signals{Traces: true},
		},
		{
			name:   "explicit empty signals are rejected",
			body:   map[string]any{"signals": map[string]bool{"traces": false}},
			status: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewRunStore()
			defer store.Close()
			s := &Server{store: store}
			body := map[string]any{
				"trace_count": 1, "rate": 1000, "topology": "flat", "span_count": 1, "output": "file",
			}
			for k, v := range tt.body {
				body[k] = v
			}
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			s.handleStartRun(w, httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader(data)))
			if w.Code != tt.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if tt.status != http.StatusAccepted {
				if runs := store.List(); len(runs) != 0 {
					t.Fatalf("rejected request created runs: %+v", runs)
				}
				return
			}
			var resp struct {
				ID     string         `json:"id"`
				Config map[string]any `json:"config"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			store.StopRun(resp.ID)
			if _, ok := resp.Config["signal_type"]; ok {
				t.Fatal("run config persists the legacy signal_type field")
			}
			signals, _ := resp.Config["signals"].(map[string]any)
			got := gen.Signals{
				Traces:  signals["traces"] == true,
				Logs:    signals["logs"] == true,
				Metrics: signals["metrics"] == true,
			}
			if got != tt.want {
				t.Fatalf("config signals = %+v, want %+v", got, tt.want)
			}
		})
	}
}
