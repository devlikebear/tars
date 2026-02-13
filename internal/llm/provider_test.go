package llm

import "testing"

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

func TestNewProvider_CodexCLIDoesNotRequireToken(t *testing.T) {
	client, err := NewProvider(ProviderOptions{
		Provider: "codex-cli",
		Model:    "gpt-5.3-codex",
	})
	if err != nil {
		t.Fatalf("expected no error for codex-cli provider, got %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}
