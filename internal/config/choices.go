package config

import (
	"fmt"
	"strings"
)

// Choice is a model users may pick for one upload or one job application.
type Choice struct {
	ID  string // stable id sent by the UI: "claude" or "openrouter"
	LLM LLM
}

// Same reports whether two settings reach the same model at the same host.
func (l LLM) Same(o LLM) bool {
	return l.Enabled() && o.Enabled() && l.Provider == o.Provider && l.Model == o.Model && l.Host() == o.Host()
}

// loadChoices builds the models users can pick from. Each one appears when its key is set:
//
//	ANTHROPIC_API_KEY (+ ANTHROPIC_MODEL)      → "claude"
//	OPENROUTER_API_KEY + OPENROUTER_MODEL      → "openrouter"
//
// The default model (LLM_*, DOCS_LLM_*, JOURNEY_LLM_*) keeps working as before and is the
// pre-selected one. Choices only matter when there are two or more models to pick from.
func loadChoices(def LLM) ([]Choice, error) {
	base := LLM{
		Structured:  "json_schema",
		ResumeMode:  strings.ToLower(env("LLM_RESUME_MODE", "auto")),
		Concurrency: envInt("LLM_CONCURRENCY", 4),
		BatchSize:   envInt("LLM_BATCH_SIZE", 3),
		Effort:      strings.ToLower(env("ANTHROPIC_EFFORT", "")),
	}
	var out []Choice

	if key := env("ANTHROPIC_API_KEY", ""); key != "" {
		l := base
		l.Provider, l.APIKey = "anthropic", key
		l.Model = env("ANTHROPIC_MODEL", "")
		if l.Model == "" && (def.Provider == "anthropic") {
			l.Model = def.Model
		}
		if l.Model == "" {
			l.Model = "claude-haiku-5-5"
		}
		l.MaxTokens = envInt("ANTHROPIC_MAX_TOKENS", 16000)
		l.DisplayName = env("ANTHROPIC_DISPLAY_NAME", ModelDisplayName(l.Model))
		l.Privacy = defaultPrivacy(l)
		out = append(out, Choice{ID: "claude", LLM: l})
	}

	if key := env("OPENROUTER_API_KEY", ""); key != "" {
		m := env("OPENROUTER_MODEL", "")
		if m == "" {
			return nil, fmt.Errorf("OPENROUTER_API_KEY is set but OPENROUTER_MODEL is not (use a model id from openrouter.ai/models)")
		}
		l := base
		l.Provider, l.APIKey, l.Model = "openai", key, m
		l.BaseURL = strings.TrimRight(env("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"), "/")
		l.Structured = strings.ToLower(env("OPENROUTER_STRUCTURED", "json_schema"))
		switch l.Structured {
		case "json_schema", "json_object", "prompt":
		default:
			return nil, fmt.Errorf("OPENROUTER_STRUCTURED must be json_schema, json_object or prompt")
		}
		l.Vision = envBool("OPENROUTER_VISION")
		l.NativePDF = envBool("OPENROUTER_NATIVE_PDF")
		l.MaxTokens = envInt("OPENROUTER_MAX_TOKENS", 8192)
		l.DisplayName = env("OPENROUTER_DISPLAY_NAME", ModelDisplayName(m))
		l.Privacy = defaultPrivacy(l)
		if v := strings.ToLower(env("OPENROUTER_PRIVACY", "")); v != "" {
			if v != "secure" && v != "not_secure" {
				return nil, fmt.Errorf("OPENROUTER_PRIVACY must be secure or not_secure")
			}
			l.Privacy = v
		}
		out = append(out, Choice{ID: "openrouter", LLM: l})
	}
	return out, nil
}
