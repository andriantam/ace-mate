package config

import "testing"

func clear(t *testing.T) {
	for _, k := range []string{"APP_ENV", "BASE_URL", "DATABASE_URL", "DEV_LOGIN", "LLM_PROVIDER", "LLM_MODEL", "LLM_BASE_URL", "LLM_API_KEY",
		"ANTHROPIC_API_KEY", "LLM_STRUCTURED", "LLM_VISION", "LLM_RESUME_MODE", "LLM_PRIVACY", "LLM_DISPLAY_NAME", "LEGAL_CONTACT_EMAIL", "PORT", "ADDR", "AUTH_GOOGLE_ID", "AUTH_GOOGLE_SECRET", "ENCRYPTION_KEYS"} {
		t.Setenv(k, "")
	}
}

func TestProductionGuards(t *testing.T) {
	clear(t)
	t.Setenv("APP_ENV", "production")
	if _, err := Load(); err == nil {
		t.Error("production without https must fail")
	}
	t.Setenv("BASE_URL", "https://example.com")
	if _, err := Load(); err == nil {
		t.Error("production without DATABASE_URL must fail")
	}
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("DEV_LOGIN", "true")
	if _, err := Load(); err == nil {
		t.Error("production must refuse DEV_LOGIN")
	}
	t.Setenv("DEV_LOGIN", "")
	if _, err := Load(); err == nil {
		t.Error("production without ENCRYPTION_KEYS must fail")
	}
	t.Setenv("ENCRYPTION_KEYS", "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if _, err := Load(); err == nil {
		t.Error("production without a privacy contact must fail")
	}
	t.Setenv("LEGAL_CONTACT_EMAIL", "privacy@example.com")
	if _, err := Load(); err != nil {
		t.Errorf("valid production config rejected: %v", err)
	}
}

func TestDevLoginOnlyOnLocalhost(t *testing.T) {
	clear(t)
	t.Setenv("DEV_LOGIN", "true")
	t.Setenv("BASE_URL", "http://myserver.example.com")
	if _, err := Load(); err == nil {
		t.Error("DEV_LOGIN on a non-local URL must fail")
	}
	t.Setenv("BASE_URL", "http://localhost:3000")
	if _, err := Load(); err != nil {
		t.Errorf("localhost dev login rejected: %v", err)
	}
}

func TestLLMSelection(t *testing.T) {
	clear(t)
	c, err := Load()
	if err != nil || c.LLM.Enabled() {
		t.Fatalf("no key → LLM disabled, got %+v %v", c.LLM, err)
	}
	t.Setenv("ANTHROPIC_API_KEY", "k")
	c, _ = Load()
	if c.LLM.Provider != "anthropic" || c.LLM.Model != "claude-haiku-5-5" {
		t.Errorf("anthropic default: %+v", c.LLM)
	}
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("LLM_PROVIDER", "openai")
	if _, err := Load(); err == nil {
		t.Error("openai provider without base URL and model must fail")
	}
	t.Setenv("LLM_BASE_URL", "http://localhost:11434/v1")
	t.Setenv("LLM_MODEL", "llama3.3")
	c, err = Load()
	if err != nil || c.LLM.Provider != "openai" || c.LLM.MaxTokens != 8192 || c.LLM.Structured != "json_schema" {
		t.Errorf("openai config: %+v %v", c.LLM, err)
	}
	t.Setenv("LLM_STRUCTURED", "bogus")
	if _, err := Load(); err == nil {
		t.Error("bad LLM_STRUCTURED must fail")
	}
}

func TestResumeModeAndVision(t *testing.T) {
	clear(t)
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", "http://localhost:1/v1")
	t.Setenv("LLM_MODEL", "m")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.LLM.ResumeMode != "auto" || c.LLM.Vision {
		t.Fatalf("defaults: mode=%q vision=%v", c.LLM.ResumeMode, c.LLM.Vision)
	}
	t.Setenv("LLM_VISION", "true")
	t.Setenv("LLM_RESUME_MODE", "vision")
	c, err = Load()
	if err != nil || !c.LLM.Vision || c.LLM.ResumeMode != "vision" {
		t.Fatalf("err=%v %+v", err, c.LLM)
	}
	t.Setenv("LLM_RESUME_MODE", "bogus")
	if _, err := Load(); err == nil {
		t.Error("an unknown LLM_RESUME_MODE must be rejected")
	}
}

