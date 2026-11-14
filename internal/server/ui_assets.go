package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// embeddedUI serves a built SPA or an explicit source-only-build diagnostic.
// root contains a tracked README and an optional, entirely generated dist/.
func embeddedUI(root fs.FS, prefix string) http.Handler {
	files := http.StripPrefix(prefix, http.FileServer(http.FS(root)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, prefix)
		if name == "" {
			name = "index.html"
		}
		// Never return HTML for a missing JS/CSS/icon: that masks broken bundles.
		_, err := fs.Stat(root, "dist/"+name)
		if err != nil && name != "index.html" && (strings.HasPrefix(name, "assets/") || path.Ext(name) != "") {
			http.NotFound(w, r)
			return
		}
		if _, err := fs.Stat(root, "dist/index.html"); err != nil {
			http.Error(w, "UI assets are not built. Run ./scripts/build-ui.sh, then rebuild the Go web binary (or use ./scripts/serve.sh).", http.StatusServiceUnavailable)
			return
		}
		if err != nil { // SPA client route, not an absent static asset.
			name = "index.html"
		}
		if name == "index.html" {
			w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = prefix + "dist/" + name
		if name == "index.html" {
			r2.URL.Path = prefix + "dist/"
		}
		files.ServeHTTP(w, r2)
	})
}
