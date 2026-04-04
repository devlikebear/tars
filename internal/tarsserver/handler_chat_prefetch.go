package tarsserver

import (
	"context"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/prompt"
)

const defaultPrefetchTimeout = 2 * time.Second

type prefetchResult struct {
	BuildResult prompt.BuildResult
	Err         error
}

// startMemoryPrefetch launches a goroutine that searches memory asynchronously.
// Results are sent to the returned channel when ready.
func startMemoryPrefetch(ctx context.Context, state chatRunState, deps chatHandlerDeps) <-chan prefetchResult {
	ch := make(chan prefetchResult, 1)
	query := strings.TrimSpace(state.history[len(state.history)-1].Content)
	if len(state.history) == 0 || query == "" {
		close(ch)
		return ch
	}

	go func() {
		defer close(ch)
		memService := buildSemanticMemoryService(state.requestWorkspaceDir, deps.tooling.MemorySemanticConfig)
		result := prompt.BuildResultFor(prompt.BuildOptions{
			WorkspaceDir:        state.requestWorkspaceDir,
			Query:               query,
			ProjectID:           state.projectID,
			SessionID:           state.sessionID,
			MemorySearcher:      memService,
			ForceRelevantMemory: shouldForceMemoryToolCall(query),
		})
		select {
		case ch <- prefetchResult{BuildResult: result}:
		case <-ctx.Done():
		}
	}()
	return ch
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
	projectID string,
	sessionID string,
	semanticCfg memory.SemanticConfig,
	cache *memoryCache,
) {
	if cache == nil || strings.TrimSpace(userMessage) == "" {
		return
	}
	go func() {
		memService := buildSemanticMemoryService(workspaceDir, semanticCfg)
		result := prompt.BuildResultFor(prompt.BuildOptions{
			WorkspaceDir:        workspaceDir,
			Query:               userMessage,
			ProjectID:           projectID,
			SessionID:           sessionID,
			MemorySearcher:      memService,
			ForceRelevantMemory: shouldForceMemoryToolCall(userMessage),
		})
		if result.RelevantMemoryCount > 0 {
			cache.Put(userMessage, projectID, sessionID, result)
		}
	}()
}
