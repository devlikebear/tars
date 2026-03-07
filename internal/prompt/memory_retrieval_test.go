package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/session"
)

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

	if !strings.Contains(result, "## Relevant Memory") {
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

	if !strings.Contains(result, "## Relevant Memory") {
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

	if strings.Contains(result, "## Relevant Memory") {
		t.Fatalf("did not expect relevant memory section without matches, got %q", result)
	}
}
