package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestTracker_TodayTokensSummarizesUTCUsageAndBudgetLevel(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{
		Now: func() time.Time { return now },
		InitialLimits: Limits{
			DailyTokens: 2000,
		},
	})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	for _, entry := range []Entry{
		{Timestamp: now.Add(-time.Hour), InputTokens: 800, OutputTokens: 600, Provider: "openai", Model: "gpt-4o-mini", Source: "chat"},
		{Timestamp: now.AddDate(0, 0, -1), InputTokens: 400, OutputTokens: 300, Provider: "openai", Model: "gpt-4o-mini", Source: "chat"},
	} {
		if err := tracker.Record(entry); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	today, err := tracker.TodayTokens()
	if err != nil {
		t.Fatalf("today tokens: %v", err)
	}
	if today.InputTokens != 800 || today.OutputTokens != 600 || today.TotalTokens != 1400 {
		t.Fatalf("unexpected today token totals: %+v", today)
	}
	if !today.BudgetEnabled || today.BudgetTokens != 2000 || today.UsagePercent != 70 || today.Level != "warning" {
		t.Fatalf("unexpected budget status: %+v", today)
	}
	if today.Date != "2026-02-22" || today.ResetAt != "2026-02-23T00:00:00Z" {
		t.Fatalf("unexpected UTC date boundary: %+v", today)
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
	if recorded.RunID != "run-1" {
		t.Fatalf("expected trimmed run id, got %q", recorded.RunID)
	}

	runSummary, err := tracker.Summary("today", "run")
	if err != nil {
		t.Fatalf("summary run: %v", err)
	}
	if len(runSummary.Rows) != 1 || runSummary.Rows[0].Key != "run-1" {
		t.Fatalf("expected run summary row run-1, got %+v", runSummary.Rows)
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

func TestTracker_UpdateLimitsWritesPrivateFile(t *testing.T) {
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}

	if _, err := tracker.UpdateLimits(Limits{DailyUSD: 3, Mode: "hard"}); err != nil {
		t.Fatalf("update limits: %v", err)
	}

	requireUsageFileMode(t, tracker.limitsPath, 0o600)
}

func TestTracker_UpdateLimitsPreservesExistingFileAndMemoryWhenAtomicTempCannotBeCreated(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{
		InitialLimits: Limits{
			DailyUSD:  1,
			WeeklyUSD: 2,
			Mode:      "soft",
		},
	})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	original := []byte("{\n  \"daily_usd\": 1,\n  \"weekly_usd\": 2,\n  \"monthly_usd\": 0,\n  \"mode\": \"soft\"\n}\n")
	if err := os.WriteFile(tracker.limitsPath, original, 0o644); err != nil {
		t.Fatalf("seed limits: %v", err)
	}
	if err := os.Chmod(filepath.Dir(tracker.limitsPath), 0o500); err != nil {
		t.Fatalf("chmod limits dir: %v", err)
	}
	defer os.Chmod(filepath.Dir(tracker.limitsPath), 0o755)

	_, err = tracker.UpdateLimits(Limits{
		DailyUSD:  9,
		WeeklyUSD: 9,
		Mode:      "hard",
	})
	if err == nil {
		t.Fatalf("expected update limits to fail when temp file cannot be created")
	}
	got, readErr := os.ReadFile(tracker.limitsPath)
	if readErr != nil {
		t.Fatalf("read limits: %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("expected original limits file to be preserved, got %q", got)
	}
	limits := tracker.Limits()
	if limits.DailyUSD != 1 || limits.WeeklyUSD != 2 || limits.Mode != "soft" {
		t.Fatalf("expected in-memory limits to remain unchanged, got %+v", limits)
	}
}

func requireUsageFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("expected %s mode %04o, got %04o", path, want, got)
	}
}

// The tools-absent final call of an agent turn sits in a different cache
// lineage than the tool-bearing loop iterations before it, because tools are
// rendered ahead of messages in the cached prefix. Grouping by request shape
// is what makes that split readable from recorded usage.
func TestTracker_SummaryGroupByShape(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}

	entries := []Entry{
		{Timestamp: now, Provider: "anthropic", Model: "claude-opus-5", ToolCount: 12,
			InputTokens: 100, CacheReadTokens: 900, PricingKnown: true},
		{Timestamp: now, Provider: "anthropic", Model: "claude-opus-5", ToolCount: 12,
			InputTokens: 100, CacheReadTokens: 900, PricingKnown: true},
		{Timestamp: now, Provider: "anthropic", Model: "claude-opus-5", ToolCount: 0,
			InputTokens: 100, CacheWriteTokens: 1200, PricingKnown: true},
	}
	for _, e := range entries {
		if err := tracker.Record(e); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	summary, err := tracker.Summary("today", "shape")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.GroupBy != "shape" {
		t.Fatalf("group_by should survive normalization, got %q", summary.GroupBy)
	}

	rows := map[string]SummaryRow{}
	for _, row := range summary.Rows {
		rows[row.Key] = row
	}
	withTools, ok := rows["with-tools"]
	if !ok {
		t.Fatalf("expected a with-tools row, got %+v", summary.Rows)
	}
	noTools, ok := rows["no-tools"]
	if !ok {
		t.Fatalf("expected a no-tools row, got %+v", summary.Rows)
	}
	if withTools.Calls != 2 || noTools.Calls != 1 {
		t.Fatalf("expected 2 tool-bearing and 1 tool-free call, got %d and %d", withTools.Calls, noTools.Calls)
	}
	// This is the shape of the suspected regression: the tool-free call pays a
	// cache write and reads nothing back.
	if noTools.CacheWriteTokens == 0 || noTools.CacheReadTokens != 0 {
		t.Fatalf("no-tools row must carry the write/no-read split, got %+v", noTools)
	}
	if withTools.CacheReadTokens == 0 {
		t.Fatalf("with-tools row should show cache reads, got %+v", withTools)
	}
}
