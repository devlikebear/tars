package tarsserver

import (
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/usage"
)

func TestRecordTierRecommendationSignal_UsesProviderReportedCost(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	tracker, err := usage.NewTracker(t.TempDir(), usage.TrackerOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	state := chatRunState{
		sessionID:     "sess-1",
		llmResolution: llm.TierResolution{Provider: "claude-code-cli", Model: "haiku"},
		tierRecommendation: chatTierRecommendationState{
			TaskType:        "chat",
			RecommendedTier: llm.TierLight,
			ChosenTier:      llm.TierLight,
		},
	}

	recordTierRecommendationSignal(tracker, state, "completed", llm.Usage{InputTokens: 100, OutputTokens: 10, CostUSD: 0.0042})

	signals, err := tracker.Signals("today")
	if err != nil {
		t.Fatalf("signals: %v", err)
	}
	if len(signals.Rows) != 1 {
		t.Fatalf("expected 1 signal row, got %+v", signals.Rows)
	}
	dims := signals.Rows[0].Dimensions
	if dims["estimated_cost_usd"] != "0.004200" || dims["pricing_known"] != "true" || dims["cost_source"] != usage.CostSourceProvider {
		t.Fatalf("unexpected cost dimensions: %+v", dims)
	}
}
