package llmdefaults

import (
	"os"
	"strings"
)

const (
	ProviderOpenAI         = "openai"
	ProviderOpenAICodex    = "openai-codex"
	ProviderClaudeCodeCLI  = "claude-code-cli"
	ProviderAntigravityCLI = "antigravity-cli"
	ProviderGemini         = "gemini"
	ProviderGeminiNative   = "gemini-native"
	ProviderAnthropic      = "anthropic"
	ProviderKimi           = "kimi"

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
	KimiBaseURL         = "https://api.moonshot.cn/v1"

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
	// antigravity-cli carries no BaseURL, Model or APIKeyEnv: the CLI holds
	// its own Google login and talks to its own backend, so TARS never sees a
	// credential or an endpoint, and the CLI picks its own default model.
	ProviderAntigravityCLI: {
		AuthMode:                 "cli",
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
	ProviderKimi: {
		AuthMode:  "api-key",
		BaseURL:   KimiBaseURL,
		APIKeyEnv: []string{"KIMI_API_KEY"},
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

// modelMaxOutputTokens maps a model-family key to that family's documented
// output-token ceiling. Lookup is longest-prefix, so a dated snapshot
// ("claude-haiku-4-5-20251001") inherits its family's limit without needing
// its own row.
//
// Only families whose ceiling is published are listed. Older models
// (opus-4-5, opus-4-1, sonnet-4-5, sonnet-4) are deliberately absent: their
// ceilings are lower than the current families' 128K, and defaulting one of
// them high would ship requests the provider rejects. They fall through to
// the caller's conservative fallback instead — see MaxOutputTokens.
//
// Prefix matching is only safe because keys with different ceilings do not
// prefix each other: "claude-sonnet-4-6" does not match "claude-sonnet-4-5",
// so Sonnet 4.5 correctly misses rather than inheriting 128K.
var modelMaxOutputTokens = map[string]int{
	"claude-fable-5":    128000,
	"claude-mythos-5":   128000,
	"claude-opus-5":     128000,
	"claude-opus-4-8":   128000,
	"claude-opus-4-7":   128000,
	"claude-opus-4-6":   128000,
	"claude-sonnet-5":   128000,
	"claude-sonnet-4-6": 128000,
	"claude-haiku-4-5":  64000,
}

// MaxOutputTokens returns the documented output-token ceiling for model, or
// 0 when the model is not recognized.
//
// Zero means "unknown", not "unlimited" — callers apply their own fallback.
// Gateway-hosted models reached over an Anthropic-compatible endpoint (the
// shipped config routes MiniMax through kind: anthropic) never match, so
// those tiers should set max_tokens explicitly.
func MaxOutputTokens(model string) int {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return 0
	}
	best, bestLen := 0, 0
	for prefix, limit := range modelMaxOutputTokens {
		if len(prefix) <= bestLen || !strings.HasPrefix(normalized, prefix) {
			continue
		}
		best, bestLen = limit, len(prefix)
	}
	return best
}
