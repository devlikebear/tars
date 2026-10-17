package tarsserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/prompt"
	"github.com/devlikebear/tars/internal/session"
)

func writePromptCacheWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"IDENTITY.md": "# IDENTITY.md\n\nName: TARS",
		"USER.md":     "# USER.md\n\nName: Alice",
		"MEMORY.md":   "User prefers black coffee.\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

// A cache hit must assemble the same prompt as a cache miss. Caching the
// assembled prompt instead of the recall payload broke this: the prefetch
// goroutine builds without the session's work dirs, so replaying its prompt
// dropped "## Working Directories" and handed the provider a different prefix.
func TestPrepareChatContext_CacheHitRebuildsLiveStaticRegion(t *testing.T) {
	root := writePromptCacheWorkspace(t)
	query := "what coffee do i prefer?"
	workDirs := []string{filepath.Join(root, "artifacts")}
	currentDir := workDirs[0]

	// Simulate the prefetch goroutine, which knows nothing about work dirs.
	prefetched := prompt.BuildResultFor(prompt.BuildOptions{
		WorkspaceDir: root,
		Query:        query,
	})
	if prefetched.RelevantMemoryCount == 0 {
		t.Fatal("expected the fixture workspace to produce recall")
	}
	if strings.Contains(prefetched.Prompt, "## Working Directories") {
		t.Fatal("fixture invalid: prefetch prompt should lack the work-dir section")
	}

	cache := newMemoryCache(time.Minute)
	cache.Put(query, "sess1", memoryRecallFromResult(prefetched))

	hit, err := prepareChatContextDetailsWithCache(
		root, "sess1", query, extensions.Snapshot{}, nil,
		cache, memory.SemanticConfig{}, workDirs, currentDir, "smart",
	)
	if err != nil {
		t.Fatalf("prepare on cache hit: %v", err)
	}
	if !strings.Contains(hit.SystemPrompt, "## Working Directories") {
		t.Fatalf("cache hit dropped the work-dir section: %q", hit.SystemPrompt)
	}
	if hit.RelevantMemoryCount != prefetched.RelevantMemoryCount {
		t.Fatalf("expected cached recall to be reused, got %d want %d", hit.RelevantMemoryCount, prefetched.RelevantMemoryCount)
	}

	miss, err := prepareChatContextDetailsWithCache(
		root, "sess2", query, extensions.Snapshot{}, nil,
		newMemoryCache(time.Minute), memory.SemanticConfig{}, workDirs, currentDir, "smart",
	)
	if err != nil {
		t.Fatalf("prepare on cache miss: %v", err)
	}
	if hit.SystemPrompt != miss.SystemPrompt {
		t.Fatalf("cache hit and miss disagree:\nhit=%q\nmiss=%q", hit.SystemPrompt, miss.SystemPrompt)
	}
}

// The per-turn region has to be handed over separately so the assembler can
// keep it behind its own static sections.
func TestPrepareChatContext_SplitsStaticPromptFromDynamicTail(t *testing.T) {
	root := writePromptCacheWorkspace(t)

	details, err := prepareChatContextDetailsWithCache(
		root, "sess1", "what coffee do i prefer?", extensions.Snapshot{}, nil,
		newMemoryCache(time.Minute), memory.SemanticConfig{}, nil, "", "smart",
	)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if strings.Contains(details.SystemPrompt, "Current time:") {
		t.Fatalf("expected no clock in the stable region: %q", details.SystemPrompt)
	}
	if !strings.Contains(details.SystemPromptTail, "Current time:") {
		t.Fatalf("expected the clock in the tail: %q", details.SystemPromptTail)
	}
	if !strings.Contains(details.SystemPromptTail, "## Prior Context") {
		t.Fatalf("expected recall in the tail: %q", details.SystemPromptTail)
	}
	// The memory-tool rule and skill policy are session-stable and must stay
	// ahead of the tail.
	if !strings.Contains(details.SystemPrompt, "## Memory Tool Policy") {
		t.Fatalf("expected the memory tool rule in the stable region: %q", details.SystemPrompt)
	}
}

// Providers cache by prefix. Every client folds system messages ahead of the
// conversation, so a per-turn tail sent as a system message moved the prefix
// before the history and no turn ever reused the previous one. The next
// turn's request must start with everything this turn sent, up to the user's
// own words.
func TestBuildLLMMessagesWithTail_NextTurnExtendsThisTurnsPrefix(t *testing.T) {
	history := []session.Message{{Role: "user", Content: "earlier"}, {Role: "assistant", Content: "reply"}}
	turn := buildLLMMessagesWithTail("stable", "## Current Time\n\nCurrent time: 2026-08-22T10:23:00Z", history, "now", nil)

	nextHistory := append(append([]session.Message(nil), history...),
		session.Message{Role: "user", Content: "now"},
		session.Message{Role: "assistant", Content: "answer"},
	)
	next := buildLLMMessagesWithTail("stable", "## Current Time\n\nCurrent time: 2026-08-22T10:24:00Z", nextHistory, "later", nil)

	last := len(turn) - 1
	for i := 0; i < last; i++ {
		if turn[i].Role != next[i].Role || turn[i].Content != next[i].Content {
			t.Fatalf("message %d moved between turns:\nthis=%+v\nnext=%+v", i, turn[i], next[i])
		}
	}
	if next[last].Content != "now" || !strings.HasPrefix(turn[last].Content, "now") {
		t.Fatalf("this turn's user message should open with the replayed text: this=%q next=%q", turn[last].Content, next[last].Content)
	}
	if !strings.Contains(turn[last].Content, "Current time: 2026-08-22T10:23:00Z") {
		t.Fatalf("the per-turn context must still reach the model: %q", turn[last].Content)
	}
	for _, msg := range turn[1:] {
		if msg.Role == "system" {
			t.Fatalf("no system message may follow the stable prompt: %+v", turn)
		}
	}
}

func TestBuildLLMMessagesWithTail_KeepsAttachmentsOnTheUserTurn(t *testing.T) {
	blocks := []llm.ContentBlock{{Type: "image", MediaType: "image/png", Data: "aGk="}}
	msgs := buildLLMMessagesWithTail("stable", "Current time: x", nil, "look", blocks)

	user := msgs[len(msgs)-1]
	if user.Role != "user" || len(user.ContentBlocks) != 1 || !strings.HasPrefix(user.Content, "look") {
		t.Fatalf("unexpected user turn: %+v", user)
	}
}

func TestBuildLLMMessagesWithTail_EmptyTailKeepsSingleSystemMessage(t *testing.T) {
	msgs := buildLLMMessagesWithTail("stable", "   ", nil, "now", []llm.ContentBlock{})

	if len(msgs) != 2 {
		t.Fatalf("expected system+user, got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Fatalf("unexpected roles: %+v", msgs)
	}
}
