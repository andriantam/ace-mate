package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/andriantam/ace-mate/internal/version"
)

// healthz answers whether the process is up (liveness). It never touches the database,
// so a database outage doesn't get the server restarted in a loop.
func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version.Version})
}

// readyz answers whether the server can do its work (readiness): the database responds.
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "database": "not configured"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "database": "unreachable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "database": "ok", "version": version.Version})
}
