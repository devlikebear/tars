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
