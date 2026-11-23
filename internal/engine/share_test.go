package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/andriantam/ace-mate/internal/llm"
	"github.com/andriantam/ace-mate/internal/model"
	"github.com/andriantam/ace-mate/internal/model/modeltest"
)

// replies plays a model that returns the given replies in order.
type replies struct {
	out  []string
	seen []string
}

func (r *replies) Name() string   { return "fake" }
func (r *replies) Model() string  { return "fake-1" }
func (r *replies) Caps() llm.Caps { return llm.Caps{Structured: llm.StructuredPrompt} }
func (r *replies) Complete(_ context.Context, q llm.Request) (string, error) {
	r.seen = append(r.seen, q.Parts[len(q.Parts)-1].Text)
	o := r.out[0]
	if len(r.out) > 1 {
		r.out = r.out[1:]
	}
	return o, nil
}

func shareJourney() model.Journey {
	j := modeltest.Journey()
	j.Nodes[0].Tasks = []string{"Push a JS project to GitHub", "Write tests for it", "Read Acme's engineering blog"}
	for i := range j.Nodes {
		if j.Nodes[i].Aspect == "hr" {
			j.Nodes[i].Tasks = []string{}
			j.Nodes[i].HRQuestions = []model.HRQuestion{{Q: "What's your biggest weakness?", Tip: "t"}, {Q: "Why Acme?", Tip: "t"}}
			break
		}
	}
	// Keep the test small: only the first tech node and the first HR node.
	var keep []model.Node
	for _, n := range j.Nodes {
		if n.ID == "t1" || n.ID == "h1" {
			keep = append(keep, n)
		}
	}
	j.Nodes = keep
	return j
}

func TestShareItemsUseProgressRefs(t *testing.T) {
	items := ShareItems(shareJourney())
	var refs []string
	for _, it := range items {
		refs = append(refs, it.Kind+" "+it.Ref)
	}
	if got := strings.Join(refs, ","); got != "task t1:0,task t1:1,task t1:2,hr h1:q0,hr h1:q1" {
		t.Fatalf("refs = %s", got)
	}
}

func TestMatchShared(t *testing.T) {
	j := shareJourney()
	items := ShareItems(j)
	good := `{"track":"frontend","matches":[
		{"ref":"t1:0","key":"frontend/js-project-on-github","label":"Push a JS project to GitHub"},
		{"ref":"t1:1","key":"frontend/js-project-on-github","label":"dup"},
		{"ref":"t1:2","key":"","label":""},
		{"ref":"h1:q0","key":"hr/greatest-weakness","label":""},
		{"ref":"h1:q1","key":"","label":""}]}`

	t.Run("keeps shared items, drops specific ones and duplicates", func(t *testing.T) {
		f := &replies{out: []string{good}}
		e := New(f, 4000, 1, 3)
		known := []Known{{Kind: "hr", Key: "hr/greatest-weakness", Label: "What is your greatest weakness?", Jobs: 2}}
		got, err := e.MatchShared(context.Background(), j.Meta, items, known)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Ref != "t1:0" || got[1].Ref != "h1:q0" {
			t.Fatalf("got %+v", got)
		}
		if got[1].Label != "What's your biggest weakness?" {
			t.Fatalf("an empty label falls back to the wording: %q", got[1].Label)
		}
		if !strings.Contains(f.seen[0], "hr/greatest-weakness") || !strings.Contains(f.seen[0], `"jobs":2`) {
			t.Fatal("the prompt should carry the known items")
		}
	})

	t.Run("rejects an HR key on a task and asks again", func(t *testing.T) {
		bad := strings.Replace(good, `"key":"frontend/js-project-on-github","label":"Push`, `"key":"hr/js","label":"Push`, 1)
		f := &replies{out: []string{bad, good}}
		got, err := New(f, 4000, 1, 3).MatchShared(context.Background(), j.Meta, items, nil)
		if err != nil || len(got) != 2 || len(f.seen) != 2 {
			t.Fatalf("got %+v %v after %d calls", got, err, len(f.seen))
		}
		if !strings.Contains(f.seen[1], "only they, use keys starting with hr/") {
			t.Fatalf("the retry should explain the problem: %s", f.seen[1])
		}
	})

	t.Run("rejects keys that aren't slugs", func(t *testing.T) {
		bad := strings.Replace(good, "frontend/js-project-on-github", "Frontend JS project", 1)
		f := &replies{out: []string{bad, bad, bad}}
		if _, err := New(f, 4000, 1, 3).MatchShared(context.Background(), j.Meta, items, nil); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("nothing to match makes no call", func(t *testing.T) {
		f := &replies{out: []string{good}}
		if got, err := New(f, 4000, 1, 3).MatchShared(context.Background(), j.Meta, nil, nil); err != nil || got != nil || len(f.seen) != 0 {
			t.Fatalf("%v %v %d", got, err, len(f.seen))
		}
	})
}
