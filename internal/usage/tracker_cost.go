package usage

import (
	"strings"

	"github.com/devlikebear/tars/internal/llm"
	zlog "github.com/rs/zerolog/log"
)

// warnPriceFallback reports that a model fell through to the provider
// wildcard price. Package-level so tests can observe emissions; callers
// must ensure it fires at most once per model (see Tracker.noteWildcardFallback).
var warnPriceFallback = func(provider, model string) {
	zlog.Warn().
		Str("provider", provider).
		Str("model", model).
		Msg("no per-model usage pricing; estimated with provider fallback rates")
}

// anthropicFamilyPrefixPrices prices Anthropic models whose exact ID is not
// in the table (dated snapshots, future point releases) at their documented
// family tier instead of the wildcard fallback.
var anthropicFamilyPrefixPrices = []struct {
	prefix string
	price  ModelPrice
}{
	{prefix: "claude-opus-", price: ModelPrice{InputPer1MUSD: 5.00, OutputPer1MUSD: 25.00, CacheReadPer1MUSD: 0.50, CacheWritePer1MUSD: 6.25}},
	{prefix: "claude-sonnet-", price: ModelPrice{InputPer1MUSD: 3.00, OutputPer1MUSD: 15.00, CacheReadPer1MUSD: 0.30, CacheWritePer1MUSD: 3.75}},
	{prefix: "claude-haiku-", price: ModelPrice{InputPer1MUSD: 1.00, OutputPer1MUSD: 5.00, CacheReadPer1MUSD: 0.10, CacheWritePer1MUSD: 1.25}},
}

func sanitizePrice(in ModelPrice) ModelPrice {
	out := in
	if out.InputPer1MUSD < 0 {
		out.InputPer1MUSD = 0
	}
	if out.OutputPer1MUSD < 0 {
		out.OutputPer1MUSD = 0
	}
	if out.CacheReadPer1MUSD < 0 {
		out.CacheReadPer1MUSD = 0
	}
	if out.CacheWritePer1MUSD < 0 {
		out.CacheWritePer1MUSD = 0
	}
	return out
}

func defaultPriceTable() map[string]ModelPrice {
	return map[string]ModelPrice{
		"openai/gpt-4o-mini":                  {InputPer1MUSD: 0.15, OutputPer1MUSD: 0.60, CacheReadPer1MUSD: 0.075},
		"openai/gpt-4.1-mini":                 {InputPer1MUSD: 0.40, OutputPer1MUSD: 1.60, CacheReadPer1MUSD: 0.10},
		"openai/gpt-4.1":                      {InputPer1MUSD: 2.00, OutputPer1MUSD: 8.00, CacheReadPer1MUSD: 0.50},
		"openai/gpt-5.3-codex":                {InputPer1MUSD: 1.75, OutputPer1MUSD: 14.00, CacheReadPer1MUSD: 0.175},
		"openai-codex/gpt-5.3-codex":          {InputPer1MUSD: 1.75, OutputPer1MUSD: 14.00, CacheReadPer1MUSD: 0.175},
		"openai/gpt-5.4":                      {InputPer1MUSD: 2.50, OutputPer1MUSD: 15.00, CacheReadPer1MUSD: 0.25},
		"openai-codex/gpt-5.4":                {InputPer1MUSD: 2.50, OutputPer1MUSD: 15.00, CacheReadPer1MUSD: 0.25},
		"anthropic/claude-opus-4-5":           {InputPer1MUSD: 5.00, OutputPer1MUSD: 25.00, CacheReadPer1MUSD: 0.50, CacheWritePer1MUSD: 6.25},
		"anthropic/claude-opus-4-6":           {InputPer1MUSD: 5.00, OutputPer1MUSD: 25.00, CacheReadPer1MUSD: 0.50, CacheWritePer1MUSD: 6.25},
		"anthropic/claude-opus-4-7":           {InputPer1MUSD: 5.00, OutputPer1MUSD: 25.00, CacheReadPer1MUSD: 0.50, CacheWritePer1MUSD: 6.25},
		"anthropic/claude-sonnet-4-5":         {InputPer1MUSD: 3.00, OutputPer1MUSD: 15.00, CacheReadPer1MUSD: 0.30, CacheWritePer1MUSD: 3.75},
		"anthropic/claude-sonnet-4-6":         {InputPer1MUSD: 3.00, OutputPer1MUSD: 15.00, CacheReadPer1MUSD: 0.30, CacheWritePer1MUSD: 3.75},
		"anthropic/claude-haiku-4-5":          {InputPer1MUSD: 1.00, OutputPer1MUSD: 5.00, CacheReadPer1MUSD: 0.10, CacheWritePer1MUSD: 1.25},
		"anthropic/claude-haiku-4-5-20251001": {InputPer1MUSD: 1.00, OutputPer1MUSD: 5.00, CacheReadPer1MUSD: 0.10, CacheWritePer1MUSD: 1.25},
		// Fallback for gateway-hosted or unrecognized Anthropic-kind models
		// (e.g. config/default.yaml routes MiniMax through kind: anthropic);
		// Sonnet-class mid rates so unknown traffic still gets an estimate.
		"anthropic/*":     {InputPer1MUSD: 3.00, OutputPer1MUSD: 15.00, CacheReadPer1MUSD: 0.30, CacheWritePer1MUSD: 3.75},
		"gemini/*":        {InputPer1MUSD: 0.30, OutputPer1MUSD: 2.50},
		"gemini-native/*": {InputPer1MUSD: 0.30, OutputPer1MUSD: 2.50},
	}
}

