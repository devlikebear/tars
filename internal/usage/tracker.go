package usage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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

func (t *Tracker) Limits() Limits {
	if t == nil {
		return normalizeLimits(Limits{})
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.limits
}
