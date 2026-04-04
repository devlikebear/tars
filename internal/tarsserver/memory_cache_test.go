package tarsserver

import (
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/prompt"
)

func TestMemoryCache_PutGet(t *testing.T) {
	cache := newMemoryCache(5 * time.Minute)
	result := prompt.BuildResult{
		Prompt:              "test prompt",
		RelevantMemoryCount: 3,
		RelevantTokens:      120,
	}
	cache.Put("coffee preference", "proj1", "sess1", result)

	got, ok := cache.Get("coffee preference", "proj1", "sess1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.RelevantMemoryCount != 3 {
		t.Fatalf("expected 3 relevant memories, got %d", got.RelevantMemoryCount)
	}
	if got.Prompt != "test prompt" {
		t.Fatalf("expected cached prompt, got %q", got.Prompt)
	}
}

func TestMemoryCache_Miss(t *testing.T) {
	cache := newMemoryCache(5 * time.Minute)
	_, ok := cache.Get("unknown query", "", "")
	if ok {
		t.Fatal("expected cache miss for unknown key")
	}
}

func TestMemoryCache_TTLExpiry(t *testing.T) {
	cache := newMemoryCache(50 * time.Millisecond)
	cache.Put("query", "", "", prompt.BuildResult{RelevantMemoryCount: 1})

	// Should hit immediately
	_, ok := cache.Get("query", "", "")
	if !ok {
		t.Fatal("expected cache hit before TTL")
	}

	time.Sleep(60 * time.Millisecond)

	// Should miss after TTL
	_, ok = cache.Get("query", "", "")
	if ok {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestMemoryCache_EvictExpired(t *testing.T) {
	cache := newMemoryCache(50 * time.Millisecond)
	cache.Put("old", "", "", prompt.BuildResult{RelevantMemoryCount: 1})
	time.Sleep(60 * time.Millisecond)
	cache.Put("new", "", "", prompt.BuildResult{RelevantMemoryCount: 2})

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
	_, ok := cache.Get("query", "", "")
	if ok {
		t.Fatal("expected miss on nil cache")
	}
	// Should not panic
	cache.Put("query", "", "", prompt.BuildResult{})
	cache.evictExpired()
}

func TestMemoryCache_CaseInsensitiveQuery(t *testing.T) {
	cache := newMemoryCache(5 * time.Minute)
	cache.Put("Coffee Preference", "proj1", "", prompt.BuildResult{RelevantMemoryCount: 1})

	_, ok := cache.Get("coffee preference", "proj1", "")
	if !ok {
		t.Fatal("expected case-insensitive cache hit")
	}
}
