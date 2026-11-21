package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/andriantam/ace-mate/internal/llm"
	"github.com/andriantam/ace-mate/internal/model"
	"github.com/andriantam/ace-mate/internal/pdfx"
)

// Reading resumes and other documents into a profile.

// maxResumePages bounds how many pages are rendered for the visual reading.
const maxResumePages = 4

type reading struct {
	p   model.Profile
	err error
}

// validateProfile accepts any document that yields something about the person. A
// certificate or a GitHub profile may have no headline, and that's fine: the merged
// profile takes the headline from another document.
func validateProfile(p *model.Profile) error {
	if strings.TrimSpace(p.Headline) == "" && len(p.Stacks) == 0 && len(p.Experience) == 0 && len(p.Industries) == 0 {
		return errors.New("nothing was extracted: fill in at least the headline, stacks or experience")
	}
	return nil
}

// ResumeInput is a resume to read: a PDF, or text already extracted from a Word document
// or pasted by the user.
type ResumeInput struct {
	PDF  []byte
	Text string
	Kind string // "pdf", "docx", "text" or "link"
	Note string // for links: what kind of page this is
}

// ParseResume reads the resume up to two ways and cross-checks them:
//
//   - text:   the PDF's text layer, which has the exact characters
//   - visual: the pages rendered as images and read by a vision model (OCR for scans)
//
// Which run depends on LLM_RESUME_MODE (auto, text or vision) and on whether the model
// accepts images. A scanned PDF has no text layer, so it is read visually only.
func (e *Engine) ParseResume(ctx context.Context, in ResumeInput, report func(Event)) (model.Profile, Verification, error) {
	emit := func(ev Event) {
		if report != nil {
			report(ev)
		}
	}
	if len(in.PDF) == 0 {
		return e.parseText(ctx, in, emit)
	}
	pdf := in.PDF
	caps := e.P.Caps()
	mode := e.ResumeMode
	if mode == "" {
		mode = "auto"
	}

	var (
		rawText, textNote string
		textErr           error
		pages             []pdfx.Page
		total             int
		renderErr         error
	)
	if mode != "vision" {
		rawText, textErr = pdfx.Text(pdf)
		if textErr == nil {
			emit(Event{Step: "extract", Detail: "text layer found", OK: true})
		} else {
			emit(Event{Step: "extract", Detail: "no text layer"})
		}
	}
	if mode != "text" && caps.Vision {
		pages, total, renderErr = pdfx.Render(ctx, pdf, maxResumePages)
		emit(Event{Step: "render", Pages: len(pages), OK: renderErr == nil})
	}
	wantText := mode != "vision" && textErr == nil
	wantVisual := mode != "text" && caps.Vision && renderErr == nil && len(pages) > 0

	var notes []string
	if mode == "vision" && !caps.Vision {
		return model.Profile{}, Verification{}, &UserError{Msg: "Visual resume reading needs a model that accepts images. Set LLM_VISION=true for a vision model, or use LLM_RESUME_MODE=auto."}
	}
	if textErr != nil && mode != "vision" {
		textNote = "The PDF has no readable text layer (it may be a scan), so it was read visually."
	}
	if renderErr != nil && mode != "text" && caps.Vision {
		slog.Warn("resume render failed", "err", renderErr)
		notes = append(notes, "The pages could not be rendered for the visual reading.")
	}
	if total > maxResumePages {
		notes = append(notes, fmt.Sprintf("Only the first %d of %d pages were read visually.", maxResumePages, total))
	}

	if !wantText && !wantVisual {
		if caps.NativePDF {
			emit(Event{Step: "read", Methods: []string{"pdf"}})
			p, err := e.readPDFNative(ctx, pdf)
			emit(Event{Step: "read_done", Method: "pdf", OK: err == nil})
			if err != nil {
				return p, Verification{}, err
			}
			_, v := Single(p, "pdf", "", append(notes, "Read with the model's built-in PDF reader.")...)
			return p, v, nil
		}
		if textErr != nil && !caps.Vision {
			return model.Profile{}, Verification{}, &UserError{Msg: "That PDF has no readable text (it looks like a scan), and the configured model cannot read images. Upload a text-based PDF, or configure a vision model with LLM_VISION=true."}
		}
		return model.Profile{}, Verification{}, &UserError{Msg: "Couldn't read that resume."}
	}

	var methods []string
	if wantText {
		methods = append(methods, "text")
	}
	if wantVisual {
		methods = append(methods, "visual")
	}
	emit(Event{Step: "read", Methods: methods, Pages: len(pages)})

	var t, vis reading
	var wg sync.WaitGroup
	if wantText {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t.p, t.err = structured(ctx, e, call{
				system: resumeSystem + "\n" + textReadNote,
				parts:  []llm.Part{{Text: "<resume>\n" + rawText + "\n</resume>"}, {Text: "Extract the profile from this resume."}},
				effort: llm.EffortLow,
			}, validateProfile)
			emit(Event{Step: "read_done", Method: "text", OK: t.err == nil})
		}()
	}
	if wantVisual {
		wg.Add(1)
		go func() {
			defer wg.Done()
			parts := []llm.Part{{Text: fmt.Sprintf("The resume has %d page image(s) below.", len(pages))}}
			for _, pg := range pages {
				parts = append(parts, llm.Part{Image: pg.JPEG})
			}
			parts = append(parts, llm.Part{Text: "Extract the profile from this resume."})
			vis.p, vis.err = structured(ctx, e, call{system: resumeSystem + "\n" + visualReadNote, parts: parts, effort: llm.EffortLow}, validateProfile)
			emit(Event{Step: "read_done", Method: "visual", OK: vis.err == nil})
		}()
	}
	wg.Wait()

	tOK, vOK := wantText && t.err == nil, wantVisual && vis.err == nil
	switch {
	case tOK && vOK:
		emit(Event{Step: "reconcile"})
		p, v := Reconcile(&t.p, &vis.p, rawText)
		v.Notes = append(v.Notes, notes...)
		if len(p.Stacks) == 0 {
			return p, v, &UserError{Msg: "Couldn't read that resume reliably. Try a cleaner PDF."}
		}
		return normalizeProfile(p), v, nil
	case tOK:
		if wantVisual {
			notes = append(notes, "The visual reading failed, so this profile was read from the text only: "+reason(vis.err))
		}
		p, v := Single(t.p, "text", rawText, notes...)
		return normalizeProfile(p), v, nil
	case vOK:
		if textNote != "" {
			notes = append(notes, textNote)
		}
		if wantText {
			notes = append(notes, "The text reading failed, so this profile was read visually only: "+reason(t.err))
		}
		p, v := Single(vis.p, "visual", "", notes...)
		return normalizeProfile(p), v, nil
	}
	if wantText {
		return model.Profile{}, Verification{}, t.err
	}
	return model.Profile{}, Verification{}, vis.err
}

