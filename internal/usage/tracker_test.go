package usage

import (
	"encoding/json"
	"os"
	"strings"
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

func TestTracker_RecordNormalizesFieldsAndSummaryUsesProjectFallback(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}

	if err := tracker.Record(Entry{
		Provider:         " OpenAI ",
		Model:            " gpt-4o-mini ",
		EstimatedCostUSD: 0.003,
		Source:           " UNKNOWN ",
		SessionID:        "  sess-1  ",
		ProjectID:        "   ",
		RunID:            " run-1 ",
	}); err != nil {
		t.Fatalf("record: %v", err)
	}

	raw, err := os.ReadFile(tracker.usagePathFor(now))
	if err != nil {
		t.Fatalf("read usage file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one usage line, got %d", len(lines))
	}
	var recorded Entry
	if err := json.Unmarshal([]byte(lines[0]), &recorded); err != nil {
		t.Fatalf("unmarshal usage line: %v", err)
	}
	if recorded.Provider != "openai" {
		t.Fatalf("expected normalized provider, got %q", recorded.Provider)
	}
	if recorded.Model != "gpt-4o-mini" {
		t.Fatalf("expected trimmed model, got %q", recorded.Model)
	}
	if recorded.Source != "chat" {
		t.Fatalf("expected unknown source to normalize to chat, got %q", recorded.Source)
	}
	if recorded.SessionID != "sess-1" {
		t.Fatalf("expected trimmed session id, got %q", recorded.SessionID)
	}
	if recorded.ProjectID != "" {
		t.Fatalf("expected blank project id, got %q", recorded.ProjectID)
	}
	if recorded.RunID != "run-1" {
		t.Fatalf("expected trimmed run id, got %q", recorded.RunID)
	}

	summary, err := tracker.Summary("today", "project")
	if err != nil {
		t.Fatalf("summary project: %v", err)
	}
	if len(summary.Rows) != 1 || summary.Rows[0].Key != "(none)" {
		t.Fatalf("expected project fallback row, got %+v", summary.Rows)
	}
}

func TestTracker_CheckLimitStatusPrefersWeekWhenDailyDisabled(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{
		Now: func() time.Time { return now },
		InitialLimits: Limits{
			DailyUSD:   0,
			WeeklyUSD:  0.005,
			MonthlyUSD: 1,
			Mode:       "soft",
		},
	})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}

	if err := tracker.Record(Entry{
		Timestamp:        now.AddDate(0, 0, -2),
		Provider:         "openai",
		Model:            "gpt-4o-mini",
		EstimatedCostUSD: 0.006,
		Source:           "chat",
	}); err != nil {
		t.Fatalf("record: %v", err)
	}

	status, err := tracker.CheckLimitStatus()
	if err != nil {
		t.Fatalf("check limit status: %v", err)
	}
	if !status.Exceeded || status.Period != "week" || status.Mode != "soft" {
		t.Fatalf("unexpected limit status: %+v", status)
	}
	if status.LimitUSD != 0.005 {
		t.Fatalf("expected weekly limit 0.005, got %v", status.LimitUSD)
	}
}
