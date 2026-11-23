package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/andriantam/ace-mate/internal/auth"
	"github.com/andriantam/ace-mate/internal/config"
	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/engine"
	"github.com/andriantam/ace-mate/internal/httpapi"
	"github.com/andriantam/ace-mate/internal/legal"
	"github.com/andriantam/ace-mate/internal/llm"
	"github.com/andriantam/ace-mate/internal/model"
	"github.com/andriantam/ace-mate/internal/model/modeltest"
)

// linker plays a model that links t1:0 and h1:q0 of every journey, and nothing else.
type linker struct {
	mu    sync.Mutex
	calls int
}

var refRe = regexp.MustCompile(`"ref":"([^"]+)"`)

func (*linker) Name() string   { return "fake" }
func (*linker) Model() string  { return "fake-linker" }
func (*linker) Caps() llm.Caps { return llm.Caps{Structured: llm.StructuredPrompt} }
func (l *linker) Complete(_ context.Context, r llm.Request) (string, error) {
	l.mu.Lock()
	l.calls++
	l.mu.Unlock()
	items := r.Parts[0].Text[strings.Index(r.Parts[0].Text, "<new_items>"):]
	var ms []engine.ShareMatch
	for _, m := range refRe.FindAllStringSubmatch(items, -1) {
		key := ""
		switch m[1] {
		case "t1:0":
			key = "frontend/js-project-on-github"
		case "h1:q0":
			key = "hr/greatest-weakness"
		}
		ms = append(ms, engine.ShareMatch{Ref: m[1], Key: key, Label: map[bool]string{true: "Shared " + key}[key != ""]})
	}
	b, _ := json.Marshal(map[string]any{"track": "frontend", "matches": ms})
	return string(b), nil
}

