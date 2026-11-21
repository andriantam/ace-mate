package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/andriantam/ace-mate/internal/llm"
	"github.com/andriantam/ace-mate/internal/model"
)

// Writing a journey: an outline first, then the nodes in parallel batches.

type outlineItem struct {
	ID        string   `json:"id" jsonschema:"description=short id such as t1 or i1 or p1 or h1"`
	Aspect    string   `json:"aspect" jsonschema:"enum=tech,enum=industry,enum=people,enum=hr"`
	Title     string   `json:"title"`
	Milestone bool     `json:"milestone"`
	Links     []string `json:"links" jsonschema:"description=ids of 1 to 3 closely related nodes and preferably in other aspects"`
}

type outlineResp struct {
	Meta    model.JobMeta `json:"meta"`
	Fit     model.Fit     `json:"fit"`
	Outline []outlineItem `json:"outline"`
}

type nodesResp struct {
	Nodes []model.Node `json:"nodes"`
}

func contextBlock(profile model.Profile, jd, notes string) string {
	pj, _ := jsonMarshal(profile)
	if strings.TrimSpace(notes) == "" {
		notes = "(none)"
	}
	return "<candidate_profile>\n" + pj + "\n</candidate_profile>\n\n<job_posting>\n" + jd + "\n</job_posting>\n\n<candidate_notes>\n" + notes + "\n</candidate_notes>"
}

func validAspect(a string) bool {
	for _, x := range model.Aspects {
		if x.Key == a {
			return true
		}
	}
	return false
}

// GenerateJourney writes a journey in two phases: one call outlines the whole journey and
// assesses fit, then the nodes are written in small batches in parallel. report (optional)
// receives "outline", "outline_done" and one "batch_done" per batch; partial (optional)
// receives a usable snapshot of the journey after the outline and after every batch.
func (e *Engine) GenerateJourney(ctx context.Context, profile model.Profile, jd, notes string, report func(Event), partial func(model.Journey)) (model.Journey, error) {
	emit := func(ev Event) {
		if report != nil {
			report(ev)
		}
	}
	base := contextBlock(profile, jd, notes)
	emit(Event{Step: "outline"})

	outline, err := structured(ctx, e, call{
		system: coachSystem,
		parts:  []llm.Part{{Text: base + "\n\nAnalyse the candidate's fit and outline the journey: 16 to 20 nodes across tech, industry, people and hr, ordered as milestones within each aspect. Use ids t1.. i1.. p1.. h1... Mark 3 to 5 key milestones. Every aspect needs at least 2 nodes. fit.match is 0 to 100."}},
		effort: llm.EffortHigh,
	}, func(o *outlineResp) error {
		if len(o.Outline) < 8 || len(o.Outline) > 30 {
			return fmt.Errorf("outline has %d nodes, expected 16 to 20", len(o.Outline))
		}
		seen := map[string]bool{}
		per := map[string]int{}
		for _, it := range o.Outline {
			if it.ID == "" || seen[it.ID] || !validAspect(it.Aspect) || strings.TrimSpace(it.Title) == "" {
				return fmt.Errorf("outline item %q is invalid or duplicated", it.ID)
			}
			seen[it.ID] = true
			per[it.Aspect]++
		}
		for _, a := range model.Aspects {
			if per[a.Key] < 2 {
				return fmt.Errorf("aspect %s needs at least 2 nodes", a.Key)
			}
		}
		if strings.TrimSpace(o.Meta.Title) == "" {
			return errors.New("meta.title is empty")
		}
		return nil
	})
	if err != nil {
		return model.Journey{}, err
	}

	fit := outline.Fit
	fit.Match = int(math.Round(math.Min(100, math.Max(0, float64(fit.Match)))))
	fit.Strengths, fit.Gaps, fit.Risks = nz(fit.Strengths), nz(fit.Gaps), nz(fit.Risks)
	meta := outline.Meta
	meta.MustHave, meta.NiceToHave = nz(meta.MustHave), nz(meta.NiceToHave)

	return e.writeJourney(ctx, base, meta, fit, outline.Outline, outline.Outline, nil, emit, partial)
}

// FillJourney rewrites the nodes of a journey that couldn't be written (j.Failed),
// keeping everything else as it is.
func (e *Engine) FillJourney(ctx context.Context, profile model.Profile, jd, notes string, j model.Journey, report func(Event), partial func(model.Journey)) (model.Journey, error) {
	emit := func(ev Event) {
		if report != nil {
			report(ev)
		}
	}
	failed := map[string]bool{}
	for _, id := range j.Failed {
		failed[id] = true
	}
	var all, targets []outlineItem
	existing := map[string]model.Node{}
	for _, n := range j.Nodes {
		it := outlineItem{ID: n.ID, Aspect: n.Aspect, Title: n.Title, Milestone: n.Milestone, Links: n.Links}
		all = append(all, it)
		if failed[n.ID] {
			targets = append(targets, it)
		} else {
			existing[n.ID] = n
		}
	}
	if len(targets) == 0 {
		return j, nil
	}
	return e.writeJourney(ctx, contextBlock(profile, jd, notes), j.Meta, j.Fit, all, targets, existing, emit, partial)
}

