package httpapi

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(c int) { w.status = c; w.ResponseWriter.WriteHeader(c) }

// Unwrap lets http.ResponseController reach the real writer (needed to flush streamed progress).
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			// Path only: query strings on the OAuth callback carry one-time codes.
			slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds())
		}
	})
}

func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				slog.Error("panic", "value", v, "path", r.URL.Path)
				writeErr(w, http.StatusInternalServerError, "Something went wrong.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	csp := strings.Join([]string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"font-src 'self'",
		"img-src 'self' data: https://lh3.googleusercontent.com",
		"connect-src 'self'",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"object-src 'none'",
	}, "; ")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), interest-cohort=()")
		if s.cfg.Secure() {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// sameOrigin is CSRF defence-in-depth on top of SameSite=Lax cookies: state-changing
// requests must come from our own origin.
func (s *Server) sameOrigin(next http.Handler) http.Handler {
	own := s.origin
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if o := r.Header.Get("Origin"); o != "" {
				u, err := url.Parse(o)
				if err != nil || u.Scheme+"://"+u.Host != own {
					writeErr(w, http.StatusForbidden, "Cross-origin request blocked.")
					return
				}
			} else if sfs := r.Header.Get("Sec-Fetch-Site"); sfs != "" && sfs != "same-origin" && sfs != "none" {
				writeErr(w, http.StatusForbidden, "Cross-origin request blocked.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
