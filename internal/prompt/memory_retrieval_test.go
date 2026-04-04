package prompt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
)

type promptStubEmbedder struct {
	vectors map[string][]float64
}

func (s promptStubEmbedder) Embed(_ context.Context, req memory.EmbedRequest) ([]float64, error) {
	key := req.TaskType + "|" + req.Text
	vector, ok := s.vectors[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	out := make([]float64, len(vector))
	copy(out, vector)
	return out, nil
}

func TestBuild_IncludesRelevantMemoryByQueryAndProject(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "IDENTITY.md"), []byte("identity"), 0o644); err != nil {
		t.Fatalf("write IDENTITY.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "PROJECT.md"), []byte("project rules"), 0o644); err != nil {
		t.Fatalf("write PROJECT.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte("I prefer green tea.\n"), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}
	if err := memory.AppendExperience(root, memory.Experience{
		Timestamp:     time.Date(2026, 3, 7, 1, 0, 0, 0, time.UTC),
		Category:      "preference",
		Summary:       "User prefers black coffee in project alpha.",
		ProjectID:     "alpha",
		SourceSession: "sess-alpha",
		Importance:    9,
	}); err != nil {
		t.Fatalf("append experience: %v", err)
	}
	if err := memory.AppendDailyLog(root, time.Date(2026, 3, 6, 9, 0, 0, 0, time.UTC), "chat session=sess-alpha user mentioned coffee beans"); err != nil {
		t.Fatalf("append daily log: %v", err)
	}

	result := Build(BuildOptions{
		WorkspaceDir: root,
		Query:        "what coffee do I prefer?",
		ProjectID:    "alpha",
		SessionID:    "sess-alpha",
	})

	if !strings.Contains(result, "## Prior Context") {
		t.Fatalf("expected relevant memory section, got %q", result)
	}
	if !strings.Contains(result, "black coffee") {
		t.Fatalf("expected experience hit in relevant memory, got %q", result)
	}
	if strings.Contains(result, "green tea") {
		t.Fatalf("did not expect unrelated MEMORY.md line in relevant memory, got %q", result)
	}
	if !strings.Contains(result, "coffee beans") {
		t.Fatalf("expected daily log hit in relevant memory, got %q", result)
	}
}

func TestBuild_RelevantMemoryUsesOtherSessionCompactionSummary(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	other, err := store.Create("older")
	if err != nil {
		t.Fatalf("create other session: %v", err)
	}
	if err := session.AppendMessage(store.TranscriptPath(other.ID), session.Message{
		Role:      "system",
		Content:   "[COMPACTION SUMMARY]\nUser prefers black coffee and concise answers.",
		Timestamp: time.Date(2026, 3, 7, 1, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("append compaction summary: %v", err)
	}

	result := Build(BuildOptions{
		WorkspaceDir: root,
		Query:        "coffee preference",
		SessionID:    "current-session",
	})

	if !strings.Contains(result, "## Prior Context") {
		t.Fatalf("expected relevant memory section, got %q", result)
	}
	if !strings.Contains(result, "black coffee") {
		t.Fatalf("expected compaction summary hit in relevant memory, got %q", result)
	}
	if !strings.Contains(result, other.ID) {
		t.Fatalf("expected relevant memory source to mention session id, got %q", result)
	}
}

func TestBuild_SkipsRelevantMemoryWhenNoMeaningfulMatches(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "IDENTITY.md"), []byte("identity"), 0o644); err != nil {
		t.Fatalf("write IDENTITY.md: %v", err)
	}

	result := Build(BuildOptions{
		WorkspaceDir: root,
		Query:        "hello there",
	})

	if strings.Contains(result, "## Prior Context") {
		t.Fatalf("did not expect relevant memory section without matches, got %q", result)
	}
}

func TestBuild_NoBriefOrProjectDocsAfterProjectRemoval(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	// After project package removal, brief and project doc matches return nil
	result := Build(BuildOptions{
		WorkspaceDir: root,
		Query:        "what is the goal for this serial?",
		SessionID:    "sess-brief",
	})

	// Should not fail; just no project-related context
	if strings.Contains(result, "_shared/project_briefs/") {
		t.Fatalf("did not expect brief content after project removal, got %q", result)
	}
}

func TestBuild_UsesSemanticMemoryForParaphraseQueries(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	semantic := memory.NewService(root, memory.ServiceOptions{
		Config: memory.SemanticConfig{
			Enabled:         true,
			EmbedProvider:   "gemini",
			EmbedBaseURL:    "https://example.test",
			EmbedAPIKey:     "secret",
			EmbedModel:      "gemini-embedding-2-preview",
			EmbedDimensions: 3,
		},
		Embedder: promptStubEmbedder{
			vectors: map[string][]float64{
				"RETRIEVAL_DOCUMENT|User prefers decaf espresso during late-night sessions.": {0.95, 0.05, 0.0},
				"RETRIEVAL_QUERY|what coffee should I order without caffeine tonight?":       {0.94, 0.06, 0.0},
			},
		},
	})
	if err := semantic.IndexExperience(context.Background(), memory.Experience{
		Timestamp:     time.Date(2026, 3, 20, 8, 0, 0, 0, time.UTC),
		Category:      "preference",
		Summary:       "User prefers decaf espresso during late-night sessions.",
		ProjectID:     "alpha",
		SourceSession: "sess-alpha",
		Importance:    9,
	}); err != nil {
		t.Fatalf("index experience: %v", err)
	}

	result := Build(BuildOptions{
		WorkspaceDir:   root,
		Query:          "what coffee should I order without caffeine tonight?",
		ProjectID:      "alpha",
		SessionID:      "sess-alpha",
		MemorySearcher: semantic,
	})

	if !strings.Contains(result, "## Prior Context") {
		t.Fatalf("expected relevant memory section, got %q", result)
	}
	if !strings.Contains(result, "decaf espresso") {
		t.Fatalf("expected semantic memory hit in prompt, got %q", result)
	}
}
