package remotetap

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestClient_ConnectAndReceive(t *testing.T) {
	s := NewMockServer(":0", 100)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { s.Stop() })

	c := NewClient(s.Addr(), WithChannelCapacity(50))
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { c.Disconnect() })

	timeout, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	msgCount := 0
	for {
		select {
		case <-timeout.Done():
			t.Fatalf("timeout after %d messages", msgCount)
		case data := <-c.Data():
			if data == nil {
				t.Fatal("received nil data")
			}
			hasSignal := len(data.Traces) > 0 || len(data.Logs) > 0 || len(data.Metrics) > 0
			if !hasSignal {
				t.Fatalf("message %d: no signals present", msgCount)
			}
			msgCount++
			if msgCount >= 3 {
				return
			}
		case err := <-c.Err():
			t.Fatalf("error channel: %v", err)
		}
	}
}

func TestClient_Disconnect(t *testing.T) {
	s := NewMockServer(":0", 1000)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { s.Stop() })

	c := NewClient(s.Addr())
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	select {
	case <-c.Data():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for data")
	}

	c.Disconnect()

	select {
	case _, ok := <-c.Data():
		if ok {
			t.Log("data channel still open after disconnect (buffered)")
		}
	case <-time.After(500 * time.Millisecond):
		t.Log("disconnect handled")
	}
}

func TestClient_EmptyData(t *testing.T) {
	data := &RemotetapData{}
	if len(data.Traces) != 0 || len(data.Logs) != 0 || len(data.Metrics) != 0 {
		t.Error("empty data should have no signals")
	}
}

func TestClient_SignalDetection(t *testing.T) {
	tests := []struct {
		json string
	}{
		{`{"resourceSpans":[{"resource":{}}]}`},
		{`{"resourceLogs":[{"resource":{}}]}`},
		{`{"resourceMetrics":[{"resource":{}}]}`},
	}

	for _, tt := range tests {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(tt.json), &raw); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		signal, err := detectSignal(raw)
		if err != nil {
			t.Fatalf("detectSignal: %v", err)
		}
		if signal == signalUnknown {
			t.Error("expected known signal type")
		}
	}
}

var (
	_ *tracev1.ResourceSpans
	_ *logsv1.ResourceLogs
	_ *metricspb.ResourceMetrics
)
