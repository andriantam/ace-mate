package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/model"
	"github.com/andriantam/ace-mate/internal/safefetch"
)

var (
	uuidRe     = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	nodeIDRe   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
	progressRe = regexp.MustCompile(`^([A-Za-z0-9_-]{1,40}):(\d{1,3}|test)$`)
)

func jobID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.ToLower(r.PathValue("id"))
	if !uuidRe.MatchString(id) {
		writeErr(w, http.StatusNotFound, "Not found.")
		return "", false
	}
	return id, true
}

// ── jobs ──

type newJobReq struct {
	URL    string `json:"url"`
	JDText string `json:"jdText"`
	Notes  string `json:"notes"`
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request, u *db.User) {
	var req newJobReq
	if !readJSON(w, r, &req) {
		return
	}
	req.URL, req.JDText, req.Notes = strings.TrimSpace(req.URL), strings.TrimSpace(req.JDText), strings.TrimSpace(req.Notes)
	if len(req.URL) > 2000 || utf8.RuneCountInString(req.JDText) > 60_000 || utf8.RuneCountInString(req.Notes) > 5_000 {
		writeErr(w, http.StatusBadRequest, "Check the form: a field is too long.")
		return
	}
	if req.URL == "" && utf8.RuneCountInString(req.JDText) < 200 {
		writeErr(w, http.StatusBadRequest, "Add a job link or paste the job description.")
		return
	}
	if req.URL != "" {
		if _, err := safefetch.Validate(req.URL); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if s.journeys == nil {
		writeErr(w, http.StatusServiceUnavailable, "Building journeys needs an AI model. See the README to configure one.")
		return
	}
	profile, err := s.db.GetProfile(r.Context(), u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if profile == nil {
		writeErr(w, http.StatusBadRequest, "Upload your resume first.")
		return
	}
	recent, pending, err := s.db.CountRecentGenerations(r.Context(), u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if recent >= s.cfg.DailyLimit {
		writeErr(w, http.StatusTooManyRequests, fmt.Sprintf("Daily limit of %d journeys reached. Try again tomorrow.", s.cfg.DailyLimit))
		return
	}
	if pending >= 2 {
		writeErr(w, http.StatusTooManyRequests, "You already have journeys being built. Wait for them to finish.")
		return
	}
	id, err := s.db.InsertJob(r.Context(), u.ID, db.NewJob{URL: req.URL, Notes: req.Notes, JDText: req.JDText})
	if err != nil {
		serverError(w, err)
		return
	}
	if !s.enqueueJourney(w, r, u.ID, id) {
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
}

// fillJob rewrites only the parts of a journey that couldn't be written.
func (s *Server) fillJob(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, ok := jobID(w, r)
	if !ok {
		return
	}
	if s.journeys == nil {
		writeErr(w, http.StatusServiceUnavailable, "No AI model is configured.")
		return
	}
	job, err := s.db.GetJob(r.Context(), u.ID, id)
	if errors.Is(err, db.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Not found.")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if job.Status != "ready" || job.Journey == nil || len(job.Journey.Failed) == 0 {
		writeErr(w, http.StatusConflict, "Nothing to retry in this journey.")
		return
	}
	if err := s.db.MarkJobPending(r.Context(), u.ID, id); err != nil {
		serverError(w, err)
		return
	}
	if !s.enqueueJourney(w, r, u.ID, id) {
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

// jobProgress returns the events of the job's journey task (see progress).
func (s *Server) jobProgress(w http.ResponseWriter, r *http.Request, u *db.User) {
	s.progress(w, r, u, func(ctx context.Context, id string) (map[string]any, error) {
		job, err := s.db.GetJob(ctx, u.ID, id)
		if err != nil {
			return nil, err
		}
		switch job.Status {
		case "ready":
			return map[string]any{"type": "done"}, nil
		case "failed":
			return map[string]any{"type": "error", "error": job.Error}, nil
		}
		return nil, nil
	})
}

// enqueueJourney queues writing (or finishing) a job's journey. It writes the error
// response itself and returns false if that failed.
func (s *Server) enqueueJourney(w http.ResponseWriter, r *http.Request, userID, jobID string) bool {
	if _, err := s.db.Enqueue(r.Context(), db.NewTask{UserID: userID, Kind: taskJourney, Subject: jobID},
		map[string]any{"type": "progress", "step": "queued"}); err != nil {
		serverError(w, err)
		return false
	}
	s.work.Wake(taskJourney)
	return true
}

type jobView struct {
	ID       string         `json:"id"`
	URL      string         `json:"url"`
	Title    string         `json:"title"`
	Status   string         `json:"status"`
	Error    string         `json:"error"`
	Journey  *model.Journey `json:"journey"`
	Progress model.Progress `json:"progress"`
	People   model.People   `json:"people"`
	// The tasks and HR questions shared with the user's other jobs, keyed by ref.
	Shared map[string]db.SharedRef `json:"shared"`
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, ok := jobID(w, r)
	if !ok {
		return
	}
	j, err := s.db.GetJob(r.Context(), u.ID, id) // RLS: another user's id is simply not found
	if errors.Is(err, db.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Not found.")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	shared, err := s.db.SharedRefs(r.Context(), u.ID, id)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, jobView{ID: j.ID, URL: j.URL, Title: j.Title, Status: j.Status, Error: j.Error, Journey: j.Journey, Progress: j.Progress, People: j.People, Shared: shared})
}

func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, ok := jobID(w, r)
	if !ok {
		return
	}
	if err := s.db.DeleteJob(r.Context(), u.ID, id); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) retryJob(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, ok := jobID(w, r)
	if !ok {
		return
	}
	j, err := s.db.GetJob(r.Context(), u.ID, id)
	if errors.Is(err, db.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Not found.")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if j.Status != "failed" || s.journeys == nil {
		writeErr(w, http.StatusConflict, "This journey can't be retried.")
		return
	}
	_, pending, err := s.db.CountRecentGenerations(r.Context(), u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if pending >= 2 {
		writeErr(w, http.StatusTooManyRequests, "You already have journeys being built. Wait for them to finish.")
		return
	}
	if err := s.db.MarkJobPending(r.Context(), u.ID, id); err != nil {
		serverError(w, err)
		return
	}
	if !s.enqueueJourney(w, r, u.ID, id) {
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}
