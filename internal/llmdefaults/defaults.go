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

// ThinkingMode says which extended-thinking request shape a model accepts.
// The two shapes are mutually exclusive: sending the wrong one is a 400, not
// a degraded response.
type ThinkingMode int

const (
	// ThinkingModeBudget takes thinking: {"type":"enabled","budget_tokens":N},
	// and rejects output_config.effort. It is the zero value so that an
	// unrecognized model — every gateway-hosted one — keeps the shape this
	// client has always sent.
	ThinkingModeBudget ThinkingMode = iota
	// ThinkingModeAdaptive takes thinking: {"type":"adaptive"} with depth set
	// by output_config.effort, and rejects budget_tokens outright.
	ThinkingModeAdaptive
)

// ModelBehavior collects the per-model facts a request builder needs. One
// table rather than several keeps them from drifting apart as families
// are added.
type ModelBehavior struct {
	// ContextWindow is the documented input+output context window.
	// 0 means unknown.
	ContextWindow int

	// MaxOutputTokens is the documented output ceiling. 0 means unknown.
	MaxOutputTokens int

	// Thinking is the extended-thinking request shape this family accepts.
	Thinking ThinkingMode

	// CanDisableThinking reports whether thinking: {"type":"disabled"} is
	// accepted. Only meaningful for ThinkingModeAdaptive — a budget-mode
	// model turns thinking off by omitting the block entirely. Fable and
	// Mythos think unconditionally and reject an explicit disable.
	CanDisableThinking bool
}

// modelBehaviors maps a model-family key to its behavior. Lookup is
// longest-prefix, so a dated snapshot ("claude-haiku-4-5-20251001") inherits
// its family's row without needing one of its own.
//
// Only families whose behavior is published are listed. Older models
// (opus-4-5, opus-4-1, sonnet-4-5, sonnet-4) are deliberately absent: their
// output ceilings are lower than the current families' 128K, and defaulting
// one of them high would ship requests the provider rejects. Missing is the
// safe state — the zero ModelBehavior means "unknown ceiling, budget-mode
// thinking", which is exactly what this client did before the table existed.
//
// Prefix matching is only safe because keys that differ in behavior do not
// prefix each other: "claude-sonnet-4-6" does not match "claude-sonnet-4-5",
// so Sonnet 4.5 correctly misses rather than inheriting 128K.
//
// On the Adaptive/Budget split: the Adaptive set is exactly the families
// where budget_tokens is a hard 400. Opus 4.6 and Sonnet 4.6 accept adaptive
// thinking too and it is the recommended shape there, but budget_tokens is
// merely deprecated on them and still works — so they stay on Budget. That
// keeps this table's Adaptive rows equivalent to "configs that are already
// broken today", which is what lets callers turn a thinking_budget on an
// adaptive model into a loud error without breaking a working deployment.
var modelBehaviors = map[string]ModelBehavior{
	"claude-fable-5":    {ContextWindow: 1000000, MaxOutputTokens: 128000, Thinking: ThinkingModeAdaptive, CanDisableThinking: false},
	"claude-mythos-5":   {ContextWindow: 1000000, MaxOutputTokens: 128000, Thinking: ThinkingModeAdaptive, CanDisableThinking: false},
	"claude-opus-5":     {ContextWindow: 1000000, MaxOutputTokens: 128000, Thinking: ThinkingModeAdaptive, CanDisableThinking: true},
	"claude-opus-4-8":   {ContextWindow: 1000000, MaxOutputTokens: 128000, Thinking: ThinkingModeAdaptive, CanDisableThinking: true},
	"claude-opus-4-7":   {ContextWindow: 1000000, MaxOutputTokens: 128000, Thinking: ThinkingModeAdaptive, CanDisableThinking: true},
	"claude-sonnet-5":   {ContextWindow: 1000000, MaxOutputTokens: 128000, Thinking: ThinkingModeAdaptive, CanDisableThinking: true},
	"claude-opus-4-6":   {ContextWindow: 1000000, MaxOutputTokens: 128000, Thinking: ThinkingModeBudget},
	"claude-sonnet-4-6": {ContextWindow: 1000000, MaxOutputTokens: 128000, Thinking: ThinkingModeBudget},
	"claude-haiku-4-5":  {ContextWindow: 200000, MaxOutputTokens: 64000, Thinking: ThinkingModeBudget},
}

// ContextWindow returns the documented context window for model, or 0 when
// the model is not recognized. Zero means "unknown", not "unlimited".
func ContextWindow(model string) int {
	behavior, _ := ModelBehaviorFor(model)
	return behavior.ContextWindow
}

// ModelBehaviorFor returns the documented behavior for model. The second
// result reports whether the model was recognized; the returned value is
// usable either way, because the zero ModelBehavior is the conservative
// choice for an unknown model.
func ModelBehaviorFor(model string) (ModelBehavior, bool) {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return ModelBehavior{}, false
	}
	var best ModelBehavior
	bestLen := 0
	found := false
	for prefix, behavior := range modelBehaviors {
		if len(prefix) <= bestLen || !strings.HasPrefix(normalized, prefix) {
			continue
		}
		best, bestLen, found = behavior, len(prefix), true
	}
	return best, found
}

// MaxOutputTokens returns the documented output-token ceiling for model, or
// 0 when the model is not recognized.
//
// Zero means "unknown", not "unlimited" — callers apply their own fallback.
// Gateway-hosted models reached over an Anthropic-compatible endpoint (the
// shipped config routes MiniMax through kind: anthropic) never match, so
// those tiers should set max_tokens explicitly.
func MaxOutputTokens(model string) int {
	behavior, _ := ModelBehaviorFor(model)
	return behavior.MaxOutputTokens
}
