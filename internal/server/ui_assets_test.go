package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func uiTestServer(t *testing.T) *Server {
	t.Helper()
	s := &Server{mux: http.NewServeMux(), pipelineEnabled: true}
	if err := s.registerRoutes(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestEmbeddedUIEntryPoints(t *testing.T) {
	s := uiTestServer(t)
	refs := regexp.MustCompile(`(?:src|href)=["']([^"']+)["']`)
	for _, prefix := range []string{"/pipeline/"} {
		t.Run(prefix, func(t *testing.T) {
			w := httptest.NewRecorder()
			s.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, prefix, nil))
			if w.Code == http.StatusServiceUnavailable {
				if os.Getenv("O11Y_REQUIRE_BUILT_UI") == "1" {
					t.Fatal("release build did not embed the generated UI")
				}
				if !strings.Contains(w.Body.String(), "scripts/build-ui.sh") {
					t.Fatal("source-only build must explain how to build its UIs")
				}
				return
			}
			if w.Code != http.StatusOK {
				t.Fatalf("entry status=%d", w.Code)
			}
			js := 0
			for _, match := range refs.FindAllStringSubmatch(w.Body.String(), -1) {
				path := match[1]
				if !strings.HasPrefix(path, prefix) {
					continue
				}
				asset := httptest.NewRecorder()
				s.mux.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, path, nil))
				if asset.Code != http.StatusOK || strings.HasPrefix(asset.Header().Get("Content-Type"), "text/html") {
					t.Errorf("entry references unavailable asset %s (status=%d, content-type=%q)", path, asset.Code, asset.Header().Get("Content-Type"))
				}
				if strings.HasSuffix(path, ".js") {
					js++
				}
			}
			if js == 0 {
				t.Error("a successful UI response must include a built JavaScript entry point")
			}
		})
	}
}

func TestEmbeddedUISourceOnlyFallback(t *testing.T) {
	for _, prefix := range []string{"/pipeline/"} {
		h := embeddedUI(fstest.MapFS{}, prefix)
		for _, suffix := range []string{"", "index.html", "client/route/"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, prefix+suffix, nil))
			if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "scripts/build-ui.sh") {
				t.Errorf("unbuilt %s%s: status=%d, body=%s", prefix, suffix, w.Code, w.Body.String())
			}
		}
	}
}

func TestEmbeddedUIMissingAssetIsNotHTML(t *testing.T) {
	s := uiTestServer(t)
	for _, prefix := range []string{"/pipeline/"} {
		w := httptest.NewRecorder()
		s.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, prefix+"assets/does-not-exist.js", nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("missing %s asset: status=%d, want 404, not an HTML SPA fallback", prefix, w.Code)
		}
	}
}
