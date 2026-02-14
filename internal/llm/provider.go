package llm

import (
	"context"
	"fmt"
	"strings"

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
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type ChatOptions struct {
	OnDelta func(text string) // SSE streaming callback (nil = no streaming)
}

type ChatResponse struct {
	Message    ChatMessage
	Usage      Usage
	StopReason string
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
		model := strings.TrimSpace(opts.Model)
		if model == "" {
			model = defaultCodexCLIModel
		}
		zlog.Debug().Str("provider", provider).Str("model", model).Msg("llm provider ready")
		return NewCodexCLIClient(model)
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
