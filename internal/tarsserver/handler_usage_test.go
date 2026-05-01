package tarsserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

func TestUsageAPI_SummaryAndLimits(t *testing.T) {
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
		InputTokens:      10,
		OutputTokens:     5,
		EstimatedCostUSD: 0.0001,
		Source:           "chat",
		PricingKnown:     true,
	}); err != nil {
		t.Fatalf("record usage: %v", err)
	}
	handler := newUsageAPIHandler(tracker, "off", zerolog.Nop())

	reqSummary := httptest.NewRequest(http.MethodGet, "/v1/usage/summary?period=today&group_by=provider", nil)
	recSummary := httptest.NewRecorder()
	handler.ServeHTTP(recSummary, reqSummary)
	if recSummary.Code != http.StatusOK {
		t.Fatalf("summary status=%d body=%s", recSummary.Code, recSummary.Body.String())
	}
	var summaryBody struct {
		Summary usage.Summary `json:"summary"`
	}
	if err := json.Unmarshal(recSummary.Body.Bytes(), &summaryBody); err != nil {
		t.Fatalf("decode summary body: %v", err)
	}
	if summaryBody.Summary.TotalCalls != 1 {
		t.Fatalf("expected 1 call, got %+v", summaryBody.Summary)
	}

	reqPatch := httptest.NewRequest(http.MethodPatch, "/v1/usage/limits", strings.NewReader(`{"daily_usd":1.5,"weekly_usd":2.5,"monthly_usd":3.5,"mode":"hard"}`))
	recPatch := httptest.NewRecorder()
	handler.ServeHTTP(recPatch, reqPatch)
	if recPatch.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", recPatch.Code, recPatch.Body.String())
	}
	var limits usage.Limits
	if err := json.Unmarshal(recPatch.Body.Bytes(), &limits); err != nil {
		t.Fatalf("decode limits body: %v", err)
	}
	if limits.Mode != "hard" || limits.DailyUSD != 1.5 {
		t.Fatalf("unexpected limits: %+v", limits)
	}
}

func TestUsageAPI_TodayTokens(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	tracker, err := usage.NewTracker(t.TempDir(), usage.TrackerOptions{
		Now: func() time.Time { return now },
		InitialLimits: usage.Limits{
			DailyTokens: 1000,
		},
	})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	if err := tracker.Record(usage.Entry{
		Timestamp:    now,
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		InputTokens:  850,
		OutputTokens: 25,
		Source:       "chat",
	}); err != nil {
		t.Fatalf("record usage: %v", err)
	}
	handler := newUsageAPIHandler(tracker, "off", zerolog.Nop())

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/usage/today", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("today status=%d body=%s", rec.Code, rec.Body.String())
	}
	var today usage.DailyTokenSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &today); err != nil {
		t.Fatalf("decode today body: %v", err)
	}
	if today.TotalTokens != 875 || today.BudgetTokens != 1000 || today.Level != "error" {
		t.Fatalf("unexpected today usage: %+v", today)
	}
}

func TestUsageAPI_Signals(t *testing.T) {
	tracker, err := usage.NewTracker(t.TempDir(), usage.TrackerOptions{})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	if err := tracker.RecordSignal(usage.SignalEntry{
		Name:   "tool_call",
		Source: "chat",
		Dimensions: map[string]string{
			"tool": "subagents_plan",
		},
	}); err != nil {
		t.Fatalf("record signal: %v", err)
	}

	handler := newUsageAPIHandler(tracker, "off", zerolog.Nop())
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/signals?period=today", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Signals usage.SignalSummary `json:"signals"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode signals: %v", err)
	}
	if out.Signals.TotalCount != 1 || len(out.Signals.Rows) != 1 {
		t.Fatalf("unexpected signal summary: %+v", out.Signals)
	}
	if out.Signals.Rows[0].Dimensions["tool"] != "subagents_plan" {
		t.Fatalf("unexpected signal row: %+v", out.Signals.Rows[0])
	}
}

func TestUsageAPI_Analytics(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	tracker, err := usage.NewTracker(t.TempDir(), usage.TrackerOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	if err := tracker.Record(usage.Entry{
		Timestamp:        now.Add(-time.Hour),
		Provider:         "openai",
		Model:            "gpt-5.4",
		InputTokens:      120,
		OutputTokens:     80,
		EstimatedCostUSD: 0.010,
		Source:           "chat",
		SessionID:        "sess-a",
	}); err != nil {
		t.Fatalf("record usage: %v", err)
	}

	handler := newUsageAPIHandler(tracker, "off", zerolog.Nop())
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/analytics?days=7", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var out usage.Analytics
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode analytics: %v", err)
	}
	if out.Days != 7 || out.Totals.TotalTokens != 200 || len(out.Daily) != 7 || len(out.Models) != 1 {
		t.Fatalf("unexpected analytics response: %+v", out)
	}
}
