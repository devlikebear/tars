package usage

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
)

func newCallCostTracker(t *testing.T, now time.Time, limits Limits) *Tracker {
	t.Helper()
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{
		Now:           func() time.Time { return now },
		InitialLimits: limits,
	})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	return tracker
}

func chatWithUsage(t *testing.T, client *TrackedClient, u llm.Usage) {
	t.Helper()
	inner := client.inner.(*llm.FakeClient)
	inner.ChatResponse = llm.ChatResponse{
		Message: llm.ChatMessage{Role: "assistant", Content: "ok"},
		Usage:   u,
	}
	ctx := WithCallMeta(context.Background(), CallMeta{Source: "chat", SessionID: "sess-1"})
	if _, err := client.Chat(ctx, nil, llm.ChatOptions{}); err != nil {
		t.Fatalf("chat: %v", err)
	}
}

func onlyEntry(t *testing.T, tracker *Tracker, now time.Time) Entry {
	t.Helper()
	entries := tracker.readEntriesInRange(now.Add(-time.Hour), now.Add(time.Hour))
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d: %+v", len(entries), entries)
	}
	return entries[0]
}

// claude-code-cli tiers name models by alias (haiku, sonnet) that the price
// table does not know, so the CLI's own total_cost_usd is the only cost signal.
func TestTrackedClient_RecordsProviderReportedCost(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	tracker := newCallCostTracker(t, now, Limits{})
	client := NewTrackedClient(&llm.FakeClient{}, tracker, "claude-code-cli", "haiku", llm.TierLight)

	chatWithUsage(t, client, llm.Usage{InputTokens: 1200, OutputTokens: 80, CostUSD: 0.0123})

	entry := onlyEntry(t, tracker, now)
	if entry.EstimatedCostUSD != 0.0123 {
		t.Fatalf("expected provider cost 0.0123, got %v", entry.EstimatedCostUSD)
	}
	if !entry.PricingKnown {
		t.Fatal("expected pricing_known=true for a provider-reported cost")
	}
	if entry.CostSource != CostSourceProvider {
		t.Fatalf("expected cost_source %q, got %q", CostSourceProvider, entry.CostSource)
	}

	summary, err := tracker.SummaryFiltered("today", "provider", SummaryFilter{SessionID: "sess-1"})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.TotalCostUSD != 0.0123 {
		t.Fatalf("expected session summary total 0.0123, got %v", summary.TotalCostUSD)
	}
}

// A provider figure is authoritative, so it wins even where the table could
// have estimated the call.
func TestTrackedClient_ProviderCostOverridesTableEstimate(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	tracker := newCallCostTracker(t, now, Limits{})
	client := NewTrackedClient(&llm.FakeClient{}, tracker, "anthropic", "claude-haiku-4-5", llm.TierLight)

	chatWithUsage(t, client, llm.Usage{InputTokens: 1_000_000, CostUSD: 0.5})

	entry := onlyEntry(t, tracker, now)
	if entry.EstimatedCostUSD != 0.5 || entry.CostSource != CostSourceProvider {
		t.Fatalf("expected provider cost 0.5, got %v (%q)", entry.EstimatedCostUSD, entry.CostSource)
	}
}

func TestTrackedClient_FallsBackToTableEstimate(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	tracker := newCallCostTracker(t, now, Limits{})
	client := NewTrackedClient(&llm.FakeClient{}, tracker, "anthropic", "claude-haiku-4-5", llm.TierLight)

	chatWithUsage(t, client, llm.Usage{InputTokens: 1_000_000})

	entry := onlyEntry(t, tracker, now)
	if entry.EstimatedCostUSD != 1.0 {
		t.Fatalf("expected table estimate 1.0, got %v", entry.EstimatedCostUSD)
	}
	if !entry.PricingKnown || entry.CostSource != CostSourceEstimate {
		t.Fatalf("expected known estimate, got pricing_known=%v cost_source=%q", entry.PricingKnown, entry.CostSource)
	}
}

func TestTrackedClient_UnpricedWithoutProviderCost(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	tracker := newCallCostTracker(t, now, Limits{})
	client := NewTrackedClient(&llm.FakeClient{}, tracker, "antigravity-cli", "gemini-3-pro", llm.TierStandard)

	chatWithUsage(t, client, llm.Usage{InputTokens: 500, OutputTokens: 20})

	entry := onlyEntry(t, tracker, now)
	if entry.EstimatedCostUSD != 0 || entry.PricingKnown || entry.CostSource != "" {
		t.Fatalf("expected unpriced entry, got cost=%v pricing_known=%v cost_source=%q",
			entry.EstimatedCostUSD, entry.PricingKnown, entry.CostSource)
	}
}

func TestTrackedClient_ProviderCostCountsTowardLimits(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	tracker := newCallCostTracker(t, now, Limits{DailyUSD: 0.05, WeeklyUSD: 1, MonthlyUSD: 5, Mode: "hard"})
	client := NewTrackedClient(&llm.FakeClient{}, tracker, "claude-code-cli", "sonnet", llm.TierStandard)

	chatWithUsage(t, client, llm.Usage{InputTokens: 10, OutputTokens: 10, CostUSD: 0.06})

	status, err := tracker.CheckLimitStatus()
	if err != nil {
		t.Fatalf("check limit: %v", err)
	}
	if !status.Exceeded || status.Period != "today" || status.SpentUSD != 0.06 {
		t.Fatalf("expected daily limit exceeded at 0.06, got %+v", status)
	}

	_, err = client.Chat(context.Background(), nil, llm.ChatOptions{})
	if err == nil || !strings.Contains(err.Error(), "usage limit exceeded") {
		t.Fatalf("expected hard limit to block the next call, got %v", err)
	}
}
