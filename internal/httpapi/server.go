// Package httpapi is the JSON API and static file server.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"

	"golang.org/x/time/rate"

	"github.com/andriantam/ace-mate/internal/auth"
	"github.com/andriantam/ace-mate/internal/config"
	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/engine"
	"github.com/andriantam/ace-mate/internal/legal"
	"github.com/andriantam/ace-mate/internal/worker"
)

type Server struct {
	cfg      *config.Config
	db       *db.DB
	auth     *auth.Auth
	docs     *engine.Engine // reads resumes, documents and links; nil when no LLM is configured
	journeys *engine.Engine // analyses job posts and writes journeys; nil when no LLM is configured
	options  []Option       // models users can pick per upload or job (see engines.go)
	ui       fs.FS
	origin   string
	ctx      context.Context

	work        *worker.Pool
	authLimit   *limiter
	apiLimit    *limiter
	resumeLimit *limiter
}

// Engines are the AI engines for each tool. They may share one model.
type Engines struct {
	Docs, Journeys *engine.Engine
	Options        []Option // optional: models users can choose between
}

// New builds the server and starts its background workers, which run until ctx is
// cancelled. Call Wait after cancelling to let them hand back unfinished work.
func New(ctx context.Context, cfg *config.Config, d *db.DB, a *auth.Auth, e Engines, ui fs.FS) *Server {
	u, _ := url.Parse(cfg.BaseURL)
	s := &Server{
		cfg: cfg, db: d, auth: a, docs: e.Docs, journeys: e.Journeys, options: e.Options, ui: ui, ctx: ctx,
		origin:      u.Scheme + "://" + u.Host,
		authLimit:   newLimiter(rate.Limit(1), 10),
		apiLimit:    newLimiter(rate.Limit(20), 60),
		resumeLimit: newLimiter(rate.Limit(1.0/120), 8), // documents: a burst of 8, then one every 2 minutes
	}
	if d != nil {
		s.work = worker.New(d,
			worker.Kind{Name: taskDocument, Concurrency: 4, Run: s.runDocument, GiveUp: s.documentGaveUp, Message: userMessage},
			worker.Kind{Name: taskJourney, Concurrency: 4, Run: s.runJourney, GiveUp: s.journeyGaveUp, Message: userMessage},
		)
		s.work.Start(ctx)
	}
	return s
}

// Wait blocks until the background workers have stopped (after ctx is cancelled).
func (s *Server) Wait() {
	if s.work != nil {
		s.work.Wait()
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)

	mux.Handle("GET /api/auth/login/{provider}", s.limitAuth(http.HandlerFunc(s.auth.Login)))
	mux.Handle("GET /api/auth/callback/{provider}", s.limitAuth(http.HandlerFunc(s.auth.Callback)))
	if s.cfg.DevLogin {
		mux.Handle("GET /api/auth/dev", s.limitAuth(http.HandlerFunc(s.auth.DevLogin)))
	}
	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		s.auth.Logout(w, r)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/session", s.session)

	// Allowed before the Terms are accepted: accepting them, and the user's own export and deletion.
	mux.Handle("POST /api/consent", s.signedIn(s.acceptTerms))
	mux.Handle("PUT /api/consent/marketing", s.signedIn(s.setMarketing))

	mux.Handle("GET /api/dashboard", s.authed(s.dashboard))
	mux.Handle("GET /api/documents", s.authed(s.listDocuments))
	mux.Handle("POST /api/documents", s.authed(s.addDocument))
	mux.Handle("GET /api/documents/{id}/progress", s.authed(s.documentProgress))
	mux.Handle("DELETE /api/documents/{id}", s.authed(s.deleteDocument))
	mux.Handle("POST /api/jobs", s.authed(s.createJob))
	mux.Handle("GET /api/jobs/{id}", s.authed(s.getJob))
	mux.Handle("GET /api/jobs/{id}/progress", s.authed(s.jobProgress))
	mux.Handle("DELETE /api/jobs/{id}", s.authed(s.deleteJob))
	mux.Handle("POST /api/jobs/{id}/retry", s.authed(s.retryJob))
	mux.Handle("POST /api/jobs/{id}/fill", s.authed(s.fillJob))
	mux.Handle("PUT /api/jobs/{id}/progress", s.authed(s.setProgress))
	mux.Handle("PUT /api/jobs/{id}/people", s.authed(s.savePeople))
	mux.Handle("GET /api/export", s.signedIn(s.export))
	mux.Handle("DELETE /api/account", s.signedIn(s.deleteAccount))

	mux.HandleFunc("GET /", s.serveUI)

	return s.recoverPanics(s.securityHeaders(s.logRequests(s.sameOrigin(mux))))
}

// ── helpers ──

type userHandler func(w http.ResponseWriter, r *http.Request, u *db.User)

// authed requires a signed-in user who has accepted the current Terms and Privacy Policy.
func (s *Server) authed(h userHandler) http.Handler {
	return s.signedIn(func(w http.ResponseWriter, r *http.Request, u *db.User) {
		if u.TermsVersion != legal.Version {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "Please accept the Terms of Service and Privacy Policy first.", "code": "consent_required",
			})
			return
		}
		h(w, r, u)
	})
}

// signedIn requires a signed-in user and rate-limits them. The user id used for every
// data access comes from here, i.e. from the server-side session.
func (s *Server) signedIn(h userHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := s.auth.CurrentUser(r)
		if u == nil {
			writeErr(w, http.StatusUnauthorized, "Please sign in.")
			return
		}
		if !s.apiLimit.allow(u.ID) {
			writeErr(w, http.StatusTooManyRequests, "Too many requests. Slow down.")
			return
		}
		h(w, r, u)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// readJSON decodes a small JSON body strictly.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request.")
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		writeErr(w, http.StatusBadRequest, "Invalid request.")
		return false
	}
	return true
}
