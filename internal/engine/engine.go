// Package engine turns a resume and a job posting into a readiness journey using
// whichever llm.Provider is configured.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/invopop/jsonschema"

	"github.com/andriantam/ace-mate/internal/llm"
	"github.com/andriantam/ace-mate/internal/pdfx"
)

// UserError carries a message that is safe and useful to show to the user.
type UserError struct {
	Msg       string
	Truncated bool // the model ran out of output tokens: a smaller request may succeed
	Transient bool // the AI service was busy or unreachable: trying again later may work
}

func (e *UserError) Error() string { return e.Msg }

// Retryable tells the background workers whether another attempt may succeed.
func (e *UserError) Retryable() bool { return e.Transient }

type Engine struct {
	P           llm.Provider
	MaxTokens   int
	Concurrency int
	BatchSize   int
	ResumeMode  string // auto | text | vision
}

func New(p llm.Provider, maxTokens, concurrency, batchSize int) *Engine {
	return &Engine{P: p, MaxTokens: maxTokens, Concurrency: max(1, concurrency), BatchSize: max(1, batchSize)}
}

// Event reports progress while a resume is read, so the UI can show what is happening.
type Event struct {
	Step    string   `json:"step"` // extract | render | read | read_done | reconcile
	Detail  string   `json:"detail,omitempty"`
	Pages   int      `json:"pages,omitempty"`
	Method  string   `json:"method,omitempty"`  // read_done: text | visual | pdf
	Methods []string `json:"methods,omitempty"` // read: the readings that are running
	OK      bool     `json:"ok,omitempty"`      // read_done: whether that reading succeeded
	Done    int      `json:"done,omitempty"`    // journeys: batches written so far
	Total   int      `json:"total,omitempty"`   // journeys: batches to write
	Aspect  string   `json:"aspect,omitempty"`  // journeys: the lane a batch belongs to
}

// ── schema ──

var schemaCache sync.Map // reflect.Type → map[string]any

func schemaFor[T any]() map[string]any {
	t := reflect.TypeFor[T]()
	if v, ok := schemaCache.Load(t); ok {
		return v.(map[string]any)
	}
	r := &jsonschema.Reflector{DoNotReference: true, ExpandedStruct: true, AllowAdditionalProperties: false}
	raw, err := json.Marshal(r.Reflect(new(T)))
	if err != nil {
		panic(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		panic(err)
	}
	delete(m, "$schema")
	delete(m, "$id")
	schemaCache.Store(t, m)
	return m
}

// ── one validated, retried structured call ──

type call struct {
	system string
	parts  []llm.Part
	effort llm.Effort
}

const maxAttempts = 3

func structured[T any](ctx context.Context, e *Engine, c call, validate func(*T) error) (T, error) {
	var zero T
	caps := e.P.Caps()
	schema := schemaFor[T]()
	system := c.system
	parts := c.parts

	for _, p := range c.parts {
		if len(p.Image) > 0 && !caps.Vision {
			return zero, &UserError{Msg: "The configured model cannot read images. Set LLM_VISION=true for a vision model."}
		}
	}
	if !caps.NativePDF {
		converted := make([]llm.Part, 0, len(parts))
		for _, p := range parts {
			if len(p.PDF) == 0 {
				converted = append(converted, p)
				continue
			}
			text, err := pdfx.Text(p.PDF)
			if err != nil {
				return zero, &UserError{Msg: err.Error()}
			}
			converted = append(converted, llm.Part{Text: "<resume>\n" + text + "\n</resume>"})
		}
		parts = converted
	}

	var req llm.Request
	if caps.Structured == llm.StructuredSchema {
		req.Schema = schema
	} else {
		raw, _ := json.Marshal(schema)
		system += "\n\nRespond with ONE JSON object and nothing else (no markdown, no commentary). It must match this JSON Schema exactly, and every field is required:\n" + string(raw)
	}
	req.System, req.MaxTokens, req.Effort = system, e.MaxTokens, c.effort

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		req.Parts = parts
		if lastErr != nil {
			req.Parts = append(append([]llm.Part(nil), parts...), llm.Part{Text: "Your previous reply was rejected: " + lastErr.Error() + "\nReturn the corrected JSON object only."})
		}
		text, err := e.P.Complete(ctx, req)
		if err != nil {
			return zero, explain(err)
		}
		var out T
		if err := json.Unmarshal([]byte(llm.ExtractJSON(text)), &out); err != nil {
			lastErr = errors.New("it was not valid JSON for the schema")
			continue
		}
		if err := validate(&out); err != nil {
			lastErr = err
			continue
		}
		return out, nil
	}
	return zero, &UserError{Msg: "The AI returned an unexpected format. Try again, or try a more capable model."}
}

func explain(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &UserError{Msg: "The AI took too long to respond. Try again, or use a faster model."}
	}
	var le *llm.Error
	if errors.As(err, &le) {
		switch le.Kind {
		case llm.KindRateLimited:
			return &UserError{Transient: true, Msg: "The AI service is busy. Try again in a minute."}
		case llm.KindAuth:
			return &UserError{Msg: "The AI engine is not configured correctly."}
		case llm.KindRefused:
			return &UserError{Msg: "The AI declined to process this input."}
		case llm.KindTruncated:
			return &UserError{Truncated: true, Msg: "The AI response was too long for this model's output limit. Raise LLM_MAX_TOKENS (reasoning models need room to think) or use a model with a larger output limit."}
		case llm.KindBadRequest:
			return &UserError{Msg: fmt.Sprintf("The model server rejected the request: %s", le.Msg)}
		}
		return &UserError{Transient: true, Msg: "Couldn't reach the AI service. Try again."}
	}
	return err
}
