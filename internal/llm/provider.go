package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/auth"
	"github.com/devlikebear/tars/internal/llmdefaults"
	zlog "github.com/rs/zerolog/log"
)

// ContentBlock represents a single block in a multimodal message.
// Type is "text", "image", or "document".
type ContentBlock struct {
	Type      string `json:"type"`                 // "text", "image", "document"
	Text      string `json:"text,omitempty"`       // for type=text
	MediaType string `json:"media_type,omitempty"` // e.g. "image/png", "application/pdf"
	Data      string `json:"data,omitempty"`       // base64-encoded binary
}

type ChatMessage struct {
	Role          string         `json:"role"` // system, user, assistant, tool
	Content       string         `json:"content"`
	ContentBlocks []ContentBlock `json:"content_blocks,omitempty"` // multimodal content (takes priority over Content when non-empty)
	ToolCalls     []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID    string         `json:"tool_call_id,omitempty"`
	// ReasoningContent is provider-specific payload metadata for tool-calling
	// requests. Kimi requires it on assistant messages that include tool calls.
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	// ThoughtSignature is provider-specific metadata used by Gemini Native
	// to preserve tool-calling context across turns.
	ThoughtSignature string `json:"thought_signature,omitempty"`
}

type ToolFunctionSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ToolSchema struct {
	Type     string             `json:"type"`
	Function ToolFunctionSchema `json:"function"`
}

