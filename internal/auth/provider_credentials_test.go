package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveProviderCredential_APIKeyStrategy(t *testing.T) {
	cred, err := ResolveProviderCredential(ProviderAuthConfig{
		Provider: "openai",
		AuthMode: "api-key",
		APIKey:   "api-key-123",
	})
	if err != nil {
		t.Fatalf("resolve provider credential: %v", err)
	}
	if cred.AccessToken != "api-key-123" {
		t.Fatalf("expected api key token, got %+v", cred)
	}
	if cred.RefreshToken != "" || cred.SourcePath != "" {
		t.Fatalf("expected api key credential without refresh metadata, got %+v", cred)
	}
}

func TestResolveProviderCredential_OpenAICodexOAuthStrategy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"tokens":{"access_token":"file-access","refresh_token":"file-refresh","account_id":"acc-file"}}`), 0o644); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	cred, err := ResolveProviderCredential(ProviderAuthConfig{
		Provider:  "openai-codex",
		AuthMode:  "oauth",
		CodexHome: filepath.Dir(path),
	})
	if err != nil {
		t.Fatalf("resolve provider credential: %v", err)
	}
	if cred.AccessToken != "file-access" || cred.RefreshToken != "file-refresh" {
		t.Fatalf("unexpected codex credential: %+v", cred)
	}
	if cred.Source != CredentialSourceFile || cred.SourcePath != path {
		t.Fatalf("expected file-backed credential, got %+v", cred)
	}
}

func TestRefreshProviderCredential_OpenAICodexStrategy(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"tokens":{"access_token":"old-access","refresh_token":"old-refresh","account_id":"acc-old"}}`), 0o644); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`))
	}))
	defer srv.Close()

	refreshed, err := RefreshProviderCredential(context.Background(), ProviderAuthConfig{
		Provider:  "openai-codex",
		AuthMode:  "oauth",
		CodexHome: filepath.Dir(path),
	}, ProviderCredential{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		AccountID:    "acc-old",
		Source:       CredentialSourceFile,
		SourcePath:   path,
	}, ProviderRefreshOptions{
		TokenURL:      srv.URL,
		HTTPClient:    srv.Client(),
		PersistSource: true,
	})
	if err != nil {
		t.Fatalf("refresh provider credential: %v", err)
	}
	if refreshed.AccessToken != "new-access" || refreshed.RefreshToken != "new-refresh" {
		t.Fatalf("unexpected refreshed credential: %+v", refreshed)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read auth file: %v", err)
	}
	var parsed struct {
		Tokens struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			AccountID    string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse auth file: %v", err)
	}
	if parsed.Tokens.AccessToken != "new-access" || parsed.Tokens.RefreshToken != "new-refresh" {
		t.Fatalf("unexpected persisted auth file: %+v", parsed)
	}
}

func TestRefreshProviderCredential_UnsupportedProvider(t *testing.T) {
	_, err := RefreshProviderCredential(context.Background(), ProviderAuthConfig{
		Provider: "openai",
		AuthMode: "api-key",
	}, ProviderCredential{
		AccessToken: "api-key-123",
	}, ProviderRefreshOptions{})
	if err == nil {
		t.Fatal("expected unsupported refresh error")
	}
	if !strings.Contains(err.Error(), "does not support credential refresh") {
		t.Fatalf("unexpected error: %v", err)
	}
}
