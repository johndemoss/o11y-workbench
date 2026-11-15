package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johndemoss/o11y-workbench/internal/gen"
	"golang.org/x/net/websocket"
)

func TestServerConcurrentCloseJoinsAndRetainsError(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	sentinel := errors.New("fixture stop failed")
	srv := &Server{store: NewRunStore(), engineStops: []func() error{func() error { close(entered); <-release; return sentinel }}}
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { first <- srv.Close() }()
	<-entered
	go func() { second <- srv.Close() }()
	select {
	case <-second:
		t.Error("concurrent Close did not join cleanup")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	for _, ch := range []<-chan error{first, second} {
		select {
		case err := <-ch:
			if !errors.Is(err, sentinel) {
				t.Errorf("lost close error: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("Close did not finish")
		}
	}
}

func TestRunStoreCloseConcurrentAdmission(t *testing.T) {
	store := NewRunStore()
	var callers sync.WaitGroup
	for range 64 {
		callers.Go(func() {
			_, err := StartRun(context.Background(), store, gen.Config{Rate: 100, SpanCount: 1, Topology: gen.TopologyFlat, Signals: gen.Signals{Traces: true}}, nil, true, "file")
			if err != nil && !errors.Is(err, ErrRunStoreClosed) {
				t.Errorf("start: %v", err)
			}
		})
	}
	callers.Go(store.Close)
	callers.Wait()
	store.Close()
	for _, run := range store.List() {
		if !isTerminalStatus(run.Status) {
			t.Errorf("Close left run %s in %s", run.ID, run.Status)
		}
	}
	srv := &Server{store: store}
	response := httptest.NewRecorder()
	srv.handleStartRun(response, httptest.NewRequest("POST", "/api/runs", strings.NewReader(`{"rate":1,"span_count":1,"topology":"flat","output":"file"}`)))
	if response.Code != http.StatusServiceUnavailable {
		t.Errorf("closed admission HTTP status=%d, want 503", response.Code)
	}
}

func TestRemoteTAPReconnectAndConcurrentSnapshots(t *testing.T) {
	peer := httptest.NewServer(websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		_ = websocket.Message.Send(conn, `{"resourceSpans":[{"scopeSpans":[{"spans":[{"name":"reconnect-fixture"}]}]}]}`)
		var msg string
		_ = websocket.Message.Receive(conn, &msg)
	}))
	defer peer.Close()
	store := NewRunStore()
	manager := NewRemotetapManager(store)
	defer manager.Close()
	stopReading, readerDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stopReading:
				return
			default:
			}
			_ = store.List()
			_, _, _, _, _ = manager.Status()
			_ = manager.GetUsageStats()
		}
	}()
	defer func() { close(stopReading); <-readerDone }()
	for range 10 {
		ctx, cancel := context.WithCancel(context.Background())
		run, err := manager.Connect(ctx, strings.TrimPrefix(peer.URL, "http://"), RemotetapFilters{})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		waitShutdownCondition(t, func() bool { return store.Get(run.ID).SpanCount == 1 }, "reconnected TAP lost data")
		if run.SpanCount != 0 {
			t.Error("Connect returned live state instead of a snapshot")
		}
		if err := manager.StopByRunID(run.ID); err != nil {
			t.Fatal(err)
		}
		if snapshot := store.Get(run.ID); snapshot.Status != RunStatusCompleted || snapshot.TraceData == nil {
			t.Error("Disconnect returned before capture finalized")
		}
	}
}
