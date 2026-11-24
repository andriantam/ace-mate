package httpapi_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/andriantam/ace-mate/internal/auth"
	"github.com/andriantam/ace-mate/internal/config"
	"github.com/andriantam/ace-mate/internal/crypt"
	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/engine"
	"github.com/andriantam/ace-mate/internal/httpapi"
	"github.com/andriantam/ace-mate/internal/legal"
	"github.com/andriantam/ace-mate/internal/llm"
	"github.com/andriantam/ace-mate/internal/model"
	"github.com/andriantam/ace-mate/internal/pdfx/pdfxtest"
)

// profiler plays a vision model that returns the same profile for every reading.
type profiler struct{}

func (profiler) Name() string  { return "fake" }
func (profiler) Model() string { return "fake-vl" }
func (profiler) Caps() llm.Caps {
	return llm.Caps{Vision: true, Structured: llm.StructuredPrompt}
}
func (profiler) Complete(context.Context, llm.Request) (string, error) {
	p := model.Profile{Headline: "Backend engineer", Location: "Singapore", WorkRights: []string{}, Industries: []string{},
		Flags: []model.Flag{}, Stacks: []model.Stack{{Name: "Go", Depth: "strong", Years: 5, LastUsed: 2026}}}
	b, _ := json.Marshal(p)
	return string(b), nil
}

const origin = "http://localhost:3000"

// How long a test waits for background work. Rendering PDF pages under the race detector on
// a small CI machine is slow, so this is generous; a real hang still fails.
const waitLimit = 3 * time.Minute

type client struct {
	t      *testing.T
	h      http.Handler
	cookie string
}

func (c *client) do(method, path string, body []byte, ctype string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	req.Header.Set("Origin", origin)
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	return rec
}

// add posts a document and returns its id.
func (c *client) add(body []byte, ctype string) string {
	rec := c.do("POST", "/api/documents", body, ctype, nil)
	if rec.Code != http.StatusAccepted {
		c.t.Fatalf("add document: %d %s", rec.Code, rec.Body)
	}
	var r struct{ ID string }
	_ = json.Unmarshal(rec.Body.Bytes(), &r)
	return r.ID
}

// waitDone polls a document's progress until it ends, returning the steps seen and the last event.
func (c *client) waitDone(id string) (string, map[string]any) {
	var steps []string
	next := 0
	for deadline := time.Now().Add(waitLimit); time.Now().Before(deadline); {
		var p struct {
			Events []map[string]any
			Next   int
			Done   bool
		}
		_ = json.Unmarshal(c.do("GET", "/api/documents/"+id+"/progress?after="+strconv.Itoa(next), nil, "", nil).Body.Bytes(), &p)
		next = p.Next
		for _, ev := range p.Events {
			if ev["type"] == "progress" {
				steps = append(steps, ev["step"].(string))
			}
			if ev["type"] == "done" || ev["type"] == "error" {
				return strings.Join(steps, ","), ev
			}
		}
		time.Sleep(120 * time.Millisecond)
	}
	c.t.Fatalf("document %s never finished", id)
	return "", nil
}