// parseText reads a resume that arrived as text (a Word document or pasted notes).
// There is no page image to compare against, so there is no cross-check.
func (e *Engine) parseText(ctx context.Context, in ResumeInput, emit func(Event)) (model.Profile, Verification, error) {
	raw := strings.TrimSpace(in.Text)
	if len(raw) < 100 {
		return model.Profile{}, Verification{}, &UserError{Msg: "That's too little text to build a profile from. Paste your full resume or upload the file."}
	}
	if len(raw) > 60_000 {
		raw = raw[:60_000]
	}
	emit(Event{Step: "extract", Detail: "text received", OK: true})
	emit(Event{Step: "read", Methods: []string{"text"}})
	note := textReadNote
	if in.Kind == "link" {
		note = in.Note + " It may not be a resume: extract only what it shows about the person, and leave fields empty otherwise."
	}
	p, err := structured(ctx, e, call{
		system: resumeSystem + "\n" + note,
		parts:  []llm.Part{{Text: "<resume>\n" + raw + "\n</resume>"}, {Text: "Extract the profile from this resume."}},
		effort: llm.EffortLow,
	}, validateProfile)
	emit(Event{Step: "read_done", Method: "text", OK: err == nil})
	if err != nil {
		return p, Verification{}, err
	}
	what := map[string]string{"docx": "Word documents are", "text": "Pasted text is", "link": "Links are"}[in.Kind]
	if what == "" {
		what = "This document is"
	}
	p, v := Single(p, "text", raw, what+" read as text only, so nothing was cross-checked. Upload a PDF to have it checked against the page images.")
	return normalizeProfile(p), v, nil
}

// readPDFNative is the fallback for providers that read PDFs themselves.
func (e *Engine) readPDFNative(ctx context.Context, pdf []byte) (model.Profile, error) {
	p, err := structured(ctx, e, call{
		system: resumeSystem,
		parts:  []llm.Part{{PDF: pdf}, {Text: "Extract the profile from this resume."}},
		effort: llm.EffortLow,
	}, validateProfile)
	if err != nil {
		return p, err
	}
	return normalizeProfile(p), nil
}

func reason(err error) string {
	var ue *UserError
	if errors.As(err, &ue) {
		return ue.Msg
	}
	return "unexpected error"
}

func normalizeProfile(p model.Profile) model.Profile {
	p.WorkRights = nz(p.WorkRights)
	p.Industries = nz(p.Industries)
	if p.Stacks == nil {
		p.Stacks = []model.Stack{}
	}
	if p.Flags == nil {
		p.Flags = []model.Flag{}
	}
	if p.Experience == nil {
		p.Experience = []model.Role{}
	}
	for i := range p.Experience {
		p.Experience[i].Skills = nz(p.Experience[i].Skills)
		p.Experience[i].Highlights = nz(p.Experience[i].Highlights)
		p.Experience[i].Start = cleanYM(p.Experience[i].Start)
		p.Experience[i].End = cleanYM(p.Experience[i].End)
	}
	return p
}

var ymRe = regexp.MustCompile(`^(19[5-9]\d|20\d\d)(?:-(0[1-9]|1[0-2]))?$`)

// cleanYM keeps "YYYY-MM" or "YYYY" and drops anything else, so the timeline never
// draws a made-up date.
func cleanYM(s string) string {
	s = strings.TrimSpace(s)
	if ymRe.MatchString(s) {
		return s
	}
	return ""
}