// writeJourney writes the nodes for targets in small batches, in parallel, starting with
// the first milestones of every lane. After the outline and after every batch it hands
// partial() a snapshot: written nodes, placeholders for the rest, and which ids are still
// pending or have failed, so the user can start on what's ready. A batch that fails marks
// only its own nodes as failed; the journey fails only if nothing could be written.
func (e *Engine) writeJourney(ctx context.Context, base string, meta model.JobMeta, fit model.Fit, all, targets []outlineItem,
	existing map[string]model.Node, emit func(Event), partial func(model.Journey)) (model.Journey, error) {
	ids := map[string]bool{}
	titles := make([]string, 0, len(all))
	for _, it := range all {
		ids[it.ID] = true
		titles = append(titles, it.ID+": "+it.Title+" ("+it.Aspect+")")
	}
	fitJSON, _ := jsonMarshal(fit)

	// Batches per lane, then interleaved so every lane's first milestones come first.
	type batch struct {
		aspect string
		items  []outlineItem
	}
	var lanes [][]batch
	for _, a := range model.Aspects {
		var items []outlineItem
		for _, it := range targets {
			if it.Aspect == a.Key {
				items = append(items, it)
			}
		}
		var lane []batch
		for len(items) > 0 {
			n := min(e.BatchSize, len(items))
			lane = append(lane, batch{a.Key, items[:n]})
			items = items[n:]
		}
		lanes = append(lanes, lane)
	}
	var batches []batch
	for round := 0; ; round++ {
		added := false
		for _, lane := range lanes {
			if round < len(lane) {
				batches = append(batches, lane[round])
				added = true
			}
		}
		if !added {
			break
		}
	}

	var mu sync.Mutex
	written := map[string]model.Node{}
	for id, n := range existing {
		written[id] = n
	}
	failed := map[string]bool{}
	var lastErr error
	done := 0

	// snapshot must be called with mu held.
	snapshot := func() model.Journey {
		nodes := make([]model.Node, 0, len(all))
		pending, failedIDs := []string{}, []string{}
		for _, a := range model.Aspects {
			for _, it := range all {
				if it.Aspect != a.Key {
					continue
				}
				if n, ok := written[it.ID]; ok {
					nodes = append(nodes, n)
					continue
				}
				nodes = append(nodes, sanitizeNode(model.Node{ID: it.ID, Aspect: it.Aspect, Title: it.Title, Milestone: it.Milestone, Links: it.Links}, ids))
				if failed[it.ID] {
					failedIDs = append(failedIDs, it.ID)
				} else {
					pending = append(pending, it.ID)
				}
			}
		}
		return model.Journey{Meta: meta, Fit: fit, Nodes: nodes, Pending: pending, Failed: failedIDs}
	}
	share := func() {
		if partial != nil {
			partial(snapshot())
		}
	}

	mu.Lock()
	emit(Event{Step: "outline_done", Total: len(batches), Pages: len(all), Detail: meta.Title})
	share()
	mu.Unlock()

	var g errgroup.Group // not WithContext: one failed batch must not cancel the others
	g.SetLimit(e.Concurrency)
	for _, b := range batches {
		g.Go(func() error {
			nodes, err := e.writeNodes(ctx, base, fitJSON, titles, ids, b.aspect, b.items)
			// A reasoning model can run out of output tokens on a batch. Rather than give up
			// on it, write that batch again one node at a time.
			var ue *UserError
			if errors.As(err, &ue) && ue.Truncated && len(b.items) > 1 {
				nodes, err = nil, nil
				for _, it := range b.items {
					one, oneErr := e.writeNodes(ctx, base, fitJSON, titles, ids, b.aspect, []outlineItem{it})
					if oneErr != nil {
						err = oneErr
						continue
					}
					nodes = append(nodes, one...)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			for _, n := range nodes {
				written[n.ID] = n
			}
			for _, it := range b.items {
				if _, ok := written[it.ID]; !ok {
					failed[it.ID] = true // not returned by the model, or its call failed
				}
			}
			if err != nil {
				lastErr = err
				slog.Warn("journey batch failed", "aspect", b.aspect, "err", err)
			}
			done++
			emit(Event{Step: "batch_done", Done: done, Total: len(batches), Aspect: b.aspect, OK: err == nil})
			share()
			return nil
		})
	}
	_ = g.Wait()

	mu.Lock()
	defer mu.Unlock()
	j := snapshot()
	if len(j.Nodes)-len(j.Failed) < 4 || len(written) == len(existing) && len(targets) > 0 {
		if lastErr != nil {
			return j, lastErr
		}
		return j, &UserError{Msg: "The generated journey was incomplete. Try again."}
	}
	return j, nil
}

// nz turns nil slices into empty ones so they serialise as [] and not null.
