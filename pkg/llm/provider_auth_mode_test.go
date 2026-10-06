package llm

import "testing"

func TestValidateProviderAuthMode(t *testing.T) {
	for _, kind := range []string{"openai", "kimi", "gemini", "gemini-native", "anthropic", "openai-codex", "claude-code-cli", "antigravity-cli"} {
		for _, mode := range []string{"", "oauth", "api-key", " OAUTH ", " API-KEY ", "cli", "invalid"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				valid := mode != "cli" && mode != "invalid" || kind == "claude-code-cli" || kind == "antigravity-cli"
				if err := ValidateProviderAuthMode(kind, mode); (err == nil) != valid {
					t.Fatalf("mode %q: %v", mode, err)
				}
			})
		}
	}
}

func TestProviderRejectsModeBeforeCredentials(t *testing.T) {
	for _, kind := range []string{"openai", "openai-codex", "anthropic", "gemini-native"} {
		if _, err := NewProvider(ProviderOptions{Provider: kind, AuthMode: "cli"}); err == nil {
			t.Fatalf("%s accepted cli", kind)
		}
	}
}
