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
