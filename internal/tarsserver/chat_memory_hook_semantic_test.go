package tarsserver

import (
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/memory"
)

func TestApplyPostChatMemoryHooks_RememberKeepsExperienceWhenEmbeddingFails(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatal(err)
	}
	// Nothing listens on this port: the experience must still be saved, and
	// reindex must see it as pending.
	semantic := memory.SemanticConfig{
		Enabled: true, EmbedProvider: "local", EmbedBaseURL: "http://127.0.0.1:1/v1",
		EmbedModel: "kakaocorp/kanana-2-1.3b-embedding", EmbedDimensions: 1024,
	}
	if err := applyPostChatMemoryHooks(chatMemoryHookInput{
		WorkspaceDir: root, SessionID: "s", UserMessage: "remember that I prefer tea",
		AssistantMessage: "ok", AssistantTime: time.Now(), Semantic: semantic,
	}); err != nil {
		t.Fatalf("hook: %v", err)
	}
	rows, err := memory.SearchExperiences(root, memory.SearchOptions{Query: "tea"})
	if err != nil || len(rows) != 1 || !strings.Contains(rows[0].Summary, "tea") {
		t.Fatalf("expected saved experience, got %+v %v", rows, err)
	}
	pending, err := memory.NewService(root, memory.ServiceOptions{Config: semantic}).ReindexPending()
	if err != nil || pending != 1 {
		t.Fatalf("expected one pending entry, got %d %v", pending, err)
	}
}
