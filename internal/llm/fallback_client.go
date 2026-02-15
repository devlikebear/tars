package llm

import (
	"context"
	"errors"

	zlog "github.com/rs/zerolog/log"
)

type fallbackClient struct {
	primary  Client
	fallback Client
}

func newFallbackClient(primary Client, fallback Client) Client {
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	return &fallbackClient{
		primary:  primary,
		fallback: fallback,
	}
}

func (c *fallbackClient) Ask(ctx context.Context, prompt string) (string, error) {
	resp, err := c.primary.Ask(ctx, prompt)
	if err == nil {
		return resp, nil
	}
	if !shouldFallback(err) {
		return "", err
	}
	zlog.Warn().Err(err).Msg("llm primary provider failed; retrying with fallback provider")
	return c.fallback.Ask(ctx, prompt)
}

func (c *fallbackClient) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error) {
	resp, err := c.primary.Chat(ctx, messages, opts)
	if err == nil {
		return resp, nil
	}
	if !shouldFallback(err) {
		return ChatResponse{}, err
	}
	zlog.Warn().Err(err).Msg("llm primary provider failed; retrying with fallback provider")
	return c.fallback.Chat(ctx, messages, opts)
}

func shouldFallback(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var perr *ProviderError
	if errors.As(err, &perr) {
		return true
	}
	return false
}
