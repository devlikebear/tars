package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveToken_APIKeyMode(t *testing.T) {
	token, err := ResolveToken(ResolveOptions{
		AuthMode: "api-key",
		APIKey:   "abc123",
	})
	if err != nil {
		t.Fatalf("resolve token: %v", err)
	}
	if token != "abc123" {
		t.Fatalf("expected api key token, got %q", token)
	}
}

func TestResolveToken_OAuthOpenAICodexRemoved(t *testing.T) {
	_, err := ResolveToken(ResolveOptions{
		Provider: "openai-codex",
		AuthMode: "oauth",
	})
	if err == nil {
		t.Fatal("expected unsupported oauth provider error")
	}
	if !strings.Contains(err.Error(), "unsupported oauth provider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveToken_OAuthClaudeFromFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".config", "claude-code", "oauth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"access_token":"claude-token"}`), 0o644); err != nil {
		t.Fatalf("write oauth file: %v", err)
	}

	token, err := ResolveToken(ResolveOptions{
		Provider:      "anthropic",
		AuthMode:      "oauth",
		OAuthProvider: "claude-code",
	})
	if err != nil {
		t.Fatalf("resolve token: %v", err)
	}
	if token != "claude-token" {
		t.Fatalf("expected claude token, got %q", token)
	}
}

func TestResolveToken_OAuthAntigravityFromEnv(t *testing.T) {
	t.Setenv("GOOGLE_ANTIGRAVITY_OAUTH_TOKEN", "ga-token")
	token, err := ResolveToken(ResolveOptions{
		Provider: "google-antigravity",
		AuthMode: "oauth",
	})
	if err != nil {
		t.Fatalf("resolve token: %v", err)
	}
	if token != "ga-token" {
		t.Fatalf("expected antigravity token, got %q", token)
	}
}

func TestResolveToken_OAuthGeminiProviderUsesAntigravityToken(t *testing.T) {
	t.Setenv("GOOGLE_ANTIGRAVITY_OAUTH_TOKEN", "ga-token")

	token, err := ResolveToken(ResolveOptions{
		Provider: "gemini",
		AuthMode: "oauth",
	})
	if err != nil {
		t.Fatalf("resolve token: %v", err)
	}
	if token != "ga-token" {
		t.Fatalf("expected antigravity token for gemini oauth, got %q", token)
	}
}

func TestResolveToken_OAuthGeminiNativeProviderUsesAntigravityToken(t *testing.T) {
	t.Setenv("GOOGLE_ANTIGRAVITY_OAUTH_TOKEN", "ga-token")

	token, err := ResolveToken(ResolveOptions{
		Provider: "gemini-native",
		AuthMode: "oauth",
	})
	if err != nil {
		t.Fatalf("resolve token: %v", err)
	}
	if token != "ga-token" {
		t.Fatalf("expected antigravity token for gemini-native oauth, got %q", token)
	}
}
