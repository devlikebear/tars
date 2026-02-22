package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveCodexCredential_EnvOnly(t *testing.T) {
	token := makeJWTWithAccountID(t, "acc-env")
	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", token)
	t.Setenv("OPENAI_CODEX_REFRESH_TOKEN", "refresh-env")
	t.Setenv("OPENAI_CODEX_ACCOUNT_ID", "")

	cred, err := ResolveCodexCredential(CodexResolveOptions{})
	if err != nil {
		t.Fatalf("resolve codex credential: %v", err)
	}
	if cred.AccessToken != token {
		t.Fatalf("expected env access token, got %q", cred.AccessToken)
	}
	if cred.RefreshToken != "refresh-env" {
		t.Fatalf("expected env refresh token, got %q", cred.RefreshToken)
	}
	if cred.AccountID != "acc-env" {
		t.Fatalf("expected account id from jwt fallback, got %q", cred.AccountID)
	}
	if cred.Source != CodexCredentialSourceEnv {
		t.Fatalf("expected env source, got %q", cred.Source)
	}
}

func TestResolveCodexCredential_File(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", "")
	t.Setenv("OPENAI_CODEX_REFRESH_TOKEN", "")
	t.Setenv("OPENAI_CODEX_ACCOUNT_ID", "")
	path := filepath.Join(home, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"tokens":{"access_token":"file-access","refresh_token":"file-refresh","account_id":"acc-file"}}`), 0o644); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	cred, err := ResolveCodexCredential(CodexResolveOptions{})
	if err != nil {
		t.Fatalf("resolve codex credential: %v", err)
	}
	if cred.AccessToken != "file-access" {
		t.Fatalf("expected file access token, got %q", cred.AccessToken)
	}
	if cred.RefreshToken != "file-refresh" {
		t.Fatalf("expected file refresh token, got %q", cred.RefreshToken)
	}
	if cred.AccountID != "acc-file" {
		t.Fatalf("expected file account id, got %q", cred.AccountID)
	}
	if cred.Source != CodexCredentialSourceFile {
		t.Fatalf("expected file source, got %q", cred.Source)
	}
	if cred.SourcePath != path {
		t.Fatalf("expected source path %q, got %q", path, cred.SourcePath)
	}
}

func TestResolveCodexCredential_PrefersEnvOverFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"tokens":{"access_token":"file-access","refresh_token":"file-refresh","account_id":"acc-file"}}`), 0o644); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", "env-access")
	t.Setenv("OPENAI_CODEX_REFRESH_TOKEN", "env-refresh")
	t.Setenv("OPENAI_CODEX_ACCOUNT_ID", "acc-env")
	cred, err := ResolveCodexCredential(CodexResolveOptions{})
	if err != nil {
		t.Fatalf("resolve codex credential: %v", err)
	}
	if cred.AccessToken != "env-access" {
		t.Fatalf("expected env access token, got %q", cred.AccessToken)
	}
	if cred.Source != CodexCredentialSourceEnv {
		t.Fatalf("expected env source, got %q", cred.Source)
	}
}

func TestRefreshCodexCredential_RequestBody(t *testing.T) {
	var captured url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Fatalf("expected form content type, got %q", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		captured = r.PostForm
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`))
	}))
	defer srv.Close()

	cred, err := RefreshCodexCredential(context.Background(), CodexCredential{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
	}, CodexRefreshOptions{
		TokenURL:    srv.URL,
		HTTPClient:  srv.Client(),
		PersistFile: false,
	})
	if err != nil {
		t.Fatalf("refresh codex credential: %v", err)
	}
	if captured.Get("grant_type") != "refresh_token" {
		t.Fatalf("expected grant_type refresh_token, got %q", captured.Get("grant_type"))
	}
	if captured.Get("refresh_token") != "old-refresh" {
		t.Fatalf("expected refresh token old-refresh, got %q", captured.Get("refresh_token"))
	}
	if captured.Get("client_id") != openAICodexOAuthClientID {
		t.Fatalf("expected client_id %q, got %q", openAICodexOAuthClientID, captured.Get("client_id"))
	}
	if cred.AccessToken != "new-access" {
		t.Fatalf("expected new access token, got %q", cred.AccessToken)
	}
	if cred.RefreshToken != "new-refresh" {
		t.Fatalf("expected new refresh token, got %q", cred.RefreshToken)
	}
}

func TestRefreshCodexCredential_RequiresFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"new-access","expires_in":3600}`))
	}))
	defer srv.Close()

	_, err := RefreshCodexCredential(context.Background(), CodexCredential{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
	}, CodexRefreshOptions{
		TokenURL:    srv.URL,
		HTTPClient:  srv.Client(),
		PersistFile: false,
	})
	if err == nil {
		t.Fatal("expected missing fields error")
	}
	if !strings.Contains(err.Error(), "missing required fields") {
		t.Fatalf("expected missing required fields error, got %v", err)
	}
}

func TestRefreshCodexCredential_PersistAtomic(t *testing.T) {
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

	_, err := RefreshCodexCredential(context.Background(), CodexCredential{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		AccountID:    "acc-old",
		Source:       CodexCredentialSourceFile,
		SourcePath:   path,
	}, CodexRefreshOptions{
		TokenURL:    srv.URL,
		HTTPClient:  srv.Client(),
		PersistFile: true,
	})
	if err != nil {
		t.Fatalf("refresh codex credential: %v", err)
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
	if parsed.Tokens.AccessToken != "new-access" {
		t.Fatalf("expected persisted access token new-access, got %q", parsed.Tokens.AccessToken)
	}
	if parsed.Tokens.RefreshToken != "new-refresh" {
		t.Fatalf("expected persisted refresh token new-refresh, got %q", parsed.Tokens.RefreshToken)
	}
	if parsed.Tokens.AccountID != "acc-old" {
		t.Fatalf("expected account id preserved, got %q", parsed.Tokens.AccountID)
	}

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".tars-codex-auth-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no leftover temp files, got %v", matches)
	}
}

func makeJWTWithAccountID(t *testing.T, accountID string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payloadJSON := `{"https://api.openai.com/auth":{"chatgpt_account_id":"` + accountID + `"}}`
	payload := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	return header + "." + payload + ".sig"
}
