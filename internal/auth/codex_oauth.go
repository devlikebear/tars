package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	openAICodexOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	openAICodexTokenURL      = "https://auth.openai.com/oauth/token"
	codexJWTClaimPath        = "https://api.openai.com/auth"
)

type CodexCredentialSource string

const (
	CodexCredentialSourceEnv  CodexCredentialSource = "env"
	CodexCredentialSourceFile CodexCredentialSource = "file"
)

type CodexCredential struct {
	AccessToken  string
	RefreshToken string
	AccountID    string
	Source       CodexCredentialSource
	SourcePath   string
}

type CodexResolveOptions struct {
	CodexHome string
}

type CodexRefreshOptions struct {
	TokenURL    string
	HTTPClient  *http.Client
	PersistFile bool
}

func ResolveCodexCredential(opts CodexResolveOptions) (CodexCredential, error) {
	if cred, ok := resolveCodexCredentialFromEnv(); ok {
		return cred, nil
	}

	path, err := resolveCodexAuthPath(opts.CodexHome)
	if err != nil {
		return CodexCredential{}, err
	}
	cred, err := resolveCodexCredentialFromFile(path)
	if err != nil {
		return CodexCredential{}, err
	}
	return cred, nil
}

func RefreshCodexCredential(ctx context.Context, cred CodexCredential, opts CodexRefreshOptions) (CodexCredential, error) {
	if strings.TrimSpace(cred.RefreshToken) == "" {
		return CodexCredential{}, fmt.Errorf("openai-codex refresh token is required")
	}

	tokenURL := strings.TrimSpace(opts.TokenURL)
	if tokenURL == "" {
		tokenURL = openAICodexTokenURL
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", cred.RefreshToken)
	form.Set("client_id", openAICodexOAuthClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return CodexCredential{}, fmt.Errorf("openai-codex create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return CodexCredential{}, fmt.Errorf("openai-codex refresh request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return CodexCredential{}, fmt.Errorf("openai-codex read refresh response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CodexCredential{}, fmt.Errorf("openai-codex refresh status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return CodexCredential{}, fmt.Errorf("openai-codex parse refresh response: %w", err)
	}
	if strings.TrimSpace(parsed.AccessToken) == "" || strings.TrimSpace(parsed.RefreshToken) == "" || parsed.ExpiresIn <= 0 {
		return CodexCredential{}, fmt.Errorf("openai-codex refresh response missing required fields: access_token, refresh_token, expires_in")
	}

	next := cred
	next.AccessToken = strings.TrimSpace(parsed.AccessToken)
	next.RefreshToken = strings.TrimSpace(parsed.RefreshToken)
	if next.AccountID == "" {
		next.AccountID = ParseCodexAccountIDFromJWT(next.AccessToken)
	}

	if opts.PersistFile && next.Source == CodexCredentialSourceFile && strings.TrimSpace(next.SourcePath) != "" {
		if err := persistCodexCredentialFile(next.SourcePath, next); err != nil {
			return CodexCredential{}, err
		}
	}
	return next, nil
}

func resolveCodexCredentialFromEnv() (CodexCredential, bool) {
	access := strings.TrimSpace(firstNonEmpty(
		os.Getenv("OPENAI_CODEX_OAUTH_TOKEN"),
		os.Getenv("TARS_OPENAI_CODEX_OAUTH_TOKEN"),
		os.Getenv("OPENAI_CODEX_ACCESS_TOKEN"),
		os.Getenv("TARS_OPENAI_CODEX_ACCESS_TOKEN"),
	))
	if access == "" {
		return CodexCredential{}, false
	}

	refresh := strings.TrimSpace(firstNonEmpty(
		os.Getenv("OPENAI_CODEX_REFRESH_TOKEN"),
		os.Getenv("TARS_OPENAI_CODEX_REFRESH_TOKEN"),
	))
	accountID := strings.TrimSpace(firstNonEmpty(
		os.Getenv("OPENAI_CODEX_ACCOUNT_ID"),
		os.Getenv("TARS_OPENAI_CODEX_ACCOUNT_ID"),
	))
	if accountID == "" {
		accountID = ParseCodexAccountIDFromJWT(access)
	}

	return CodexCredential{
		AccessToken:  access,
		RefreshToken: refresh,
		AccountID:    accountID,
		Source:       CodexCredentialSourceEnv,
	}, true
}

func resolveCodexCredentialFromFile(path string) (CodexCredential, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CodexCredential{}, fmt.Errorf("openai-codex auth file not found: %w", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return CodexCredential{}, fmt.Errorf("openai-codex parse auth file %q: %w", path, err)
	}

	tokens, _ := parsed["tokens"].(map[string]any)
	if tokens == nil {
		return CodexCredential{}, fmt.Errorf("openai-codex auth file %q missing tokens", path)
	}

	access := strings.TrimSpace(asString(tokens["access_token"]))
	if access == "" {
		return CodexCredential{}, fmt.Errorf("openai-codex auth file %q missing tokens.access_token", path)
	}
	refresh := strings.TrimSpace(asString(tokens["refresh_token"]))
	accountID := strings.TrimSpace(asString(tokens["account_id"]))
	if accountID == "" {
		accountID = ParseCodexAccountIDFromJWT(access)
	}

	return CodexCredential{
		AccessToken:  access,
		RefreshToken: refresh,
		AccountID:    accountID,
		Source:       CodexCredentialSourceFile,
		SourcePath:   path,
	}, nil
}

func persistCodexCredentialFile(path string, cred CodexCredential) error {
	var parsed map[string]any
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &parsed)
	}
	if parsed == nil {
		parsed = map[string]any{}
	}

	tokens, _ := parsed["tokens"].(map[string]any)
	if tokens == nil {
		tokens = map[string]any{}
	}
	tokens["access_token"] = cred.AccessToken
	tokens["refresh_token"] = cred.RefreshToken
	if strings.TrimSpace(cred.AccountID) != "" {
		tokens["account_id"] = cred.AccountID
	}
	parsed["tokens"] = tokens
	parsed["last_refresh"] = time.Now().UTC().Format(time.RFC3339)

	body, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return fmt.Errorf("openai-codex marshal auth file %q: %w", path, err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("openai-codex create auth dir %q: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".tars-codex-auth-*")
	if err != nil {
		return fmt.Errorf("openai-codex create temp auth file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("openai-codex write temp auth file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("openai-codex close temp auth file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil && !os.IsPermission(err) {
		return fmt.Errorf("openai-codex chmod temp auth file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("openai-codex replace auth file %q: %w", path, err)
	}
	return nil
}

func resolveCodexAuthPath(codexHomeOverride string) (string, error) {
	base := strings.TrimSpace(codexHomeOverride)
	if base == "" {
		base = strings.TrimSpace(os.Getenv("CODEX_HOME"))
	}
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		base = filepath.Join(home, ".codex")
	}
	if strings.HasPrefix(base, "~"+string(os.PathSeparator)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		base = filepath.Join(home, strings.TrimPrefix(base, "~"+string(os.PathSeparator)))
	}
	return filepath.Join(base, "auth.json"), nil
}

func ParseCodexAccountIDFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return ""
	}
	claim, _ := parsed[codexJWTClaimPath].(map[string]any)
	if claim == nil {
		return ""
	}
	return strings.TrimSpace(asString(claim["chatgpt_account_id"]))
}

func asString(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
