package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/tarsncase/internal/auth"
)

type Client interface {
	Ask(ctx context.Context, prompt string) (string, error)
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

	if provider == "codex-cli" {
		model := strings.TrimSpace(opts.Model)
		if model == "" {
			model = defaultCodexCLIModel
		}
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
		return NewBifrostClient(opts.BaseURL, token, opts.Model)
	case "openai":
		return NewOpenAIClient(opts.BaseURL, token, opts.Model)
	case "anthropic":
		return NewAnthropicClient(opts.BaseURL, token, opts.Model, opts.MaxTokens)
	default:
		return nil, fmt.Errorf("unsupported llm provider: %s", provider)
	}
}
