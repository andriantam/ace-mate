package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/model"
)

// The profile's FAQ: HR questions asked across the user's jobs with the user's own answers,
// and the tasks their journeys share.

type prepPlace struct {
	JobID  string `json:"jobId"`
	Title  string `json:"title"`
	NodeID string `json:"nodeId"`
	Text   string `json:"text"`          // the task or question as this journey words it
	Tip    string `json:"tip,omitempty"` // HR questions: this journey's tip
}

type prepItem struct {
	ID     string      `json:"id"`
	Kind   string      `json:"kind"`
	Key    string      `json:"key"`
	Label  string      `json:"label"`
	Done   bool        `json:"done"`
	Answer string      `json:"answer"`
	Places []prepPlace `json:"places"`
}

func (s *Server) getPrep(w http.ResponseWriter, r *http.Request, u *db.User) {
	items, err := s.db.ListShared(r.Context(), u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	jobs, err := s.db.ListJobs(r.Context(), u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	journeys := map[string]*model.Journey{}
	for _, j := range jobs {
		journeys[j.ID] = j.Journey
	}
	unlinked, err := s.db.UnsyncedJobs(r.Context(), u.ID) // still to be compared with the rest
	if err != nil {
		serverError(w, err)
		return
	}
	out := make([]prepItem, 0, len(items))
	for _, it := range items {
		p := prepItem{ID: it.ID, Kind: it.Kind, Key: it.Key, Label: it.Label, Done: it.Done, Answer: it.Answer, Places: []prepPlace{}}
		for _, l := range it.Links {
			pl := prepPlace{JobID: l.JobID, Title: l.Title}
			pl.NodeID, pl.Text, pl.Tip = resolveRef(journeys[l.JobID], l.Ref)
			p.Places = append(p.Places, pl)
		}
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "unlinked": len(unlinked), "matching": s.journeys != nil})
}

// resolveRef finds the node and wording behind a link's ref.
func resolveRef(j *model.Journey, ref string) (nodeID, text, tip string) {
	nodeID, idx, ok := strings.Cut(ref, ":")
	if !ok || j == nil {
		return nodeID, "", ""
	}
	hr := strings.HasPrefix(idx, "q")
	i, err := strconv.Atoi(strings.TrimPrefix(idx, "q"))
	if err != nil {
		return nodeID, "", ""
	}
	for _, n := range j.Nodes {
		if n.ID != nodeID {
			continue
		}
		if hr && i < len(n.HRQuestions) {
			return nodeID, n.HRQuestions[i].Q, n.HRQuestions[i].Tip
		}
		if !hr && i < len(n.Tasks) {
			return nodeID, n.Tasks[i], ""
		}
	}
	return nodeID, "", ""
}

func itemID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.ToLower(r.PathValue("id"))
	if !uuidRe.MatchString(id) {
		writeErr(w, http.StatusNotFound, "Not found.")
		return "", false
	}
	return id, true
}

func (s *Server) setPrepAnswer(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, ok := itemID(w, r)
	if !ok {
		return
	}
	var req struct {
		Answer string `json:"answer"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if utf8.RuneCountInString(req.Answer) > 5000 {
		writeErr(w, http.StatusBadRequest, "Keep your answer under 5,000 characters.")
		return
	}
	err := s.db.SetSharedAnswer(r.Context(), u.ID, id, strings.TrimSpace(req.Answer))
	if errors.Is(err, db.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Not found.")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) setPrepDone(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, ok := itemID(w, r)
	if !ok {
		return
	}
	var req struct {
		Done bool `json:"done"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	changed, err := s.db.SetSharedDone(r.Context(), u.ID, id, req.Done)
	if errors.Is(err, db.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Not found.")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "changed": changed})
}

// resyncPrep compares all the user's journeys again, for example after the model changed.
// Ticks and answers are kept: links are only ever merged.
func (s *Server) resyncPrep(w http.ResponseWriter, r *http.Request, u *db.User) {
	if s.journeys == nil {
		writeErr(w, http.StatusServiceUnavailable, "Finding shared prep needs an AI model. See the README to configure one.")
		return
	}
	if err := s.db.ResetShareSync(r.Context(), u.ID); err != nil {
		serverError(w, err)
		return
	}
	s.enqueueUnshared(r.Context(), []string{u.ID})
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}
