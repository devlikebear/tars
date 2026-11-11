package usage

import (
	"testing"
	"time"
)

func TestTracker_SummaryFilteredBySession(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	for _, entry := range []Entry{
		{Timestamp: now, Provider: "openai", Model: "m", InputTokens: 100, OutputTokens: 10, EstimatedCostUSD: 0.01, Source: "chat", SessionID: "s1"},
		{Timestamp: now.Add(-time.Hour), Provider: "openai", Model: "m", InputTokens: 200, OutputTokens: 20, EstimatedCostUSD: 0.02, Source: "chat", SessionID: "s1"},
		{Timestamp: now, Provider: "openai", Model: "m", InputTokens: 999, OutputTokens: 99, EstimatedCostUSD: 0.5, Source: "chat", SessionID: "s2"},
		{Timestamp: now, Provider: "openai", Model: "m", InputTokens: 5, OutputTokens: 5, EstimatedCostUSD: 0.001, Source: "pulse"},
	} {
		if err := tracker.Record(entry); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	got, err := tracker.SummaryFiltered("month", "provider", SummaryFilter{SessionID: " s1 "})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if got.SessionID != "s1" {
		t.Fatalf("summary should echo the session filter, got %q", got.SessionID)
	}
	if got.TotalCalls != 2 || got.TotalInput != 300 || got.TotalOutput != 30 {
		t.Fatalf("session s1 totals wrong: %+v", got)
	}
	if diff := got.TotalCostUSD - 0.03; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("session s1 cost: got %v want 0.03", got.TotalCostUSD)
	}

	// An unfiltered summary is unchanged by the new option.
	all, err := tracker.Summary("month", "provider")
	if err != nil {
		t.Fatalf("summary all: %v", err)
	}
	if all.TotalCalls != 4 || all.SessionID != "" {
		t.Fatalf("unfiltered summary should include every call: %+v", all)
	}
}

func TestTracker_SummaryGroupBySession(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	for _, entry := range []Entry{
		{Timestamp: now, Provider: "p", EstimatedCostUSD: 0.2, SessionID: "a"},
		{Timestamp: now, Provider: "p", EstimatedCostUSD: 0.1, SessionID: "b"},
		{Timestamp: now, Provider: "p", EstimatedCostUSD: 0.05},
	} {
		if err := tracker.Record(entry); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	got, err := tracker.Summary("today", "session")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if got.GroupBy != "session" {
		t.Fatalf("group_by should be session, got %q", got.GroupBy)
	}
	keys := []string{}
	for _, row := range got.Rows {
		keys = append(keys, row.Key)
	}
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "(none)" {
		t.Fatalf("rows should be keyed by session, costliest first: %v", keys)
	}
}

// A subagent run's calls are recorded with no session and a run id of the
// form "<run id>:<agent>"; a run filter has to find them by the run id.
func TestTracker_SummaryFilteredByRun(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	for _, entry := range []Entry{
		{Timestamp: now, Provider: "openai", Model: "m", InputTokens: 100, OutputTokens: 10, EstimatedCostUSD: 0.01, Source: "agent_run", RunID: "run_4:explorer"},
		{Timestamp: now, Provider: "openai", Model: "m", InputTokens: 50, OutputTokens: 5, EstimatedCostUSD: 0.02, Source: "agent_run", RunID: "run_4"},
		{Timestamp: now, Provider: "openai", Model: "m", InputTokens: 999, OutputTokens: 99, EstimatedCostUSD: 0.5, Source: "agent_run", RunID: "run_45:explorer"},
		{Timestamp: now, Provider: "openai", Model: "m", InputTokens: 7, OutputTokens: 7, EstimatedCostUSD: 0.3, Source: "chat", SessionID: "s1"},
	} {
		if err := tracker.Record(entry); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	got, err := tracker.SummaryFiltered("month", "", SummaryFilter{RunID: " run_4 "})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	// run_45 shares the prefix "run_4" and must not be counted.
	if got.TotalCalls != 2 || got.TotalInput != 150 || got.TotalOutput != 15 {
		t.Fatalf("run_4 totals wrong: %+v", got)
	}
	if diff := got.TotalCostUSD - 0.03; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("run_4 cost: got %v want 0.03", got.TotalCostUSD)
	}
}

func TestTracker_SessionCostsSince(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	for _, entry := range []Entry{
		// s1: one call before its start is left out.
		{Timestamp: now.AddDate(0, 0, -60), Provider: "p", InputTokens: 1, PricingKnown: true, EstimatedCostUSD: 9, SessionID: "s1"},
		{Timestamp: now.AddDate(0, 0, -40), Provider: "p", InputTokens: 1, PricingKnown: true, EstimatedCostUSD: 0.5, SessionID: "s1"},
		{Timestamp: now, Provider: "p", InputTokens: 1, PricingKnown: true, EstimatedCostUSD: 0.25, SessionID: "s1"},
		{Timestamp: now, Provider: "p", InputTokens: 10, SessionID: "s2"},
		// s2: read (s1 starts earlier) but before s2's own start.
		{Timestamp: now.Add(-2 * time.Hour), Provider: "p", InputTokens: 1, PricingKnown: true, EstimatedCostUSD: 5, SessionID: "s2"},
		{Timestamp: now, Provider: "p", InputTokens: 1, PricingKnown: true, EstimatedCostUSD: 3, SessionID: "other"},
	} {
		if err := tracker.Record(entry); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	got := tracker.SessionCostsSince(map[string]time.Time{
		"s1": now.AddDate(0, 0, -41), "s2": now.Add(-time.Hour), "s3": now.Add(-time.Hour),
	})
	if len(got) != 2 {
		t.Fatalf("costs %+v", got)
	}
	if c := got["s1"]; c.CostUSD < 0.7499 || c.CostUSD > 0.7501 || c.UnpricedCalls != 0 {
		t.Fatalf("s1 %+v", c)
	}
	if c := got["s2"]; c.CostUSD != 0 || c.UnpricedCalls != 1 {
		t.Fatalf("s2 %+v", c)
	}
	if got := (*Tracker)(nil).SessionCostsSince(map[string]time.Time{"s1": now}); len(got) != 0 {
		t.Fatalf("nil tracker %+v", got)
	}
	if got := tracker.SessionCostsSince(nil); len(got) != 0 {
		t.Fatalf("no sessions %+v", got)
	}
}
