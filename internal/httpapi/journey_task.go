package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/engine"
	"github.com/andriantam/ace-mate/internal/model"
	"github.com/andriantam/ace-mate/internal/safefetch"
	"github.com/andriantam/ace-mate/internal/worker"
)

const taskJourney = "journey"

// runJourney writes a job's journey on a background worker. A job with no journey yet gets
// a new one. A job that already has one (a previous attempt was interrupted, or the user
// asked to retry the parts that failed) keeps its written nodes and only the missing ones
// are written.
func (s *Server) runJourney(ctx context.Context, t *db.Task, emit func(map[string]any)) error {
	job, err := s.db.GetJob(ctx, t.UserID, t.Subject)
	if errors.Is(err, db.ErrNotFound) {
		return nil // deleted while it waited
	}
	if err != nil {
		return err
	}
	eng, _, ok := s.pick(toolJourneys, job.Engine)
	if !ok {
		return worker.Permanent(&engine.UserError{Msg: "The AI model you chose is no longer available. Create the application again and pick another."})
	}
	profile, err := s.db.GetProfile(ctx, t.UserID)
	if err != nil {
		return err
	}
	if profile == nil {
		return worker.Permanent(&engine.UserError{Msg: "Add your resume first."})
	}
	// A journey is about ten model calls. Free and reasoning models can take a minute each.
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	events := journeyEvents(emit)
	partial := s.savePartial(ctx, t.UserID, t.Subject)
	started := time.Now()

	var j model.Journey
	if job.Journey == nil {
		jd := job.JDText
		if jd == "" && job.URL != "" {
			emit(map[string]any{"type": "progress", "step": "fetch", "detail": job.URL})
			if jd, err = safefetch.Fetch(ctx, job.URL); err != nil {
				return worker.Permanent(err)
			}
			if err := s.db.SetJobJD(ctx, t.UserID, t.Subject, jd); err != nil {
				return err
			}
		}
		emit(map[string]any{"type": "progress", "step": "fetched", "chars": len(jd)})
		if j, err = eng.GenerateJourney(ctx, profile.Data, jd, job.Notes, events, partial); err != nil {
			return err
		}
	} else {
		j = *job.Journey
		j.Failed, j.Pending = append(j.Failed, j.Pending...), nil
		j, err = eng.FillJourney(ctx, profile.Data, job.JDText, job.Notes, j, events, partial)
		if err != nil && len(j.Failed) > 0 {
			// Keep what was written; the next attempt carries on from here.
			s.savePartial(context.WithoutCancel(ctx), t.UserID, t.Subject)(j)
			return err
		}
	}
	if err := s.db.MarkJobReady(ctx, t.UserID, t.Subject, j); err != nil {
		return err
	}
	slog.Info("journey written", "job", t.Subject, "seconds", int(time.Since(started).Seconds()), "model", eng.P.Model(), "failedParts", len(j.Failed))
	emit(map[string]any{"type": "done"})
	return nil
}

// journeyGaveUp records the final failure. A journey that was partly written keeps its
// written parts, and the rest are marked failed so the user can retry just those.
func (s *Server) journeyGaveUp(ctx context.Context, t *db.Task, msg string) {
	job, err := s.db.GetJob(ctx, t.UserID, t.Subject)
	if err != nil {
		return
	}
	if job.Journey != nil {
		j := *job.Journey
		j.Failed, j.Pending = append(j.Failed, j.Pending...), nil
		if err := s.db.MarkJobReady(ctx, t.UserID, t.Subject, j); err != nil {
			slog.Error("journey: keep partial", "job", t.Subject, "err", err)
		}
		return
	}
	if err := s.db.MarkJobFailed(ctx, t.UserID, t.Subject, msg); err != nil {
		slog.Error("journey: mark failed", "job", t.Subject, "err", err)
	}
}

func journeyEvents(emit func(map[string]any)) func(engine.Event) {
	return func(ev engine.Event) {
		emit(map[string]any{"type": "progress", "step": ev.Step, "detail": ev.Detail, "nodes": ev.Pages,
			"done": ev.Done, "total": ev.Total, "aspect": ev.Aspect, "ok": ev.OK})
	}
}

// savePartial stores each snapshot, so the job page can show finished nodes while the rest
// are still being written.
func (s *Server) savePartial(ctx context.Context, userID, jobID string) func(model.Journey) {
	return func(j model.Journey) {
		if err := s.db.SaveJourneyPartial(ctx, userID, jobID, j); err != nil {
			slog.Warn("save partial journey", "job", jobID, "err", err)
		}
	}
}

// userMessage turns a task error into something safe to show the user.
func userMessage(err error) string {
	var ue *engine.UserError
	var fe *safefetch.ErrUser
	switch {
	case errors.As(err, &ue):
		return ue.Msg
	case errors.As(err, &fe):
		return fe.Msg
	}
	return "Something went wrong. Please try again."
}
