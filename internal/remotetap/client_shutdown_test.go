package remotetap

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestClientCanceledDial(t *testing.T) {
	peer := httptest.NewServer(websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		var msg string
		_ = websocket.Message.Receive(conn, &msg)
	}))
	defer peer.Close()
	client := NewClient(strings.TrimPrefix(peer.URL, "http://"))
	defer client.Disconnect()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.Connect(ctx); err == nil {
		t.Error("Connect ignored canceled admission context")
	}
}

func TestClientDisconnectJoinsBlockedErrorWriter(t *testing.T) {
	sent := make(chan struct{})
	peer := httptest.NewServer(websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		for range 30 {
			if err := websocket.Message.Send(conn, `invalid-json`); err != nil {
				break
			}
		}
		close(sent)
		var msg string
		_ = websocket.Message.Receive(conn, &msg)
	}))
	defer peer.Close()
	client := NewClient(strings.TrimPrefix(peer.URL, "http://"))
	if err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect()
	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatal("fixture did not send errors")
	}
	deadline := time.Now().Add(time.Second)
	for len(client.errCh) < cap(client.errCh) {
		if time.Now().After(deadline) {
			t.Fatal("error buffer did not fill")
		}
		time.Sleep(time.Millisecond)
	}
	done := make(chan struct{})
	go func() { _ = client.Disconnect(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Disconnect stuck on a full error channel")
	}
	select {
	case _, ok := <-client.Data():
		if ok {
			t.Error("unexpected data")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Disconnect returned without closing the data channel")
	}
}
