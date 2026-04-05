package tarsserver

import (
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/prompt"
)

func TestCollectPrefetchResult_WithResult(t *testing.T) {
	ch := make(chan prefetchResult, 1)
	ch <- prefetchResult{
		BuildResult: prompt.BuildResult{
			RelevantMemoryCount: 3,
			Prompt:              "test",
		},
	}
	close(ch)

	result := collectPrefetchResult(ch, time.Second)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.RelevantMemoryCount != 3 {
		t.Fatalf("expected 3 memories, got %d", result.RelevantMemoryCount)
	}
}

func TestCollectPrefetchResult_Timeout(t *testing.T) {
	ch := make(chan prefetchResult) // unbuffered, no sender
	result := collectPrefetchResult(ch, 50*time.Millisecond)
	if result != nil {
		t.Fatal("expected nil result on timeout")
	}
}

func TestCollectPrefetchResult_EmptyResult(t *testing.T) {
	ch := make(chan prefetchResult, 1)
	ch <- prefetchResult{
		BuildResult: prompt.BuildResult{RelevantMemoryCount: 0},
	}
	close(ch)

	result := collectPrefetchResult(ch, time.Second)
	if result != nil {
		t.Fatal("expected nil for zero memory count")
	}
}

func TestCollectPrefetchResult_NilChannel(t *testing.T) {
	result := collectPrefetchResult(nil, time.Second)
	if result != nil {
		t.Fatal("expected nil for nil channel")
	}
}

func TestCollectPrefetchResult_ClosedEmpty(t *testing.T) {
	ch := make(chan prefetchResult)
	close(ch)
	result := collectPrefetchResult(ch, time.Second)
	if result != nil {
		t.Fatal("expected nil for closed empty channel")
	}
}

func TestStartMemoryPrefetchForNextTurn_NilCache(t *testing.T) {
	// Should not panic with nil cache
	startMemoryPrefetchForNextTurn("/tmp/test", "query", "", memory.SemanticConfig{}, nil)
}

func TestStartMemoryPrefetchForNextTurn_EmptyMessage(t *testing.T) {
	cache := newMemoryCache(5 * time.Minute)
	// Should not launch goroutine for empty message
	startMemoryPrefetchForNextTurn("/tmp/test", "", "", memory.SemanticConfig{}, cache)
	time.Sleep(50 * time.Millisecond)
	_, ok := cache.Get("", "")
	if ok {
		t.Fatal("expected no cache entry for empty message")
	}
}
