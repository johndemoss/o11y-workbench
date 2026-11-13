package server

import (
	"context"
	"testing"

	"github.com/johndemoss/o11y-workbench/internal/gen"
)

func TestStartRunReturnsDetachedSnapshot(t *testing.T) {
	store := NewRunStore()
	cfg := gen.Config{Seed: 42, TraceCount: 1, SpanCount: 1, Topology: gen.TopologyFlat}
	run, err := StartRun(context.Background(), store, cfg, nil, true, "file")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.StopRun(run.ID) })

	store.mu.RLock()
	live := store.runs[run.ID]
	store.mu.RUnlock()
	if run == live {
		t.Fatal("StartRun returned mutable store state; callers cannot safely encode the response")
	}
	// Changing the returned response must not affect the running job.
	run.Error = "caller-owned response"
	if got := store.Get(run.ID).Error; got == run.Error {
		t.Fatal("response mutation changed the stored run")
	}
}
