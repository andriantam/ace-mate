// Package config loads settings from environment variables (and .env.local in development).
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type OAuth struct{ ID, Secret, Issuer string }

func (o OAuth) Enabled() bool { return o.ID != "" && o.Secret != "" }

type LLM struct {
	Provider    string // "anthropic", "openai" (any OpenAI-compatible API) or "" (disabled)
	Model       string
	APIKey      string
	BaseURL     string
	Structured  string // json_schema | json_object | prompt (openai provider only)
	NativePDF   bool   // openai provider: whether the model accepts PDFs directly
	Vision      bool   // openai provider: whether the model accepts images (enables the visual resume read)
	ResumeMode  string // auto | text | vision: how the resume is read (auto reads both ways and cross-checks)
	MaxTokens   int
	Concurrency int
	BatchSize   int
	Effort      string // anthropic: low|medium|high|off
	DisplayName string // shown to users, e.g. "Claude Opus 5.5"; derived from Model when empty
	Privacy     string // secure | not_secure: the label users see before uploading (see README)
}

func (l LLM) Enabled() bool { return l.Provider != "" }

// Legal fills the Terms and Privacy Policy. Forks must set their own values.
type Legal struct {
	Operator     string // who runs this instance (person or company)
	Contact      string // privacy and data-protection contact email
	Hosting      string // where the servers and database run, e.g. "Fly.io (Singapore)"
	Jurisdiction string // governing law and the main data-protection regime
	MinAge       int
}

type Config struct {
	Addr        string
	BaseURL     string
	Production  bool
	DatabaseURL string
	DataDir     string
	// EncryptionKeys are the master keys that wrap each user's data key:
	// "version:base64key" entries, comma-separated. Required in production.
	EncryptionKeys string
	Google         OAuth
	Okta           OAuth
	DevLogin       bool
	TrustProxy     bool // honour X-Forwarded-For for rate limiting (set behind a proxy)
	DailyLimit     int
	LLM            LLM      // the default model
	DocsLLM        LLM      // reads resumes, documents and links (DOCS_LLM_*, else the default)
	JourneyLLM     LLM      // analyses job posts and writes journeys (JOURNEY_LLM_*, else the default)
	Choices        []Choice // models users can pick per upload or job (ANTHROPIC_*, OPENROUTER_*)
	Legal          Legal
	// GoogleBirthday requests the user's birthday from Google (a sensitive scope that
	// needs Google's app verification in production). Off by default: see README.
	GoogleBirthday bool
}

