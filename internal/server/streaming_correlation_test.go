package server

import (
	"context"
	"testing"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/gen"
)

// TestStreamingLogCorrelationDoesNotStarve verifies that an indefinite trace run
// streams generated span references into the log generator, so logs progress
// while traces keep running instead of waiting for a capture snapshot.
func TestStreamingLogCorrelationDoesNotStarve(t *testing.T) {
	store := NewRunStore()
	defer store.Close()

	cfg := gen.Config{
		Seed: 7,
		Signals: gen.Signals{
			Traces: true,
			Logs:   true,
		},
		TraceCount:  0, // unlimited until stopped
		Topology:    gen.TopologyFlat,
		SpanCount:   4,
		ServiceName: "corr",
		Rate:        200,
		LogConfig: gen.LogConfig{
			Enabled:     true,
			LogsPerSpan: 1,
			LogCount:    10,
		},
	}

	run, err := StartRun(context.Background(), store, cfg, nil, true, "file")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		snap := store.Get(run.ID)
		if snap == nil {
			t.Fatal("run disappeared")
		}
		if snap.LogCountCompleted > 0 {
			break
		}
		if snap.Status != RunStatusPending && snap.Status != RunStatusRunning {
			t.Fatalf("run finished without correlated logs: status=%s error=%s", snap.Status, snap.Error)
		}
		if time.Now().After(deadline) {
			t.Fatal("log generation did not progress while traces were running")
		}
		time.Sleep(25 * time.Millisecond)
	}

	store.StopRun(run.ID)

	stopDeadline := time.Now().Add(5 * time.Second)
	for {
		snap := store.Get(run.ID)
		if snap != nil && isTerminalStatus(snap.Status) {
			if snap.LogData == nil || snap.LogRecordCount == 0 {
				t.Fatalf("correlated logs were not retained: data=%v count=%d", snap.LogData != nil, snap.LogRecordCount)
			}
			if snap.LogCountCompleted == 0 {
				t.Fatal("no log progress was recorded")
			}
			return
		}
		if time.Now().After(stopDeadline) {
			t.Fatal("run did not stop")
		}
		time.Sleep(25 * time.Millisecond)
	}
}
