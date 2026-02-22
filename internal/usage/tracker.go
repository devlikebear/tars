package usage

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tarsncase/internal/llm"
)

type ModelPrice struct {
	InputPer1MUSD      float64 `json:"input_per_1m_usd"`
	OutputPer1MUSD     float64 `json:"output_per_1m_usd"`
	CacheReadPer1MUSD  float64 `json:"cache_read_per_1m_usd,omitempty"`
	CacheWritePer1MUSD float64 `json:"cache_write_per_1m_usd,omitempty"`
}

type Limits struct {
	DailyUSD   float64 `json:"daily_usd"`
	WeeklyUSD  float64 `json:"weekly_usd"`
	MonthlyUSD float64 `json:"monthly_usd"`
	Mode       string  `json:"mode"`
}

type Entry struct {
	Timestamp        time.Time `json:"timestamp"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	InputTokens      int       `json:"input_tokens"`
	OutputTokens     int       `json:"output_tokens"`
	CachedTokens     int       `json:"cached_tokens,omitempty"`
	CacheReadTokens  int       `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int       `json:"cache_write_tokens,omitempty"`
	EstimatedCostUSD float64   `json:"estimated_cost_usd"`
	Source           string    `json:"source"`
	SessionID        string    `json:"session_id,omitempty"`
	ProjectID        string    `json:"project_id,omitempty"`
	RunID            string    `json:"run_id,omitempty"`
	PricingKnown     bool      `json:"pricing_known"`
}

type Summary struct {
	Period          string       `json:"period"`
	GroupBy         string       `json:"group_by"`
	TotalCalls      int          `json:"total_calls"`
	TotalCostUSD    float64      `json:"total_cost_usd"`
	TotalInput      int          `json:"total_input_tokens"`
	TotalOutput     int          `json:"total_output_tokens"`
	TotalCached     int          `json:"total_cached_tokens"`
	TotalCacheRead  int          `json:"total_cache_read_tokens"`
	TotalCacheWrite int          `json:"total_cache_write_tokens"`
	Rows            []SummaryRow `json:"rows"`
}

type SummaryRow struct {
	Key              string  `json:"key"`
	Calls            int     `json:"calls"`
	CostUSD          float64 `json:"cost_usd"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CachedTokens     int     `json:"cached_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
}

type TrackerOptions struct {
	Now            func() time.Time
	InitialLimits  Limits
	PriceOverrides map[string]ModelPrice
}

type Tracker struct {
	mu         sync.Mutex
	workspace  string
	usageDir   string
	limitsPath string
	nowFn      func() time.Time
	limits     Limits
	priceByKey map[string]ModelPrice
}

type LimitStatus struct {
	Exceeded bool
	Mode     string
	Period   string
	SpentUSD float64
	LimitUSD float64
}

type callMetaKey struct{}

type CallMeta struct {
	Source    string
	SessionID string
	ProjectID string
	RunID     string
}

func WithCallMeta(ctx context.Context, meta CallMeta) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	normalized := normalizeCallMeta(meta)
	return context.WithValue(ctx, callMetaKey{}, normalized)
}

func CallMetaFromContext(ctx context.Context) CallMeta {
	if ctx == nil {
		return CallMeta{Source: "chat"}
	}
	if value, ok := ctx.Value(callMetaKey{}).(CallMeta); ok {
		return normalizeCallMeta(value)
	}
	return CallMeta{Source: "chat"}
}

func normalizeCallMeta(meta CallMeta) CallMeta {
	out := CallMeta{
		Source:    strings.TrimSpace(strings.ToLower(meta.Source)),
		SessionID: strings.TrimSpace(meta.SessionID),
		ProjectID: strings.TrimSpace(meta.ProjectID),
		RunID:     strings.TrimSpace(meta.RunID),
	}
	switch out.Source {
	case "chat", "cron", "heartbeat", "agent_run":
	default:
		out.Source = "chat"
	}
	return out
}

func NewTracker(workspaceDir string, opts TrackerOptions) (*Tracker, error) {
	root := strings.TrimSpace(workspaceDir)
	if root == "" {
		return nil, fmt.Errorf("workspace dir is required")
	}
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	usageDir := filepath.Join(root, "usage")
	if err := os.MkdirAll(usageDir, 0o755); err != nil {
		return nil, err
	}
	t := &Tracker{
		workspace:  root,
		usageDir:   usageDir,
		limitsPath: filepath.Join(usageDir, "limits.json"),
		nowFn:      nowFn,
		limits:     normalizeLimits(opts.InitialLimits),
		priceByKey: defaultPriceTable(),
	}
	for key, price := range opts.PriceOverrides {
		k := strings.TrimSpace(strings.ToLower(key))
		if k == "" {
			continue
		}
		t.priceByKey[k] = sanitizePrice(price)
	}
	_ = t.loadPersistedLimits()
	return t, nil
}

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

