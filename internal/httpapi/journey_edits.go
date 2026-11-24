package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/model"
)

// Changes the user makes to a journey: ticking tasks and tests, and notes on people.

type progressReq struct {
	Item string `json:"item"`
	Done bool   `json:"done"`
}

func (s *Server) setProgress(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, ok := jobID(w, r)
	if !ok {
		return
	}
	var req progressReq
	if !readJSON(w, r, &req) {
		return
	}
	m := progressRe.FindStringSubmatch(req.Item)
	if m == nil {
		writeErr(w, http.StatusBadRequest, "Invalid item.")
		return
	}
	j, err := s.db.GetJob(r.Context(), u.ID, id)
	if errors.Is(err, db.ErrNotFound) || (err == nil && j.Journey == nil) {
		writeErr(w, http.StatusNotFound, "Not found.")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	// Only accept items that exist in this journey, so rows can't be spammed.
	valid := false
	for _, n := range j.Journey.Nodes {
		if n.ID != m[1] {
			continue
		}
		if m[2] == "test" {
			valid = n.Test.Kind != "none"
		} else {
			var idx int
			fmt.Sscanf(m[2], "%d", &idx)
			valid = idx < len(n.Tasks)
		}
	}
	if !valid {
		writeErr(w, http.StatusBadRequest, "Invalid item.")
		return
	}
	// A shared task is ticked in every journey it's linked to: changed lists them all.
	changed, err := s.db.SetProgress(r.Context(), u.ID, id, req.Item, req.Done)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "changed": changed})
}

func (s *Server) savePeople(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, ok := jobID(w, r)
	if !ok {
		return
	}
	var people model.People
	if !readJSON(w, r, &people) {
		return
	}
	if len(people) > 40 {
		writeErr(w, http.StatusBadRequest, "Too many entries.")
		return
	}
	for k, cards := range people {
		if !nodeIDRe.MatchString(k) || len(cards) > 20 {
			writeErr(w, http.StatusBadRequest, "Invalid entry.")
			return
		}
		for _, c := range cards {
			if utf8.RuneCountInString(c.Name) > 200 || utf8.RuneCountInString(c.Work) > 1000 || utf8.RuneCountInString(c.Fit) > 1000 {
				writeErr(w, http.StatusBadRequest, "A field is too long.")
				return
			}
		}
	}
	if _, err := s.db.GetJob(r.Context(), u.ID, id); errors.Is(err, db.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Not found.")
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	if err := s.db.SavePeople(r.Context(), u.ID, id, people); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
