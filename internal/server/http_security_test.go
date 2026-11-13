package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestHTTPRejectsCrossOriginMutations(t *testing.T) {
	// Exercise the real serving entry point, not a test-only middleware stack.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	s := &Server{addr: addr, mux: http.NewServeMux()}
	s.mux.HandleFunc("/probe", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.ListenAndServeContext(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	client := &http.Client{Timeout: time.Second}
	url := "http://" + addr + "/probe"
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server not ready: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, tt := range []struct {
		name, origin, fetchSite string
		want                    int
	}{
		{name: "CLI without origin", want: http.StatusNoContent},
		{name: "same origin", origin: "http://" + addr, want: http.StatusNoContent},
		{name: "foreign origin", origin: "https://untrusted.example", want: http.StatusForbidden},
		{name: "opaque origin", origin: "null", want: http.StatusForbidden},
		{name: "cross-site fetch", fetchSite: "cross-site", want: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, url, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.fetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tt.fetchSite)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
			if resp.StatusCode != tt.want {
				t.Errorf("status=%d, want %d", resp.StatusCode, tt.want)
			}
		})
	}
}

func TestHTTPServerHasConnectionTimeouts(t *testing.T) {
	s := &Server{addr: "127.0.0.1:0", mux: http.NewServeMux()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.ListenAndServeContext(ctx); err != nil {
		t.Fatal(err)
	}
	if s.httpSrv.ReadHeaderTimeout <= 0 || s.httpSrv.IdleTimeout <= 0 {
		t.Fatal("HTTP connections must have header and idle timeouts")
	}
}
