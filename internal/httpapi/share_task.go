package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/engine"
	"github.com/andriantam/ace-mate/internal/worker"
)

// taskShare links a finished journey's tasks and HR questions to the same ones in the
// user's other journeys (see db.ApplyShareLinks). It runs after every journey is written,
// and once for every journey written before this feature existed.
const taskShare = "share"

func (s *Server) runShare(ctx context.Context, t *db.Task, _ func(map[string]any)) error {
	job, err := s.db.GetJob(ctx, t.UserID, t.Subject)
	if errors.Is(err, db.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if job.Status != "ready" || job.Journey == nil {
		return nil // still being written: linked when it's done
	}
	known, err := s.db.SharedCatalogue(ctx, t.UserID, t.Subject)
	if err != nil {
		return err
	}
	kn := make([]engine.Known, len(known))
	for i, k := range known {
		kn[i] = engine.Known{Kind: k.Kind, Key: k.Key, Label: k.Label, Jobs: len(k.Jobs)}
	}
	items := engine.ShareItems(*job.Journey)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	// The job's data goes only to the model the user picked (and confirmed) for this job.
	e, _, ok := s.pick(toolJourneys, job.Engine)
	if !ok {
		return worker.Permanent(&engine.UserError{Msg: "The model picked for this job is no longer available."})
	}
	matches, err := e.MatchShared(ctx, job.Journey.Meta, items, kn)
	if err != nil {
		return err
	}
	kind := map[string]string{}
	for _, it := range items {
		kind[it.Ref] = it.Kind
	}
	links := make([]db.ShareLink, 0, len(matches))
	for _, m := range matches {
		links = append(links, db.ShareLink{Ref: m.Ref, Kind: kind[m.Ref], Key: m.Key, Label: m.Label})
	}
	if err := s.db.ApplyShareLinks(ctx, t.UserID, t.Subject, links); err != nil {
		return err
	}
	slog.Info("journey linked to other jobs", "job", t.Subject, "items", len(items), "shared", len(links))
	return nil
}

// shareGaveUp records the attempt so it isn't repeated at every start. The user can ask
// for another pass from their FAQ.
func (s *Server) shareGaveUp(ctx context.Context, t *db.Task, msg string) {
	slog.Warn("linking a journey to other jobs failed", "job", t.Subject, "err", msg)
	if err := s.db.MarkShareSynced(ctx, t.UserID, t.Subject); err != nil {
		slog.Error("share: mark synced", "job", t.Subject, "err", err)
	}
}

// enqueueShare queues linking a job, unless that is already queued.
func (s *Server) enqueueShare(ctx context.Context, userID, jobID string) error {
	if s.journeys == nil {
		return nil
	}
	open, err := s.db.OpenTask(ctx, taskShare, jobID)
	if err != nil || open {
		return err
	}
	if _, err := s.db.Enqueue(ctx, db.NewTask{UserID: userID, Kind: taskShare, Subject: jobID}, nil); err != nil {
		return err
	}
	s.work.Wake(taskShare)
	return nil
}

// enqueueUnshared queues linking for every journey that hasn't been linked yet: on start-up
// this picks up the journeys written before shared prep existed, so nothing is lost.
func (s *Server) enqueueUnshared(ctx context.Context, userIDs []string) {
	for _, uid := range userIDs {
		ids, err := s.db.UnsyncedJobs(ctx, uid)
		if err != nil {
			slog.Warn("share: list unlinked jobs", "err", err)
			return
		}
		for _, id := range ids {
			if err := s.enqueueShare(ctx, uid, id); err != nil {
				slog.Warn("share: queue", "job", id, "err", err)
			}
		}
	}
}

// backfillShared links the journeys of every user that were written before this version.
func (s *Server) backfillShared(ctx context.Context) {
	if s.journeys == nil {
		return
	}
	users, err := s.db.UserIDs(ctx)
	if err != nil {
		slog.Warn("share: list users", "err", err)
		return
	}
	s.enqueueUnshared(ctx, users)
}
