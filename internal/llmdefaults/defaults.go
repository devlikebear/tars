package llmdefaults

import (
	"os"
	"strings"
)

const (
	ProviderOpenAI        = "openai"
	ProviderOpenAICodex   = "openai-codex"
	ProviderClaudeCodeCLI = "claude-code-cli"
	ProviderGemini        = "gemini"
	ProviderGeminiNative  = "gemini-native"
	ProviderAnthropic     = "anthropic"

	OpenAIBaseURL       = "https://api.openai.com/v1"
	OpenAIModel         = "gpt-4o-mini"
	OpenAICodexBaseURL  = "https://chatgpt.com/backend-api"
	OpenAICodexModel    = "gpt-5.3-codex"
	ClaudeCodeCLIModel  = "sonnet"
	GeminiBaseURL       = "https://generativelanguage.googleapis.com/v1beta/openai"
	GeminiNativeBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	GeminiModel         = "gemini-2.5-flash"
	AnthropicBaseURL    = "https://api.anthropic.com"
	AnthropicModel      = "claude-haiku-4-5-20251001"

	OpenAICodexOAuthProvider = "openai-codex"
	ClaudeOAuthProvider      = "claude-code"
	GeminiOAuthProvider      = "google-antigravity"
)

type KindDefaults struct {
	AuthMode                 string
	OAuthProvider            string
	BaseURL                  string
	Model                    string
	APIKeyEnv                []string
	AuthModeWhenAPIKeyAbsent string
}

var kindDefaults = map[string]KindDefaults{
	ProviderOpenAI: {
		AuthMode:  "api-key",
		BaseURL:   OpenAIBaseURL,
		Model:     OpenAIModel,
		APIKeyEnv: []string{"OPENAI_API_KEY"},
	},
	ProviderOpenAICodex: {
		AuthMode:                 "oauth",
		OAuthProvider:            OpenAICodexOAuthProvider,
		BaseURL:                  OpenAICodexBaseURL,
		Model:                    OpenAICodexModel,
		APIKeyEnv:                []string{"OPENAI_CODEX_OAUTH_TOKEN", "TARS_OPENAI_CODEX_OAUTH_TOKEN"},
		AuthModeWhenAPIKeyAbsent: "oauth",
	},
	ProviderClaudeCodeCLI: {
		AuthMode:                 "cli",
		Model:                    ClaudeCodeCLIModel,
		AuthModeWhenAPIKeyAbsent: "cli",
	},
	ProviderGemini: {
		AuthMode:      "api-key",
		OAuthProvider: GeminiOAuthProvider,
		BaseURL:       GeminiBaseURL,
		Model:         GeminiModel,
		APIKeyEnv:     []string{"GEMINI_API_KEY"},
	},
	ProviderGeminiNative: {
		AuthMode:      "api-key",
		OAuthProvider: GeminiOAuthProvider,
		BaseURL:       GeminiNativeBaseURL,
		Model:         GeminiModel,
		APIKeyEnv:     []string{"GEMINI_API_KEY"},
	},
	ProviderAnthropic: {
		AuthMode:      "api-key",
		OAuthProvider: ClaudeOAuthProvider,
		BaseURL:       AnthropicBaseURL,
		Model:         AnthropicModel,
		APIKeyEnv:     []string{"ANTHROPIC_API_KEY"},
	},
}

func ForKind(kind string) (KindDefaults, bool) {
	defaults, ok := kindDefaults[NormalizeKind(kind)]
	if !ok {
		return KindDefaults{}, false
	}
	return defaults, true
}

func NormalizeKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

func APIKeyFromEnv(defaults KindDefaults) string {
	for _, key := range defaults.APIKeyEnv {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func OAuthProvider(kind string) string {
	defaults, ok := ForKind(kind)
	if !ok {
		return ""
	}
	return strings.TrimSpace(defaults.OAuthProvider)
}
