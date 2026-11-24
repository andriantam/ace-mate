package config

import "testing"

func TestChoicesAppearWithTheirKeys(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	if c, err := loadChoices(LLM{}); err != nil || len(c) != 0 {
		t.Fatalf("no keys: %v %v", c, err)
	}

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-x")
	t.Setenv("OPENROUTER_API_KEY", "sk-or-x")
	t.Setenv("OPENROUTER_MODEL", "meta-llama/llama-3.3-70b-instruct:free")
	c, err := loadChoices(LLM{})
	if err != nil || len(c) != 2 {
		t.Fatalf("both keys: %v %v", c, err)
	}
	claude, or := c[0], c[1]
	if claude.ID != "claude" || claude.LLM.Provider != "anthropic" || claude.LLM.Privacy != "secure" {
		t.Errorf("claude: %+v", claude)
	}
	if or.ID != "openrouter" || or.LLM.Host() != "openrouter.ai" || or.LLM.Privacy != "not_secure" {
		t.Errorf("openrouter: %+v host=%s", or, or.LLM.Host())
	}
	if claude.LLM.Same(or.LLM) {
		t.Error("different providers must not match")
	}
	// The default's model carries over to the Claude choice.
	t.Setenv("ANTHROPIC_MODEL", "")
	c, _ = loadChoices(LLM{Provider: "anthropic", Model: "claude-sonnet-5-5"})
	if c[0].LLM.Model != "claude-sonnet-5-5" {
		t.Errorf("model = %s", c[0].LLM.Model)
	}
}

func TestOpenRouterKeyNeedsAModel(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "sk-or-x")
	t.Setenv("OPENROUTER_MODEL", "")
	if _, err := loadChoices(LLM{}); err == nil {
		t.Fatal("want an error")
	}
}
