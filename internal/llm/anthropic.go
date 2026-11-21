package llm

import (
	"context"
	"errors"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// Anthropic calls Claude through the official Go SDK, with schema-enforced output
// and native PDF input.
type Anthropic struct {
	client anthropic.Client
	model  string
	effort string
}

func NewAnthropic(apiKey, model, effort string) *Anthropic {
	return &Anthropic{client: anthropic.NewClient(option.WithAPIKey(apiKey)), model: model, effort: effort}
}

func (a *Anthropic) Name() string  { return "anthropic" }
func (a *Anthropic) Model() string { return a.model }
func (a *Anthropic) Caps() Caps {
	return Caps{NativePDF: true, Vision: true, Structured: StructuredSchema}
}

// supportsEffort: the effort control exists on the Claude 5 family only.
func (a *Anthropic) supportsEffort() bool {
	return a.effort != "off" && strings.HasPrefix(a.model, "claude-") && strings.Contains(a.model, "-5")
}

func (a *Anthropic) Complete(ctx context.Context, r Request) (string, error) {
	var blocks []anthropic.ContentBlockParamUnion
	for _, p := range r.Parts {
		switch {
		case len(p.PDF) > 0:
			blocks = append(blocks, anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{Data: b64(p.PDF)}))
		case len(p.Image) > 0:
			blocks = append(blocks, anthropic.NewImageBlockBase64("image/jpeg", b64(p.Image)))
		case p.Text != "":
			blocks = append(blocks, anthropic.NewTextBlock(p.Text))
		}
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: int64(r.MaxTokens),
		System:    []anthropic.TextBlockParam{{Text: r.System}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(blocks...)},
	}
	if r.Schema != nil {
		params.OutputConfig.Format = anthropic.JSONOutputFormatParam{Schema: r.Schema}
	}
	if a.supportsEffort() {
		eff := string(r.Effort)
		if a.effort != "" {
			eff = a.effort
		}
		if eff != "" {
			params.OutputConfig.Effort = anthropic.OutputConfigEffort(eff)
		}
	}

	msg, err := a.client.Messages.New(ctx, params)
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			switch apiErr.StatusCode {
			case 429:
				return "", &Error{Kind: KindRateLimited, Status: 429, Msg: "rate limited"}
			case 401, 403:
				return "", &Error{Kind: KindAuth, Status: apiErr.StatusCode, Msg: "authentication failed"}
			case 400:
				return "", &Error{Kind: KindBadRequest, Status: 400, Msg: "bad request"}
			}
			return "", &Error{Kind: KindOther, Status: apiErr.StatusCode, Msg: "api error"}
		}
		return "", err
	}
	switch msg.StopReason {
	case anthropic.StopReasonRefusal:
		return "", &Error{Kind: KindRefused, Msg: "declined"}
	case anthropic.StopReasonMaxTokens:
		return "", &Error{Kind: KindTruncated, Msg: "output hit the token limit"}
	}
	var sb strings.Builder
	for _, b := range msg.Content {
		if t, ok := b.AsAny().(anthropic.TextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	return sb.String(), nil
}
