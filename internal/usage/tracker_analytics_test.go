package usage

import (
	"testing"
	"time"
)

func TestTracker_AnalyticsSummarizesDailyModelsAndTools(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	entries := []Entry{
		{
			Timestamp:        time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC),
			Provider:         "openai",
			Model:            "gpt-5.4",
			InputTokens:      100,
			OutputTokens:     50,
			EstimatedCostUSD: 0.010,
			Source:           "chat",
			SessionID:        "sess-a",
		},
		{
			Timestamp:        time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
			Provider:         "openai",
			Model:            "gpt-5.4",
			InputTokens:      50,
			OutputTokens:     50,
			EstimatedCostUSD: 0.005,
			Source:           "chat",
			SessionID:        "sess-a",
		},
		{
			Timestamp:        time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
			Provider:         "anthropic",
			Model:            "claude-sonnet-4-6",
			InputTokens:      30,
			OutputTokens:     20,
			EstimatedCostUSD: 0.002,
			Source:           "agent_run",
			SessionID:        "sess-b",
		},
		{
			Timestamp:        time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
			Provider:         "openai",
			Model:            "old-model",
			InputTokens:      999,
			OutputTokens:     999,
			EstimatedCostUSD: 9.990,
			Source:           "chat",
			SessionID:        "old",
		},
	}
	for _, entry := range entries {
		if err := tracker.Record(entry); err != nil {
			t.Fatalf("record usage: %v", err)
		}
	}
	if err := tracker.RecordSignal(SignalEntry{
		Timestamp: time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC),
		Name:      "tool_call",
		Count:     2,
		Source:    "chat",
		Dimensions: map[string]string{
			"tool": "filesystem_read",
		},
	}); err != nil {
		t.Fatalf("record signal: %v", err)
	}
	if err := tracker.RecordSignal(SignalEntry{
		Timestamp: time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
		Name:      "tool_call",
		Count:     1,
		Source:    "chat",
		Dimensions: map[string]string{
			"skill": "daily-briefing",
		},
	}); err != nil {
		t.Fatalf("record signal: %v", err)
	}
	if err := tracker.RecordSignal(SignalEntry{
		Timestamp: time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC),
		Name:      "agentruntime.persist_snapshot.retry",
		Count:     3,
		Source:    "runtime",
	}); err != nil {
		t.Fatalf("record non-tool signal: %v", err)
	}

	analytics, err := tracker.Analytics(7)
	if err != nil {
		t.Fatalf("analytics: %v", err)
	}
	if analytics.Days != 7 || len(analytics.Daily) != 7 {
		t.Fatalf("expected seven daily rows, got days=%d len=%d", analytics.Days, len(analytics.Daily))
	}
	if analytics.Daily[0].Day != "2026-04-25" || analytics.Daily[6].Day != "2026-05-01" {
		t.Fatalf("unexpected day range: first=%s last=%s", analytics.Daily[0].Day, analytics.Daily[6].Day)
	}
	if analytics.Totals.InputTokens != 180 || analytics.Totals.OutputTokens != 120 || analytics.Totals.TotalTokens != 300 {
		t.Fatalf("unexpected totals: %+v", analytics.Totals)
	}
	if analytics.Totals.Sessions != 2 || analytics.Totals.AvgTokensPerSession != 150 {
		t.Fatalf("unexpected session totals: %+v", analytics.Totals)
	}
	if len(analytics.Models) != 2 {
		t.Fatalf("expected two model rows, got %+v", analytics.Models)
	}
	if analytics.Models[0].Provider != "openai" || analytics.Models[0].Model != "gpt-5.4" || analytics.Models[0].TotalTokens != 250 {
		t.Fatalf("unexpected top model row: %+v", analytics.Models[0])
	}
	if len(analytics.Skills) != 2 || analytics.Skills[0].Name != "filesystem_read" || analytics.Skills[0].Calls != 2 {
		t.Fatalf("unexpected skill/tool rows: %+v", analytics.Skills)
	}
}

func TestTracker_AnalyticsReturnsZeroFilledEmptyDays(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	analytics, err := tracker.Analytics(30)
	if err != nil {
		t.Fatalf("analytics: %v", err)
	}
	if analytics.Days != 30 || len(analytics.Daily) != 30 {
		t.Fatalf("expected 30 zero-filled days, got days=%d len=%d", analytics.Days, len(analytics.Daily))
	}
	if analytics.Totals.TotalTokens != 0 || analytics.Totals.Sessions != 0 || len(analytics.Models) != 0 {
		t.Fatalf("unexpected non-empty analytics: %+v", analytics)
	}
}

func TestClampAnalyticsDaysBounds(t *testing.T) {
	tests := []struct {
		name string
		days int
		want int
	}{
		{name: "negative defaults to seven", days: -1, want: 7},
		{name: "zero defaults to seven", days: 0, want: 7},
		{name: "huge defaults to seven", days: int(^uint(0) >> 1), want: 7},
		{name: "thirty accepted", days: 30, want: 30},
		{name: "ninety accepted", days: 90, want: 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampAnalyticsDays(tt.days); got != tt.want {
				t.Fatalf("clampAnalyticsDays(%d) = %d, want %d", tt.days, got, tt.want)
			}
			if got := normalizeAnalyticsDays(tt.days); got != tt.want {
				t.Fatalf("normalizeAnalyticsDays(%d) = %d, want %d", tt.days, got, tt.want)
			}
		})
	}
}

