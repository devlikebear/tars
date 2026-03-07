package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func normalizeLimits(in Limits) Limits {
	out := in
	if out.DailyUSD < 0 {
		out.DailyUSD = 0
	}
	if out.WeeklyUSD < 0 {
		out.WeeklyUSD = 0
	}
	if out.MonthlyUSD < 0 {
		out.MonthlyUSD = 0
	}
	mode := strings.TrimSpace(strings.ToLower(out.Mode))
	switch mode {
	case "hard", "soft":
		out.Mode = mode
	default:
		out.Mode = "soft"
	}
	return out
}

func (t *Tracker) loadPersistedLimits() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	raw, err := os.ReadFile(t.limitsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var parsed Limits
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	t.limits = normalizeLimits(parsed)
	return nil
}

func (t *Tracker) UpdateLimits(next Limits) (Limits, error) {
	if t == nil {
		return Limits{}, fmt.Errorf("usage tracker is nil")
	}

	normalized := normalizeLimits(next)
	t.mu.Lock()
	defer t.mu.Unlock()

	t.limits = normalized
	payload, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return Limits{}, err
	}
	if err := os.WriteFile(t.limitsPath, append(payload, '\n'), 0o644); err != nil {
		return Limits{}, err
	}
	return normalized, nil
}

func (t *Tracker) CheckLimitStatus() (LimitStatus, error) {
	if t == nil {
		return LimitStatus{}, fmt.Errorf("usage tracker is nil")
	}

	limits := t.Limits()
	for _, item := range buildLimitChecks(limits) {
		if item.limit <= 0 {
			continue
		}
		summary, err := t.Summary(item.period, "provider")
		if err != nil {
			return LimitStatus{}, err
		}
		if summary.TotalCostUSD >= item.limit {
			return LimitStatus{
				Exceeded: true,
				Mode:     limits.Mode,
				Period:   item.period,
				SpentUSD: summary.TotalCostUSD,
				LimitUSD: item.limit,
			}, nil
		}
	}

	return LimitStatus{
		Exceeded: false,
		Mode:     limits.Mode,
	}, nil
}

type limitCheck struct {
	period string
	limit  float64
}

func buildLimitChecks(limits Limits) []limitCheck {
	return []limitCheck{
		{period: "today", limit: limits.DailyUSD},
		{period: "week", limit: limits.WeeklyUSD},
		{period: "month", limit: limits.MonthlyUSD},
	}
}
