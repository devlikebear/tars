package usage

import (
	"sync"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
)

func TestEstimateCost_AnthropicPerModelPricing(t *testing.T) {
	cases := []struct {
		name           string
		model          string
		wantInput      float64
		wantOutput     float64
		wantCacheRead  float64
		wantCacheWrite float64
	}{
		{name: "opus-4-5", model: "claude-opus-4-5", wantInput: 5.00, wantOutput: 25.00, wantCacheRead: 0.50, wantCacheWrite: 6.25},
		{name: "opus-4-6", model: "claude-opus-4-6", wantInput: 5.00, wantOutput: 25.00, wantCacheRead: 0.50, wantCacheWrite: 6.25},
		{name: "opus-4-7", model: "claude-opus-4-7", wantInput: 5.00, wantOutput: 25.00, wantCacheRead: 0.50, wantCacheWrite: 6.25},
		{name: "sonnet-4-5", model: "claude-sonnet-4-5", wantInput: 3.00, wantOutput: 15.00, wantCacheRead: 0.30, wantCacheWrite: 3.75},
		{name: "sonnet-4-6", model: "claude-sonnet-4-6", wantInput: 3.00, wantOutput: 15.00, wantCacheRead: 0.30, wantCacheWrite: 3.75},
		{name: "haiku-4-5", model: "claude-haiku-4-5", wantInput: 1.00, wantOutput: 5.00, wantCacheRead: 0.10, wantCacheWrite: 1.25},
		{name: "haiku-4-5-dated", model: "claude-haiku-4-5-20251001", wantInput: 1.00, wantOutput: 5.00, wantCacheRead: 0.10, wantCacheWrite: 1.25},
	}

	tracker := newCostTestTracker(t)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotInput, ok := tracker.EstimateCost("anthropic", tc.model, llm.Usage{InputTokens: 1_000_000})
			if !ok {
				t.Fatalf("expected known pricing for %s", tc.model)
			}
			if diff := gotInput - tc.wantInput; diff < -1e-9 || diff > 1e-9 {
				t.Fatalf("input cost = %v, want %v", gotInput, tc.wantInput)
			}

			gotOutput, ok := tracker.EstimateCost("anthropic", tc.model, llm.Usage{OutputTokens: 1_000_000})
			if !ok {
				t.Fatalf("expected known pricing for %s", tc.model)
			}
			if diff := gotOutput - tc.wantOutput; diff < -1e-9 || diff > 1e-9 {
				t.Fatalf("output cost = %v, want %v", gotOutput, tc.wantOutput)
			}

			gotCacheRead, ok := tracker.EstimateCost("anthropic", tc.model, llm.Usage{CacheReadTokens: 1_000_000})
			if !ok {
				t.Fatalf("expected known pricing for %s", tc.model)
			}
			if diff := gotCacheRead - tc.wantCacheRead; diff < -1e-9 || diff > 1e-9 {
				t.Fatalf("cache read cost = %v, want %v", gotCacheRead, tc.wantCacheRead)
			}

			gotCacheWrite, ok := tracker.EstimateCost("anthropic", tc.model, llm.Usage{CacheWriteTokens: 1_000_000})
			if !ok {
				t.Fatalf("expected known pricing for %s", tc.model)
			}
			if diff := gotCacheWrite - tc.wantCacheWrite; diff < -1e-9 || diff > 1e-9 {
				t.Fatalf("cache write cost = %v, want %v", gotCacheWrite, tc.wantCacheWrite)
			}
		})
	}
}

func TestEstimateCost_AnthropicFamiliesProduceDifferentCosts(t *testing.T) {
	tracker := newCostTestTracker(t)

	opus, _ := tracker.EstimateCost("anthropic", "claude-opus-4-6", llm.Usage{InputTokens: 10_000, OutputTokens: 10_000})
	sonnet, _ := tracker.EstimateCost("anthropic", "claude-sonnet-4-6", llm.Usage{InputTokens: 10_000, OutputTokens: 10_000})
	haiku, _ := tracker.EstimateCost("anthropic", "claude-haiku-4-5", llm.Usage{InputTokens: 10_000, OutputTokens: 10_000})

	if opus <= sonnet || sonnet <= haiku || haiku <= 0 {
		t.Fatalf("expected opus > sonnet > haiku > 0 for identical tokens, got opus=%v sonnet=%v haiku=%v", opus, sonnet, haiku)
	}
}

