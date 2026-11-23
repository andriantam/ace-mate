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
		if len(got) != 2 || got[0].Ref != "h1:q0" || got[1].Ref != "t1:0" {
			t.Fatalf("got %+v", got)
		}
		if got[0].Label != "What's your biggest weakness?" {
			t.Fatalf("an empty label falls back to the wording: %q", got[0].Label)
		}
		// The HR call carries the known HR items and the common keys; the task call neither.
		if !strings.Contains(f.seen[0], "hr/greatest-weakness") || !strings.Contains(f.seen[0], `"jobs":2`) || !strings.Contains(f.seen[0], "<common_hr_keys>") {
			t.Fatal("the HR prompt should carry the known HR items")
		}
		if strings.Contains(f.seen[1], "hr/greatest-weakness") || strings.Contains(f.seen[1], "<common_hr_keys>") {
			t.Fatal("the task prompt should not carry HR items")
		}
	})

	t.Run("drops unusable entries instead of failing the whole reply", func(t *testing.T) {
		bad := strings.Replace(good, `"key":"frontend/js-project-on-github","label":"Push`, `"key":"hr/js","label":"Push`, 1) // HR key on a task
		bad = strings.Replace(bad, `{"ref":"t1:2"`, `{"ref":"zz:9","key":"general/x","label":"x"},{"ref":"t1:2"`, 1)          // unknown ref
		bad = strings.Replace(bad, `"hr/greatest-weakness"`, `"HR/Greatest Weakness "`, 1)                                    // repairable slip
		f := &replies{out: []string{bad}}
		got, err := New(f, 4000, 1, 3).MatchShared(context.Background(), j.Meta, items, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(f.seen) != 2 {
			t.Fatalf("one call for HR questions and one for tasks, got %d", len(f.seen))
		}
		// t1:0 is dropped (HR key on a task); t1:1 keeps the task key; h1:q0's key is repaired.
		var refs []string
		for _, m := range got {
			refs = append(refs, m.Ref+"="+m.Key)
		}
		if strings.Join(refs, ",") != "h1:q0=hr/greatest-weakness,t1:1=frontend/js-project-on-github" {
			t.Fatalf("got %v", refs)
		}
	})

	t.Run("asks again only when nothing in the reply is usable", func(t *testing.T) {
		junk := `{"track":"x","matches":[{"ref":"nope","key":"","label":""}]}`
		f := &replies{out: []string{junk, good, good}}
		got, err := New(f, 4000, 1, 3).MatchShared(context.Background(), j.Meta, items, nil)
		if err != nil || len(got) != 2 || len(f.seen) != 3 {
			t.Fatalf("got %+v %v after %d calls", got, err, len(f.seen))
		}
		if !strings.Contains(f.seen[1], "must use the refs given") {
			t.Fatalf("the retry should explain the problem: %s", f.seen[1])
		}
	})

	t.Run("one group failing keeps the other", func(t *testing.T) {
		junk := `{"track":"x","matches":[]}`
		f := &replies{out: []string{good, junk, junk, junk}} // HR succeeds, tasks never do
		got, err := New(f, 4000, 1, 3).MatchShared(context.Background(), j.Meta, items, nil)
		if err != nil || len(got) != 1 || got[0].Ref != "h1:q0" {
			t.Fatalf("got %+v %v", got, err)
		}
		f = &replies{out: []string{junk}}
		if _, err := New(f, 4000, 1, 3).MatchShared(context.Background(), j.Meta, items, nil); err == nil {
			t.Fatal("when every group fails, the error is returned")
		}
	})

	t.Run("nothing to match makes no call", func(t *testing.T) {
		f := &replies{out: []string{good}}
		if got, err := New(f, 4000, 1, 3).MatchShared(context.Background(), j.Meta, nil, nil); err != nil || got != nil || len(f.seen) != 0 {
			t.Fatalf("%v %v %d", got, err, len(f.seen))
		}
	})
}
