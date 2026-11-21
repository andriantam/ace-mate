package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/andriantam/ace-mate/internal/db"
)

// progress answers a progress poll for a document or job: the events of its latest task
// after "after" (an opaque cursor the client sends back as "next"). Once the task is gone
// (finished tasks are purged after an hour), it answers from the subject's own status,
// via final, which returns nil while the subject is still in progress.
func (s *Server) progress(w http.ResponseWriter, r *http.Request, u *db.User, final func(ctx context.Context, id string) (map[string]any, error)) {
	id, ok := jobID(w, r)
	if !ok {
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	p, err := s.db.Progress(r.Context(), u.ID, id, after)
	if err != nil {
		serverError(w, err)
		return
	}
	if p.Found {
		writeJSON(w, http.StatusOK, map[string]any{"events": p.Events, "next": p.Next, "done": p.Done})
		return
	}
	ev, err := final(r.Context(), id)
	if errors.Is(err, db.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Not found.")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if ev == nil {
		writeJSON(w, http.StatusOK, map[string]any{"events": []any{}, "next": after, "done": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": []any{ev}, "next": after + 1, "done": true})
}