func TestResolvePrice_AnthropicFamilyPrefixMatchesDatedVariants(t *testing.T) {
	tracker := newCostTestTracker(t)

	cases := []struct {
		model      string
		checkPrice func(price ModelPrice) bool
	}{
		{model: "claude-opus-4-8", checkPrice: func(p ModelPrice) bool { return p.InputPer1MUSD == 5.00 && p.OutputPer1MUSD == 25.00 }},
		{model: "claude-sonnet-4-7-20260101", checkPrice: func(p ModelPrice) bool { return p.InputPer1MUSD == 3.00 && p.OutputPer1MUSD == 15.00 }},
		{model: "CLAUDE-HAIKU-4-5-SNAPSHOT", checkPrice: func(p ModelPrice) bool { return p.InputPer1MUSD == 1.00 && p.OutputPer1MUSD == 5.00 }},
	}

	for _, tc := range cases {
		price, ok := tracker.resolvePrice("anthropic", tc.model)
		if !ok {
			t.Fatalf("expected family price for %s", tc.model)
		}
		if !tc.checkPrice(price) {
			t.Fatalf("unexpected family price for %s: %+v", tc.model, price)
		}
		if price.CacheReadPer1MUSD <= 0 || price.CacheWritePer1MUSD <= 0 {
			t.Fatalf("expected cache rates for %s: %+v", tc.model, price)
		}
	}
}

func TestEstimateCost_WildcardFallbackEmitsDiagnosticOncePerModel(t *testing.T) {
	var mu sync.Mutex
	warned := map[string]int{}
	original := warnPriceFallback
	warnPriceFallback = func(provider, model string) {
		mu.Lock()
		defer mu.Unlock()
		warned[provider+"/"+model]++
	}
	defer func() { warnPriceFallback = original }()

	tracker := newCostTestTracker(t)
	usage := llm.Usage{InputTokens: 1_000, OutputTokens: 100}

	first, ok := tracker.EstimateCost("anthropic", "MiniMax-M2.7", usage)
	if !ok {
		t.Fatalf("expected wildcard fallback to still produce an estimate")
	}
	if first <= 0 {
		t.Fatalf("expected positive fallback estimate, got %v", first)
	}
	if _, ok := tracker.EstimateCost("anthropic", "minimax-m2.7", usage); !ok {
		t.Fatalf("expected wildcard fallback estimate on repeat call")
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = tracker.EstimateCost("anthropic", "minimax-m2.7", usage)
		}()
	}
	wg.Wait()

	mu.Lock()
	count := warned["anthropic/minimax-m2.7"]
	mu.Unlock()
	if count != 1 {
		t.Fatalf("expected exactly one fallback warning for minimax-m2.7, got %d", count)
	}

	if _, ok := tracker.EstimateCost("anthropic", "some-gateway-model", usage); !ok {
		t.Fatalf("expected wildcard fallback estimate for second unknown model")
	}
	mu.Lock()
	count = warned["anthropic/some-gateway-model"]
	mu.Unlock()
	if count != 1 {
		t.Fatalf("expected one fallback warning for some-gateway-model, got %d", count)
	}
}

func TestEstimateCost_FamilyPrefixMatchDoesNotWarnFallback(t *testing.T) {
	var mu sync.Mutex
	warnCount := 0
	original := warnPriceFallback
	warnPriceFallback = func(string, string) {
		mu.Lock()
		defer mu.Unlock()
		warnCount++
	}
	defer func() { warnPriceFallback = original }()

	tracker := newCostTestTracker(t)
	if _, ok := tracker.EstimateCost("anthropic", "claude-opus-4-8", llm.Usage{InputTokens: 10}); !ok {
		t.Fatalf("expected family price match")
	}
	mu.Lock()
	defer mu.Unlock()
	if warnCount != 0 {
		t.Fatalf("expected no fallback warnings for family prefix match, got %d", warnCount)
	}
}

