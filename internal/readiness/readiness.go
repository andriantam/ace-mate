// Package readiness computes readiness scores. The UI has the same logic in
// web/src/readiness.ts so scores update instantly; readiness_test.go pins the numbers.
package readiness

import (
	"math"
	"sort"

	"github.com/andriantam/ace-mate/internal/model"
)

func clamp01(x float64) float64 {
	if math.IsNaN(x) {
		return 0
	}
	return math.Min(1, math.Max(0, x))
}

func hasTest(n model.Node) bool { return n.Test.Kind != "none" && n.Test.Kind != "" }

func Units(n model.Node) int {
	u := len(n.Tasks)
	if hasTest(n) {
		u++
	}
	return u
}

func doneUnits(n model.Node, p model.Progress) int {
	d := 0
	for i := range n.Tasks {
		if p[key(n.ID, i)] {
			d++
		}
	}
	if hasTest(n) && p[n.ID+":test"] {
		d++
	}
	return d
}

func key(id string, i int) string { return id + ":" + itoa(i) }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [12]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}

func NodeScore(n model.Node, p model.Progress) float64 {
	b := clamp01(n.Baseline)
	u := Units(n)
	if u == 0 {
		return b
	}
	return b + (1-b)*float64(doneUnits(n, p))/float64(u)
}

func IsDone(n model.Node, p model.Progress) bool { u := Units(n); return u > 0 && doneUnits(n, p) == u }

// Scores count only written nodes: placeholders of a journey still being built (or that
// failed) are left out until they arrive.

func AspectScore(j model.Journey, key string, p model.Progress) float64 {
	j = j.Written()
	var sum float64
	var count int
	for _, n := range j.Nodes {
		if n.Aspect == key {
			sum += NodeScore(n, p)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func hasAspect(j model.Journey, key string) bool {
	for _, n := range j.Written().Nodes {
		if n.Aspect == key {
			return true
		}
	}
	return false
}

func Overall(j model.Journey, p model.Progress) float64 {
	var total, w float64
	for _, a := range model.Aspects {
		if hasAspect(j, a.Key) {
			total += a.Weight * AspectScore(j, a.Key, p)
			w += a.Weight
		}
	}
	if w == 0 {
		return 0
	}
	return total / w
}

type AspectResult struct {
	Key   string  `json:"key"`
	Label string  `json:"label"`
	Score float64 `json:"score"`
}

func Aspects(j model.Journey, p model.Progress) []AspectResult {
	out := make([]AspectResult, 0, len(model.Aspects))
	for _, a := range model.Aspects {
		out = append(out, AspectResult{a.Key, a.Label, AspectScore(j, a.Key, p)})
	}
	return out
}

// BestNext is the first unfinished node in the weakest aspect.
func BestNext(j model.Journey, p model.Progress) *model.Node {
	j = j.Written()
	ranked := append([]model.Aspect(nil), model.Aspects...)
	sort.SliceStable(ranked, func(a, b int) bool {
		return AspectScore(j, ranked[a].Key, p) < AspectScore(j, ranked[b].Key, p)
	})
	for _, a := range ranked {
		for i := range j.Nodes {
			if j.Nodes[i].Aspect == a.Key && !IsDone(j.Nodes[i], p) {
				return &j.Nodes[i]
			}
		}
	}
	return nil
}
