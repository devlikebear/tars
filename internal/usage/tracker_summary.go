package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

func (t *Tracker) Summary(period, groupBy string) (Summary, error) {
	if t == nil {
		return Summary{}, fmt.Errorf("usage tracker is nil")
	}

	now := t.nowFn().UTC()
	start, normalizedPeriod, err := periodRange(period, now)
	if err != nil {
		return Summary{}, err
	}

	group := normalizeGroupBy(groupBy)
	out := Summary{
		Period:  normalizedPeriod,
		GroupBy: group,
	}
	rows := map[string]*SummaryRow{}
	for _, entry := range t.readEntriesInRange(start, now) {
		applySummaryEntry(&out, rows, entry, group)
	}
	out.Rows = materializeSummaryRows(rows)
	return out, nil
}

func applySummaryEntry(out *Summary, rows map[string]*SummaryRow, entry Entry, group string) {
	key := summaryKey(entry, group)
	row, ok := rows[key]
	if !ok {
		row = &SummaryRow{Key: key}
		rows[key] = row
	}

	row.Calls++
	row.CostUSD += entry.EstimatedCostUSD
	row.InputTokens += entry.InputTokens
	row.OutputTokens += entry.OutputTokens
	row.CachedTokens += entry.CachedTokens
	row.CacheReadTokens += entry.CacheReadTokens
	row.CacheWriteTokens += entry.CacheWriteTokens

	out.TotalCalls++
	out.TotalCostUSD += entry.EstimatedCostUSD
	out.TotalInput += entry.InputTokens
	out.TotalOutput += entry.OutputTokens
	out.TotalCached += entry.CachedTokens
	out.TotalCacheRead += entry.CacheReadTokens
	out.TotalCacheWrite += entry.CacheWriteTokens
}

func materializeSummaryRows(rows map[string]*SummaryRow) []SummaryRow {
	out := make([]SummaryRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CostUSD == out[j].CostUSD {
			return out[i].Key < out[j].Key
		}
		return out[i].CostUSD > out[j].CostUSD
	})
	return out
}

func (t *Tracker) readEntriesInRange(start, end time.Time) []Entry {
	entries := []Entry{}
	if end.Before(start) {
		return entries
	}

	for day := dayStartUTC(start); !day.After(dayStartUTC(end)); day = day.AddDate(0, 0, 1) {
		for _, item := range readUsageFile(t.usagePathFor(day)) {
			ts := item.Timestamp.UTC()
			if ts.Before(start) || ts.After(end) {
				continue
			}
			entries = append(entries, item)
		}
	}
	return entries
}

func readUsageFile(path string) []Entry {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	entries := []Entry{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item Entry
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		entries = append(entries, item)
	}
	return entries
}

func periodRange(raw string, now time.Time) (time.Time, string, error) {
	mode := strings.TrimSpace(strings.ToLower(raw))
	if mode == "" {
		mode = "today"
	}
	switch mode {
	case "today":
		return dayStartUTC(now), mode, nil
	case "week":
		return dayStartUTC(now).AddDate(0, 0, -6), mode, nil
	case "month":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC), mode, nil
	default:
		return time.Time{}, "", fmt.Errorf("period must be one of: today|week|month")
	}
}

func normalizeGroupBy(raw string) string {
	v := strings.TrimSpace(strings.ToLower(raw))
	switch v {
	case "provider", "model", "source", "project":
		return v
	default:
		return "provider"
	}
}

func summaryKey(entry Entry, groupBy string) string {
	switch groupBy {
	case "provider":
		return firstNonEmptyTrimmed(entry.Provider, "(none)")
	case "model":
		return firstNonEmptyTrimmed(entry.Model, "(none)")
	case "source":
		return firstNonEmptyTrimmed(entry.Source, "(none)")
	case "project":
		return firstNonEmptyTrimmed(entry.ProjectID, "(none)")
	default:
		return firstNonEmptyTrimmed(entry.Provider, "(none)")
	}
}

func dayStartUTC(v time.Time) time.Time {
	t := v.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func firstNonEmptyTrimmed(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