func TestEstimateCost_PriceOverridesTakePrecedenceOverBuiltIns(t *testing.T) {
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{
		PriceOverrides: map[string]ModelPrice{
			"anthropic/claude-opus-4-6": {InputPer1MUSD: 9.00, OutputPer1MUSD: 9.00, CacheReadPer1MUSD: 9.00, CacheWritePer1MUSD: 9.00},
			"anthropic/claude-opus-4-8": {InputPer1MUSD: 7.00, OutputPer1MUSD: 7.00, CacheReadPer1MUSD: 7.00, CacheWritePer1MUSD: 7.00},
			"anthropic/*":               {InputPer1MUSD: 2.00, OutputPer1MUSD: 2.00, CacheReadPer1MUSD: 2.00, CacheWritePer1MUSD: 2.00},
		},
	})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}

	got, ok := tracker.EstimateCost("anthropic", "claude-opus-4-6", llm.Usage{InputTokens: 1_000_000})
	if !ok || got != 9.00 {
		t.Fatalf("override should beat built-in exact entry, got %v ok=%v", got, ok)
	}

	got, ok = tracker.EstimateCost("anthropic", "claude-opus-4-8", llm.Usage{InputTokens: 1_000_000})
	if !ok || got != 7.00 {
		t.Fatalf("override should beat built-in family prefix entry, got %v ok=%v", got, ok)
	}

	got, ok = tracker.EstimateCost("anthropic", "unknown-gateway-model", llm.Usage{InputTokens: 1_000_000})
	if !ok || got != 2.00 {
		t.Fatalf("wildcard override should beat built-in wildcard fallback, got %v ok=%v", got, ok)
	}
}

func TestEstimateCost_OpenAIEntriesReflectDocumentedPricing(t *testing.T) {
	tracker := newCostTestTracker(t)

	cases := []struct {
		provider     string
		model        string
		wantInput    float64
		wantOutput   float64
		wantCacheHit float64
	}{
		{provider: "openai", model: "gpt-5.3-codex", wantInput: 1.75, wantOutput: 14.00, wantCacheHit: 0.175},
		{provider: "openai-codex", model: "gpt-5.3-codex", wantInput: 1.75, wantOutput: 14.00, wantCacheHit: 0.175},
		{provider: "openai", model: "gpt-5.4", wantInput: 2.50, wantOutput: 15.00, wantCacheHit: 0.25},
		{provider: "openai-codex", model: "gpt-5.4", wantInput: 2.50, wantOutput: 15.00, wantCacheHit: 0.25},
	}

	for _, tc := range cases {
		gotInput, ok := tracker.EstimateCost(tc.provider, tc.model, llm.Usage{InputTokens: 1_000_000})
		if !ok {
			t.Fatalf("expected known pricing for %s/%s", tc.provider, tc.model)
		}
		if gotInput != tc.wantInput {
			t.Fatalf("%s/%s input cost = %v, want %v", tc.provider, tc.model, gotInput, tc.wantInput)
		}

		gotOutput, ok := tracker.EstimateCost(tc.provider, tc.model, llm.Usage{OutputTokens: 1_000_000})
		if !ok {
			t.Fatalf("expected known pricing for %s/%s", tc.provider, tc.model)
		}
		if gotOutput != tc.wantOutput {
			t.Fatalf("%s/%s output cost = %v, want %v", tc.provider, tc.model, gotOutput, tc.wantOutput)
		}

		gotCached, ok := tracker.EstimateCost(tc.provider, tc.model, llm.Usage{CacheReadTokens: 1_000_000})
		if !ok {
			t.Fatalf("expected known pricing for %s/%s", tc.provider, tc.model)
		}
		if diff := gotCached - tc.wantCacheHit; diff < -1e-9 || diff > 1e-9 {
			t.Fatalf("%s/%s cache read cost = %v, want %v", tc.provider, tc.model, gotCached, tc.wantCacheHit)
		}
	}
}

func TestResolvePrice_UnknownProviderReturnsNoPrice(t *testing.T) {
	tracker := newCostTestTracker(t)

	if _, ok := tracker.resolvePrice("unknown-provider", "some-model"); ok {
		t.Fatalf("expected no price for unknown provider")
	}
	if _, ok := tracker.resolvePrice("", "claude-opus-4-6"); ok {
		t.Fatalf("expected no price for empty provider")
	}
}

func newCostTestTracker(t *testing.T) *Tracker {
	t.Helper()
	tracker, err := NewTracker(t.TempDir(), TrackerOptions{})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	return tracker
}
