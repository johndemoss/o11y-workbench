package server

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func waitUntilHTTPReady(t *testing.T, client *http.Client, url string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server not ready: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func startListenerAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func runServerOnAddr(t *testing.T, s *Server) (client *http.Client, baseURL string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.ListenAndServeContext(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	client = &http.Client{Timeout: time.Second}
	baseURL = "http://" + s.addr
	waitUntilHTTPReady(t, client, baseURL+"/api/health")
	return client, baseURL
}

func assertSecurityHeaders(t *testing.T, h http.Header) {
	t.Helper()
	if got := h.Get("Content-Security-Policy"); got != contentSecurityPolicy {
		t.Errorf("CSP = %q, want %q", got, contentSecurityPolicy)
	}
	if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := h.Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy = %q, want no-referrer", got)
	}
	if got := h.Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
	}
}

func rawHTTPGet(t *testing.T, addr, hostLine, extraHeader string) (status int, headers http.Header) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if hostLine == "" {
		hostLine = "Host:\r\n"
	} else {
		hostLine = "Host: " + hostLine + "\r\n"
	}
	req := fmt.Sprintf("GET /api/health HTTP/1.1\r\n%s%s\r\n", hostLine, extraHeader)
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, resp.Header
}

func requestWithHost(t *testing.T, client *http.Client, method, url, host string, extra func(*http.Request)) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if host != "" {
		req.Host = host
	}
	if extra != nil {
		extra(req)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

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

func TestHTTPHostAllowsLoopbackAndConfiguredHosts(t *testing.T) {
	mappedPort := "18080"
	addr := startListenerAddr(t)
	srv, err := New(addr, false, []string{"allowed.example"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.Close()
	client, baseURL := runServerOnAddr(t, srv)
	hostPort := strings.TrimPrefix(addr, "127.0.0.1:")
	for _, host := range []string{
		"127.0.0.1:" + hostPort,
		"127.0.0.1:" + mappedPort,
		"localhost:" + hostPort,
		"[::1]:" + hostPort,
		"allowed.example:" + mappedPort,
	} {
		t.Run(host, func(t *testing.T) {
			resp := requestWithHost(t, client, http.MethodGet, baseURL+"/api/health", host, nil)
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusOK)
			}
		})
	}
}

func TestHTTPHostRejectsForeignHosts(t *testing.T) {
	addr := startListenerAddr(t)
	srv, err := New(addr, false, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.Close()
	client, baseURL := runServerOnAddr(t, srv)
	for _, tt := range []struct {
		name   string
		method string
		host   string
		extra  func(*http.Request)
	}{
		{
			name:   "GET foreign DNS",
			method: http.MethodGet,
			host:   "evil.example:18080",
			extra: func(req *http.Request) {
				req.Header.Set("Origin", "http://evil.example:18080")
			},
		},
		{
			name:   "POST foreign DNS",
			method: http.MethodPost,
			host:   "evil.example:18080",
			extra: func(req *http.Request) {
				req.Header.Set("Origin", "http://evil.example:18080")
			},
		},
		{name: "malformed host", method: http.MethodGet, host: "not a host!"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp := requestWithHost(t, client, tt.method, baseURL+"/api/health", tt.host, tt.extra)
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusForbidden)
			}
			assertSecurityHeaders(t, resp.Header)
		})
	}
	t.Run("empty host", func(t *testing.T) {
		status, hdr := rawHTTPGet(t, srv.addr, "", "")
		if status != http.StatusForbidden {
			t.Fatalf("status=%d, want %d", status, http.StatusForbidden)
		}
		assertSecurityHeaders(t, hdr)
	})
	t.Run("forwarded host only", func(t *testing.T) {
		status, hdr := rawHTTPGet(t, srv.addr, "", "X-Forwarded-Host: evil.example:18080\r\n")
		if status != http.StatusForbidden {
			t.Fatalf("status=%d, want %d", status, http.StatusForbidden)
		}
		assertSecurityHeaders(t, hdr)
	})
}

