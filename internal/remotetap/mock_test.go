package remotetap

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestMockServer_StartStop(t *testing.T) {
	s := NewMockServer(":0", 10)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Stop(); err != nil {
			t.Logf("Stop: %v", err)
		}
	})

	conn, err := net.DialTimeout("tcp", s.Addr(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial server: %v", err)
	}
	conn.Close()

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	_, err = net.DialTimeout("tcp", s.Addr(), 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected dial to fail after stop, but it succeeded")
	}
}

func TestMockServer_StreamsData(t *testing.T) {
	s := NewMockServer(":0", 100)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Stop(); err != nil {
			t.Logf("Stop: %v", err)
		}
	})

	conn, err := dialWebSocket(s.Addr(), "/", 5*time.Second)
	if err != nil {
		t.Fatalf("dial WebSocket: %v", err)
	}
	defer conn.Close()

	for i := 0; i < 3; i++ {
		frame, err := readWSTextFrame(conn, 5*time.Second)
		if err != nil {
			t.Fatalf("read frame %d: %v", i, err)
		}

		if !json.Valid(frame[:len(frame)-1]) {
			t.Fatalf("frame %d is not valid JSON: %s", i, string(frame))
		}

		var v map[string]any
		if err := json.Unmarshal(frame[:len(frame)-1], &v); err != nil {
			t.Fatalf("frame %d unmarshal: %v", i, err)
		}

		switch i {
		case 0:
			if _, ok := v["resourceSpans"]; !ok {
				t.Errorf("frame %d: missing resourceSpans", i)
			}
		case 1:
			if _, ok := v["resourceLogs"]; !ok {
				t.Errorf("frame %d: missing resourceLogs", i)
			}
		case 2:
			if _, ok := v["resourceMetrics"]; !ok {
				t.Errorf("frame %d: missing resourceMetrics", i)
			}
		}
	}
}

func TestMockServer_DoubleStart(t *testing.T) {
	s := NewMockServer(":0", 1)
	if err := s.Start(); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer s.Stop()

	if err := s.Start(); err == nil {
		t.Fatal("expected error on second Start, got nil")
	}
}

func TestMockServer_MultipleClients(t *testing.T) {
	s := NewMockServer(":0", 50)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Stop(); err != nil {
			t.Logf("Stop: %v", err)
		}
	})

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn, err := dialWebSocket(s.Addr(), "/", 5*time.Second)
			if err != nil {
				t.Errorf("client %d: dial: %v", id, err)
				return
			}
			defer conn.Close()

			frame, err := readWSTextFrame(conn, 3*time.Second)
			if err != nil {
				t.Errorf("client %d: read: %v", id, err)
				return
			}
			if !json.Valid(frame) {
				t.Errorf("client %d: invalid JSON", id)
			}
		}(i)
	}
	wg.Wait()
}

func dialWebSocket(addr, path string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		conn.Close()
		return nil, fmt.Errorf("rand: %w", err)
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)

	req := fmt.Sprintf("GET %s HTTP/1.1\r\n"+
		"Host: %s\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\n"+
		"Sec-WebSocket-Version: 13\r\n\r\n", path, addr, key)

	if _, err := fmt.Fprint(conn, req); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write upgrade: %w", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read response: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, fmt.Errorf("expected 101, got %s", resp.Status)
	}

	expectedAccept := computeAcceptKey(key)
	gotAccept := resp.Header.Get("Sec-WebSocket-Accept")
	if gotAccept != expectedAccept {
		conn.Close()
		return nil, fmt.Errorf("accept mismatch: got %q, want %q", gotAccept, expectedAccept)
	}

	return conn, nil
}

func readWSTextFrame(conn net.Conn, timeout time.Duration) ([]byte, error) {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}

	header := make([]byte, 2)
	if _, err := conn.Read(header); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	opcode := header[0] & 0x0f
	if opcode != 1 {
		return nil, fmt.Errorf("expected text frame (opcode=1), got opcode=%d", opcode)
	}

	length := int64(header[1] & 0x7f)
	switch {
	case length == 126:
		buf := make([]byte, 2)
		if _, err := conn.Read(buf); err != nil {
			return nil, fmt.Errorf("read extended length: %w", err)
		}
		length = int64(binary.BigEndian.Uint16(buf))
	case length == 127:
		buf := make([]byte, 8)
		if _, err := conn.Read(buf); err != nil {
			return nil, fmt.Errorf("read extended length 64: %w", err)
		}
		length = int64(binary.BigEndian.Uint64(buf))
	}

	if length > 10*1024*1024 {
		return nil, fmt.Errorf("frame too large: %d bytes", length)
	}

	payload := make([]byte, length)
	if _, err := conn.Read(payload); err != nil {
		return nil, fmt.Errorf("read payload: %w", err)
	}

	return payload, nil
}

func computeAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte(websocketGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