func (c Config) Secure() bool { return strings.HasPrefix(c.BaseURL, "https://") }

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func envBool(k string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(k))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func Load() (*Config, error) {
	// Development convenience; real environment variables always win.
	_ = godotenv.Load(".env.local", ".env")

	port := env("PORT", "3000")
	c := &Config{
		Production:     env("APP_ENV", "development") == "production",
		DatabaseURL:    env("DATABASE_URL", ""),
		DataDir:        env("DATA_DIR", ".data"),
		EncryptionKeys: env("ENCRYPTION_KEYS", ""),
		DevLogin:       envBool("DEV_LOGIN"),
		TrustProxy:     envBool("TRUST_PROXY"),
		DailyLimit:     envInt("GENERATION_DAILY_LIMIT", 3),
		Google:         OAuth{ID: env("AUTH_GOOGLE_ID", ""), Secret: env("AUTH_GOOGLE_SECRET", "")},
		Okta:           OAuth{ID: env("AUTH_OKTA_ID", ""), Secret: env("AUTH_OKTA_SECRET", ""), Issuer: strings.TrimRight(env("AUTH_OKTA_ISSUER", ""), "/")},
		Legal: Legal{
			Operator:     env("LEGAL_OPERATOR", ""),
			Contact:      env("LEGAL_CONTACT_EMAIL", ""),
			Hosting:      env("LEGAL_HOSTING", "our hosting provider"),
			Jurisdiction: env("LEGAL_JURISDICTION", "Singapore"),
			MinAge:       envInt("LEGAL_MIN_AGE", 18),
		},
		GoogleBirthday: envBool("AUTH_GOOGLE_BIRTHDAY"),
	}
	c.BaseURL = strings.TrimRight(env("BASE_URL", "http://localhost:"+port), "/")
	c.Addr = env("ADDR", ":"+port)

	if _, err := url.ParseRequestURI(c.BaseURL); err != nil {
		return nil, fmt.Errorf("BASE_URL is not a valid URL: %w", err)
	}
	if c.Production {
		if !c.Secure() {
			return nil, errors.New("APP_ENV=production requires BASE_URL to start with https://")
		}
		if c.DatabaseURL == "" {
			return nil, errors.New("APP_ENV=production requires DATABASE_URL")
		}
		if c.EncryptionKeys == "" {
			return nil, errors.New("APP_ENV=production requires ENCRYPTION_KEYS (generate one with: openssl rand -base64 32)")
		}
		if c.DevLogin {
			return nil, errors.New("DEV_LOGIN must not be enabled in production")
		}
	}
	if c.DevLogin && !strings.HasPrefix(c.BaseURL, "http://localhost") && !strings.HasPrefix(c.BaseURL, "http://127.0.0.1") {
		return nil, errors.New("DEV_LOGIN only works when BASE_URL is http://localhost or http://127.0.0.1")
	}

	// ── LLM ──
	// One default model (LLM_*), optionally overridden per tool: DOCS_LLM_* for reading
	// resumes, documents and links, JOURNEY_LLM_* for job analysis and journeys.
	def, err := loadLLM("", nil)
	if err != nil {
		return nil, err
	}
	if c.DocsLLM, err = loadLLM("DOCS_", &def); err != nil {
		return nil, err
	}
	if c.JourneyLLM, err = loadLLM("JOURNEY_", &def); err != nil {
		return nil, err
	}
	c.LLM = def
	if c.Choices, err = loadChoices(def); err != nil {
		return nil, err
	}
	if c.Production && c.Legal.Contact == "" {
		return nil, errors.New("APP_ENV=production requires LEGAL_CONTACT_EMAIL (shown in the Privacy Policy)")
	}
	return c, nil
}

// loadLLM reads the model settings for one tool. With base == nil it reads the default
// LLM_* settings. Otherwise every PREFIX+LLM_* variable that is set overrides the default,
// and anything not set is inherited. Choosing a different provider or host inherits nothing
// provider-specific, and never the default's API key, so a key can't be sent to the wrong service.
func loadLLM(prefix string, base *LLM) (LLM, error) {
	val := func(name string) string { return strings.TrimSpace(os.Getenv(prefix + name)) }
	name := func(v string) string { return prefix + v }

	var l LLM
	provider := strings.ToLower(val("LLM_PROVIDER"))
	switch {
	case base == nil:
		l = LLM{
			Structured:  "json_schema",
			ResumeMode:  strings.ToLower(env("LLM_RESUME_MODE", "auto")),
			Concurrency: envInt("LLM_CONCURRENCY", 4),
			BatchSize:   envInt("LLM_BATCH_SIZE", 3),
			Effort:      strings.ToLower(env("ANTHROPIC_EFFORT", "")),
		}
		if provider == "" && env("ANTHROPIC_API_KEY", "") != "" {
			provider = "anthropic"
		}
		l.Provider = provider
	case provider != "" && provider != base.Provider:
		l = LLM{Provider: provider, Structured: "json_schema", ResumeMode: base.ResumeMode,
			Concurrency: base.Concurrency, BatchSize: base.BatchSize, Effort: base.Effort}
	default:
		l = *base
	}
	changed := base == nil || l.Provider != base.Provider
	if v := val("LLM_MODEL"); v != "" {
		changed = changed || v != l.Model
		l.Model = v
	}
	if v := strings.TrimRight(val("LLM_BASE_URL"), "/"); v != "" && v != l.BaseURL {
		changed = true
		l.BaseURL, l.APIKey = v, "" // a different host must not receive the default's key
	}
	if v := val("LLM_API_KEY"); v != "" {
		l.APIKey = v
	}
	if v := val("LLM_STRUCTURED"); v != "" {
		l.Structured = strings.ToLower(v)
	}
	if v := val("LLM_NATIVE_PDF"); v != "" {
		l.NativePDF = parseBool(v)
	}
	if v := val("LLM_VISION"); v != "" {
		l.Vision = parseBool(v)
	}
	if v := val("LLM_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			l.MaxTokens = n
		}
	}
	if changed && base != nil {
		l.DisplayName, l.Privacy = "", "" // describe the new model, not the default's
	}
	if v := val("LLM_DISPLAY_NAME"); v != "" {
		l.DisplayName = v
	}
	if v := val("LLM_PRIVACY"); v != "" {
		l.Privacy = strings.ToLower(v)
	}

	switch l.Provider {
	case "":
		return l, nil // no model: the app runs without AI features
	case "anthropic":
		if l.APIKey == "" {
			l.APIKey = val("ANTHROPIC_API_KEY")
		}
		if l.APIKey == "" {
			l.APIKey = env("ANTHROPIC_API_KEY", "") // only ever an Anthropic key, so safe for any Anthropic tool
		}
		if l.APIKey == "" {
			return l, fmt.Errorf("%s=anthropic needs ANTHROPIC_API_KEY", name("LLM_PROVIDER"))
		}
		if l.Model == "" {
			l.Model = "claude-haiku-5-5"
		}
		if l.MaxTokens == 0 {
			l.MaxTokens = 16000
		}
	case "openai":
		if l.BaseURL == "" || l.Model == "" {
			return l, fmt.Errorf("%s=openai needs %s and %s (see README)", name("LLM_PROVIDER"), name("LLM_BASE_URL"), name("LLM_MODEL"))
		}
		switch l.Structured {
		case "json_schema", "json_object", "prompt":
		default:
			return l, fmt.Errorf("%s must be json_schema, json_object or prompt", name("LLM_STRUCTURED"))
		}
		if l.MaxTokens == 0 {
			l.MaxTokens = 8192
		}
	default:
		return l, fmt.Errorf("unknown %s %q (use anthropic or openai)", name("LLM_PROVIDER"), l.Provider)
	}
	if l.DisplayName == "" {
		l.DisplayName = ModelDisplayName(l.Model)
	}
	switch l.Privacy {
	case "":
		l.Privacy = defaultPrivacy(l)
	case "secure", "not_secure":
	default:
		return l, fmt.Errorf("%s must be secure or not_secure", name("LLM_PRIVACY"))
	}
	switch l.ResumeMode {
	case "auto", "text", "vision":
	default:
		return l, errors.New("LLM_RESUME_MODE must be auto, text or vision")
	}
	return l, nil
}

