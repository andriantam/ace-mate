package readiness_test

import (
	"math"
	"testing"

	"github.com/andriantam/ace-mate/internal/model"
	"github.com/andriantam/ace-mate/internal/readiness"
)

func pct(x float64) int { return int(math.Round(x * 100)) }

// A small journey with hand-checked numbers. web/src/readiness.ts uses the same formula.
//
//	t1 tech     baseline 0.2, 2 tasks + quiz (3 units)
//	t2 tech     baseline 0,   1 task
//	i1 industry baseline 0.5, nothing to do (scores its baseline)
//	h1 hr       baseline 0.4, 1 task
//	(no people lane: it is left out of the weighting)
func journey() model.Journey {
	quiz := model.Test{Kind: "quiz", Quiz: []model.QuizItem{{Q: "q", Options: []string{"a", "b"}, Correct: 0}}}
	return model.Journey{Nodes: []model.Node{
		{ID: "t1", Aspect: "tech", Baseline: 0.2, Tasks: []string{"a", "b"}, Test: quiz},
		{ID: "t2", Aspect: "tech", Baseline: 0, Tasks: []string{"a"}, Test: model.Test{Kind: "none"}},
		{ID: "i1", Aspect: "industry", Baseline: 0.5, Test: model.Test{Kind: "none"}},
		{ID: "h1", Aspect: "hr", Baseline: 0.4, Tasks: []string{"a"}, Test: model.Test{Kind: "none"}},
	}}
}

func TestScores(t *testing.T) {
	j := journey()
	p := model.Progress{}
	// tech (0.2+0)/2 = 0.1, industry 0.5, hr 0.4 → (0.35·0.1 + 0.2·0.5 + 0.25·0.4) / 0.8 = 0.29375
	if got := pct(readiness.Overall(j, p)); got != 29 {
		t.Fatalf("initial overall = %d, want 29", got)
	}
	if n := readiness.BestNext(j, p); n == nil || n.ID != "t1" {
		t.Fatalf("best next = %v, want t1 (tech is weakest)", n)
	}
	p["t1:0"], p["t1:test"] = true, true
	// t1 = 0.2 + 0.8·2/3
	if got := pct(readiness.NodeScore(j.Nodes[0], p)); got != 73 {
		t.Fatalf("t1 = %d, want 73", got)
	}
	p["t1:1"] = true
	// t1 = 1, tech 0.5 → (0.175 + 0.1 + 0.1) / 0.8 = 0.46875
	if got := pct(readiness.Overall(j, p)); got != 47 {
		t.Fatalf("overall after progress = %d, want 47", got)
	}
	if n := readiness.BestNext(j, p); n == nil || n.ID != "h1" {
		t.Fatalf("best next = %v, want h1 (hr is now weakest)", n)
	}
}

func TestPlaceholdersDontCount(t *testing.T) {
	j := journey()
	j.Pending = []string{"t2"}
	// tech is just t1 (0.2) while t2 is still being written.
	if got := pct(readiness.AspectScore(j, "tech", model.Progress{})); got != 20 {
		t.Fatalf("tech = %d, want 20", got)
	}
	j.Pending, j.Failed = nil, []string{"i1"}
	for _, a := range readiness.Aspects(j, model.Progress{}) {
		if a.Key == "industry" && a.Score != 0 {
			t.Fatalf("a failed node must not score: %v", a)
		}
	}
}
