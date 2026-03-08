package tool

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/usage"
)

func TestUsageReportTool_ReturnsSummaryAndLimits(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	tracker, err := usage.NewTracker(t.TempDir(), usage.TrackerOptions{
		Now: func() time.Time { return now },
		InitialLimits: usage.Limits{
			DailyUSD:   10,
			WeeklyUSD:  50,
			MonthlyUSD: 150,
			Mode:       "soft",
		},
	})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	if err := tracker.Record(usage.Entry{
		Timestamp:        now,
		Provider:         "openai",
		Model:            "gpt-4o-mini",
		InputTokens:      100,
		OutputTokens:     50,
		EstimatedCostUSD: 0.0001,
		Source:           "chat",
		PricingKnown:     true,
	}); err != nil {
		t.Fatalf("record: %v", err)
	}

	tl := NewUsageReportTool(tracker)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"period":"today","group_by":"provider"}`))
	if err != nil {
		t.Fatalf("execute usage_report: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %s", result.Text())
	}
	var payload struct {
		Summary usage.Summary `json:"summary"`
		Limits  usage.Limits  `json:"limits"`
	}
	if err := json.Unmarshal([]byte(result.Text()), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Summary.TotalCalls != 1 {
		t.Fatalf("expected total_calls 1, got %+v", payload.Summary)
	}
	if payload.Limits.Mode != "soft" {
		t.Fatalf("expected soft mode, got %+v", payload.Limits)
	}
}
