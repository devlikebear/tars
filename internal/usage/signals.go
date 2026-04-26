package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type SignalEntry struct {
	Timestamp  time.Time         `json:"timestamp"`
	Name       string            `json:"name"`
	Count      int               `json:"count"`
	Source     string            `json:"source,omitempty"`
	SessionID  string            `json:"session_id,omitempty"`
	RunID      string            `json:"run_id,omitempty"`
	Dimensions map[string]string `json:"dimensions,omitempty"`
}

type SignalSummary struct {
	Period     string             `json:"period"`
	TotalCount int                `json:"total_count"`
	Rows       []SignalSummaryRow `json:"rows"`
}

type SignalSummaryRow struct {
	Name       string            `json:"name"`
	Source     string            `json:"source,omitempty"`
	Count      int               `json:"count"`
	Dimensions map[string]string `json:"dimensions,omitempty"`
	FirstAt    string            `json:"first_at,omitempty"`
	LastAt     string            `json:"last_at,omitempty"`
}

func (t *Tracker) signalPathFor(ts time.Time) string {
	return filepath.Join(t.usageDir, "signals-"+ts.UTC().Format("2006-01-02")+".jsonl")
}

func (t *Tracker) RecordSignal(entry SignalEntry) error {
	if t == nil {
		return fmt.Errorf("usage tracker is nil")
	}
	e := normalizeSignalEntry(entry, t.nowFn)
	if e.Name == "" {
		return nil
	}
	path := t.signalPathFor(e.Timestamp)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(payload, '\n'))
	return err
}

func (t *Tracker) Signals(period string) (SignalSummary, error) {
	if t == nil {
		return SignalSummary{}, fmt.Errorf("usage tracker is nil")
	}
	now := t.nowFn().UTC()
	start, normalizedPeriod, err := periodRange(period, now)
	if err != nil {
		return SignalSummary{}, err
	}
	out := SignalSummary{Period: normalizedPeriod}
	rows := map[string]*signalSummaryAccumulator{}
	for _, entry := range t.readSignalsInRange(start, now) {
		count := entry.Count
		if count <= 0 {
			count = 1
		}
		key := signalSummaryKey(entry)
		row, ok := rows[key]
		if !ok {
			row = &signalSummaryAccumulator{
				Name:       entry.Name,
				Source:     entry.Source,
				Dimensions: cloneSignalDimensions(entry.Dimensions),
				First:      entry.Timestamp.UTC(),
				Last:       entry.Timestamp.UTC(),
			}
			rows[key] = row
		}
		row.Count += count
		if entry.Timestamp.Before(row.First) {
			row.First = entry.Timestamp.UTC()
		}
		if entry.Timestamp.After(row.Last) {
			row.Last = entry.Timestamp.UTC()
		}
		out.TotalCount += count
	}
	out.Rows = materializeSignalRows(rows)
	return out, nil
}

type signalSummaryAccumulator struct {
	Name       string
	Source     string
	Count      int
	Dimensions map[string]string
	First      time.Time
	Last       time.Time
}

func materializeSignalRows(rows map[string]*signalSummaryAccumulator) []SignalSummaryRow {
	out := make([]SignalSummaryRow, 0, len(rows))
	for _, row := range rows {
		item := SignalSummaryRow{
			Name:       row.Name,
			Source:     row.Source,
			Count:      row.Count,
			Dimensions: cloneSignalDimensions(row.Dimensions),
		}
		if !row.First.IsZero() {
			item.FirstAt = row.First.UTC().Format(time.RFC3339)
		}
		if !row.Last.IsZero() {
			item.LastAt = row.Last.UTC().Format(time.RFC3339)
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return signalRowSortKey(out[i]) < signalRowSortKey(out[j])
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func (t *Tracker) readSignalsInRange(start, end time.Time) []SignalEntry {
	entries := []SignalEntry{}
	if end.Before(start) {
		return entries
	}
	for day := dayStartUTC(start); !day.After(dayStartUTC(end)); day = day.AddDate(0, 0, 1) {
		for _, item := range readSignalFile(t.signalPathFor(day)) {
			ts := item.Timestamp.UTC()
			if ts.Before(start) || ts.After(end) {
				continue
			}
			entries = append(entries, item)
		}
	}
	return entries
}

func readSignalFile(path string) []SignalEntry {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var entries []SignalEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item SignalEntry
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		entries = append(entries, normalizeSignalEntry(item, time.Now))
	}
	return entries
}

func normalizeSignalEntry(entry SignalEntry, nowFn func() time.Time) SignalEntry {
	e := entry
	if nowFn == nil {
		nowFn = time.Now
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = nowFn().UTC()
	} else {
		e.Timestamp = e.Timestamp.UTC()
	}
	e.Name = normalizeSignalToken(e.Name)
	e.Source = normalizeCallMeta(CallMeta{Source: e.Source}).Source
	e.SessionID = strings.TrimSpace(e.SessionID)
	e.RunID = strings.TrimSpace(e.RunID)
	if e.Count <= 0 {
		e.Count = 1
	}
	e.Dimensions = normalizeSignalDimensions(e.Dimensions)
	return e
}

func normalizeSignalDimensions(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := map[string]string{}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		k := normalizeSignalToken(key)
		v := strings.TrimSpace(values[key])
		if k == "" || v == "" {
			continue
		}
		if len(v) > 96 {
			v = v[:96]
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeSignalToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func signalSummaryKey(entry SignalEntry) string {
	return entry.Name + "|" + entry.Source + "|" + dimensionsKey(entry.Dimensions)
}

func signalRowSortKey(row SignalSummaryRow) string {
	return row.Name + "|" + row.Source + "|" + dimensionsKey(row.Dimensions)
}

func dimensionsKey(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values[key])
	}
	return strings.Join(parts, ",")
}

func cloneSignalDimensions(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}