func TestModelDisplayName(t *testing.T) {
	cases := map[string]string{
		"claude-opus-5-5":                   "Claude Opus 5.5",
		"claude-haiku-5-5":                  "Claude Haiku 5.5",
		"nvidia/nemotron-3.5-super-vl:free": "Nemotron 3.5 Super VL",
		"qwen2.5:14b":                       "Qwen2.5",
		"deepseek-chat":                     "Deepseek Chat",
	}
	for in, want := range cases {
		if got := ModelDisplayName(in); got != want {
			t.Errorf("ModelDisplayName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrivacyLabel(t *testing.T) {
	clear(t)
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_MODEL", "m")
	for base, want := range map[string]string{
		"https://openrouter.ai/api/v1": "not_secure",
		"http://localhost:11434/v1":    "secure",
	} {
		t.Setenv("LLM_BASE_URL", base)
		c, err := Load()
		if err != nil || c.LLM.Privacy != want {
			t.Errorf("%s: privacy=%q err=%v, want %q", base, c.LLM.Privacy, err, want)
		}
	}
	t.Setenv("LLM_BASE_URL", "https://openrouter.ai/api/v1")
	t.Setenv("LLM_PRIVACY", "secure")
	if c, _ := Load(); c.LLM.Privacy != "secure" {
		t.Error("LLM_PRIVACY should override the default")
	}
	t.Setenv("LLM_PRIVACY", "very")
	if _, err := Load(); err == nil {
		t.Error("an unknown LLM_PRIVACY must be rejected")
	}
}

func TestPerToolModels(t *testing.T) {
	clear(t)
	for _, p := range []string{"DOCS_", "JOURNEY_"} {
		for _, k := range []string{"LLM_PROVIDER", "LLM_MODEL", "LLM_BASE_URL", "LLM_API_KEY", "LLM_STRUCTURED", "LLM_VISION", "LLM_PRIVACY", "LLM_DISPLAY_NAME", "ANTHROPIC_API_KEY"} {
			t.Setenv(p+k, "")
		}
	}
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", "https://models.example.com/v1")
	t.Setenv("LLM_MODEL", "vision-model")
	t.Setenv("LLM_API_KEY", "default-key")
	t.Setenv("LLM_VISION", "true")

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.DocsLLM != c.LLM || c.JourneyLLM != c.LLM {
		t.Fatal("with only LLM_* set, every tool uses the default")
	}

	// Same provider and host, different model: the key is kept, the name is re-derived.
	t.Setenv("JOURNEY_LLM_MODEL", "writer-model")
	c, _ = Load()
	if j := c.JourneyLLM; j.Model != "writer-model" || j.APIKey != "default-key" || j.DisplayName != "Writer Model" || !j.Vision {
		t.Fatalf("journey = %+v", j)
	}
	if c.DocsLLM.Model != "vision-model" {
		t.Fatal("the documents model must not change")
	}

	// A different host never receives the default key.
	t.Setenv("DOCS_LLM_BASE_URL", "https://other.example.org/v1")
	c, _ = Load()
	if c.DocsLLM.APIKey != "" || c.DocsLLM.BaseURL != "https://other.example.org/v1" {
		t.Fatalf("docs = %+v", c.DocsLLM)
	}
	t.Setenv("DOCS_LLM_API_KEY", "docs-key")
	if c, _ = Load(); c.DocsLLM.APIKey != "docs-key" {
		t.Fatal("DOCS_LLM_API_KEY should be used")
	}

	// A different provider inherits nothing provider-specific.
	t.Setenv("JOURNEY_LLM_MODEL", "")
	t.Setenv("JOURNEY_LLM_PROVIDER", "anthropic")
	if _, err := Load(); err == nil {
		t.Fatal("anthropic without ANTHROPIC_API_KEY must fail")
	}
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-key")
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if j := c.JourneyLLM; j.Provider != "anthropic" || j.Model != "claude-haiku-5-5" || j.APIKey != "anthropic-key" || j.BaseURL != "" || j.Privacy != "secure" {
		t.Fatalf("journey = %+v", j)
	}
	if c.LLM.Provider != "openai" {
		t.Fatal("the default must not change")
	}
}
