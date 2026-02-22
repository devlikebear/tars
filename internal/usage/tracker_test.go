package usage

import (
	"testing"
	"time"
)

func TestTracker_RecordAndSummary(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{
		Now: func() time.Time { return now },
		InitialLimits: Limits{
			DailyUSD:   10,
			WeeklyUSD:  50,
			MonthlyUSD: 150,
			Mode:       "soft",
		},
	})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}

	if err := tracker.Record(Entry{
		Timestamp:        now,
		Provider:         "openai",
		Model:            "gpt-4o-mini",
		InputTokens:      1000,
		OutputTokens:     500,
		EstimatedCostUSD: 0.001,
		Source:           "chat",
		ProjectID:        "proj_demo",
		PricingKnown:     true,
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := tracker.Record(Entry{
		Timestamp:        now.AddDate(0, 0, -3),
		Provider:         "anthropic",
		Model:            "claude",
		InputTokens:      400,
		OutputTokens:     200,
		EstimatedCostUSD: 0.002,
		Source:           "cron",
		PricingKnown:     true,
	}); err != nil {
		t.Fatalf("record: %v", err)
	}

	today, err := tracker.Summary("today", "provider")
	if err != nil {
		t.Fatalf("summary today: %v", err)
	}
	if today.TotalCalls != 1 {
		t.Fatalf("expected today calls 1, got %d", today.TotalCalls)
	}
	if len(today.Rows) != 1 || today.Rows[0].Key != "openai" {
		t.Fatalf("unexpected today rows: %+v", today.Rows)
	}

	weekBySource, err := tracker.Summary("week", "source")
	if err != nil {
		t.Fatalf("summary week: %v", err)
	}
	if weekBySource.TotalCalls != 2 {
		t.Fatalf("expected week calls 2, got %d", weekBySource.TotalCalls)
	}
}

func TestTracker_CheckLimitStatus(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{
		Now: func() time.Time { return now },
		InitialLimits: Limits{
			DailyUSD:   0.001,
			WeeklyUSD:  1,
			MonthlyUSD: 1,
			Mode:       "hard",
		},
	})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	if err := tracker.Record(Entry{
		Timestamp:        now,
		Provider:         "openai",
		Model:            "gpt-4o-mini",
		EstimatedCostUSD: 0.002,
		Source:           "chat",
		PricingKnown:     true,
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	status, err := tracker.CheckLimitStatus()
	if err != nil {
		t.Fatalf("check limit status: %v", err)
	}
	if !status.Exceeded || status.Mode != "hard" || status.Period != "today" {
		t.Fatalf("unexpected limit status: %+v", status)
	}
}
