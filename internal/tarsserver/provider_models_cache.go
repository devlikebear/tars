package tarsserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/config"
)

const (
	providerModelsCacheFile = "provider_models_cache.json"
	providerModelsCacheTTL  = 24 * time.Hour
)

type providerModelsCacheEntry struct {
	Provider  string   `json:"provider"`
	BaseURL   string   `json:"base_url"`
	AuthMode  string   `json:"auth_mode"`
	Models    []string `json:"models"`
	FetchedAt string   `json:"fetched_at"`
}

type providerModelsCache struct {
	mu      sync.Mutex
	path    string
	ttl     time.Duration
	nowFn   func() time.Time
	entries map[string]providerModelsCacheEntry
}

func providerModelsCachePath(cfg config.Config) string {
	return filepath.Join(strings.TrimSpace(cfg.GatewayPersistenceDir), providerModelsCacheFile)
}

func newProviderModelsCache(path string, ttl time.Duration, nowFn func() time.Time) (*providerModelsCache, error) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return nil, fmt.Errorf("provider models cache path is required")
	}
	if ttl <= 0 {
		ttl = providerModelsCacheTTL
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	cache := &providerModelsCache{
		path:    trimmedPath,
		ttl:     ttl,
		nowFn:   nowFn,
		entries: map[string]providerModelsCacheEntry{},
	}
	if err := cache.load(); err != nil {
		return nil, err
	}
	return cache, nil
}

func (c *providerModelsCache) get(provider, baseURL, authMode string) (providerModelsCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := providerModelsCacheKey(provider, baseURL, authMode)
	entry, ok := c.entries[key]
	return entry, ok
}

func (c *providerModelsCache) isFresh(entry providerModelsCacheEntry, now time.Time) bool {
	fetchedAt, ok := parseRFC3339(entry.FetchedAt)
	if !ok {
		return false
	}
	return now.UTC().Before(fetchedAt.Add(c.ttl))
}

func (c *providerModelsCache) put(provider, baseURL, authMode string, models []string, fetchedAt time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := providerModelsCacheEntry{
		Provider:  normalizeProviderValue(provider),
		BaseURL:   normalizeBaseURL(baseURL),
		AuthMode:  normalizeAuthMode(authMode),
		Models:    append([]string(nil), models...),
		FetchedAt: fetchedAt.UTC().Format(time.RFC3339),
	}
	c.entries[providerModelsCacheKey(provider, baseURL, authMode)] = entry
	return c.persistLocked()
}

func (c *providerModelsCache) load() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return fmt.Errorf("create provider models cache directory: %w", err)
	}
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read provider models cache: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil
	}

	var payload struct {
		Entries []providerModelsCacheEntry `json:"entries"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("decode provider models cache: %w", err)
	}
	for _, entry := range payload.Entries {
		key := providerModelsCacheKey(entry.Provider, entry.BaseURL, entry.AuthMode)
		c.entries[key] = providerModelsCacheEntry{
			Provider:  normalizeProviderValue(entry.Provider),
			BaseURL:   normalizeBaseURL(entry.BaseURL),
			AuthMode:  normalizeAuthMode(entry.AuthMode),
			Models:    append([]string(nil), entry.Models...),
			FetchedAt: strings.TrimSpace(entry.FetchedAt),
		}
	}
	return nil
}

func (c *providerModelsCache) persistLocked() error {
	entries := make([]providerModelsCacheEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		entries = append(entries, providerModelsCacheEntry{
			Provider:  normalizeProviderValue(entry.Provider),
			BaseURL:   normalizeBaseURL(entry.BaseURL),
			AuthMode:  normalizeAuthMode(entry.AuthMode),
			Models:    append([]string(nil), entry.Models...),
			FetchedAt: strings.TrimSpace(entry.FetchedAt),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		left := providerModelsCacheKey(entries[i].Provider, entries[i].BaseURL, entries[i].AuthMode)
		right := providerModelsCacheKey(entries[j].Provider, entries[j].BaseURL, entries[j].AuthMode)
		return left < right
	})
	payload := struct {
		Entries []providerModelsCacheEntry `json:"entries"`
	}{
		Entries: entries,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode provider models cache: %w", err)
	}
	tmpPath := c.path + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write provider models cache temp: %w", err)
	}
	if err := os.Rename(tmpPath, c.path); err != nil {
		return fmt.Errorf("replace provider models cache: %w", err)
	}
	return nil
}

func providerModelsCacheKey(provider, baseURL, authMode string) string {
	return normalizeProviderValue(provider) + "|" + normalizeBaseURL(baseURL) + "|" + normalizeAuthMode(authMode)
}

func normalizeProviderValue(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func normalizeAuthMode(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func normalizeBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func parseRFC3339(raw string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}
