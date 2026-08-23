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

// memoryRecall is the only part of a prompt build worth caching across turns:
// the semantic search behind "## Prior Context". The assembled prompt itself is
// deliberately NOT cached — its static region depends on live inputs (session
// work dirs, current dir, plan-clarify mode, workspace bootstrap files) that
// the prefetch path does not carry, so replaying a stored prompt used to drop
// whole sections and hand the provider a different prefix on a cache hit than
// on a cache miss. Rebuilding from live options with this payload injected
// makes hit and miss byte-identical by construction.
type memoryRecall struct {
	Section string
	Items   []prompt.RelevantMemoryItem
	Tokens  int
	Count   int
	Budget  int
}

// Preset converts the cached recall into the builder's injection shape.
func (r memoryRecall) Preset() *prompt.PresetRelevantMemory {
	return &prompt.PresetRelevantMemory{
		Section: r.Section,
		Items:   r.Items,
		Tokens:  r.Tokens,
	}
}

// memoryRecallFromResult extracts the cacheable recall payload from a build.
func memoryRecallFromResult(result prompt.BuildResult) memoryRecall {
	return memoryRecall{
		Section: result.RelevantSection,
		Items:   append([]prompt.RelevantMemoryItem(nil), result.RelevantMemoryItems...),
		Tokens:  result.RelevantTokens,
		Count:   result.RelevantMemoryCount,
		Budget:  result.RelevantBudgetTokens,
	}
}

type memoryCacheEntry struct {
	Recall    memoryRecall
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

func (c *memoryCache) Get(query, sessionID string) (memoryRecall, bool) {
	if c == nil {
		return memoryRecall{}, false
	}
	key := memoryCacheKey(query, sessionID)
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return memoryRecall{}, false
	}
	if time.Since(entry.CreatedAt) > c.ttl {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return memoryRecall{}, false
	}
	return entry.Recall, true
}

func (c *memoryCache) Put(query, sessionID string, recall memoryRecall) {
	if c == nil {
		return
	}
	key := memoryCacheKey(query, sessionID)
	c.mu.Lock()
	c.entries[key] = memoryCacheEntry{
		Recall:    recall,
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

func memoryCacheKey(query, sessionID string) string {
	raw := strings.ToLower(strings.TrimSpace(query)) + "|" +
		strings.TrimSpace(sessionID)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:16])
}
