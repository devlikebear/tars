package usage

import (
	"testing"
	"time"
)

func TestTracker_SummaryCountsUnpricedCalls(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	for _, entry := range []Entry{
		{Timestamp: now, Provider: "openai-codex", Model: "gpt-6-astra", InputTokens: 500, OutputTokens: 50, SessionID: "s1"},
		{Timestamp: now, Provider: "openai-codex", Model: "gpt-6-astra", InputTokens: 300, OutputTokens: 30, SessionID: "s1"},
		{Timestamp: now, Provider: "anthropic", Model: "claude-sonnet-4-6", InputTokens: 100, OutputTokens: 10, EstimatedCostUSD: 0.01, PricingKnown: true, SessionID: "s1"},
		// No tokens spent: nothing is missing from the cost.
		{Timestamp: now, Provider: "openai-codex", Model: "gpt-6-astra", SessionID: "s1"},
		{Timestamp: now, Provider: "openai-codex", Model: "gpt-6-astra", InputTokens: 7, OutputTokens: 7, SessionID: "s2"},
	} {
		if err := tracker.Record(entry); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	got, err := tracker.SummaryFiltered("month", "provider", SummaryFilter{SessionID: "s1"})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if got.TotalCalls != 4 || got.TotalUnpricedCalls != 2 {
		t.Fatalf("totals: calls=%d unpriced=%d, want 4 and 2", got.TotalCalls, got.TotalUnpricedCalls)
	}
	byKey := map[string]SummaryRow{}
	for _, row := range got.Rows {
		byKey[row.Key] = row
	}
	if byKey["openai-codex"].UnpricedCalls != 2 || byKey["anthropic"].UnpricedCalls != 0 {
		t.Fatalf("rows: %+v", got.Rows)
	}
}
