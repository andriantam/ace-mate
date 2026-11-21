package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAIConfig struct {
	BaseURL    string // e.g. http://localhost:11434/v1
	APIKey     string // optional for local servers
	Model      string
	Structured StructuredMode
	NativePDF  bool
	Vision     bool // the model accepts images (image_url content parts)
}

// OpenAI talks to any server implementing the OpenAI chat-completions API.
type OpenAI struct {
	cfg    OpenAIConfig
	client *http.Client
}

func NewOpenAI(c OpenAIConfig) *OpenAI {
	return &OpenAI{cfg: c, client: &http.Client{Timeout: 10 * time.Minute}}
}

func (o *OpenAI) Name() string  { return "openai-compatible" }
func (o *OpenAI) Model() string { return o.cfg.Model }
func (o *OpenAI) Caps() Caps {
	return Caps{NativePDF: o.cfg.NativePDF, Vision: o.cfg.Vision, Structured: o.cfg.Structured}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	ResponseFormat map[string]any `json:"response_format,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (o *OpenAI) Complete(ctx context.Context, r Request) (string, error) {
	var user []map[string]any
	for _, p := range r.Parts {
		switch {
		case len(p.PDF) > 0:
			// Only reached when NativePDF is set; uses the widely supported "file" content part.
			user = append(user, map[string]any{"type": "file", "file": map[string]any{
				"filename":  "resume.pdf",
				"file_data": "data:application/pdf;base64," + b64(p.PDF),
			}})
		case len(p.Image) > 0:
			user = append(user, map[string]any{"type": "image_url", "image_url": map[string]any{
				"url": "data:image/jpeg;base64," + b64(p.Image),
			}})
		case p.Text != "":
			user = append(user, map[string]any{"type": "text", "text": p.Text})
		}
	}
	// Plain string content is the most compatible form when there are no non-text parts.
	var content any = user
	if allText(user) {
		var sb strings.Builder
		for i, u := range user {
			if i > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(u["text"].(string))
		}
		content = sb.String()
	}

	body := chatRequest{
		Model:     o.cfg.Model,
		MaxTokens: r.MaxTokens,
		Messages:  []chatMessage{{Role: "system", Content: r.System}, {Role: "user", Content: content}},
	}
	switch o.cfg.Structured {
	case StructuredSchema:
		if r.Schema != nil {
			body.ResponseFormat = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "result", "strict": true, "schema": r.Schema}}
		}
	case StructuredObject:
		body.ResponseFormat = map[string]any{"type": "json_object"}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt*attempt) * 2 * time.Second):
			}
		}
		out, retry, err := o.do(ctx, payload)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return "", lastErr
}

func (o *OpenAI) do(ctx context.Context, payload []byte) (text string, retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.cfg.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.cfg.APIKey)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return "", ctx.Err() == nil, &Error{Kind: KindOther, Msg: "could not reach the model server"}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return "", true, &Error{Kind: KindRateLimited, Status: resp.StatusCode, Msg: "rate limited"}
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return "", false, &Error{Kind: KindAuth, Status: resp.StatusCode, Msg: "authentication failed"}
	case resp.StatusCode >= 500:
		return "", true, &Error{Kind: KindOther, Status: resp.StatusCode, Msg: "server error"}
	case resp.StatusCode >= 400:
		return "", false, &Error{Kind: KindBadRequest, Status: resp.StatusCode, Msg: apiMessage(raw)}
	}

	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil || len(cr.Choices) == 0 {
		return "", false, &Error{Kind: KindOther, Msg: "unexpected response shape"}
	}
	ch := cr.Choices[0]
	if ch.FinishReason == "length" {
		return "", false, &Error{Kind: KindTruncated, Msg: "output hit the token limit"}
	}
	if ch.FinishReason == "content_filter" {
		return "", false, &Error{Kind: KindRefused, Msg: "content filtered"}
	}
	return ch.Message.Content, false, nil
}

func apiMessage(raw []byte) string {
	var e chatResponse
	if json.Unmarshal(raw, &e) == nil && e.Error != nil && e.Error.Message != "" {
		m := e.Error.Message
		if len(m) > 300 {
			m = m[:300]
		}
		return m
	}
	return fmt.Sprintf("bad request (%d bytes)", len(raw))
}

func allText(parts []map[string]any) bool {
	for _, p := range parts {
		if p["type"] != "text" {
			return false
		}
	}
	return true
}
