package tarsserver

import (
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/memory"
)

func TestStartMemoryPrefetchForNextTurn_NilCache(t *testing.T) {
	// Should not panic with nil cache
	startMemoryPrefetchForNextTurn("/tmp/test", "query", "", memory.SemanticConfig{}, nil, "")
}

func TestStartMemoryPrefetchForNextTurn_EmptyMessage(t *testing.T) {
	cache := newMemoryCache(5 * time.Minute)
	// Should not launch goroutine for empty message
	startMemoryPrefetchForNextTurn("/tmp/test", "", "", memory.SemanticConfig{}, cache, "")
	time.Sleep(50 * time.Millisecond)
	_, ok := cache.Get("", "")
	if ok {
		t.Fatal("expected no cache entry for empty message")
	}
}
