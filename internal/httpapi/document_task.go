package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/engine"
	"github.com/andriantam/ace-mate/internal/model"
	"github.com/andriantam/ace-mate/internal/sources"
	"github.com/andriantam/ace-mate/internal/worker"
)

const taskDocument = "document"

// runDocument reads one document with the AI and merges it into the user's profile. It
// runs on a background worker; the task's input is the file or text, and is cleared once
// the task finishes.
func (s *Server) runDocument(ctx context.Context, t *db.Task, emit func(map[string]any)) error {
	doc, err := s.db.GetDocument(ctx, t.UserID, t.Subject)
	if errors.Is(err, db.ErrNotFound) {
		return nil // removed while it waited
	}
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	in, title := engine.ResumeInput{Kind: doc.Kind}, ""
	switch doc.Kind {
	case "pdf":
		in.PDF = t.Input
	case "link":
		emit(map[string]any{"type": "progress", "step": "fetch", "detail": doc.Name})
		page, err := sources.Read(ctx, doc.URL)
		if err != nil {
			return worker.Permanent(err)
		}
		title, in.Text, in.Note = page.Title, page.Text, page.Note
		emit(map[string]any{"type": "progress", "step": "fetched", "detail": page.Title, "chars": len(page.Text)})
	default:
		in.Text = string(t.Input)
	}

	eng, llm, ok := s.pick(toolDocs, t.Engine)
	if !ok {
		return worker.Permanent(&engine.UserError{Msg: "The AI model you chose is no longer available. Add the document again and pick another."})
	}
	slog.Info("reading document", "doc", t.Subject, "choice", t.Engine, "model", llm.Model, "host", llm.Host())
	p, verification, err := eng.ParseResume(ctx, in, func(ev engine.Event) {
		emit(map[string]any{"type": "progress", "step": ev.Step, "detail": ev.Detail, "pages": ev.Pages,
			"method": ev.Method, "methods": ev.Methods, "ok": ev.OK})
	})
	if err != nil {
		return err
	}
	emit(map[string]any{"type": "progress", "step": "saving"})
	if err := s.db.FinishDocument(ctx, t.UserID, doc.ID, title, p, verification); err != nil {
		return err
	}
	if err := s.rebuildProfile(ctx, t.UserID); err != nil {
		return err
	}
	emit(map[string]any{"type": "done", "verification": verification})
	return nil
}

// documentGaveUp records that a document couldn't be read.
func (s *Server) documentGaveUp(ctx context.Context, t *db.Task, msg string) {
	_ = s.db.FailDocument(ctx, t.UserID, t.Subject, msg)
}

// rebuildProfile merges every finished document into the profile that journeys use.
// Resumes (files and pasted text) take priority over links, newest first.
func (s *Server) rebuildProfile(ctx context.Context, userID string) error {
	docs, err := s.db.ListDocuments(ctx, userID)
	if err != nil {
		return err
	}
	var files, links []model.Profile
	for i := len(docs) - 1; i >= 0; i-- {
		d := docs[i]
		if d.Status != "ready" || d.Extracted == nil {
			continue
		}
		if d.Kind == "link" {
			links = append(links, *d.Extracted)
		} else {
			files = append(files, *d.Extracted)
		}
	}
	all := append(files, links...)
	if len(all) == 0 {
		return s.db.DeleteProfile(ctx, userID)
	}
	return s.db.UpsertProfile(ctx, userID, engine.MergeProfiles(all))
}
