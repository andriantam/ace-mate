package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/andriantam/ace-mate/internal/llm"
	"github.com/andriantam/ace-mate/internal/model"
	"github.com/andriantam/ace-mate/internal/pdfx/pdfxtest"
)

// reader plays a model that reads a resume two ways: from text and from page images.
type reader struct {
	caps      llm.Caps
	fromText  model.Profile
	fromImage model.Profile
	imageErr  error
	textCalls int
	imgCalls  int
	imgParts  int
}

func (r *reader) Name() string   { return "reader" }
func (r *reader) Model() string  { return "reader-1" }
func (r *reader) Caps() llm.Caps { return r.caps }

func (r *reader) Complete(_ context.Context, req llm.Request) (string, error) {
	var imgs int
	for _, p := range req.Parts {
		if len(p.Image) > 0 {
			imgs++
		}
	}
	var out model.Profile
	if imgs > 0 {
		r.imgCalls++
		r.imgParts = imgs
		if r.imageErr != nil {
			return "", r.imageErr
		}
		out = r.fromImage
	} else {
		r.textCalls++
		out = r.fromText
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

func prof(stacks ...model.Stack) model.Profile {
	return model.Profile{Headline: "Senior backend engineer", Location: "Singapore", WorkRights: []string{"SG"}, Industries: []string{"fintech"}, Stacks: stacks, Flags: []model.Flag{}}
}

func st(name string, years float64) model.Stack {
	return model.Stack{Name: name, Depth: "strong", Years: years, LastUsed: 2026}
}

func textResume() []byte {
	return pdfxtest.TextPDF([]string{
		"Senior backend engineer, Singapore. Eight years of distributed systems.",
		"Skills: Go, PostgreSQL, Kafka, Node.js and Docker. Worked on payments and fintech platforms.",
		"Led a team of five engineers and shipped a ledger service in Golang with Postgres.",
		"Education: Computer Science. Languages: English. Work rights: Singapore citizen.",
	})
}

func newEngine(r *reader) *Engine { return New(r, 4000, 2, 3) }

func TestResumeBothReadingsAgree(t *testing.T) {
	r := &reader{
		caps:      llm.Caps{Vision: true, Structured: llm.StructuredPrompt},
		fromText:  prof(st("Go", 6), st("PostgreSQL", 6), st("Kafka", 3)),
		fromImage: prof(st("Golang", 6), st("Postgres", 6), st("Kafka", 3)),
	}
	p, v, err := newEngine(r).ParseResume(context.Background(), ResumeInput{PDF: textResume(), Kind: "pdf"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.textCalls != 1 || r.imgCalls != 1 || r.imgParts != 1 {
		t.Fatalf("calls: text=%d image=%d imageParts=%d", r.textCalls, r.imgCalls, r.imgParts)
	}
	if v.Agreement != 100 || len(v.Confirmed) != 3 || len(v.Dropped) != 0 {
		t.Fatalf("aliases should match: %+v", v)
	}
	if len(p.Stacks) != 3 {
		t.Fatalf("stacks = %v", p.Stacks)
	}
}

func TestResumeDropsSkillOnlyTheVisualReadingInvented(t *testing.T) {
	r := &reader{
		caps:      llm.Caps{Vision: true, Structured: llm.StructuredPrompt},
		fromText:  prof(st("Go", 6), st("Kafka", 3)),
		fromImage: prof(st("Go", 6), st("Kafka", 3), st("Rust", 4)), // Rust is nowhere in the resume
	}
	p, v, err := newEngine(r).ParseResume(context.Background(), ResumeInput{PDF: textResume(), Kind: "pdf"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Dropped) != 1 || !strings.Contains(v.Dropped[0], "Rust") {
		t.Fatalf("dropped = %v", v.Dropped)
	}
	for _, s := range p.Stacks {
		if s.Name == "Rust" {
			t.Fatal("an ungrounded skill must not reach the profile")
		}
	}
	if v.Agreement != 80 { // 4 of the 5 skills named across both readings were matched
		t.Fatalf("agreement = %d, want 80", v.Agreement)
	}
}

func TestResumeKeepsSkillTheTextProvesEvenIfOneReadingMissedIt(t *testing.T) {
	r := &reader{
		caps:      llm.Caps{Vision: true, Structured: llm.StructuredPrompt},
		fromText:  prof(st("Go", 6), st("Docker", 4)),
		fromImage: prof(st("Go", 6)), // OCR missed Docker, but the text layer contains it
	}
	p, v, err := newEngine(r).ParseResume(context.Background(), ResumeInput{PDF: textResume(), Kind: "pdf"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Stacks) != 2 || len(v.OnlyText) != 1 || v.OnlyText[0] != "Docker" {
		t.Fatalf("stacks=%v onlyText=%v", p.Stacks, v.OnlyText)
	}
}

func TestResumeReportsMismatches(t *testing.T) {
	r := &reader{
		caps:      llm.Caps{Vision: true, Structured: llm.StructuredPrompt},
		fromText:  prof(st("Go", 8)),
		fromImage: prof(st("Go", 3)),
	}
	r.fromImage.Location = "Kuala Lumpur"
	_, v, err := newEngine(r).ParseResume(context.Background(), ResumeInput{PDF: textResume(), Kind: "pdf"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(v.Mismatches, "\n")
	if !strings.Contains(joined, "Go:") || !strings.Contains(joined, "Location") {
		t.Fatalf("mismatches = %v", v.Mismatches)
	}
}

func TestScannedResumeIsReadVisually(t *testing.T) {
	r := &reader{
		caps:      llm.Caps{Vision: true, Structured: llm.StructuredPrompt},
		fromImage: prof(st("Go", 6)),
	}
	p, v, err := newEngine(r).ParseResume(context.Background(), ResumeInput{PDF: pdfxtest.ScanPDF(), Kind: "pdf"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.textCalls != 0 || r.imgCalls != 1 {
		t.Fatalf("a scan has no text to send: text=%d image=%d", r.textCalls, r.imgCalls)
	}
	if len(v.Methods) != 1 || v.Methods[0] != "visual" || v.Agreement != -1 || len(p.Stacks) != 1 {
		t.Fatalf("%+v", v)
	}
	if !strings.Contains(strings.Join(v.Notes, " "), "no readable text") {
		t.Fatalf("notes should explain why: %v", v.Notes)
	}
}

func TestScannedResumeWithoutVisionModelExplainsWhatToDo(t *testing.T) {
	r := &reader{caps: llm.Caps{Structured: llm.StructuredPrompt}}
	_, _, err := newEngine(r).ParseResume(context.Background(), ResumeInput{PDF: pdfxtest.ScanPDF(), Kind: "pdf"}, nil)
	var ue *UserError
	if !errors.As(err, &ue) || !strings.Contains(ue.Msg, "LLM_VISION") {
		t.Fatalf("err = %v", err)
	}
	if r.textCalls+r.imgCalls != 0 {
		t.Fatal("no model call should be made")
	}
}

func TestTextOnlyWhenModelHasNoVision(t *testing.T) {
	r := &reader{caps: llm.Caps{Structured: llm.StructuredPrompt}, fromText: prof(st("Go", 6))}
	_, v, err := newEngine(r).ParseResume(context.Background(), ResumeInput{PDF: textResume(), Kind: "pdf"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.imgCalls != 0 || len(v.Methods) != 1 || v.Methods[0] != "text" || v.Agreement != -1 {
		t.Fatalf("%+v", v)
	}
}

func TestVisualFailureDegradesToText(t *testing.T) {
	r := &reader{
		caps:     llm.Caps{Vision: true, Structured: llm.StructuredPrompt},
		fromText: prof(st("Go", 6)),
		imageErr: &llm.Error{Kind: llm.KindBadRequest, Msg: "model does not support images"},
	}
	p, v, err := newEngine(r).ParseResume(context.Background(), ResumeInput{PDF: textResume(), Kind: "pdf"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Stacks) != 1 || v.Methods[0] != "text" || !strings.Contains(strings.Join(v.Notes, " "), "visual reading failed") {
		t.Fatalf("%+v", v)
	}
}

func TestResumeModeText(t *testing.T) {
	r := &reader{caps: llm.Caps{Vision: true, Structured: llm.StructuredPrompt}, fromText: prof(st("Go", 6))}
	e := newEngine(r)
	e.ResumeMode = "text"
	if _, _, err := e.ParseResume(context.Background(), ResumeInput{PDF: textResume(), Kind: "pdf"}, nil); err != nil {
		t.Fatal(err)
	}
	if r.imgCalls != 0 {
		t.Fatal("text mode must not call the vision path")
	}
}

func TestResumeModeVision(t *testing.T) {
	r := &reader{caps: llm.Caps{Vision: true, Structured: llm.StructuredPrompt}, fromImage: prof(st("Go", 6))}
	e := newEngine(r)
	e.ResumeMode = "vision"
	if _, _, err := e.ParseResume(context.Background(), ResumeInput{PDF: textResume(), Kind: "pdf"}, nil); err != nil {
		t.Fatal(err)
	}
	if r.textCalls != 0 || r.imgCalls != 1 {
		t.Fatalf("vision mode: text=%d image=%d", r.textCalls, r.imgCalls)
	}
}

func TestSkillKey(t *testing.T) {
	same := [][]string{{"Node.js", "NodeJS", "node"}, {"Go", "Golang"}, {"PostgreSQL", "Postgres"}, {"React.js", "React"}, {"K8s", "Kubernetes"}}
	for _, g := range same {
		for _, n := range g[1:] {
			if skillKey(g[0]) != skillKey(n) {
				t.Errorf("%q and %q should match", g[0], n)
			}
		}
	}
	if skillKey("C") == skillKey("C++") || skillKey("Java") == skillKey("JavaScript") {
		t.Error("distinct skills must stay distinct")
	}
}

func TestMentionsIsWholeWord(t *testing.T) {
	if mentions("I like Django and the Google cloud", "Go") {
		t.Error("Go must not match inside other words")
	}
	if !mentions("Built services in Golang", "Go") || !mentions("Used Postgres daily", "PostgreSQL") {
		t.Error("aliases should match")
	}
}

// The two readings often name the same skill at different granularity, and must not count
// them as different skills.
func TestSameSkillAtDifferentGranularity(t *testing.T) {
	same := [][2]string{
		{"Docker", "docker-compose"},
		{"CDP", "Customer Data Platform (CDP)"},
		{"Claude", "Claude/MCP integrations"},
		{"MCP", "Claude/MCP integrations"},
		{"Node.js", "NodeJS"},
		{"Spring", "Spring Boot"},
	}
	for _, c := range same {
		if !sameSkill(c[0], c[1]) {
			t.Errorf("%q and %q should be the same skill", c[0], c[1])
		}
	}
	diff := [][2]string{{"Java", "JavaScript"}, {"C", "C++"}, {"C", "C#/.NET"}, {"Go", "Rust"}}
	for _, c := range diff {
		if sameSkill(c[0], c[1]) {
			t.Errorf("%q and %q must stay different", c[0], c[1])
		}
	}
}

func TestCompoundSkillIsGroundedWhenEveryPartIsInTheText(t *testing.T) {
	raw := "Skills: Claude and MCP integrations, Docker Compose, Customer Data Platform (CDP) work."
	for _, name := range []string{"Claude/MCP integrations", "docker-compose", "CDP", "Customer Data Platform (CDP)"} {
		if !mentions(raw, name) {
			t.Errorf("%q should be grounded in %q", name, raw)
		}
	}
	if mentions(raw, "Rust/Kubernetes integrations") {
		t.Error("a compound with missing parts must not be grounded")
	}
}

func TestGranularityDoesNotCreateDuplicatesOrFalseDrops(t *testing.T) {
	raw := "Skills: Docker Compose, Claude and MCP integrations, Customer Data Platform (CDP), Go."
	r := &reader{
		caps:      llm.Caps{Vision: true, Structured: llm.StructuredPrompt},
		fromText:  prof(st("Go", 5), st("docker-compose", 1), st("Claude/MCP integrations", 1), st("Customer Data Platform (CDP)", 1)),
		fromImage: prof(st("Go", 5), st("Docker", 2), st("Claude", 1), st("MCP", 1), st("CDP", 1)),
	}
	pdf := pdfxtest.TextPDF([]string{raw, raw, raw})
	p, v, err := newEngine(r).ParseResume(context.Background(), ResumeInput{PDF: pdf, Kind: "pdf"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Stacks) != 4 {
		t.Fatalf("want 4 skills with no duplicates, got %v", p.Stacks)
	}
	if len(v.Dropped) != 0 || len(v.OnlyVisual) != 0 || len(v.OnlyText) != 0 {
		t.Fatalf("everything should be confirmed: %+v", v)
	}
	if v.Agreement != 100 {
		t.Fatalf("agreement = %d, want 100", v.Agreement)
	}
}

func TestEstimateNoiseAndWordingAreNotFlagged(t *testing.T) {
	r := &reader{
		caps:      llm.Caps{Vision: true, Structured: llm.StructuredPrompt},
		fromText:  prof(st("Go", 6)),
		fromImage: prof(st("Go", 8)), // a 2-year difference in an estimate is not a disagreement
	}
	r.fromText.WorkRights = []string{"Malaysia", "Singapore"}
	r.fromImage.WorkRights = []string{"Malaysia (Permanent Resident)", "Singapore (Permanent Resident)"}
	p, v, err := newEngine(r).ParseResume(context.Background(), ResumeInput{PDF: textResume(), Kind: "pdf"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Mismatches) != 0 {
		t.Fatalf("no real disagreement: %v", v.Mismatches)
	}
	if len(p.WorkRights) != 2 || !strings.Contains(p.WorkRights[0], "Permanent Resident") {
		t.Fatalf("the more detailed wording should win: %v", p.WorkRights)
	}
}

func TestGroundingSurvivesGluedWordsInExtractedText(t *testing.T) {
	// PDF text extraction can lose the space between neighbouring words ("ExampleCoNext.js").
	raw := "delivering software at ExampleCoNext.js, React, Angular, C#/.NET, Node.js across 5 markets"
	if !mentions(raw, "Next.js") {
		t.Error("Next.js is in the text even though the words are glued together")
	}
	if mentions("Django and Google Cloud", "Go") || mentions("I use JavaScript only", "Java") {
		t.Error("short names must still need a whole-word match")
	}
}

func TestResumeProgressEvents(t *testing.T) {
	r := &reader{
		caps:      llm.Caps{Vision: true, Structured: llm.StructuredPrompt},
		fromText:  prof(st("Go", 6)),
		fromImage: prof(st("Go", 6)),
	}
	var mu sync.Mutex
	var steps []string
	_, _, err := newEngine(r).ParseResume(context.Background(), ResumeInput{PDF: textResume(), Kind: "pdf"}, func(ev Event) {
		mu.Lock()
		defer mu.Unlock()
		steps = append(steps, ev.Step+":"+ev.Method)
	})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(steps, " ")
	for _, want := range []string{"extract:", "render:", "read:", "read_done:text", "read_done:visual", "reconcile:"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if !strings.HasPrefix(got, "extract: render: read:") || !strings.HasSuffix(got, "reconcile:") {
		t.Errorf("wrong order: %q", got)
	}
}

func TestPastedTextIsReadAsTextOnly(t *testing.T) {
	r := &reader{caps: llm.Caps{Vision: true, Structured: llm.StructuredPrompt}, fromText: prof(st("Go", 6))}
	text := strings.Repeat("Senior backend engineer building Go services on Postgres. ", 5)
	p, v, err := newEngine(r).ParseResume(context.Background(), ResumeInput{Text: text, Kind: "text"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.imgCalls != 0 || r.textCalls != 1 || len(p.Stacks) != 1 || v.Methods[0] != "text" {
		t.Fatalf("text=%d image=%d %+v", r.textCalls, r.imgCalls, v)
	}
	if !strings.Contains(strings.Join(v.Notes, " "), "Pasted text") {
		t.Fatalf("notes = %v", v.Notes)
	}
	if _, _, err := newEngine(r).ParseResume(context.Background(), ResumeInput{Text: "too short", Kind: "text"}, nil); err == nil {
		t.Error("too little text must be rejected")
	}
}
