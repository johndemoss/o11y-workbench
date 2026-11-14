package server

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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

func TestHTTPServerRejectsOversizedBody(t *testing.T) {
	if (&Server{}).httpBodyLimit() != defaultMaxHTTPBodyBytes {
		t.Fatalf("default body limit = %d, want %d", (&Server{}).httpBodyLimit(), defaultMaxHTTPBodyBytes)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	const limit = 64
	s := &Server{addr: addr, mux: http.NewServeMux(), maxBody: limit}
	s.mux.HandleFunc("/read", func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body exceeds limit"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.ListenAndServeContext(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	client := &http.Client{Timeout: 2 * time.Second}
	url := "http://" + addr + "/read"
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
	post := func(body []byte, contentLength int64) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if contentLength != 0 {
			req.ContentLength = contentLength
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}
	if got := post(bytes.Repeat([]byte("a"), limit), 0); got != http.StatusNoContent {
		t.Errorf("limit-sized body status=%d, want %d", got, http.StatusNoContent)
	}
	if got := post(bytes.Repeat([]byte("b"), limit+1), -1); got != http.StatusRequestEntityTooLarge {
		t.Errorf("chunked oversized body status=%d, want %d", got, http.StatusRequestEntityTooLarge)
	}
	called := false
	req := httptest.NewRequest(http.MethodPost, "/read", bytes.NewReader([]byte("x")))
	req.ContentLength = limit + 1
	rr := httptest.NewRecorder()
	limitRequestBody(limit, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})).ServeHTTP(rr, req)
	if called {
		t.Fatal("declared oversized body reached the handler")
	}
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("declared oversized body status=%d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
}