func (t *Tracker) Limits() Limits {
	if t == nil {
		return normalizeLimits(Limits{})
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.limits
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

func (t *Tracker) usagePathFor(ts time.Time) string {
	return filepath.Join(t.usageDir, ts.UTC().Format("2006-01-02")+".jsonl")
}

func (t *Tracker) Record(entry Entry) error {
	if t == nil {
		return fmt.Errorf("usage tracker is nil")
	}
	e := entry
	if e.Timestamp.IsZero() {
		e.Timestamp = t.nowFn().UTC()
	}
	e.Provider = strings.TrimSpace(strings.ToLower(e.Provider))
	e.Model = strings.TrimSpace(e.Model)
	e.Source = normalizeCallMeta(CallMeta{Source: e.Source}).Source
	e.SessionID = strings.TrimSpace(e.SessionID)
	e.ProjectID = strings.TrimSpace(e.ProjectID)
	e.RunID = strings.TrimSpace(e.RunID)

	path := t.usagePathFor(e.Timestamp)
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

func (t *Tracker) EstimateCost(provider, model string, u llm.Usage) (float64, bool) {
	if t == nil {
		return 0, false
	}
	price, ok := t.resolvePrice(provider, model)
	if !ok {
		return 0, false
	}
	input := u.InputTokens
	output := u.OutputTokens
	cached := u.CachedTokens
	cacheRead := u.CacheReadTokens
	cacheWrite := u.CacheWriteTokens
	if input < 0 {
		input = 0
	}
	if output < 0 {
		output = 0
	}
	if cached < 0 {
		cached = 0
	}
	if cacheRead < 0 {
		cacheRead = 0
	}
	if cacheWrite < 0 {
		cacheWrite = 0
	}

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
	if cacheRead > 0 {
		rate := price.CacheReadPer1MUSD
		if rate <= 0 {
			rate = price.InputPer1MUSD
		}
		cost += float64(cacheRead) * rate / 1_000_000.0
	} else if cached > 0 {
		rate := price.CacheReadPer1MUSD
		if rate <= 0 {
			rate = price.InputPer1MUSD
		}
		cost += float64(cached) * rate / 1_000_000.0
	}
	if cacheWrite > 0 {
		rate := price.CacheWritePer1MUSD
		if rate <= 0 {
			rate = price.InputPer1MUSD
		}
		cost += float64(cacheWrite) * rate / 1_000_000.0
	}
	if cost < 0 {
		cost = 0
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
	rows := map[string]*SummaryRow{}
	out := Summary{
		Period:  normalizedPeriod,
		GroupBy: group,
	}
	for _, entry := range t.readEntriesInRange(start, now) {
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
	out.Rows = make([]SummaryRow, 0, len(rows))
	for _, row := range rows {
		out.Rows = append(out.Rows, *row)
	}
	sort.Slice(out.Rows, func(i, j int) bool {
		if out.Rows[i].CostUSD == out.Rows[j].CostUSD {
			return out.Rows[i].Key < out.Rows[j].Key
		}
		return out.Rows[i].CostUSD > out.Rows[j].CostUSD
	})
	return out, nil
}

func (t *Tracker) readEntriesInRange(start, end time.Time) []Entry {
	entries := []Entry{}
	if end.Before(start) {
		return entries
	}
	for day := dayStartUTC(start); !day.After(dayStartUTC(end)); day = day.AddDate(0, 0, 1) {
		path := t.usagePathFor(day)
		file, err := os.Open(path)
		if err != nil {
			continue
		}
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
			ts := item.Timestamp.UTC()
			if ts.Before(start) || ts.After(end) {
				continue
			}
			entries = append(entries, item)
		}
		_ = file.Close()
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

func (t *Tracker) CheckLimitStatus() (LimitStatus, error) {
	if t == nil {
		return LimitStatus{}, fmt.Errorf("usage tracker is nil")
	}
	limits := t.Limits()
	check := []struct {
		period string
		limit  float64
	}{
		{period: "today", limit: limits.DailyUSD},
		{period: "week", limit: limits.WeeklyUSD},
		{period: "month", limit: limits.MonthlyUSD},
	}
	for _, item := range check {
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