func form(t *testing.T, fields map[string]string, file string, data []byte) ([]byte, string) {
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if file != "" {
		fw, _ := mw.CreateFormFile("resume", file)
		_, _ = fw.Write(data)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes(), mw.FormDataContentType()
}

func docxFile(text string) []byte {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ := z.Create("word/document.xml")
	_, _ = w.Write([]byte(`<w:document xmlns:w="w"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`))
	_ = z.Close()
	return b.Bytes()
}

func TestConsentAndResumeFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("needs the embedded Postgres")
	}
	ctx := context.Background()
	keys, err := crypt.NewKeyring(1, make([]byte, crypt.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(ctx, os.Getenv("TEST_DATABASE_URL"), t.TempDir(), false, keys)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	cfg := &config.Config{BaseURL: origin, DevLogin: true, DailyLimit: 3, DocsLLM: config.LLM{Model: "fake-vl", DisplayName: "Fake VL", Privacy: "not_secure", Provider: "openai", BaseURL: "https://models.example.com/v1"}}
	a := auth.New(ctx, cfg, d)
	e := engine.New(profiler{}, 4000, 2, 3)
	wctx, stop := context.WithCancel(ctx)
	srv := httpapi.New(wctx, cfg, d, a, httpapi.Engines{Docs: e, Journeys: e}, fstest.MapFS{"index.html": {Data: []byte("x")}})
	t.Cleanup(func() { stop(); srv.Wait() }) // runs before d.Close
	h := srv.Handler()
	c := &client{t: t, h: h}

	if rec := c.do("GET", "/readyz", nil, "", nil); rec.Code != http.StatusOK {
		t.Fatalf("readyz with a database: %d %s", rec.Code, rec.Body)
	}

	// Sign in.
	rec := c.do("GET", "/api/auth/dev?user=tester", nil, "", nil)
	c.cookie = strings.Split(rec.Header().Get("Set-Cookie"), ";")[0]
	if c.cookie == "" {
		t.Fatal("no session cookie")
	}

	// The session shows the model, where data goes, and that the terms are not yet accepted.
	var sess struct {
		User struct {
			TermsAccepted bool `json:"termsAccepted"`
		} `json:"user"`
		Models struct {
			Documents struct{ DisplayName, Host, Privacy string } `json:"documents"`
		} `json:"models"`
		Legal struct{ Version string } `json:"legal"`
	}
	_ = json.Unmarshal(c.do("GET", "/api/session", nil, "", nil).Body.Bytes(), &sess)
	if sess.User.TermsAccepted || sess.Models.Documents.DisplayName != "Fake VL" || sess.Models.Documents.Host != "models.example.com" || sess.Models.Documents.Privacy != "not_secure" || sess.Legal.Version != legal.Version {
		t.Fatalf("session = %+v", sess)
	}

	// Everything except consent, export and deletion is blocked until the terms are accepted.
	if rec := c.do("GET", "/api/dashboard", nil, "", nil); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "consent_required") {
		t.Fatalf("dashboard before consent: %d %s", rec.Code, rec.Body)
	}
	if rec := c.do("GET", "/api/export", nil, "", nil); rec.Code != http.StatusOK {
		t.Fatalf("export must work before consent: %d", rec.Code)
	}
	if rec := c.do("POST", "/api/consent", []byte(`{"accept":true,"version":"old","marketing":false}`), "application/json", nil); rec.Code != http.StatusConflict {
		t.Fatalf("a stale terms version must be refused: %d", rec.Code)
	}
	if rec := c.do("POST", "/api/consent", []byte(`{"accept":false,"version":"`+legal.Version+`","marketing":false}`), "application/json", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("not accepting must be refused: %d", rec.Code)
	}
	if rec := c.do("POST", "/api/consent", []byte(`{"accept":true,"version":"`+legal.Version+`","marketing":false}`), "application/json", nil); rec.Code != http.StatusOK {
		t.Fatalf("accept: %d %s", rec.Code, rec.Body)
	}
	if rec := c.do("GET", "/api/dashboard", nil, "", nil); rec.Code != http.StatusOK {
		t.Fatalf("dashboard after consent: %d", rec.Code)
	}

	// The upload must name the model the user ticked.
	body, ct := form(t, map[string]string{"ackModel": "some-other-model"}, "cv.pdf", pdfxtest.TextPDF(pdfxtest.LongLines()))
	if rec := c.do("POST", "/api/documents", body, ct, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong model acknowledgement must be refused: %d", rec.Code)
	}

	// A PDF is accepted at once and processed in the background; progress can be polled.
	body, ct = form(t, map[string]string{"ackModel": "fake-vl"}, "cv.pdf", pdfxtest.TextPDF(pdfxtest.LongLines()))
	pdfID := c.add(body, ct)
	steps, last := c.waitDone(pdfID)
	for _, want := range []string{"received", "extract", "render", "read", "read_done", "reconcile", "saving"} {
		if !strings.Contains(steps, want) {
			t.Errorf("missing step %q in %s", want, steps)
		}
	}
	if last["type"] != "done" || last["verification"].(map[string]any)["agreement"].(float64) != 100 {
		t.Fatalf("last event = %v", last)
	}
	// Polling after the end starts from where the client left off.
	var tail struct{ Events []map[string]any }
	_ = json.Unmarshal(c.do("GET", "/api/documents/"+pdfID+"/progress?after=999", nil, "", nil).Body.Bytes(), &tail)
	if len(tail.Events) != 0 {
		t.Fatalf("no new events expected: %v", tail.Events)
	}

	// Word documents and pasted text are read as text only.
	long := strings.Repeat("Backend engineer building Go services with Postgres in Singapore. ", 4)
	body, ct = form(t, map[string]string{"ackModel": "fake-vl"}, "cv.docx", docxFile(long))
	if _, last := c.waitDone(c.add(body, ct)); last["type"] != "done" {
		t.Fatalf("docx: %v", last)
	}
	body, ct = form(t, map[string]string{"ackModel": "fake-vl", "text": long}, "", nil)
	textID := c.add(body, ct)
	c.waitDone(textID)
	body, ct = form(t, map[string]string{"ackModel": "fake-vl"}, "cv.doc", []byte("\xd0\xcf\x11\xe0 old word"))
	if rec := c.do("POST", "/api/documents", body, ct, nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), ".docx") {
		t.Fatalf("old .doc: %d %s", rec.Code, rec.Body)
	}
	// Links must be https and public: an internal address is refused before anything is fetched.
	body, ct = form(t, map[string]string{"ackModel": "fake-vl", "url": "https://127.0.0.1/admin"}, "", nil)
	if rec := c.do("POST", "/api/documents", body, ct, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("internal link: %d %s", rec.Code, rec.Body)
	}

	// The documents list, and the merged profile used by journeys.
	var docs struct {
		Documents []struct{ ID, Kind, Status string } `json:"documents"`
	}
	_ = json.Unmarshal(c.do("GET", "/api/documents", nil, "", nil).Body.Bytes(), &docs)
	if len(docs.Documents) != 3 {
		t.Fatalf("documents = %+v", docs.Documents)
	}
	var dash struct {
		Profile *struct{ Data model.Profile } `json:"profile"`
	}
	_ = json.Unmarshal(c.do("GET", "/api/dashboard", nil, "", nil).Body.Bytes(), &dash)
	if dash.Profile == nil || len(dash.Profile.Data.Stacks) != 1 || dash.Profile.Data.Stacks[0].Name != "Go" {
		t.Fatalf("merged profile = %+v", dash.Profile)
	}

	// A job application reports its progress while the journey is built. The fake model
	// can't write a journey, so this one ends in an error event: what matters is the plumbing.
	jd := strings.Repeat("We build distributed payment systems in Go on Kubernetes. ", 6)
	rec = c.do("POST", "/api/jobs", []byte(`{"url":"","jdText":"`+jd+`","notes":""}`), "application/json", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create job: %d %s", rec.Code, rec.Body)
	}
	var job struct{ ID string }
	_ = json.Unmarshal(rec.Body.Bytes(), &job)
	var jobSteps []string
	var jobEnd map[string]any
	next := 0
	for deadline := time.Now().Add(waitLimit); jobEnd == nil && time.Now().Before(deadline); {
		var p struct {
			Events []map[string]any
			Next   int
		}
		_ = json.Unmarshal(c.do("GET", "/api/jobs/"+job.ID+"/progress?after="+strconv.Itoa(next), nil, "", nil).Body.Bytes(), &p)
		next = p.Next
		for _, ev := range p.Events {
			if ev["type"] == "progress" {
				jobSteps = append(jobSteps, ev["step"].(string))
			} else {
				jobEnd = ev
			}
		}
		time.Sleep(120 * time.Millisecond)
	}
	if got := strings.Join(jobSteps, ","); !strings.HasPrefix(got, "queued,fetched,outline") || jobEnd == nil || jobEnd["type"] != "error" {
		t.Fatalf("job progress: steps=%s end=%v", got, jobEnd)
	}

	// Another user can't see or delete these documents.
	other := &client{t: t, h: h}
	rec = other.do("GET", "/api/auth/dev?user=intruder", nil, "", nil)
	other.cookie = strings.Split(rec.Header().Get("Set-Cookie"), ";")[0]
	other.do("POST", "/api/consent", []byte(`{"accept":true,"version":"`+legal.Version+`","marketing":false}`), "application/json", nil)
	if rec := other.do("GET", "/api/documents/"+pdfID+"/progress", nil, "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("other user's progress: %d", rec.Code)
	}
	if rec := other.do("GET", "/api/jobs/"+job.ID+"/progress", nil, "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("other user's job progress: %d", rec.Code)
	}
	if rec := other.do("DELETE", "/api/documents/"+pdfID, nil, "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("other user's delete: %d", rec.Code)
	}

	// Removing every document removes the merged profile.
	for _, d := range docs.Documents {
		if rec := c.do("DELETE", "/api/documents/"+d.ID, nil, "", nil); rec.Code != http.StatusOK {
			t.Fatalf("delete: %d", rec.Code)
		}
	}
	dash.Profile = nil
	_ = json.Unmarshal(c.do("GET", "/api/dashboard", nil, "", nil).Body.Bytes(), &dash)
	if dash.Profile != nil {
		t.Fatalf("profile should be gone: %+v", dash.Profile)
	}

	// Marketing is a separate choice the user can change, and the export shows the consent record.
	if rec := c.do("PUT", "/api/consent/marketing", []byte(`{"optIn":true}`), "application/json", nil); rec.Code != http.StatusOK {
		t.Fatalf("marketing: %d", rec.Code)
	}
	var exp struct {
		Account struct {
			TermsVersion    string   `json:"termsVersion"`
			MarketingOptIn  bool     `json:"marketingOptIn"`
			SignInProviders []string `json:"signInProviders"`
		} `json:"account"`
	}
	_ = json.Unmarshal(c.do("GET", "/api/export", nil, "", nil).Body.Bytes(), &exp)
	if exp.Account.TermsVersion != legal.Version || !exp.Account.MarketingOptIn || len(exp.Account.SignInProviders) != 1 {
		t.Fatalf("export account = %+v", exp.Account)
	}
}
