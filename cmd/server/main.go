// Command server runs AceMate: the JSON API, the background workers and the embedded React UI.
package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/andriantam/ace-mate/internal/auth"
	"github.com/andriantam/ace-mate/internal/config"
	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/engine"
	"github.com/andriantam/ace-mate/internal/httpapi"
	"github.com/andriantam/ace-mate/internal/llm"
	"github.com/andriantam/ace-mate/internal/version"
	"github.com/andriantam/ace-mate/web"
)

func main() {
	// "server healthcheck" probes a running server, for container health checks
	// (the runtime image has no shell or curl).
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	slog.Info("starting AceMate", "version", version.Version)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.DatabaseURL == "" {
		slog.Info("no DATABASE_URL: starting an embedded Postgres under " + cfg.DataDir + " (first run downloads it)")
	}
	d, err := db.Open(ctx, cfg.DatabaseURL, cfg.DataDir, cfg.Production)
	if err != nil {
		return err
	}
	defer d.Close()

	docs, err := buildEngine("documents", cfg.DocsLLM)
	if err != nil {
		return err
	}
	journeys, err := buildEngine("journeys", cfg.JourneyLLM)
	if err != nil {
		return err
	}

	a := auth.New(ctx, cfg, d)
	for _, p := range a.Providers {
		slog.Info("sign-in provider ready", "provider", p.ID)
	}
	if cfg.DevLogin {
		slog.Warn("DEV_LOGIN is on: /api/auth/dev signs in as any local user. Never enable this on a public server.")
	}
	if cfg.Production {
		slog.Warn("before serving the public, review the Terms of Service and Privacy Policy and adapt them to how you run AceMate: see docs/legal-checklist.md")
	}
	if len(a.Providers) == 0 && !cfg.DevLogin {
		slog.Warn("no sign-in provider is configured: set AUTH_GOOGLE_* or AUTH_OKTA_* (or DEV_LOGIN=true locally)")
	}

	ui, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return err
	}
	srv := httpapi.New(ctx, cfg, d, a, httpapi.Engines{Docs: docs, Journeys: journeys}, ui)

	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				d.PurgeExpiredSessions(ctx)
				d.PurgeTasks(ctx)
			}
		}
	}()

	hs := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    32 << 10,
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Addr, "url", cfg.BaseURL)
		errc <- hs.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		err := hs.Shutdown(sctx)
		srv.Wait() // workers hand unfinished tasks back to the queue
		return err
	}
	return nil
}

// buildEngine sets up the AI engine for one tool, or returns nil when no model is configured.
func buildEngine(tool string, l config.LLM) (*engine.Engine, error) {
	p, err := llm.New(l)
	if err != nil || p == nil {
		if p == nil && err == nil {
			slog.Warn("no LLM configured: adding documents and building journeys are disabled (see README)", "tool", tool)
		}
		return nil, err
	}
	e := engine.New(p, l.MaxTokens, l.Concurrency, l.BatchSize)
	e.ResumeMode = l.ResumeMode
	slog.Info("LLM configured", "tool", tool, "provider", p.Name(), "model", p.Model())
	return e, nil
}

// healthcheck asks the server on this machine whether it is ready. It returns the exit code.
func healthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + port + "/readyz")
	if err != nil {
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