func TestHTTPHostPolicyOnDirectServerConstruction(t *testing.T) {
	addr := startListenerAddr(t)
	s := &Server{addr: addr, mux: http.NewServeMux()}
	s.mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	client, baseURL := runServerOnAddr(t, s)
	hostPort := strings.TrimPrefix(addr, "127.0.0.1:")
	resp := requestWithHost(t, client, http.MethodGet, baseURL+"/api/health", "127.0.0.1:"+hostPort, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("allowed host status=%d, want %d", resp.StatusCode, http.StatusOK)
	}
	resp2 := requestWithHost(t, client, http.MethodGet, baseURL+"/api/health", "evil.example:18080", nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign host status=%d, want %d", resp2.StatusCode, http.StatusForbidden)
	}
}

func TestHTTPHostWildcardBindAllowsIPLiteralNotDNS(t *testing.T) {
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	wildcardAddr := "0.0.0.0:" + port
	srv, err := New(wildcardAddr, false, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.Close()
	clientURL := "http://127.0.0.1:" + port
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServeContext(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	client := &http.Client{Timeout: time.Second}
	waitUntilHTTPReady(t, client, clientURL+"/api/health")
	resp := requestWithHost(t, client, http.MethodGet, clientURL+"/api/health", "203.0.113.7:"+port, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("IP literal host status=%d, want %d", resp.StatusCode, http.StatusOK)
	}
	resp2 := requestWithHost(t, client, http.MethodGet, clientURL+"/api/health", "foreign.example:"+port, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("DNS on wildcard bind status=%d, want %d", resp2.StatusCode, http.StatusForbidden)
	}
}

func TestHTTPNewRejectsInvalidAllowedHosts(t *testing.T) {
	for _, entry := range []string{"https://x", "a/b", "*", "host:1234"} {
		t.Run(entry, func(t *testing.T) {
			_, err := New("127.0.0.1:0", false, []string{entry})
			if err == nil {
				t.Fatal("expected configuration error")
			}
			if !strings.Contains(err.Error(), "host configuration") {
				t.Fatalf("error = %v, want host configuration wrapper", err)
			}
		})
	}
}

func TestHTTPSecurityResponseHeaders(t *testing.T) {
	addr := startListenerAddr(t)
	srv, err := New(addr, true, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.Close()
	client, baseURL := runServerOnAddr(t, srv)
	hostPort := strings.TrimPrefix(addr, "127.0.0.1:")
	host := "127.0.0.1:" + hostPort

	t.Run("dashboard", func(t *testing.T) {
		resp := requestWithHost(t, client, http.MethodGet, baseURL+"/", host, nil)
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		assertSecurityHeaders(t, resp.Header)
	})

	t.Run("api error", func(t *testing.T) {
		resp := requestWithHost(t, client, http.MethodGet, baseURL+"/api/runs/does-not-exist", host, nil)
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusNotFound)
		}
		assertSecurityHeaders(t, resp.Header)
	})

	t.Run("pipeline", func(t *testing.T) {
		resp := requestWithHost(t, client, http.MethodGet, baseURL+"/pipeline/", host, nil)
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		assertSecurityHeaders(t, resp.Header)
	})
}

func TestHTTPHostForbiddenIncludesSecurityHeaders(t *testing.T) {
	addr := startListenerAddr(t)
	srv, err := New(addr, false, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.Close()
	client, baseURL := runServerOnAddr(t, srv)
	resp := requestWithHost(t, client, http.MethodGet, baseURL+"/", fmt.Sprintf("evil.example:%s", strings.TrimPrefix(addr, "127.0.0.1:")), nil)
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	assertSecurityHeaders(t, resp.Header)
}

func TestHTTPAbsoluteDNSHosts(t *testing.T) {
	policy, err := buildHostPolicy("127.0.0.1:18080", []string{"Example.COM."})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		host string
		want bool
	}{
		{"localhost.", true}, {"localhost.:18080", true},
		{"example.com.", true}, {"EXAMPLE.COM.:1234", true},
		{"example.com..", false}, {"localhost..:18080", false},
	} {
		key, err := parseRequestHost(tc.host)
		got := err == nil && policy.allowedRequestHost(key)
		if got != tc.want {
			t.Errorf("host %q allowed=%v, error=%v; want %v", tc.host, got, err, tc.want)
		}
	}
	if _, err := buildHostPolicy("127.0.0.1:18080", []string{"example.com.."}); err == nil {
		t.Error("double trailing dot accepted in allowed host")
	}
}