// Journeys written before shared prep are linked when the new version starts; then a tick
// in one journey reaches the other, and HR answers are kept once in the FAQ.
func TestSharedPrepFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("needs Postgres")
	}
	ctx := context.Background()
	d, err := db.Open(ctx, os.Getenv("TEST_DATABASE_URL"), t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	cfg := &config.Config{BaseURL: origin, DevLogin: true, DailyLimit: 3}
	a := auth.New(ctx, cfg, d)
	ui := fstest.MapFS{"index.html": {Data: []byte("x")}}

	// The old version: no linking.
	octx, ostop := context.WithCancel(ctx)
	old := httpapi.New(octx, cfg, d, a, httpapi.Engines{}, ui)
	c := &client{t: t, h: old.Handler()}
	rec := c.do("GET", "/api/auth/dev?user=sharer", nil, "", nil)
	c.cookie = strings.Split(rec.Header().Get("Set-Cookie"), ";")[0]
	c.do("POST", "/api/consent", []byte(`{"accept":true,"version":"`+legal.Version+`","marketing":false}`), "application/json", nil)
	_, token, _ := strings.Cut(c.cookie, "=")
	u, err := d.UserBySession(ctx, token)
	if err != nil || u == nil {
		t.Fatalf("no user: %v", err)
	}
	uid := u.ID
	j := modeltest.Journey()
	for i := range j.Nodes {
		if j.Nodes[i].ID == "h1" {
			j.Nodes[i].HRQuestions = []model.HRQuestion{{Q: "What is your biggest weakness?", Tip: "Pick a real one."}, {Q: "Why us?", Tip: "Know the product."}}
		}
	}
	job1, _ := d.InsertJob(ctx, uid, db.NewJob{Title: "Frontend A", Journey: &j})
	job2, _ := d.InsertJob(ctx, uid, db.NewJob{Title: "Frontend B", Journey: &j})
	if rec := c.do("PUT", "/api/jobs/"+job1+"/progress", []byte(`{"item":"t1:0","done":true}`), "application/json", nil); rec.Code != http.StatusOK {
		t.Fatalf("tick: %d %s", rec.Code, rec.Body)
	}

	ostop()
	old.Wait()

	// The new version links both journeys on start.
	fake := &linker{}
	e := engine.New(fake, 4000, 1, 3)
	wctx, stop := context.WithCancel(ctx)
	srv := httpapi.New(wctx, cfg, d, a, httpapi.Engines{Docs: e, Journeys: e}, ui)
	t.Cleanup(func() { stop(); srv.Wait() })
	c.h = srv.Handler()

	type prep struct {
		Items []struct {
			ID, Kind, Key, Label, Answer string
			Done                         bool
			Places                       []struct{ JobID, Title, Text, Tip string }
		}
		Unlinked int
	}
	var p prep
	for deadline := time.Now().Add(waitLimit); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		p = prep{}
		_ = json.Unmarshal(c.do("GET", "/api/prep", nil, "", nil).Body.Bytes(), &p)
		if p.Unlinked == 0 && len(p.Items) == 2 && len(p.Items[0].Places) == 2 && len(p.Items[1].Places) == 2 {
			break
		}
	}
	if p.Unlinked != 0 || len(p.Items) != 2 {
		t.Fatalf("prep = %+v", p)
	}
	var hrID, taskID string
	for _, it := range p.Items {
		switch it.Kind {
		case "hr":
			hrID = it.ID
			if it.Places[0].Text != "What is your biggest weakness?" || it.Places[0].Tip != "Pick a real one." {
				t.Fatalf("hr places = %+v", it.Places)
			}
		case "task":
			taskID = it.ID
			if !it.Done {
				t.Fatal("the tick from before the upgrade should carry over")
			}
		}
	}

	var job struct {
		Progress map[string]bool
		Shared   map[string]struct {
			ID     string
			Kind   string
			Others []struct{ JobID string }
		}
	}
	_ = json.Unmarshal(c.do("GET", "/api/jobs/"+job2, nil, "", nil).Body.Bytes(), &job)
	if !job.Progress["t1:0"] || job.Shared["t1:0"].ID != taskID || len(job.Shared["t1:0"].Others) != 1 || job.Shared["t1:0"].Others[0].JobID != job1 {
		t.Fatalf("job 2 = %+v", job)
	}

	// Unticking in one journey unticks the other.
	rec = c.do("PUT", "/api/jobs/"+job2+"/progress", []byte(`{"item":"t1:0","done":false}`), "application/json", nil)
	var changed struct{ Changed []db.Ticked }
	_ = json.Unmarshal(rec.Body.Bytes(), &changed)
	if rec.Code != http.StatusOK || len(changed.Changed) != 2 {
		t.Fatalf("untick: %d %s", rec.Code, rec.Body)
	}
	_ = json.Unmarshal(c.do("GET", "/api/jobs/"+job1, nil, "", nil).Body.Bytes(), &job)
	if job.Progress["t1:0"] {
		t.Fatal("job 1 should be unticked too")
	}

	// The answer is saved once, for every job that asks it.
	if rec := c.do("PUT", "/api/prep/"+hrID+"/answer", []byte(`{"answer":"  I over-polish.  "}`), "application/json", nil); rec.Code != http.StatusOK {
		t.Fatalf("answer: %d %s", rec.Code, rec.Body)
	}
	if rec := c.do("PUT", "/api/prep/"+hrID+"/answer", []byte(`{"answer":"`+strings.Repeat("x", 5001)+`"}`), "application/json", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("a long answer must be refused: %d", rec.Code)
	}
	if rec := c.do("PUT", "/api/prep/"+taskID+"/done", []byte(`{"done":true}`), "application/json", nil); rec.Code != http.StatusOK {
		t.Fatalf("done: %d %s", rec.Code, rec.Body)
	}
	var exp struct{ Shared []struct{ Answer string } }
	_ = json.Unmarshal(c.do("GET", "/api/export", nil, "", nil).Body.Bytes(), &exp)
	found := false
	for _, s := range exp.Shared {
		found = found || s.Answer == "I over-polish."
	}
	if !found {
		t.Fatalf("export = %+v", exp)
	}

	// Another user sees none of it and can't change it.
	other := &client{t: t, h: c.h}
	rec = other.do("GET", "/api/auth/dev?user=snoop", nil, "", nil)
	other.cookie = strings.Split(rec.Header().Get("Set-Cookie"), ";")[0]
	other.do("POST", "/api/consent", []byte(`{"accept":true,"version":"`+legal.Version+`","marketing":false}`), "application/json", nil)
	var op prep
	_ = json.Unmarshal(other.do("GET", "/api/prep", nil, "", nil).Body.Bytes(), &op)
	if len(op.Items) != 0 {
		t.Fatalf("other user saw %+v", op)
	}
	if rec := other.do("PUT", "/api/prep/"+hrID+"/answer", []byte(`{"answer":"x"}`), "application/json", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("other user's answer: %d", rec.Code)
	}
	if rec := other.do("PUT", "/api/prep/"+taskID+"/done", []byte(`{"done":false}`), "application/json", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("other user's tick: %d", rec.Code)
	}

	// Asking for a fresh pass compares the journeys again and keeps the answer.
	before := fake.calls
	if rec := c.do("POST", "/api/prep/resync", nil, "", nil); rec.Code != http.StatusAccepted {
		t.Fatalf("resync: %d", rec.Code)
	}
	for deadline := time.Now().Add(waitLimit); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		p = prep{}
		_ = json.Unmarshal(c.do("GET", "/api/prep", nil, "", nil).Body.Bytes(), &p)
		fake.mu.Lock()
		n := fake.calls
		fake.mu.Unlock()
		if p.Unlinked == 0 && n >= before+2 {
			break
		}
	}
	for _, it := range p.Items {
		if it.Kind == "hr" && it.Answer != "I over-polish." {
			t.Fatalf("answer lost: %+v", it)
		}
		if it.Kind == "task" && !it.Done {
			t.Fatalf("tick lost: %+v", it)
		}
	}
}
