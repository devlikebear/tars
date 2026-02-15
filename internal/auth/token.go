package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ResolveOptions struct {
	Provider      string
	AuthMode      string
	OAuthProvider string
	APIKey        string
}

func ResolveToken(opts ResolveOptions) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(opts.AuthMode))
	if mode == "" {
		mode = "api-key"
	}

	switch mode {
	case "api-key":
		if strings.TrimSpace(opts.APIKey) == "" {
			return "", fmt.Errorf("api key is required for auth mode api-key")
		}
		return opts.APIKey, nil
	case "oauth":
		return resolveOAuthToken(opts.Provider, opts.OAuthProvider)
	default:
		return "", fmt.Errorf("unsupported auth mode: %s", opts.AuthMode)
	}
}

func resolveOAuthToken(provider, oauthProvider string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(oauthProvider))
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(provider))
	}

	switch key {
	case "anthropic-claude-code", "claude-code":
		return resolveClaudeCodeOAuthToken()
	case "google-antigravity", "antigravity":
		return resolveGoogleAntigravityOAuthToken()
	default:
		return "", fmt.Errorf("unsupported oauth provider: %s", key)
	}
}

func resolveClaudeCodeOAuthToken() (string, error) {
	home, _ := os.UserHomeDir()
	return resolveFromEnvOrFiles(
		[]string{"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_CLAUDE_CODE_OAUTH_TOKEN"},
		[]string{
			filepath.Join(home, ".claude", "oauth.json"),
			filepath.Join(home, ".claude-code", "oauth.json"),
			filepath.Join(home, ".config", "claude-code", "oauth.json"),
		},
		"claude-code",
	)
}

func resolveGoogleAntigravityOAuthToken() (string, error) {
	home, _ := os.UserHomeDir()
	return resolveFromEnvOrFiles(
		[]string{"GOOGLE_ANTIGRAVITY_OAUTH_TOKEN", "ANTIGRAVITY_OAUTH_TOKEN"},
		[]string{
			filepath.Join(home, ".google-antigravity", "oauth.json"),
			filepath.Join(home, ".antigravity", "oauth.json"),
			filepath.Join(home, ".config", "google-antigravity", "oauth.json"),
		},
		"google-antigravity",
	)
}

func resolveFromEnvOrFiles(envKeys []string, files []string, label string) (string, error) {
	for _, key := range envKeys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v, nil
		}
	}

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if token := parseToken(data); token != "" {
			return token, nil
		}
	}

	return "", fmt.Errorf("oauth token not found for %s", label)
}

func parseToken(data []byte) string {
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return ""
	}
	if !strings.HasPrefix(raw, "{") {
		return raw
	}

	var obj any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return ""
	}
	return findToken(obj)
}

func findToken(v any) string {
	switch val := v.(type) {
	case map[string]any:
		priority := []string{"access_token", "oauth_token", "token", "id_token"}
		for _, key := range priority {
			if raw, ok := val[key]; ok {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
		for _, child := range val {
			if found := findToken(child); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range val {
			if found := findToken(child); found != "" {
				return found
			}
		}
	}
	return ""
}
