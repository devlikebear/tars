package usage

import (
	"strings"

	"github.com/devlikebear/tars/internal/llm"
)

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
		"openai/gpt-4o-mini":         {InputPer1MUSD: 0.15, OutputPer1MUSD: 0.60, CacheReadPer1MUSD: 0.075},
		"openai/gpt-4.1-mini":        {InputPer1MUSD: 0.40, OutputPer1MUSD: 1.60, CacheReadPer1MUSD: 0.10},
		"openai/gpt-4.1":             {InputPer1MUSD: 2.00, OutputPer1MUSD: 8.00, CacheReadPer1MUSD: 0.50},
		"openai/gpt-5.3-codex":       {InputPer1MUSD: 1.50, OutputPer1MUSD: 6.00, CacheReadPer1MUSD: 0.375},
		"openai-codex/gpt-5.3-codex": {InputPer1MUSD: 1.50, OutputPer1MUSD: 6.00, CacheReadPer1MUSD: 0.375},
		"anthropic/*":                {InputPer1MUSD: 3.00, OutputPer1MUSD: 15.00, CacheReadPer1MUSD: 0.30, CacheWritePer1MUSD: 3.75},
		"gemini/*":                   {InputPer1MUSD: 0.30, OutputPer1MUSD: 2.50},
		"gemini-native/*":            {InputPer1MUSD: 0.30, OutputPer1MUSD: 2.50},
		"bifrost/*":                  {InputPer1MUSD: 0.00, OutputPer1MUSD: 0.00},
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
	if price, ok := t.priceByKey[p+"/*"]; ok {
		return price, true
	}
	if price, ok := t.priceByKey["*/"+m]; ok {
		return price, true
	}
	return ModelPrice{}, false
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
