package tarsserver

import (
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/prompt"
)

type prefetchResult struct {
	BuildResult prompt.BuildResult
	Err         error
}

// collectPrefetchResult waits for prefetch to complete within the given timeout.
// Returns nil BuildResult if timeout or channel closed without result.
func collectPrefetchResult(ch <-chan prefetchResult, timeout time.Duration) *prompt.BuildResult {
	if ch == nil {
		return nil
	}
	select {
	case result, ok := <-ch:
		if !ok || result.Err != nil {
			return nil
		}
		if result.BuildResult.RelevantMemoryCount == 0 {
			return nil
		}
		return &result.BuildResult
	case <-time.After(timeout):
		return nil
	}
}

// startMemoryPrefetchForNextTurn launches an async prefetch and caches the result.
// This is a fire-and-forget operation for warming the cache for the next turn.
func startMemoryPrefetchForNextTurn(
	workspaceDir string,
	userMessage string,
	sessionID string,
	semanticCfg memory.SemanticConfig,
	cache *memoryCache,
	planClarifyMode string,
) {
	if cache == nil || strings.TrimSpace(userMessage) == "" {
		return
	}
	go func() {
		memService := buildSemanticMemoryService(workspaceDir, semanticCfg)
		result := prompt.BuildResultFor(prompt.BuildOptions{
			WorkspaceDir:        workspaceDir,
			Query:               userMessage,
			SessionID:           sessionID,
			PlanClarifyMode:     planClarifyMode,
			MemorySearcher:      memService,
			ForceRelevantMemory: shouldForceMemoryToolCall(userMessage),
		})
		if result.RelevantMemoryCount > 0 {
			// Only the recall payload is cached — the prompt this goroutine
			// assembled is discarded. It was built without the session's work
			// dirs or current dir, so storing it would hand the next turn a
			// prompt missing whole sections; the live path rebuilds instead.
			cache.Put(userMessage, sessionID, memoryRecallFromResult(result))
		}
	}()
}
