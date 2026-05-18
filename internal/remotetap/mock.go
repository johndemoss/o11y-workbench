// Package remotetap provides a mock WebSocket server that streams NDJSON OTLP data,
// mimicking the real remotetap processor for testing purposes.
package remotetap

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// MockServer is a mock WebSocket server that streams OTLP telemetry as NDJSON
// lines to connected clients.
type MockServer struct {
	addr  string
	limit int // msgs/sec; 0 = unlimited

	listener net.Listener
	server   *http.Server
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	started  bool
}

// NewMockServer creates a mock server that listens on addr and streams up to
// limit messages per second. If limit <= 0, defaults to 1.
func NewMockServer(addr string, limit int) *MockServer {
	if limit <= 0 {
		limit = 1
	}
	return &MockServer{
		addr:  addr,
		limit: limit,
	}
}

// Start begins listening and serving WebSocket connections.
func (s *MockServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return fmt.Errorf("mock server already started")
	}

	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("mock server listen: %w", err)
	}
	s.listener = listener
	s.ctx, s.cancel = context.WithCancel(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWS)
	s.server = &http.Server{Handler: mux}

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("remotetap mock server error: %v", err)
		}
	}()

	s.started = true
	return nil
}

// Stop shuts down the server and all active connections.
func (s *MockServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		return nil
	}
	s.cancel()
	err := s.server.Close()
	s.started = false
	return err
}

// Addr returns the listening address. Useful when the port is dynamically
// assigned (e.g., ":0").
func (s *MockServer) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.addr
}

func (s *MockServer) handleWS(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	// Compute accept per RFC 6455
	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte(websocketGUID))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	conn, buf, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, fmt.Sprintf("hijack: %v", err), http.StatusInternalServerError)
		return
	}

	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := buf.WriteString(resp); err != nil {
		conn.Close()
		return
	}
	if err := buf.Flush(); err != nil {
		conn.Close()
		return
	}

	s.streamData(r.Context(), buf, conn)
}

func (s *MockServer) streamData(ctx context.Context, buf *bufio.ReadWriter, conn net.Conn) {
	defer conn.Close()

	rate := s.limit
	if rate <= 0 {
		rate = 1
	}
	ticker := time.NewTicker(time.Second / time.Duration(rate))
	defer ticker.Stop()

	seq := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			data := generateMessage(seq)
			if err := writeWSTextFrame(buf, data); err != nil {
				return
			}
			seq++
		}
	}
}

func generateMessage(seq int) []byte {
	switch seq % 3 {
	case 0:
		return generateTrace(seq)
	case 1:
		return generateLog(seq)
	case 2:
		return generateMetric(seq)
	}
	return nil
}

func generateTrace(seq int) []byte {
	svc := fmt.Sprintf("mock-svc-%d", seq%3)
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", svc)

	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("mock-scope")

	traceID := pcommon.TraceID([16]byte{byte(seq)})
	now := time.Now()

	span := ss.Spans().AppendEmpty()
	span.SetName(fmt.Sprintf("operation-%d", seq))
	span.SetSpanID(pcommon.SpanID([8]byte{byte(seq), 0, 0, 0, 0, 0, 0, 0}))
	span.SetTraceID(traceID)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(now))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(50 * time.Millisecond)))

	span2 := ss.Spans().AppendEmpty()
	span2.SetName(fmt.Sprintf("child-%d", seq))
	span2.SetSpanID(pcommon.SpanID([8]byte{byte(seq), 1, 0, 0, 0, 0, 0, 0}))
	span2.SetTraceID(traceID)
	span2.SetParentSpanID(pcommon.SpanID([8]byte{byte(seq), 0, 0, 0, 0, 0, 0, 0}))
	span2.SetStartTimestamp(pcommon.NewTimestampFromTime(now.Add(5 * time.Millisecond)))
	span2.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(45 * time.Millisecond)))

	m := &ptrace.JSONMarshaler{}
	b, err := m.MarshalTraces(traces)
	if err != nil {
		return nil
	}
	return append(b, '\n')
}

func generateLog(seq int) []byte {
	svc := fmt.Sprintf("mock-svc-%d", seq%3)
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", svc)

	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName("mock-scope")

	lr := sl.LogRecords().AppendEmpty()
	lr.SetSeverityNumber(plog.SeverityNumberInfo)
	lr.SetSeverityText("INFO")
	lr.Body().SetStr(fmt.Sprintf("log message %d", seq))
	lr.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	lr.SetTraceID(pcommon.TraceID([16]byte{byte(seq)}))
	lr.SetSpanID(pcommon.SpanID([8]byte{byte(seq), 0, 0, 0, 0, 0, 0, 0}))

	m := &plog.JSONMarshaler{}
	b, err := m.MarshalLogs(logs)
	if err != nil {
		return nil
	}
	return append(b, '\n')
}

func generateMetric(seq int) []byte {
	svc := fmt.Sprintf("mock-svc-%d", seq%3)
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", svc)

	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("mock-scope")

	metric := sm.Metrics().AppendEmpty()
	metric.SetName(fmt.Sprintf("requests-%d", seq))
	metric.SetDescription("mock request count")
	metric.SetUnit("1")

	sum := metric.SetEmptySum()
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	sum.SetIsMonotonic(true)

	dp := sum.DataPoints().AppendEmpty()
	dp.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(-1 * time.Minute)))
	dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	dp.SetIntValue(int64(seq * 100))

	m := &pmetric.JSONMarshaler{}
	b, err := m.MarshalMetrics(metrics)
	if err != nil {
		return nil
	}
	return append(b, '\n')
}

// writeWSTextFrame writes a WebSocket text frame (FIN=1, opcode=0x1) with the
// given payload. Server-to-client frames are not masked per RFC 6455.
func writeWSTextFrame(buf *bufio.ReadWriter, payload []byte) error {
	frame := []byte{0x81} // FIN=1, opcode=text

	length := len(payload)
	switch {
	case length <= 125:
		frame = append(frame, byte(length))
	case length <= 65535:
		frame = append(frame, 126, byte(length>>8), byte(length))
	default:
		frame = append(frame, 127)
		lenBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(lenBytes, uint64(length))
		frame = append(frame, lenBytes...)
	}

	frame = append(frame, payload...)

	if _, err := buf.Write(frame); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}
	if err := buf.Flush(); err != nil {
		return fmt.Errorf("flush frame: %w", err)
	}
	return nil
}
