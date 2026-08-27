package tarsserver

import (
	"io"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/rs/zerolog"
)

func shippedChatCompactionOptions() chatCompactionOptions {
	shipped := config.Default().CompactionConfig
	return chatCompactionOptions{
		TriggerTokens:    shipped.CompactionTriggerTokens,
		KeepRecentTokens: shipped.CompactionKeepRecentTokens,
	}
}

func TestApplyTierContextWindow_ResizesAgainstTheTierModel(t *testing.T) {
	logger := zerolog.New(io.Discard)
	base := shippedChatCompactionOptions()

	large := applyTierContextWindow(base, llm.TierResolution{
		Model:         "claude-opus-5",
		ContextWindow: 1000000,
		MaxTokens:     16000,
	}, logger)
	small := applyTierContextWindow(base, llm.TierResolution{
		Model:         "claude-haiku-4-5",
		ContextWindow: 200000,
		MaxTokens:     16000,
	}, logger)

	if large.TriggerTokens <= small.TriggerTokens {
		t.Fatalf("1M tier trigger %d is not above the 200k tier's %d", large.TriggerTokens, small.TriggerTokens)
	}
	if large.ContextWindow != 1000000 {
		t.Errorf("ContextWindow = %d, want it carried for the pre-flight check", large.ContextWindow)
	}
}

func TestApplyTierContextWindow_UnknownWindowLeavesOptionsAlone(t *testing.T) {
	// Gateway-hosted models must not be guessed at.
	logger := zerolog.New(io.Discard)
	base := shippedChatCompactionOptions()

	got := applyTierContextWindow(base, llm.TierResolution{Model: "MiniMax-M2.7"}, logger)
	if got.TriggerTokens != base.TriggerTokens || got.KeepRecentTokens != base.KeepRecentTokens {
		t.Fatalf("got %+v, want the incoming options unchanged (%+v)", got, base)
	}
	if got.ContextWindow != 0 {
		t.Errorf("ContextWindow = %d, want 0 for an unknown window", got.ContextWindow)
	}
}

func TestApplyTierContextWindow_CustomizedCompactionIsPreserved(t *testing.T) {
	logger := zerolog.New(io.Discard)
	custom := chatCompactionOptions{TriggerTokens: 42000, KeepRecentTokens: 7000}

	got := applyTierContextWindow(custom, llm.TierResolution{
		Model:         "claude-opus-5",
		ContextWindow: 1000000,
		MaxTokens:     16000,
	}, logger)
	if got.TriggerTokens != 42000 || got.KeepRecentTokens != 7000 {
		t.Fatalf("got %+v, want the operator's tuning untouched", got)
	}
}

func TestApplyTierContextWindow_KeepsUnrelatedOptions(t *testing.T) {
	logger := zerolog.New(io.Discard)
	base := shippedChatCompactionOptions()
	base.LLMMode = "deterministic"
	base.LLMTimeoutSeconds = 42
	base.KeepRecentFraction = 0.25

	got := applyTierContextWindow(base, llm.TierResolution{
		Model:         "claude-opus-5",
		ContextWindow: 1000000,
	}, logger)
	if got.LLMMode != "deterministic" || got.LLMTimeoutSeconds != 42 || got.KeepRecentFraction != 0.25 {
		t.Fatalf("resizing clobbered unrelated options: %+v", got)
	}
}

func TestChatRequestedTier(t *testing.T) {
	if got := chatRequestedTier(chatRequestPayload{}); got != "" {
		t.Errorf("no recommendation payload = %q, want empty", got)
	}
	if got := chatRequestedTier(chatRequestPayload{TierRecommendation: &chatTierRecommendationPayload{}}); got != "" {
		t.Errorf("empty chosen tier = %q, want empty", got)
	}
	if got := chatRequestedTier(chatRequestPayload{TierRecommendation: &chatTierRecommendationPayload{ChosenTier: " heavy "}}); got != "heavy" {
		t.Errorf("chosen tier = %q, want heavy", got)
	}
}
