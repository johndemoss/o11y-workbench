package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"golang.org/x/net/websocket"
)

func waitShutdownCondition(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal(message)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestServerCloseStopsDetachedAndDeletedRuns(t *testing.T) {
	collector, endpoint := startRoutingCollector(t)
	store := NewRunStore()
	srv := &Server{store: store}
	cfg := gen.Config{Seed: 42, Rate: 100, SpanCount: 1, Topology: gen.TopologyFlat}
	var live []*Run
	for range 2 {
		requestCtx, cancel := context.WithCancel(context.Background())
		run, err := StartRun(requestCtx, store, cfg, []string{endpoint}, true, "otlp")
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		store.mu.RLock()
		raw := store.runs[run.ID]
		store.mu.RUnlock()
		live = append(live, raw)
		t.Cleanup(raw.Stop)
	}
	waitShutdownCondition(t, func() bool { return collector.traces.Load() >= 4 }, "request cancellation stopped a detached run")
	store.PauseRun(live[0].ID)
	store.Delete(live[1].ID) // Removing UI history must not orphan an executing job.
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	for _, run := range live {
		if got := run.snapshot().Status; got != RunStatusStopped {
			t.Errorf("after Close: %s status=%s, want stopped", run.ID, got)
		}
	}
	run, err := StartRun(context.Background(), store, cfg, []string{endpoint}, true, "otlp")
	if err == nil {
		store.StopRun(run.ID)
		t.Error("closed server admitted a new run")
	}
	waitShutdownCondition(t, func() bool { return collector.connections.Load() == 0 }, "Close retained OTLP connections")
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestServerCloseDisconnectsRemoteTAP(t *testing.T) {
	peerClosed := make(chan struct{})
	var peerCloseOnce sync.Once
	peer := httptest.NewServer(websocket.Handler(func(conn *websocket.Conn) {
		defer peerCloseOnce.Do(func() { close(peerClosed) })
		defer conn.Close()
		if err := websocket.Message.Send(conn, `{"resourceSpans":[{"scopeSpans":[{"spans":[{"name":"shutdown-fixture"}]}]}]}`); err != nil {
			return
		}
		var message string
		_ = websocket.Message.Receive(conn, &message)
	}))
	defer peer.Close()
	store := NewRunStore()
	manager := NewRemotetapManager(store)
	defer manager.Disconnect()
	run, err := manager.Connect(context.Background(), strings.TrimPrefix(peer.URL, "http://"), RemotetapFilters{MaxSpans: 10, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	waitShutdownCondition(t, func() bool { return store.Get(run.ID).SpanCount == 1 }, "TAP did not receive fixture")
	srv := &Server{store: store, remotetap: manager}
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	if connected, _, _, _, _ := manager.Status(); connected {
		t.Error("Close left TAP connected")
	}
	if snapshot := store.Get(run.ID); snapshot.Status != RunStatusCompleted || snapshot.TraceData == nil {
		t.Error("Close did not finalize captured trace data")
	}
	select {
	case <-peerClosed:
	case <-time.After(time.Second):
		t.Error("Close left TAP socket open")
	}
	_ = manager.Disconnect() // Close rejection must not merely mean "already connected".
	_, err = manager.Connect(context.Background(), strings.TrimPrefix(peer.URL, "http://"), RemotetapFilters{})
	if err == nil {
		t.Error("closed TAP manager admitted a connection")
	}
}
