package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/andriantam/ace-mate/internal/auth"
	"github.com/andriantam/ace-mate/internal/config"
	"github.com/andriantam/ace-mate/internal/httpapi"
)

// These tests exercise routing, the security middleware and the signed-out paths.
// None of them reaches the database.
func newHandler(t *testing.T, devLogin bool) http.Handler {
	t.Helper()
	ui := fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>app</title>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	cfg := &config.Config{BaseURL: "http://localhost:3000", DevLogin: devLogin}
	// The router must build without a route conflict (Go's ServeMux panics on one).
	return httpapi.New(context.Background(), cfg, nil, &auth.Auth{}, httpapi.Engines{}, ui).Handler()
}

func do(h http.Handler, method, path string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSignedOutAPIIs401(t *testing.T) {
	h := newHandler(t, false)
	for _, p := range []string{"/api/dashboard", "/api/export", "/api/jobs/00000000-0000-0000-0000-000000000000"} {
		rec := do(h, "GET", p, nil)
		if rec.Code != 401 || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
			t.Errorf("%s → %d %s", p, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}

func TestUnknownAPIPathIsJSON404(t *testing.T) {
	rec := do(newHandler(t, false), "GET", "/api/nope", nil)
	if rec.Code != 404 || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
		t.Fatalf("%d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestCrossOriginWritesAreBlocked(t *testing.T) {
	h := newHandler(t, false)
	for _, hdr := range []map[string]string{{"Origin": "https://evil.example"}, {"Sec-Fetch-Site": "cross-site"}} {
		for _, m := range []string{"POST", "PUT", "DELETE"} {
			if rec := do(h, m, "/api/documents", hdr); rec.Code != 403 {
				t.Errorf("%s with %v → %d, want 403", m, hdr, rec.Code)
			}
		}
	}
	// Same-origin writes pass the origin check (and then need a session).
	if rec := do(h, "POST", "/api/documents", map[string]string{"Origin": "http://localhost:3000"}); rec.Code != 401 {
		t.Errorf("same-origin POST → %d, want 401", rec.Code)
	}
}

func TestStaticAndSPAFallback(t *testing.T) {
	h := newHandler(t, false)
	if rec := do(h, "GET", "/jobs/abc", nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>app</title>") {
		t.Errorf("SPA fallback → %d", rec.Code)
	}
	rec := do(h, "GET", "/assets/app.js", nil)
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("asset → %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec := do(h, "GET", "/assets/missing.js", nil); rec.Code != 404 {
		t.Errorf("missing asset → %d", rec.Code)
	}
	// The mux canonicalises "../" with a redirect; either way no file outside the UI is served.
	if rec := do(h, "GET", "/../../etc/passwd", nil); (rec.Code != 200 && rec.Code != 307) || strings.Contains(rec.Body.String(), "root:") {
		t.Errorf("path traversal → %d", rec.Code)
	}
	if rec := do(h, "GET", "/assets/..%2f..%2findex.html", nil); strings.Contains(rec.Body.String(), "root:") || rec.Code >= 500 {
		t.Errorf("encoded traversal → %d", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	rec := do(newHandler(t, false), "GET", "/", nil)
	for k, want := range map[string]string{
		"X-Frame-Options": "DENY", "X-Content-Type-Options": "nosniff",
	} {
		if rec.Header().Get(k) != want {
			t.Errorf("%s = %q", k, rec.Header().Get(k))
		}
	}
	csp := rec.Header().Get("Content-Security-Policy")
	for _, must := range []string{"default-src 'self'", "frame-ancestors 'none'", "object-src 'none'"} {
		if !strings.Contains(csp, must) {
			t.Errorf("CSP missing %q", must)
		}
	}
	if strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
		t.Error("CSP must not allow inline scripts")
	}
}

func TestDevLoginRouteOnlyWhenEnabled(t *testing.T) {
	if rec := do(newHandler(t, false), "GET", "/api/auth/dev", nil); rec.Code != 404 {
		t.Errorf("dev login must not exist when disabled, got %d", rec.Code)
	}
	if rec := do(newHandler(t, false), "GET", "/api/auth/login/google", nil); rec.Code != 404 {
		t.Errorf("unconfigured provider → %d, want 404", rec.Code)
	}
}

func TestHealthChecks(t *testing.T) {
	h := newHandler(t, false)
	if rec := do(h, "GET", "/healthz", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"version"`) {
		t.Fatalf("healthz: %d %s", rec.Code, rec.Body)
	}
	// No database: alive, but not ready for traffic.
	if rec := do(h, "GET", "/readyz", nil); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz without a database: %d", rec.Code)
	}
}
