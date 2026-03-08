package usage

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
)

type WarningNotifier func(ctx context.Context, message string)

type TrackedClient struct {
	inner    llm.Client
	tracker  *Tracker
	provider string
	model    string
	notifier WarningNotifier
}

func NewTrackedClient(inner llm.Client, tracker *Tracker, provider string, model string) *TrackedClient {
	return &TrackedClient{
		inner:    inner,
		tracker:  tracker,
		provider: strings.TrimSpace(strings.ToLower(provider)),
		model:    strings.TrimSpace(model),
	}
}

func (c *TrackedClient) SetNotifier(notifier WarningNotifier) {
	if c == nil {
		return
	}
	c.notifier = notifier
}

func (c *TrackedClient) Ask(ctx context.Context, prompt string) (string, error) {
	if c == nil || c.inner == nil {
		return "", fmt.Errorf("llm client is not configured")
	}
	return c.inner.Ask(ctx, prompt)
}

func (c *TrackedClient) Chat(ctx context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	if c == nil || c.inner == nil {
		return llm.ChatResponse{}, fmt.Errorf("llm client is not configured")
	}
	if c.tracker != nil {
		status, err := c.tracker.CheckLimitStatus()
		if err == nil && status.Exceeded && status.Mode == "hard" {
			return llm.ChatResponse{}, fmt.Errorf("usage limit exceeded (%s: %.6f >= %.6f USD)", status.Period, status.SpentUSD, status.LimitUSD)
		}
	}
	resp, err := c.inner.Chat(ctx, messages, opts)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	if c.tracker == nil {
		return resp, nil
	}

	meta := CallMetaFromContext(ctx)
	estimatedCost, pricingKnown := c.tracker.EstimateCost(c.provider, c.model, resp.Usage)
	_ = c.tracker.Record(Entry{
		Provider:         c.provider,
		Model:            c.model,
		InputTokens:      resp.Usage.InputTokens,
		OutputTokens:     resp.Usage.OutputTokens,
		CachedTokens:     resp.Usage.CachedTokens,
		CacheReadTokens:  resp.Usage.CacheReadTokens,
		CacheWriteTokens: resp.Usage.CacheWriteTokens,
		EstimatedCostUSD: estimatedCost,
		Source:           meta.Source,
		SessionID:        meta.SessionID,
		ProjectID:        meta.ProjectID,
		RunID:            meta.RunID,
		PricingKnown:     pricingKnown,
	})

	status, checkErr := c.tracker.CheckLimitStatus()
	if checkErr == nil && status.Exceeded && status.Mode == "soft" && c.notifier != nil {
		c.notifier(ctx, fmt.Sprintf("usage limit exceeded (%s: %.6f >= %.6f USD)", status.Period, status.SpentUSD, status.LimitUSD))
	}

	return resp, nil
}