func (t *Tracker) EstimateCost(provider, model string, u llm.Usage) (float64, bool) {
	if t == nil {
		return 0, false
	}

	price, ok := t.resolvePrice(provider, model)
	if !ok {
		return 0, false
	}

	input, output, cached, cacheRead, cacheWrite := clampUsageTokens(u)
	normalInput := input
	if cached > 0 {
		normalInput -= cached
		if normalInput < 0 {
			normalInput = 0
		}
	}

	cost := 0.0
	cost += float64(normalInput) * price.InputPer1MUSD / 1_000_000.0
	cost += float64(output) * price.OutputPer1MUSD / 1_000_000.0
	cost += cachedReadCost(price, cached, cacheRead)
	cost += cacheWriteCost(price, cacheWrite)
	if cost < 0 {
		return 0, true
	}
	return cost, true
}

func (t *Tracker) resolvePrice(provider, model string) (ModelPrice, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	p := strings.TrimSpace(strings.ToLower(provider))
	m := strings.TrimSpace(strings.ToLower(model))
	if p == "" || m == "" {
		return ModelPrice{}, false
	}
	if price, ok := t.priceByKey[p+"/"+m]; ok {
		return price, true
	}
	if price, ok := matchFamilyPrice(p, m); ok {
		return price, true
	}
	if price, ok := t.priceByKey[p+"/*"]; ok {
		t.noteWildcardFallback(p, m)
		return price, true
	}
	if price, ok := t.priceByKey["*/"+m]; ok {
		t.noteWildcardFallback(p, m)
		return price, true
	}
	return ModelPrice{}, false
}

func matchFamilyPrice(provider, model string) (ModelPrice, bool) {
	if provider != "anthropic" {
		return ModelPrice{}, false
	}
	for _, family := range anthropicFamilyPrefixPrices {
		if strings.HasPrefix(model, family.prefix) {
			return family.price, true
		}
	}
	return ModelPrice{}, false
}

// noteWildcardFallback emits the fallback diagnostic at most once per model.
// Callers must hold t.mu.
func (t *Tracker) noteWildcardFallback(provider, model string) {
	key := provider + "/" + model
	if _, ok := t.warnedFallbackModels[key]; ok {
		return
	}
	t.warnedFallbackModels[key] = struct{}{}
	warnPriceFallback(provider, model)
}

func clampUsageTokens(u llm.Usage) (input int, output int, cached int, cacheRead int, cacheWrite int) {
	input = maxInt(u.InputTokens, 0)
	output = maxInt(u.OutputTokens, 0)
	cached = maxInt(u.CachedTokens, 0)
	cacheRead = maxInt(u.CacheReadTokens, 0)
	cacheWrite = maxInt(u.CacheWriteTokens, 0)
	return input, output, cached, cacheRead, cacheWrite
}

func cachedReadCost(price ModelPrice, cached int, cacheRead int) float64 {
	if cacheRead <= 0 && cached <= 0 {
		return 0
	}
	rate := price.CacheReadPer1MUSD
	if rate <= 0 {
		rate = price.InputPer1MUSD
	}
	if cacheRead > 0 {
		return float64(cacheRead) * rate / 1_000_000.0
	}
	return float64(cached) * rate / 1_000_000.0
}

func cacheWriteCost(price ModelPrice, cacheWrite int) float64 {
	if cacheWrite <= 0 {
		return 0
	}
	rate := price.CacheWritePer1MUSD
	if rate <= 0 {
		rate = price.InputPer1MUSD
	}
	return float64(cacheWrite) * rate / 1_000_000.0
}

func maxInt(v int, floor int) int {
	if v < floor {
		return floor
	}
	return v
}
