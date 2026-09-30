package usage

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
)

type failingClient struct{ err error }

func (c failingClient) Ask(context.Context, string) (string, error) { return "", c.err }

func (c failingClient) Chat(context.Context, []llm.ChatMessage, llm.ChatOptions) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, c.err
}

func chatFailing(t *testing.T, tracker *Tracker, err error) error {
	t.Helper()
	return chatFailingAs(t, tracker, "claude-code-cli", err)
}

func chatFailingAs(t *testing.T, tracker *Tracker, provider string, err error) error {
	t.Helper()
	client := NewTrackedClient(failingClient{err: err}, tracker, provider, "sonnet", llm.TierStandard)
	ctx := WithCallMeta(context.Background(), CallMeta{Source: "chat", SessionID: "sess-1"})
	_, got := client.Chat(ctx, nil, llm.ChatOptions{})
	return got
}

// A call that failed after spending tokens is recorded, and the caller still
// gets its error. Without a reported cost the spend is priced per upstream
// model: claude-code-cli runs Anthropic models, so their ids price at the
// Anthropic rates even though the tier names the model by alias.
func TestTrackedClient_RecordsFailedCallSpend(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	tracker := newCallCostTracker(t, now, Limits{})
	sonnet := llm.Usage{InputTokens: 1000, OutputTokens: 1000, CacheReadTokens: 1_000_000, CacheWriteTokens: 100_000}
	haiku := llm.Usage{InputTokens: 1_000_000}
	cause := errors.New("cli timed out")
	err := chatFailing(t, tracker, &llm.PartialUsageError{
		Usage:   llm.Usage{InputTokens: 1_001_000, OutputTokens: 1000, CacheReadTokens: 1_000_000, CacheWriteTokens: 100_000},
		ByModel: map[string]llm.Usage{"claude-sonnet-4-6": sonnet, "claude-haiku-4-5-20251001": haiku},
		Err:     cause,
	})
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want the call's error", err)
	}
	entry := onlyEntry(t, tracker, now)
	if entry.SessionID != "sess-1" || entry.Model != "sonnet" || entry.CacheReadTokens != 1_000_000 || entry.InputTokens != 1_001_000 {
		t.Fatalf("entry = %+v", entry)
	}
	// sonnet: 1000*3 + 1000*15 + 1M*0.30 + 100k*3.75 per 1M; haiku: 1M*1.00.
	want := (1000*3.0+1000*15.0)/1e6 + 0.30 + 0.375 + 1.00
	if math.Abs(entry.EstimatedCostUSD-want) > 1e-9 || !entry.PricingKnown || entry.CostSource != CostSourceEstimate {
		t.Fatalf("cost = %v known=%v source=%q, want %v estimate", entry.EstimatedCostUSD, entry.PricingKnown, entry.CostSource, want)
	}
}

// A cost the provider reported (an error result) wins over the estimate.
func TestTrackedClient_FailedCallProviderCostWins(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	tracker := newCallCostTracker(t, now, Limits{})
	_ = chatFailing(t, tracker, &llm.PartialUsageError{Usage: llm.Usage{InputTokens: 10, CostUSD: 0.42}, Err: errors.New("x")})
	if entry := onlyEntry(t, tracker, now); entry.EstimatedCostUSD != 0.42 || entry.CostSource != CostSourceProvider {
		t.Fatalf("entry = %+v", entry)
	}
}

// An unlisted Claude id from the CLI prices like any unlisted Anthropic
// model: at the anthropic/* fallback rates.
func TestTrackedClient_FailedCallUnlistedClaudeModelUsesAnthropicFallback(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	tracker := newCallCostTracker(t, now, Limits{})
	_ = chatFailing(t, tracker, &llm.PartialUsageError{
		Usage:   llm.Usage{InputTokens: 1_000_000},
		ByModel: map[string]llm.Usage{"claude-next-9": {InputTokens: 1_000_000}},
		Err:     errors.New("x"),
	})
	if entry := onlyEntry(t, tracker, now); math.Abs(entry.EstimatedCostUSD-3.0) > 1e-9 || !entry.PricingKnown {
		t.Fatalf("entry = %+v", entry)
	}
}

// A model the table cannot price leaves the tokens recorded with no cost.
func TestTrackedClient_FailedCallUnpricedModelKeepsTokens(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	tracker := newCallCostTracker(t, now, Limits{})
	_ = chatFailingAs(t, tracker, "antigravity-cli", &llm.PartialUsageError{
		Usage:   llm.Usage{InputTokens: 10},
		ByModel: map[string]llm.Usage{"mystery-1": {InputTokens: 10}},
		Err:     errors.New("x"),
	})
	entry := onlyEntry(t, tracker, now)
	if entry.InputTokens != 10 || entry.EstimatedCostUSD != 0 || entry.PricingKnown {
		t.Fatalf("entry = %+v", entry)
	}
}

// A failure that spent nothing records nothing, as before.
func TestTrackedClient_FailedCallWithoutSpendRecordsNothing(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	tracker := newCallCostTracker(t, now, Limits{})
	_ = chatFailing(t, tracker, errors.New("boom"))
	if entries := tracker.readEntriesInRange(now.Add(-time.Hour), now.Add(time.Hour)); len(entries) != 0 {
		t.Fatalf("entries = %+v", entries)
	}
}
