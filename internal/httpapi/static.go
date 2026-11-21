package httpapi

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// ── static UI ──

func (s *Server) serveUI(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "Not found.")
		return
	}
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" {
		p = "index.html"
	}
	if strings.HasPrefix(p, "assets/") {
		if f, err := s.ui.Open(p); err == nil {
			f.Close()
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			http.ServeFileFS(w, r, s.ui, p)
			return
		}
		http.NotFound(w, r)
		return
	}
	if p != "index.html" {
		if f, err := s.ui.Open(p); err == nil {
			f.Close()
			http.ServeFileFS(w, r, s.ui, p)
			return
		}
	}
	// Single-page app: every other route serves the shell.
	idx, err := fs.ReadFile(s.ui, "index.html")
	if err != nil {
		http.Error(w, "The UI is not built. Run `make web`, then rebuild.", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(idx)
}
