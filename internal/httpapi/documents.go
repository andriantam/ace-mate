package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/docx"
	"github.com/andriantam/ace-mate/internal/engine"
	"github.com/andriantam/ace-mate/internal/safefetch"
)

const (
	maxUpload    = 5 << 20
	maxDocuments = 30
)

// ── handlers ──

func (s *Server) listDocuments(w http.ResponseWriter, r *http.Request, u *db.User) {
	docs, err := s.db.ListDocuments(r.Context(), u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": docs})
}

type docInput struct {
	in   engine.ResumeInput
	name string
	url  string
}

// input is what the task carries: the PDF itself, or the document's text. A link carries
// nothing; its URL is on the document.
func (di docInput) input() []byte {
	if len(di.in.PDF) > 0 {
		return di.in.PDF
	}
	return []byte(di.in.Text)
}

// addDocument accepts a file (PDF, Word .docx, .txt, .md), pasted text or a website link,
// starts processing it in the background and returns its id at once.
func (s *Server) addDocument(w http.ResponseWriter, r *http.Request, u *db.User) {
	if s.docs == nil {
		writeErr(w, http.StatusServiceUnavailable, "Adding documents needs an AI model. See the README to configure one.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		writeErr(w, http.StatusBadRequest, "Files must be under 5 MB.")
		return
	}
	// The user must have seen and ticked the AI model that will read the document. Checking
	// the model id means a page left open before the operator switched models can't send silently.
	_, llm, ok := s.pick(toolDocs, r.FormValue("engine"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "That AI model isn't available. Reload the page and choose again.")
		return
	}
	if r.FormValue("ackModel") != llm.Model {
		writeErr(w, http.StatusBadRequest, "Please confirm which AI model will read your document, then try again.")
		return
	}
	di, msg := readDocInput(r)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if docs, err := s.db.ListDocuments(r.Context(), u.ID); err != nil {
		serverError(w, err)
		return
	} else if len(docs) >= maxDocuments {
		writeErr(w, http.StatusBadRequest, "You have reached the limit of 30 documents. Remove some first.")
		return
	}
	if !s.resumeLimit.allow(u.ID) {
		writeErr(w, http.StatusTooManyRequests, "You're adding documents quickly. Wait a couple of minutes and try again.")
		return
	}
	id, err := s.db.InsertDocument(r.Context(), u.ID, di.in.Kind, di.name, di.url)
	if err != nil {
		serverError(w, err)
		return
	}
	// The file waits in the queue (in the database) only until it has been read.
	if _, err := s.db.Enqueue(r.Context(), db.NewTask{UserID: u.ID, Kind: taskDocument, Subject: id, Input: di.input(), Engine: engineID(r.FormValue("engine"))},
		map[string]any{"type": "progress", "step": "received", "kind": di.in.Kind}); err != nil {
		serverError(w, err)
		return
	}
	s.work.Wake(taskDocument)
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
}

func (s *Server) documentProgress(w http.ResponseWriter, r *http.Request, u *db.User) {
	s.progress(w, r, u, func(ctx context.Context, id string) (map[string]any, error) {
		doc, err := s.db.GetDocument(ctx, u.ID, id)
		if err != nil {
			return nil, err
		}
		switch doc.Status {
		case "ready":
			return map[string]any{"type": "done", "verification": doc.Verification}, nil
		case "failed":
			return map[string]any{"type": "error", "error": doc.Error}, nil
		}
		return nil, nil
	})
}

func (s *Server) deleteDocument(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, ok := jobID(w, r)
	if !ok {
		return
	}
	if err := s.db.DeleteDocument(r.Context(), u.ID, id); errors.Is(err, db.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Not found.")
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	if err := s.rebuildProfile(r.Context(), u.ID); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// readDocInput accepts a link (field "url"), pasted text (field "text") or a file (field
// "resume"): PDF, Word .docx, or plain text / Markdown. It returns a user-facing message on failure.
func readDocInput(r *http.Request) (docInput, string) {
	if raw := strings.TrimSpace(r.FormValue("url")); raw != "" {
		u, err := safefetch.Validate(raw)
		if err != nil {
			return docInput{}, err.Error()
		}
		name := strings.TrimPrefix(u.Hostname(), "www.") + strings.TrimSuffix(u.EscapedPath(), "/")
		return docInput{in: engine.ResumeInput{Kind: "link"}, name: clipName(name), url: u.String()}, ""
	}
	if txt := strings.TrimSpace(r.FormValue("text")); txt != "" {
		if !utf8.ValidString(txt) {
			return docInput{}, "That text couldn't be read."
		}
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			name = "Pasted notes"
		}
		return docInput{in: engine.ResumeInput{Text: txt, Kind: "text"}, name: clipName(name)}, ""
	}
	f, hdr, err := r.FormFile("resume")
	if err != nil {
		return docInput{}, "Choose a file, paste some text or add a link."
	}
	defer f.Close()
	if hdr.Size == 0 || hdr.Size > maxUpload {
		return docInput{}, "Files must be under 5 MB."
	}
	buf, err := io.ReadAll(io.LimitReader(f, maxUpload+1))
	if err != nil || len(buf) > maxUpload {
		return docInput{}, "Files must be under 5 MB."
	}
	name := clipName(hdr.Filename)
	lower := strings.ToLower(hdr.Filename)
	switch {
	case bytes.HasPrefix(buf, []byte("%PDF-")):
		return docInput{in: engine.ResumeInput{PDF: buf, Kind: "pdf"}, name: name}, ""
	case docx.IsDocx(buf):
		text, err := docx.Text(buf)
		if err != nil {
			return docInput{}, upperFirst(err.Error()) + "."
		}
		return docInput{in: engine.ResumeInput{Text: text, Kind: "docx"}, name: name}, ""
	case strings.HasSuffix(lower, ".doc"):
		return docInput{}, "Old Word .doc files aren't supported. Save it as .docx or PDF and upload that."
	case (strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".md")) && utf8.Valid(buf) && !bytes.ContainsRune(buf, 0):
		return docInput{in: engine.ResumeInput{Text: string(buf), Kind: "text"}, name: name}, ""
	}
	return docInput{}, "Upload a PDF, a Word document (.docx) or a text file (.txt, .md)."
}

func clipName(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > 120 {
		s = string([]rune(s)[:120])
	}
	return s
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
