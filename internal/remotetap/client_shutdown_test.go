package remotetap

import (
	"bytes"
	"context"
	"encoding/hex"
	"net"
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

func TestParseLineLogAndMetricIdentities(t *testing.T) {
	trace := "000102030405060708090a0b0c0d0e0f"
	span := "0a0b0c0d0e0f1011"
	wantTrace, _ := hex.DecodeString(trace)
	wantSpan, _ := hex.DecodeString(span)
	client := NewClient("unused")
	logLine := `{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"traceId":"` + trace + `","spanId":"` + span + `","timeUnixNano":1700000000000000001}]}]}]}`
	logs, err := client.parseLine([]byte(logLine))
	if err != nil {
		t.Fatal(err)
	}
	record := logs.Logs[0].ScopeLogs[0].LogRecords[0]
	if !bytes.Equal(record.TraceId, wantTrace) || !bytes.Equal(record.SpanId, wantSpan) || record.TimeUnixNano != 1700000000000000001 {
		t.Fatalf("log: %+v", record)
	}
	metricLine := `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"x","gauge":{"dataPoints":[{"asInt":"1","exemplars":[{"asInt":"1","traceId":"` + trace + `","spanId":"` + span + `"}]}]}}]}]}]}`
	metrics, err := client.parseLine([]byte(metricLine))
	if err != nil {
		t.Fatal(err)
	}
	exemplar := metrics.Metrics[0].ScopeMetrics[0].Metrics[0].GetGauge().DataPoints[0].Exemplars[0]
	if !bytes.Equal(exemplar.TraceId, wantTrace) || !bytes.Equal(exemplar.SpanId, wantSpan) {
		t.Fatalf("exemplar: %+v", exemplar)
	}
}

func TestMockStopClosesHijackedConnection(t *testing.T) {
	s := NewMockServer("127.0.0.1:0", 1000)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = conn.Write([]byte("GET / HTTP/1.1\r\nHost: localhost\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Key: dGVzdA==\r\n\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 512)
	if _, err := conn.Read(buf); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Stop() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not join handlers")
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestMockTraceIDsUnique(t *testing.T) {
	client := NewClient("unused")
	seen := make(map[string]bool)
	for i := range 1000 {
		data, err := client.parseLine(generateTrace(i))
		if err != nil {
			t.Fatal(err)
		}
		spans := data.Traces[0].ScopeSpans[0].Spans
		id := spans[0].TraceId
		if len(id) != 16 || bytes.Equal(id, make([]byte, 16)) || seen[string(id)] {
			t.Fatalf("invalid or duplicate trace id at %d: %x", i, id)
		}
		seen[string(id)] = true
		if len(spans[0].SpanId) != 8 || bytes.Equal(spans[0].SpanId, make([]byte, 8)) || bytes.Equal(spans[0].SpanId, spans[1].SpanId) {
			t.Fatalf("invalid span ids at %d", i)
		}
	}
}
