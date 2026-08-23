package tarsserver

import (
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/prompt"
)

func TestMemoryCache_PutGet(t *testing.T) {
	cache := newMemoryCache(5 * time.Minute)
	cache.Put("coffee preference", "sess1", memoryRecallFromResult(prompt.BuildResult{
		Prompt:              "test prompt",
		RelevantSection:     "## Prior Context\n\n- likes coffee\n",
		RelevantMemoryCount: 3,
		RelevantTokens:      120,
	}))

	got, ok := cache.Get("coffee preference", "sess1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Count != 3 {
		t.Fatalf("expected 3 relevant memories, got %d", got.Count)
	}
	if got.Section != "## Prior Context\n\n- likes coffee\n" {
		t.Fatalf("expected cached recall section, got %q", got.Section)
	}
	if got.Tokens != 120 {
		t.Fatalf("expected 120 recall tokens, got %d", got.Tokens)
	}
}

// The cache deliberately stores only the recall payload: the assembled prompt
// depends on live session inputs the prefetch path never sees, so keeping it
// would let a cache hit ship a different prompt than a cache miss.
func TestMemoryCache_StoresRecallOnly(t *testing.T) {
	recall := memoryRecallFromResult(prompt.BuildResult{
		Prompt:          "assembled prompt that must not be reused",
		RelevantSection: "## Prior Context\n\n- fact\n",
		RelevantTokens:  10,
	})
	preset := recall.Preset()
	if preset.Section != "## Prior Context\n\n- fact\n" {
		t.Fatalf("unexpected preset section %q", preset.Section)
	}
	if preset.Tokens != 10 {
		t.Fatalf("unexpected preset tokens %d", preset.Tokens)
	}
}

func TestMemoryCache_Miss(t *testing.T) {
	cache := newMemoryCache(5 * time.Minute)
	_, ok := cache.Get("unknown query", "")
	if ok {
		t.Fatal("expected cache miss for unknown key")
	}
}

func TestMemoryCache_TTLExpiry(t *testing.T) {
	cache := newMemoryCache(50 * time.Millisecond)
	cache.Put("query", "", memoryRecall{Count: 1})

	// Should hit immediately
	_, ok := cache.Get("query", "")
	if !ok {
		t.Fatal("expected cache hit before TTL")
	}

	time.Sleep(60 * time.Millisecond)

	// Should miss after TTL
	_, ok = cache.Get("query", "")
	if ok {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestMemoryCache_EvictExpired(t *testing.T) {
	cache := newMemoryCache(50 * time.Millisecond)
	cache.Put("old", "", memoryRecall{Count: 1})
	time.Sleep(60 * time.Millisecond)
	cache.Put("new", "", memoryRecall{Count: 2})

	// evictExpired was called by Put, old entry should be gone
	cache.mu.RLock()
	count := len(cache.entries)
	cache.mu.RUnlock()
	if count != 1 {
		t.Fatalf("expected 1 entry after eviction, got %d", count)
	}
}

func TestMemoryCache_NilSafe(t *testing.T) {
	var cache *memoryCache
	_, ok := cache.Get("query", "")
	if ok {
		t.Fatal("expected miss on nil cache")
	}
	// Should not panic
	cache.Put("query", "", memoryRecall{})
	cache.evictExpired()
}

func TestMemoryCache_CaseInsensitiveQuery(t *testing.T) {
	cache := newMemoryCache(5 * time.Minute)
	cache.Put("Coffee Preference", "", memoryRecall{Count: 1})

	_, ok := cache.Get("coffee preference", "")
	if !ok {
		t.Fatal("expected case-insensitive cache hit")
	}
}
