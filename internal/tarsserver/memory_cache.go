package tarsserver

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/prompt"
)

const defaultMemoryCacheTTL = 5 * time.Minute

type memoryCacheEntry struct {
	Result    prompt.BuildResult
	CreatedAt time.Time
}

type memoryCache struct {
	mu      sync.RWMutex
	entries map[string]memoryCacheEntry
	ttl     time.Duration
}

func newMemoryCache(ttl time.Duration) *memoryCache {
	if ttl <= 0 {
		ttl = defaultMemoryCacheTTL
	}
	return &memoryCache{
		entries: make(map[string]memoryCacheEntry),
		ttl:     ttl,
	}
}

func (c *memoryCache) Get(query, projectID, sessionID string) (prompt.BuildResult, bool) {
	if c == nil {
		return prompt.BuildResult{}, false
	}
	key := memoryCacheKey(query, projectID, sessionID)
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return prompt.BuildResult{}, false
	}
	if time.Since(entry.CreatedAt) > c.ttl {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return prompt.BuildResult{}, false
	}
	return entry.Result, true
}

func (c *memoryCache) Put(query, projectID, sessionID string, result prompt.BuildResult) {
	if c == nil {
		return
	}
	key := memoryCacheKey(query, projectID, sessionID)
	c.mu.Lock()
	c.entries[key] = memoryCacheEntry{
		Result:    result,
		CreatedAt: time.Now(),
	}
	c.mu.Unlock()
	c.evictExpired()
}

func (c *memoryCache) evictExpired() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for key, entry := range c.entries {
		if now.Sub(entry.CreatedAt) > c.ttl {
			delete(c.entries, key)
		}
	}
}

func memoryCacheKey(query, projectID, sessionID string) string {
	raw := strings.ToLower(strings.TrimSpace(query)) + "|" +
		strings.TrimSpace(projectID) + "|" +
		strings.TrimSpace(sessionID)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:16])
}
