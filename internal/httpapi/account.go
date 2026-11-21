package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/andriantam/ace-mate/internal/db"
)

// ── account ──

func (s *Server) export(w http.ResponseWriter, r *http.Request, u *db.User) {
	data, err := s.db.ExportAll(r.Context(), u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="acemate-export.json"`)
	writeJSON(w, http.StatusOK, data)
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request, u *db.User) {
	var req struct {
		Confirm string `json:"confirm"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Confirm != "DELETE" {
		writeErr(w, http.StatusBadRequest, "Type DELETE to confirm.")
		return
	}
	if err := s.db.DeleteUser(r.Context(), u.ID); err != nil {
		serverError(w, err)
		return
	}
	s.auth.ClearCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func serverError(w http.ResponseWriter, err error) {
	slog.Error("server error", "err", err)
	writeErr(w, http.StatusInternalServerError, "Something went wrong.")
}
