package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/auth"
	zlog "github.com/rs/zerolog/log"
)

type ChatMessage struct {
	Role       string     `json:"role"` // system, user, assistant, tool
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
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

type ChatOptions struct {
	OnDelta func(text string) // SSE streaming callback (nil = no streaming)
	Tools   []ToolSchema
	// ToolChoice follows OpenAI-compatible values like "auto", "none", "required".
	ToolChoice string
}

type ChatResponse struct {
	Message    ChatMessage
	Usage      Usage
	StopReason string
}

type ClientConfig struct {
	HTTPTimeout time.Duration
	MaxTokens   int
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
	Provider      string
	AuthMode      string
	OAuthProvider string
	BaseURL       string
	Model         string
	APIKey        string
	MaxTokens     int
}

func NewProvider(opts ProviderOptions) (Client, error) {
	provider := strings.ToLower(strings.TrimSpace(opts.Provider))
	if provider == "" {
		provider = "bifrost"
	}
	zlog.Debug().
		Str("provider", provider).
		Str("auth_mode", strings.TrimSpace(strings.ToLower(opts.AuthMode))).
		Str("model", strings.TrimSpace(opts.Model)).
		Str("base_url", strings.TrimSpace(opts.BaseURL)).
		Msg("llm new provider request")

	if provider == "codex-cli" {
		return nil, fmt.Errorf("unsupported llm provider: codex-cli (removed)")
	}
	if provider == "openai-codex" {
		zlog.Debug().Str("provider", provider).Msg("llm provider ready")
		return NewOpenAICodexClient(
			firstNonEmptyTrimmed(opts.BaseURL, "https://chatgpt.com/backend-api"),
			firstNonEmptyTrimmed(opts.Model, "gpt-5.3-codex"),
			firstNonEmptyTrimmed(opts.AuthMode, "oauth"),
			firstNonEmptyTrimmed(opts.OAuthProvider, "openai-codex"),
			strings.TrimSpace(opts.APIKey),
		)
	}

	token, err := auth.ResolveToken(auth.ResolveOptions{
		Provider:      provider,
		AuthMode:      opts.AuthMode,
		OAuthProvider: opts.OAuthProvider,
		APIKey:        opts.APIKey,
	})
	if err != nil {
		return nil, err
	}

	switch provider {
	case "bifrost":
		zlog.Debug().Str("provider", provider).Msg("llm provider ready")
		return NewBifrostClient(opts.BaseURL, token, opts.Model)
	case "openai":
		zlog.Debug().Str("provider", provider).Msg("llm provider ready")
		return NewOpenAIClient(opts.BaseURL, token, opts.Model)
	case "gemini":
		zlog.Debug().Str("provider", provider).Msg("llm provider ready")
		return NewGeminiClient(
			firstNonEmptyTrimmed(opts.BaseURL, "https://generativelanguage.googleapis.com/v1beta/openai"),
			token,
			firstNonEmptyTrimmed(opts.Model, "gemini-2.5-flash"),
		)
	case "gemini-native":
		zlog.Debug().Str("provider", provider).Msg("llm provider ready")
		return NewGeminiNativeClient(
			firstNonEmptyTrimmed(opts.BaseURL, "https://generativelanguage.googleapis.com/v1beta"),
			token,
			firstNonEmptyTrimmed(opts.Model, "gemini-2.5-flash"),
		)
	case "anthropic":
		zlog.Debug().Str("provider", provider).Msg("llm provider ready")
		return NewAnthropicClient(opts.BaseURL, token, opts.Model, opts.MaxTokens)
	default:
		return nil, fmt.Errorf("unsupported llm provider: %s", provider)
	}
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
