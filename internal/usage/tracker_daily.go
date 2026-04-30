package usage

import (
	"fmt"
	"math"
	"time"
)

func (t *Tracker) TodayTokens() (DailyTokenSummary, error) {
	if t == nil {
		return DailyTokenSummary{}, fmt.Errorf("usage tracker is nil")
	}

	now := t.nowFn().UTC()
	start := dayStartUTC(now)
	summary, err := t.Summary("today", "provider")
	if err != nil {
		return DailyTokenSummary{}, err
	}

	budget := t.Limits().DailyTokens
	total := summary.TotalInput + summary.TotalOutput
	out := DailyTokenSummary{
		Date:          start.Format("2006-01-02"),
		ResetAt:       start.AddDate(0, 0, 1).Format(time.RFC3339),
		InputTokens:   summary.TotalInput,
		OutputTokens:  summary.TotalOutput,
		TotalTokens:   total,
		BudgetTokens:  budget,
		BudgetEnabled: budget > 0,
		Level:         "disabled",
	}
	if budget <= 0 {
		return out, nil
	}

	out.UsagePercent = math.Round((float64(total)/float64(budget))*1000) / 10
	switch {
	case out.UsagePercent >= 85:
		out.Level = "error"
	case out.UsagePercent >= 60:
		out.Level = "warning"
	default:
		out.Level = "default"
	}
	return out, nil
}