func parseBool(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Host is where model requests go, for showing users where their data is sent.
func (l LLM) Host() string {
	switch l.Provider {
	case "anthropic":
		return "api.anthropic.com"
	case "openai":
		if u, err := url.Parse(l.BaseURL); err == nil {
			return u.Hostname()
		}
	}
	return ""
}

// defaultPrivacy labels a provider "secure" only when data stays on our own machine or
// the provider's standard API terms say inputs are not used for training (Anthropic's
// commercial API). Every other hosted API, free tiers especially, defaults to not secure.
// Operators can override with LLM_PRIVACY after reading their provider's terms.
func defaultPrivacy(l LLM) string {
	switch l.Provider {
	case "anthropic":
		return "secure"
	case "openai":
		h := l.Host()
		if h == "localhost" || h == "127.0.0.1" || h == "::1" || strings.HasSuffix(h, ".internal") {
			return "secure"
		}
	}
	return "not_secure"
}

// ModelDisplayName turns a model id into a readable name:
// "claude-opus-5-5" → "Claude Opus 5.5", "nvidia/nemotron-3.5-super-vl:free" → "Nemotron 3.5 Super VL".
func ModelDisplayName(id string) string {
	if id == "" {
		return ""
	}
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	if i := strings.Index(id, ":"); i >= 0 {
		id = id[:i]
	}
	var words []string
	for _, p := range strings.FieldsFunc(id, func(r rune) bool { return r == '-' || r == '_' || r == ' ' }) {
		isNum := p != "" && strings.Trim(p, "0123456789.") == ""
		if isNum && len(words) > 0 && strings.Trim(words[len(words)-1], "0123456789.") == "" {
			words[len(words)-1] += "." + p // "5", "5" → "5.5"
			continue
		}
		switch {
		case isNum:
		case len(p) <= 3 && strings.Trim(p, "abcdefghijklmnopqrstuvwxyz") == "" && p != "pro" && p != "max":
			p = strings.ToUpper(p) // vl → VL, llm → LLM
		default:
			p = strings.ToUpper(p[:1]) + p[1:]
		}
		words = append(words, p)
	}
	return strings.Join(words, " ")
}
