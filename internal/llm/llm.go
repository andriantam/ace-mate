// Package llm is the provider-agnostic LLM layer. The engine talks only to the
// Provider interface, so switching models is configuration, not code:
//
//	LLM_PROVIDER=anthropic  → Claude via the official SDK
//	LLM_PROVIDER=openai     → anything that speaks the OpenAI chat API
//	                          (Ollama, DeepSeek, Qwen, Llama hosts, Groq, OpenRouter, …)
//
// To add a new native provider, implement Provider and register it in New.
package llm

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/andriantam/ace-mate/internal/config"
)

// StructuredMode is how a provider can be made to return JSON.
type StructuredMode string

const (
	// StructuredSchema: the API enforces a JSON Schema.
	StructuredSchema StructuredMode = "json_schema"
	// StructuredObject: the API guarantees valid JSON but not the shape; the schema goes in the prompt.
	StructuredObject StructuredMode = "json_object"
	// StructuredPrompt: nothing is enforced; the schema goes in the prompt.
	StructuredPrompt StructuredMode = "prompt"
)

type Effort string

const (
	EffortLow    Effort = "low"
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
)

// Part is one piece of the user message: text, a PDF document, or an image (JPEG).
type Part struct {
	Text  string
	PDF   []byte
	Image []byte // JPEG
}

type Request struct {
	System    string
	Parts     []Part
	Schema    map[string]any // set only when the provider's mode is StructuredSchema
	MaxTokens int
	Effort    Effort // a hint; providers that have no such control ignore it
}

type Caps struct {
	NativePDF  bool
	Vision     bool // accepts images
	Structured StructuredMode
}

type Provider interface {
	Name() string
	Model() string
	Caps() Caps
	// Complete returns the model's raw text reply.
	Complete(ctx context.Context, r Request) (string, error)
}

type Kind int

const (
	KindOther Kind = iota
	KindRateLimited
	KindAuth
	KindRefused
	KindTruncated
	KindBadRequest
)

// Error is a provider failure with a coarse category the engine can explain to users.
type Error struct {
	Kind   Kind
	Status int
	Msg    string
}

func (e *Error) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("llm: %s (status %d)", e.Msg, e.Status)
	}
	return "llm: " + e.Msg
}

func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindOther
}

// New builds the configured provider, or (nil, nil) when no provider is configured.
func New(c config.LLM) (Provider, error) {
	switch c.Provider {
	case "":
		return nil, nil
	case "anthropic":
		return NewAnthropic(c.APIKey, c.Model, c.Effort), nil
	case "openai":
		return NewOpenAI(OpenAIConfig{
			BaseURL: c.BaseURL, APIKey: c.APIKey, Model: c.Model,
			Structured: StructuredMode(c.Structured), NativePDF: c.NativePDF, Vision: c.Vision,
		}), nil
	}
	return nil, fmt.Errorf("unknown provider %q", c.Provider)
}

var (
	thinkRe = regexp.MustCompile(`(?s)<think>.*?</think>`)
	fenceRe = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```")
)

// ExtractJSON pulls the JSON object out of a model reply that may be wrapped in
// reasoning tags, markdown fences or commentary.
func ExtractJSON(s string) string {
	s = thinkRe.ReplaceAllString(s, "")
	if m := fenceRe.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	s = strings.TrimSpace(s)
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
