package engine

import (
	"context"
	"errors"
	"math"
	"net/url"
	"strings"

	"github.com/andriantam/ace-mate/internal/llm"
	"github.com/andriantam/ace-mate/internal/model"
)

// Writing and cleaning up the nodes of a journey.

func nz[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func httpsURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != ""
}

// sanitizeNode clamps and cleans model output so the UI can trust it.
func sanitizeNode(n model.Node, ids map[string]bool) model.Node {
	n.Baseline = math.Min(0.6, math.Max(0, n.Baseline))
	links := []string{}
	for _, l := range n.Links {
		if ids[l] && l != n.ID {
			links = append(links, l)
		}
	}
	n.Links = links
	if len(n.Tasks) > 8 {
		n.Tasks = n.Tasks[:8]
	}
	n.Tasks = nz(n.Tasks)

	quiz := []model.QuizItem{}
	for _, q := range n.Test.Quiz {
		if len(q.Options) >= 2 && q.Correct >= 0 && q.Correct < len(q.Options) {
			q.Options = nz(q.Options)
			quiz = append(quiz, q)
		}
	}
	n.Test.Quiz = quiz
	n.Test.Rubric = nz(n.Test.Rubric)
	switch {
	case n.Test.Kind == "quiz" && len(quiz) == 0,
		n.Test.Kind == "scenario" && (len(n.Test.Rubric) == 0 || strings.TrimSpace(n.Test.Prompt) == ""),
		n.Test.Kind != "quiz" && n.Test.Kind != "scenario":
		n.Test.Kind = "none"
	}

	reading := []model.Reading{}
	for _, r := range n.Reading {
		if httpsURL(r.URL) && len(reading) < 5 {
			reading = append(reading, r)
		}
	}
	n.Reading = reading
	n.Bridge = nz(n.Bridge)
	n.HRQuestions = nz(n.HRQuestions)
	sections := []model.Section{}
	for _, s := range n.Sections {
		s.Items = nz(s.Items)
		sections = append(sections, s)
	}
	n.Sections = sections
	n.Project.Milestones = nz(n.Project.Milestones)
	if strings.TrimSpace(n.Project.Repo) == "" {
		n.Project = model.Project{Milestones: []string{}}
	}
	return n
}

// writeNodes writes the full nodes for some outline items of one aspect.
func (e *Engine) writeNodes(ctx context.Context, base, fitJSON string, titles []string, ids map[string]bool, aspect string, items []outlineItem) ([]model.Node, error) {
	planned, _ := jsonMarshal(items)
	prompt := base + "\n\n<fit>\n" + fitJSON + "\n</fit>\n\n<whole_journey_outline>\n" + strings.Join(titles, "\n") + "\n</whole_journey_outline>\n\n" +
		"Write the full nodes for exactly these outline items. Keep each id, title and order, and keep links to ids from the outline:\n" + planned + "\n\n" +
		aspectGuide[aspect] + "\nFields that do not apply: empty arrays, empty strings, project.repo empty, test.kind \"none\"."
	resp, err := structured(ctx, e, call{system: coachSystem, parts: []llm.Part{{Text: prompt}}, effort: llm.EffortMedium},
		func(r *nodesResp) error {
			if len(r.Nodes) == 0 {
				return errors.New("nodes is empty")
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	nodes := make([]model.Node, 0, len(items))
	for _, p := range items {
		var found *model.Node
		for k := range resp.Nodes {
			if resp.Nodes[k].ID == p.ID {
				found = &resp.Nodes[k]
				break
			}
		}
		if found == nil {
			for k := range resp.Nodes {
				if resp.Nodes[k].Title == p.Title {
					found = &resp.Nodes[k]
					break
				}
			}
		}
		if found == nil {
			continue
		}
		n := *found
		// The outline owns identity, order and cross-links.
		n.ID, n.Aspect, n.Title, n.Milestone, n.Links = p.ID, p.Aspect, p.Title, p.Milestone, p.Links
		nodes = append(nodes, sanitizeNode(n, ids))
	}
	return nodes, nil
}
