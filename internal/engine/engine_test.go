package engine

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/andriantam/ace-mate/internal/llm"
	"github.com/andriantam/ace-mate/internal/model"
	"github.com/andriantam/ace-mate/internal/model/modeltest"
)

// fake plays the model: it answers the outline request and each node batch using the
// test journey, and can be told to fail its first reply.
type fake struct {
	caps       llm.Caps
	journey    model.Journey
	badFirst   atomic.Bool
	truncMulti atomic.Bool  // fail any node batch of more than one item as "out of output tokens"
	failAspect atomic.Value // string: fail every node batch for this aspect
	calls      atomic.Int32
	sawSchema  atomic.Bool
	sawPrompts []string
}

func (f *fake) Name() string   { return "fake" }
func (f *fake) Model() string  { return "fake-1" }
func (f *fake) Caps() llm.Caps { return f.caps }

func (f *fake) Complete(_ context.Context, r llm.Request) (string, error) {
	f.calls.Add(1)
	if r.Schema != nil {
		f.sawSchema.Store(true)
	}
	if f.badFirst.CompareAndSwap(true, false) {
		return "I'm sorry, here is some prose instead of JSON.", nil
	}
	text := ""
	for _, p := range r.Parts {
		text += p.Text
	}
	if strings.Contains(text, "outline the journey") {
		o := outlineResp{Meta: f.journey.Meta, Fit: f.journey.Fit}
		for _, n := range f.journey.Nodes {
			o.Outline = append(o.Outline, outlineItem{ID: n.ID, Aspect: n.Aspect, Title: n.Title, Milestone: n.Milestone, Links: n.Links})
		}
		b, _ := json.Marshal(o)
		return "```json\n" + string(b) + "\n```", nil // fenced on purpose
	}
	// Node batch: find the ids listed after the marker.
	i := strings.Index(text, "links to ids from the outline:\n")
	rest := text[i+len("links to ids from the outline:\n"):]
	rest = rest[:strings.Index(rest, "\n\n")]
	var items []outlineItem
	if err := json.Unmarshal([]byte(rest), &items); err != nil {
		panic(err)
	}
	if a, _ := f.failAspect.Load().(string); a != "" && len(items) > 0 && items[0].Aspect == a {
		return "", &llm.Error{Kind: llm.KindOther, Msg: "server error"}
	}
	if f.truncMulti.Load() && len(items) > 1 {
		return "", &llm.Error{Kind: llm.KindTruncated, Msg: "output hit the token limit"}
	}
	var resp nodesResp
	for _, it := range items {
		for _, n := range f.journey.Nodes {
			if n.ID == it.ID {
				resp.Nodes = append(resp.Nodes, n)
			}
		}
	}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

func newFake(mode llm.StructuredMode) *fake {
	j := modeltest.Journey()
	return &fake{caps: llm.Caps{NativePDF: true, Structured: mode}, journey: j}
}

func TestGenerateJourney(t *testing.T) {
	prof, want := modeltest.Profile(), modeltest.Journey()
	for _, mode := range []llm.StructuredMode{llm.StructuredSchema, llm.StructuredObject, llm.StructuredPrompt} {
		f := newFake(mode)
		f.badFirst.Store(true) // the first reply is garbage: the engine must retry
		e := New(f, 8000, 4, 3)
		got, err := e.GenerateJourney(context.Background(), prof, strings.Repeat("job posting ", 50), "notes", nil, nil)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if len(got.Nodes) != len(want.Nodes) {
			t.Fatalf("%s: %d nodes, want %d", mode, len(got.Nodes), len(want.Nodes))
		}
		// Lane order is stable and ids/links come from the outline.
		lastLane := -1
		for _, n := range got.Nodes {
			lane := map[string]int{"tech": 0, "industry": 1, "people": 2, "hr": 3}[n.Aspect]
			if lane < lastLane {
				t.Fatalf("%s: lanes out of order", mode)
			}
			lastLane = lane
			if n.Links == nil || n.Tasks == nil || n.Reading == nil {
				t.Fatalf("%s: node %s has null slices", mode, n.ID)
			}
		}
		if (mode == llm.StructuredSchema) != f.sawSchema.Load() {
			t.Errorf("%s: schema passed to provider = %v", mode, f.sawSchema.Load())
		}
		// outline + ceil(7/3)+ceil(5/3)+ceil(4/3)+ceil(3/3) batches (+1 retry)
		if f.calls.Load() != 1+3+2+2+1+1 {
			t.Errorf("%s: %d provider calls", mode, f.calls.Load())
		}
	}
}

func TestSchemaIsStrictAndComplete(t *testing.T) {
	s := schemaFor[nodesResp]()
	raw, _ := json.Marshal(s)
	str := string(raw)
	for _, must := range []string{`"additionalProperties":false`, `"enum":["tech","industry","people","hr"]`, `"required"`} {
		if !strings.Contains(str, must) {
			t.Errorf("schema missing %s", must)
		}
	}
	if strings.Contains(str, `"$ref"`) || strings.Contains(str, `"$defs"`) {
		t.Error("schema must be fully inlined (no $ref)")
	}
}

func TestSanitizeNode(t *testing.T) {
	ids := map[string]bool{"a": true, "b": true}
	n := model.Node{
		ID: "a", Baseline: 5, Links: []string{"a", "b", "zzz"},
		Test:    model.Test{Kind: "quiz", Quiz: []model.QuizItem{{Q: "q", Options: []string{"x", "y"}, Correct: 9}}},
		Reading: []model.Reading{{Title: "ok", URL: "https://go.dev"}, {Title: "bad", URL: "javascript:alert(1)"}, {Title: "http", URL: "http://x.com"}},
		Project: model.Project{Repo: "  "},
	}
	got := sanitizeNode(n, ids)
	if got.Baseline != 0.6 {
		t.Errorf("baseline = %v", got.Baseline)
	}
	if len(got.Links) != 1 || got.Links[0] != "b" {
		t.Errorf("links = %v", got.Links)
	}
	if got.Test.Kind != "none" {
		t.Errorf("a quiz with no valid items must become none, got %s", got.Test.Kind)
	}
	if len(got.Reading) != 1 || got.Reading[0].URL != "https://go.dev" {
		t.Errorf("reading = %v", got.Reading)
	}
	if got.Project.Repo != "" || got.Project.Milestones == nil {
		t.Errorf("project = %+v", got.Project)
	}
}

func TestProviderErrorsBecomeUserMessages(t *testing.T) {
	for kind, want := range map[llm.Kind]string{
		llm.KindRateLimited: "busy", llm.KindAuth: "not configured", llm.KindRefused: "declined", llm.KindTruncated: "too long",
	} {
		err := explain(&llm.Error{Kind: kind, Msg: "x"})
		ue, ok := err.(*UserError)
		if !ok || !strings.Contains(ue.Msg, want) {
			t.Errorf("kind %v → %v", kind, err)
		}
	}
}

func TestGenerateJourneyReportsProgress(t *testing.T) {
	prof := modeltest.Profile()
	e := New(newFake(llm.StructuredSchema), 8000, 3, 3)
	var mu sync.Mutex
	var evs []Event
	_, err := e.GenerateJourney(context.Background(), prof, strings.Repeat("job posting ", 50), "notes", func(ev Event) {
		mu.Lock()
		evs = append(evs, ev)
		mu.Unlock()
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) < 3 || evs[0].Step != "outline" || evs[1].Step != "outline_done" || evs[1].Total == 0 {
		t.Fatalf("events = %+v", evs)
	}
	last := evs[len(evs)-1]
	if last.Step != "batch_done" || last.Done != last.Total || len(evs) != 2+last.Total {
		t.Fatalf("every batch should be reported once, last = %+v of %d events", last, len(evs))
	}
}

func TestGenerateJourneySplitsBatchesThatRunOutOfTokens(t *testing.T) {
	prof, want := modeltest.Profile(), modeltest.Journey()
	f := newFake(llm.StructuredSchema)
	f.truncMulti.Store(true)
	got, err := New(f, 8000, 3, 3).GenerateJourney(context.Background(), prof, strings.Repeat("job posting ", 50), "notes", nil, nil)
	if err != nil {
		t.Fatalf("a batch that runs out of tokens should be retried node by node: %v", err)
	}
	if len(got.Nodes) != len(want.Nodes) {
		t.Fatalf("%d nodes, want %d", len(got.Nodes), len(want.Nodes))
	}
}

func TestJourneyIsSharedAsItIsWritten(t *testing.T) {
	prof, want := modeltest.Profile(), modeltest.Journey()
	var snaps []model.Journey
	// One call at a time, so batches finish in the order they start.
	got, err := New(newFake(llm.StructuredSchema), 8000, 1, 3).GenerateJourney(context.Background(), prof, strings.Repeat("job posting ", 50), "notes", nil,
		func(j model.Journey) { snaps = append(snaps, j) }) // called under the engine's lock
	if err != nil {
		t.Fatal(err)
	}
	first, last := snaps[0], snaps[len(snaps)-1]
	if len(first.Nodes) != len(want.Nodes) || len(first.Pending) != len(want.Nodes) {
		t.Fatalf("the first snapshot should have every node as a placeholder: %d nodes, %d pending", len(first.Nodes), len(first.Pending))
	}
	if len(last.Pending) != 0 || len(got.Pending) != 0 || len(got.Failed) != 0 {
		t.Fatalf("nothing should be pending at the end: %v %v", last.Pending, got.Failed)
	}
	for i := 1; i < len(snaps); i++ {
		if len(snaps[i].Pending) > len(snaps[i-1].Pending) {
			t.Fatal("pending should only shrink")
		}
	}
	// The first four batches are the first milestones of all four lanes.
	early := snaps[4]
	seen := map[string]bool{}
	for _, n := range early.Nodes {
		if !early.Unwritten(n.ID) {
			seen[n.Aspect] = true
		}
	}
	if len(seen) != 4 {
		t.Fatalf("the first four batches should cover every lane: %v", seen)
	}
}

func TestFailedBatchKeepsTheRestAndCanBeFilled(t *testing.T) {
	prof, want := modeltest.Profile(), modeltest.Journey()
	f := newFake(llm.StructuredSchema)
	f.failAspect.Store("people")
	e := New(f, 8000, 3, 3)
	j, err := e.GenerateJourney(context.Background(), prof, strings.Repeat("job posting ", 50), "notes", nil, nil)
	if err != nil {
		t.Fatalf("one lane failing must not fail the journey: %v", err)
	}
	people := 0
	for _, n := range want.Nodes {
		if n.Aspect == "people" {
			people++
		}
	}
	if len(j.Failed) != people || len(j.Nodes) != len(want.Nodes) {
		t.Fatalf("failed=%v nodes=%d", j.Failed, len(j.Nodes))
	}
	if r := j.Written(); len(r.Nodes) != len(want.Nodes)-people {
		t.Fatalf("written nodes = %d", len(r.Nodes))
	}

	f.failAspect.Store("")
	filled, err := e.FillJourney(context.Background(), prof, strings.Repeat("job posting ", 50), "notes", j, nil, nil)
	if err != nil || len(filled.Failed) != 0 || len(filled.Pending) != 0 || len(filled.Nodes) != len(want.Nodes) {
		t.Fatalf("fill: err=%v failed=%v", err, filled.Failed)
	}
	for _, n := range filled.Nodes {
		if n.Aspect == "people" && len(n.Tasks) == 0 && n.Brief == "" {
			t.Fatalf("node %s is still a placeholder", n.ID)
		}
	}
}