type Usage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CachedTokens     int `json:"cached_tokens,omitempty"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

// ToolChoiceMode enumerates the provider-agnostic tool selection modes.
type ToolChoiceMode string

const (
	ToolChoiceModeAuto     ToolChoiceMode = "auto"
	ToolChoiceModeNone     ToolChoiceMode = "none"
	ToolChoiceModeRequired ToolChoiceMode = "required"
	ToolChoiceModeSpecific ToolChoiceMode = "specific"
)

// ToolChoice expresses how the LLM should pick (or be forced to pick) a tool.
// nil means "no preference" — equivalent to ToolChoiceAuto for providers that
// require a value. Mode == ToolChoiceModeSpecific requires Name.
type ToolChoice struct {
	Mode ToolChoiceMode
	Name string
}

// ToolChoiceAuto returns the default "auto" choice.
func ToolChoiceAuto() *ToolChoice { return &ToolChoice{Mode: ToolChoiceModeAuto} }

// ToolChoiceNone forbids tool calls.
func ToolChoiceNone() *ToolChoice { return &ToolChoice{Mode: ToolChoiceModeNone} }

// ToolChoiceRequired forces *some* tool call.
func ToolChoiceRequired() *ToolChoice { return &ToolChoice{Mode: ToolChoiceModeRequired} }

// ToolChoiceSpecific forces calling a specific tool by name.
func ToolChoiceSpecific(name string) *ToolChoice {
	return &ToolChoice{Mode: ToolChoiceModeSpecific, Name: strings.TrimSpace(name)}
}

// String returns a short label for logging.
func (tc *ToolChoice) String() string {
	if tc == nil {
		return ""
	}
	if tc.Mode == ToolChoiceModeSpecific {
		return string(tc.Mode) + ":" + tc.Name
	}
	return string(tc.Mode)
}

// ResponseFormatType enumerates the supported response shapes.
type ResponseFormatType string

const (
	ResponseFormatText       ResponseFormatType = "text"
	ResponseFormatJSONObject ResponseFormatType = "json_object"
	ResponseFormatJSONSchema ResponseFormatType = "json_schema"
)

// ResponseFormat constrains the model output. Schema is required when
// Type == ResponseFormatJSONSchema. Strict enables provider-side strict
// validation (currently OpenAI-style only).
type ResponseFormat struct {
	Type   ResponseFormatType
	Name   string
	Schema json.RawMessage
	Strict bool
}

type ChatOptions struct {
	OnDelta func(text string) // SSE streaming callback (nil = no streaming)
	// OnReasoningDelta receives provider-native chain-of-thought / thinking
	// deltas as they stream. Called only when the provider exposes a
	// distinct reasoning channel (kimi reasoning_content, anthropic
	// thinking_delta, openai responses reasoning summary). nil = ignore.
	// OnDelta governs whether streaming is requested; reasoning deltas only
	// fire when streaming is active.
	OnReasoningDelta func(text string)
	Tools            []ToolSchema
	// ToolChoice picks how the LLM selects tools. nil = provider default (auto).
	ToolChoice *ToolChoice
	// ResponseFormat constrains the response shape. nil = free-form text.
	ResponseFormat *ResponseFormat
	// ReasoningEffort is a provider-agnostic hint. Supported values are
	// none, minimal, low, medium, high.
	ReasoningEffort string
	// ThinkingBudget enables provider-native thinking when budgeted tokens are supported.
	ThinkingBudget int
	// ServiceTier controls provider-side latency tier when supported.
	ServiceTier string
	// ResumeSessionID, when non-empty, asks providers that expose a resumable
	// upstream session (currently only claude-code-cli) to continue that
	// session instead of replaying the full transcript. The caller is
	// expected to read ChatResponse.SessionID on the first turn and feed it
	// back here on subsequent turns. Providers that don't support resume
	// ignore the field silently.
	ResumeSessionID string
	// ClaudeCodeMCPServers, when non-empty, asks the claude-code-cli provider
	// to materialize a Claude Code MCP config file (`{"mcpServers": {...}}`)
	// for the duration of one Chat call and pass it via `--mcp-config`. Other
	// providers ignore this field. Callers translate TARS' own MCPServer
	// configuration into this provider-agnostic shape outside the llm
	// package so internal/llm stays decoupled from internal/config.
	ClaudeCodeMCPServers []ClaudeCodeMCPServer
	// ClaudeCodePermissionMode selects the value passed to `--permission-mode`
	// for the claude-code-cli provider. Recognized values: "auto" (default),
	// "acceptEdits", "plan", "bypassPermissions". Empty or unknown values
	// fall back to "auto" so the provider stays callable even when callers
	// haven't been updated. Other providers ignore this field.
	ClaudeCodePermissionMode string
	// ClaudeCodeSkills, when non-empty, asks the claude-code-cli provider to
	// materialize a session-only Claude Code plugin directory containing
	// these skills and pass it via `--plugin-dir` for the duration of one
	// Chat call. The skills surface as `tars-skills:<name>` in Claude Code's
	// slash-command / skill registry. Other providers ignore this field.
	// Callers translate TARS' own skill catalog into this provider-agnostic
	// shape outside the llm package so internal/llm stays decoupled from
	// internal/skill.
	ClaudeCodeSkills []ClaudeCodeSkill
}

// ClaudeCodeSkill is the minimal shape needed to render one Claude Code
// SKILL.md (frontmatter name + description, then the markdown body) inside a
// session-scoped plugin directory.
type ClaudeCodeSkill struct {
	Name        string
	Description string
	Content     string
}

// ClaudeCodeMCPServer is the minimal subset of an MCP server config that
// Claude Code understands when it receives `--mcp-config`. It covers both
// stdio servers (Command + Args + Env) and remote servers (URL + Headers).
// Transport selects the shape: "stdio" (default when Command is set) or
// "http"/"sse" for remote servers.
type ClaudeCodeMCPServer struct {
	Name      string
	Transport string
	Command   string
	Args      []string
	Env       map[string]string
	URL       string
	Headers   map[string]string
}

type ChatResponse struct {
	Message    ChatMessage
	Usage      Usage
	StopReason string
	// SessionID is set by providers that expose a resumable upstream session
	// (currently only claude-code-cli via stream-json). Empty for stateless
	// providers.
	SessionID string
	// ProviderExecutedTools enumerates tool invocations the upstream provider
	// has *already* executed on its own (currently only claude-code-cli,
	// which runs Read/Edit/Bash/Glob/etc internally and reports them as
	// stream-json tool_use blocks). These are observation-only — callers
	// must NOT route them through TARS' tool registry, since the work is
	// already done and the names won't match. Message.ToolCalls retains the
	// "model wants TARS to execute this" semantic and stays nil for
	// self-executing providers.
	ProviderExecutedTools []ToolCall
}

type ClientConfig struct {
	HTTPTimeout     time.Duration
	MaxTokens       int
	ReasoningEffort string
	ThinkingBudget  int
	ServiceTier     string
}

func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		HTTPTimeout: defaultHTTPTimeout,
		MaxTokens:   0,
	}
}

type Client interface {
	Ask(ctx context.Context, prompt string) (string, error)
	Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error)
}

type ProviderOptions struct {
	Provider        string
	AuthMode        string
	OAuthProvider   string
	AuthConfig      auth.ProviderAuthConfig
	BaseURL         string
	WorkDir         string
	Model           string
	APIKey          string
	MaxTokens       int
	ReasoningEffort string
	ThinkingBudget  int
	ServiceTier     string
}

func NewProvider(opts ProviderOptions) (Client, error) {
	provider := strings.ToLower(strings.TrimSpace(opts.Provider))
	if provider == "" {
		provider = "anthropic"
	}
	authOpts := opts
	authOpts.Provider = provider
	authConfig := providerAuthConfig(authOpts)
	zlog.Debug().
		Str("provider", provider).
		Str("auth_mode", authConfig.AuthMode).
		Str("model", strings.TrimSpace(opts.Model)).
		Str("base_url", strings.TrimSpace(opts.BaseURL)).
		Msg("llm new provider request")

	if provider == "openai-codex" {
		zlog.Debug().Str("provider", provider).Msg("llm provider ready")
		return newOpenAICodexClientWithAuthConfig(
			firstNonEmptyTrimmed(opts.BaseURL, llmdefaults.OpenAICodexBaseURL),
			firstNonEmptyTrimmed(opts.Model, llmdefaults.OpenAICodexModel),
			authConfig,
			DefaultClientConfig(),
			nil,
			nil,
		)
	}
	if provider == "claude-code-cli" {
		zlog.Debug().Str("provider", provider).Msg("llm provider ready")
		return NewClaudeCodeCLIClient(opts.WorkDir, opts.Model)
	}

	cred, err := auth.ResolveProviderCredential(authConfig)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(cred.AccessToken)

	switch provider {
	case "openai":
		zlog.Debug().Str("provider", provider).Msg("llm provider ready")
		return newOpenAICompatibleClientWithConfig("openai", opts.BaseURL, token, opts.Model, providerClientConfig(opts))
	case "kimi":
		zlog.Debug().Str("provider", provider).Msg("llm provider ready")
		return newOpenAICompatibleClientWithConfig(
			"kimi",
			firstNonEmptyTrimmed(opts.BaseURL, llmdefaults.KimiBaseURL),
			token,
			firstNonEmptyTrimmed(opts.Model, llmdefaults.OpenAIModel),
			providerClientConfig(opts),
		)
	case "gemini":
		zlog.Debug().Str("provider", provider).Msg("llm provider ready")
		return newOpenAICompatibleClientWithConfig(
			"gemini",
			firstNonEmptyTrimmed(opts.BaseURL, llmdefaults.GeminiBaseURL),
			token,
			firstNonEmptyTrimmed(opts.Model, llmdefaults.GeminiModel),
			providerClientConfig(opts),
		)
	case "gemini-native":
		zlog.Debug().Str("provider", provider).Msg("llm provider ready")
		return newGeminiNativeClientWithConfig(
			firstNonEmptyTrimmed(opts.BaseURL, llmdefaults.GeminiNativeBaseURL),
			token,
			firstNonEmptyTrimmed(opts.Model, llmdefaults.GeminiModel),
			providerClientConfig(opts),
		)
	case "anthropic":
		zlog.Debug().Str("provider", provider).Msg("llm provider ready")
		config := providerClientConfig(opts)
		if config.MaxTokens <= 0 {
			config.MaxTokens = 4096
		}
		return newAnthropicClientWithConfig(opts.BaseURL, token, opts.Model, config)
	default:
		return nil, fmt.Errorf("unsupported llm provider: %s", provider)
	}
}

func providerClientConfig(opts ProviderOptions) ClientConfig {
	config := DefaultClientConfig()
	if opts.MaxTokens > 0 {
		config.MaxTokens = opts.MaxTokens
	}
	config.ReasoningEffort = normalizeReasoningEffort(opts.ReasoningEffort)
	if opts.ThinkingBudget > 0 {
		config.ThinkingBudget = opts.ThinkingBudget
	}
	config.ServiceTier = normalizeServiceTier(opts.ServiceTier)
	return config
}

func providerAuthConfig(opts ProviderOptions) auth.ProviderAuthConfig {
	config := opts.AuthConfig
	if strings.TrimSpace(config.Provider) == "" {
		config.Provider = normalizeLowerTrimmed(opts.Provider)
	}
	if strings.TrimSpace(config.AuthMode) == "" {
		config.AuthMode = normalizeLowerTrimmed(opts.AuthMode)
	}
	if strings.TrimSpace(config.OAuthProvider) == "" {
		config.OAuthProvider = normalizeLowerTrimmed(opts.OAuthProvider)
	}
	if strings.TrimSpace(config.APIKey) == "" {
		config.APIKey = strings.TrimSpace(opts.APIKey)
	}

	if defaults, ok := llmdefaults.ForKind(config.Provider); ok {
		if strings.TrimSpace(config.AuthMode) == "" {
			config.AuthMode = defaults.AuthMode
		}
		if normalizeLowerTrimmed(config.AuthMode) == "oauth" && strings.TrimSpace(config.OAuthProvider) == "" {
			config.OAuthProvider = defaults.OAuthProvider
		}
	}

	config.Provider = normalizeLowerTrimmed(config.Provider)
	config.AuthMode = normalizeLowerTrimmed(config.AuthMode)
	config.OAuthProvider = normalizeLowerTrimmed(config.OAuthProvider)
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.CodexHome = strings.TrimSpace(config.CodexHome)
	return config
}

func truncateForLog(value string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(value) <= max {
		return value
	}
	return value[:max] + "...(truncated)"
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeLowerTrimmed(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeReasoningEffort(raw string) string {
	switch normalizeLowerTrimmed(raw) {
	case "":
		return ""
	case "none", "off", "disabled":
		return "none"
	case "minimal", "min":
		return "minimal"
	case "low":
		return "low"
	case "medium", "med":
		return "medium"
	case "high":
		return "high"
	case "veryhigh", "very-high", "very_high", "xhigh":
		return "high"
	default:
		return ""
	}
}

func normalizeServiceTier(raw string) string {
	switch normalized := normalizeLowerTrimmed(raw); normalized {
	case "":
		return ""
	case "auto", "default", "flex", "priority":
		return normalized
	default:
		return ""
	}
}

func effectiveReasoningEffort(config ClientConfig, opts ChatOptions) string {
	if value := normalizeReasoningEffort(opts.ReasoningEffort); value != "" {
		return value
	}
	return normalizeReasoningEffort(config.ReasoningEffort)
}

func effectiveThinkingBudget(config ClientConfig, opts ChatOptions) int {
	if opts.ThinkingBudget > 0 {
		return opts.ThinkingBudget
	}
	if config.ThinkingBudget > 0 {
		return config.ThinkingBudget
	}
	return 0
}

func effectiveServiceTier(config ClientConfig, opts ChatOptions) string {
	if value := normalizeServiceTier(opts.ServiceTier); value != "" {
		return value
	}
	return normalizeServiceTier(config.ServiceTier)
}
