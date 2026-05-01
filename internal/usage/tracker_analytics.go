package usage

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Analytics struct {
	Days   int                 `json:"days"`
	Totals AnalyticsTotals     `json:"totals"`
	Daily  []AnalyticsDailyRow `json:"daily"`
	Models []AnalyticsModelRow `json:"models"`
	Skills []AnalyticsSkillRow `json:"skills"`
}

type AnalyticsTotals struct {
	Calls               int     `json:"calls"`
	Sessions            int     `json:"sessions"`
	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	TotalTokens         int     `json:"total_tokens"`
	CostUSD             float64 `json:"cost_usd"`
	AvgTokensPerSession float64 `json:"avg_tokens_per_session"`
}

type AnalyticsDailyRow struct {
	Day          string  `json:"day"`
	Calls        int     `json:"calls"`
	Sessions     int     `json:"sessions"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

type AnalyticsModelRow struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Calls        int     `json:"calls"`
	Sessions     int     `json:"sessions"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

type AnalyticsSkillRow struct {
	Name    string `json:"name"`
	Source  string `json:"source,omitempty"`
	Calls   int    `json:"calls"`
	FirstAt string `json:"first_at,omitempty"`
	LastAt  string `json:"last_at,omitempty"`
}

type analyticsDailyAccumulator struct {
	row      AnalyticsDailyRow
	sessions map[string]struct{}
}

type analyticsModelAccumulator struct {
	row      AnalyticsModelRow
	sessions map[string]struct{}
}

type analyticsSkillAccumulator struct {
	row AnalyticsSkillRow
}

func (t *Tracker) Analytics(days int) (Analytics, error) {
	if t == nil {
		return Analytics{}, fmt.Errorf("usage tracker is nil")
	}
	days = normalizeAnalyticsDays(days)
	now := t.nowFn().UTC()
	start := dayStartUTC(now).AddDate(0, 0, -(days - 1))

	out := Analytics{Days: days}
	dailyRows := make([]*analyticsDailyAccumulator, 0, days)
	dailyByDay := map[string]*analyticsDailyAccumulator{}
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		acc := &analyticsDailyAccumulator{
			row:      AnalyticsDailyRow{Day: day},
			sessions: map[string]struct{}{},
		}
		dailyRows = append(dailyRows, acc)
		dailyByDay[day] = acc
	}

	totalSessions := map[string]struct{}{}
	models := map[string]*analyticsModelAccumulator{}
	for _, entry := range t.readEntriesInRange(start, now) {
		day := dayStartUTC(entry.Timestamp).Format("2006-01-02")
		daily, ok := dailyByDay[day]
		if !ok {
			continue
		}
		applyAnalyticsEntry(&out.Totals, totalSessions, daily, models, entry)
	}

	out.Totals.Sessions = len(totalSessions)
	out.Totals.TotalTokens = out.Totals.InputTokens + out.Totals.OutputTokens
	if out.Totals.Sessions > 0 {
		out.Totals.AvgTokensPerSession = float64(out.Totals.TotalTokens) / float64(out.Totals.Sessions)
	}
	for _, acc := range dailyRows {
		acc.row.Sessions = len(acc.sessions)
		acc.row.TotalTokens = acc.row.InputTokens + acc.row.OutputTokens
		out.Daily = append(out.Daily, acc.row)
	}
	out.Models = materializeAnalyticsModels(models)
	out.Skills = t.analyticsSkillRows(start, now)
	return out, nil
}

func normalizeAnalyticsDays(days int) int {
	switch days {
	case 30, 90:
		return days
	default:
		return 7
	}
}

func applyAnalyticsEntry(
	totals *AnalyticsTotals,
	totalSessions map[string]struct{},
	daily *analyticsDailyAccumulator,
	models map[string]*analyticsModelAccumulator,
	entry Entry,
) {
	input := entry.InputTokens
	output := entry.OutputTokens
	total := input + output

	totals.Calls++
	totals.InputTokens += input
	totals.OutputTokens += output
	totals.TotalTokens += total
	totals.CostUSD += entry.EstimatedCostUSD
	if entry.SessionID != "" {
		totalSessions[entry.SessionID] = struct{}{}
		daily.sessions[entry.SessionID] = struct{}{}
	}

	daily.row.Calls++
	daily.row.InputTokens += input
	daily.row.OutputTokens += output
	daily.row.TotalTokens += total
	daily.row.CostUSD += entry.EstimatedCostUSD

	provider := firstNonEmptyTrimmed(entry.Provider, "(none)")
	model := firstNonEmptyTrimmed(entry.Model, "(none)")
	key := provider + "\x00" + model
	modelAcc, ok := models[key]
	if !ok {
		modelAcc = &analyticsModelAccumulator{
			row: AnalyticsModelRow{
				Provider: provider,
				Model:    model,
			},
			sessions: map[string]struct{}{},
		}
		models[key] = modelAcc
	}
	modelAcc.row.Calls++
	modelAcc.row.InputTokens += input
	modelAcc.row.OutputTokens += output
	modelAcc.row.TotalTokens += total
	modelAcc.row.CostUSD += entry.EstimatedCostUSD
	if entry.SessionID != "" {
		modelAcc.sessions[entry.SessionID] = struct{}{}
	}
}

func materializeAnalyticsModels(models map[string]*analyticsModelAccumulator) []AnalyticsModelRow {
	out := make([]AnalyticsModelRow, 0, len(models))
	for _, acc := range models {
		acc.row.Sessions = len(acc.sessions)
		acc.row.TotalTokens = acc.row.InputTokens + acc.row.OutputTokens
		out = append(out, acc.row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalTokens == out[j].TotalTokens {
			if out[i].Provider == out[j].Provider {
				return out[i].Model < out[j].Model
			}
			return out[i].Provider < out[j].Provider
		}
		return out[i].TotalTokens > out[j].TotalTokens
	})
	return out
}

func (t *Tracker) analyticsSkillRows(start, end time.Time) []AnalyticsSkillRow {
	rows := map[string]*analyticsSkillAccumulator{}
	for _, entry := range t.readSignalsInRange(start, end) {
		name := analyticsSkillName(entry)
		if name == "" {
			continue
		}
		count := entry.Count
		if count <= 0 {
			count = 1
		}
		key := strings.ToLower(name)
		row, ok := rows[key]
		if !ok {
			row = &analyticsSkillAccumulator{row: AnalyticsSkillRow{Name: name, Source: entry.Source}}
			rows[key] = row
		}
		row.row.Calls += count
		ts := entry.Timestamp.UTC().Format(time.RFC3339)
		if row.row.FirstAt == "" || ts < row.row.FirstAt {
			row.row.FirstAt = ts
		}
		if row.row.LastAt == "" || ts > row.row.LastAt {
			row.row.LastAt = ts
		}
	}
	out := make([]AnalyticsSkillRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Calls == out[j].Calls {
			return out[i].Name < out[j].Name
		}
		return out[i].Calls > out[j].Calls
	})
	return out
}

func analyticsSkillName(entry SignalEntry) string {
	if strings.TrimSpace(entry.Name) == "tool_call" {
		if tool := strings.TrimSpace(entry.Dimensions["tool"]); tool != "" {
			return tool
		}
		if skill := strings.TrimSpace(entry.Dimensions["skill"]); skill != "" {
			return skill
		}
	}
	return ""
}
