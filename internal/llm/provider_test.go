package llm

import (
	"strings"
	"testing"
)

func TestNewProvider_Unsupported(t *testing.T) {
	_, err := NewProvider(ProviderOptions{
		Provider: "unknown",
		AuthMode: "api-key",
		APIKey:   "k",
	})
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestNewProvider_CodexCLIIsRemoved(t *testing.T) {
	_, err := NewProvider(ProviderOptions{
		Provider: "codex-cli",
		Model:    "gpt-4o-mini",
	})
	if err == nil {
		t.Fatal("expected error for removed codex-cli provider")
	}
}

func TestNewProvider_OpenAICodexIsRemoved(t *testing.T) {
	_, err := NewProvider(ProviderOptions{
		Provider: "openai-codex",
		Model:    "gpt-5.3-codex",
	})
	if err == nil {
		t.Fatal("expected error for removed openai-codex provider")
	}
	if !strings.Contains(err.Error(), "removed") {
		t.Fatalf("expected removed error, got %v", err)
	}
}
