package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
)

type searchStubEmbedder struct {
	vectors map[string][]float64
}

func (s searchStubEmbedder) Embed(_ context.Context, req memory.EmbedRequest) ([]float64, error) {
	vector, ok := s.vectors[req.TaskType+"|"+req.Text]
	if !ok {
		return nil, os.ErrNotExist
	}
	out := make([]float64, len(vector))
	copy(out, vector)
	return out, nil
}

func TestMemorySearchTool_QueryAndMetadata(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	memPath := filepath.Join(root, "MEMORY.md")
	if err := os.WriteFile(memPath, []byte("# MEMORY\n- Coffee preference is long-term.\n"), 0o644); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	dailyPath := filepath.Join(root, "memory", "2026-02-14.md")
	if err := os.WriteFile(dailyPath, []byte("coffee run note\n"), 0o644); err != nil {
		t.Fatalf("write daily: %v", err)
	}
	_ = os.Chtimes(memPath, time.Date(2026, 2, 13, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 13, 0, 0, 0, 0, time.UTC))
	_ = os.Chtimes(dailyPath, time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC))

	tl := NewMemorySearchTool(root, nil)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"coffee","limit":5}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}

	var payload struct {
		Results []struct {
			Source  string `json:"source"`
			Date    string `json:"date"`
			Line    int    `json:"line"`
			Snippet string `json:"snippet"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.Text()), &payload); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(payload.Results) == 0 {
		t.Fatalf("expected search results")
	}
	first := payload.Results[0]
	if first.Source != "memory/2026-02-14.md" {
		t.Fatalf("expected latest file first, got source=%q", first.Source)
	}
	if first.Date == "" || first.Line <= 0 || strings.TrimSpace(first.Snippet) == "" {
		t.Fatalf("expected metadata fields, got %+v", first)
	}
}

func TestMemorySearchTool_LimitCap(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	lines := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		lines = append(lines, "capword line")
	}
	path := filepath.Join(root, "memory", "2026-02-14.md")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write daily file: %v", err)
	}

	tl := NewMemorySearchTool(root, nil)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"capword","limit":100,"include_memory":false,"include_daily":true}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload struct {
		Results []any `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.Text()), &payload); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(payload.Results) != 30 {
		t.Fatalf("expected capped 30 results, got %d", len(payload.Results))
	}
}

func TestMemorySearchTool_IncludeFlags(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte("alpha memory only\n"), 0o644); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory", "2026-02-14.md"), []byte("alpha daily only\n"), 0o644); err != nil {
		t.Fatalf("write daily: %v", err)
	}

	tl := NewMemorySearchTool(root, nil)

	onlyMemory, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"alpha","include_memory":true,"include_daily":false}`))
	if err != nil {
		t.Fatalf("execute memory-only: %v", err)
	}
	if !strings.Contains(onlyMemory.Text(), `"source":"MEMORY.md"`) {
		t.Fatalf("expected memory-only result, got %q", onlyMemory.Text())
	}
	if strings.Contains(onlyMemory.Text(), `"source":"memory/`) {
		t.Fatalf("did not expect daily source in memory-only result, got %q", onlyMemory.Text())
	}

	onlyDaily, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"alpha","include_memory":false,"include_daily":true}`))
	if err != nil {
		t.Fatalf("execute daily-only: %v", err)
	}
	if !strings.Contains(onlyDaily.Text(), `"source":"memory/2026-02-14.md"`) {
		t.Fatalf("expected daily-only result, got %q", onlyDaily.Text())
	}
	if strings.Contains(onlyDaily.Text(), `"source":"MEMORY.md"`) {
		t.Fatalf("did not expect MEMORY.md source in daily-only result, got %q", onlyDaily.Text())
	}
}

func TestMemorySearchTool_UsesSemanticSearchBeforeLexicalFallback(t *testing.T) {
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
		Embedder: searchStubEmbedder{
			vectors: map[string][]float64{
				"RETRIEVAL_DOCUMENT|User prefers decaf espresso during late-night sessions.": {0.92, 0.08, 0.0},
				"RETRIEVAL_QUERY|what coffee should I order without caffeine tonight?":       {0.91, 0.09, 0.0},
			},
		},
	})
	if err := semantic.IndexExperience(context.Background(), memory.Experience{
		Timestamp:     time.Date(2026, 3, 20, 8, 0, 0, 0, time.UTC),
		Category:      "preference",
		Summary:       "User prefers decaf espresso during late-night sessions.",
		ProjectID:     "alpha",
		SourceSession: "sess-alpha",
		Importance:    8,
	}); err != nil {
		t.Fatalf("index experience: %v", err)
	}

	tl := NewMemorySearchTool(root, semantic)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"what coffee should I order without caffeine tonight?","limit":5}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(result.Text(), "decaf espresso") {
		t.Fatalf("expected semantic result in output, got %q", result.Text())
	}
}

func TestMemorySearchTool_IncludeSessions(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("test session")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	transcriptPath := store.TranscriptPath(sess.ID)
	if err := session.AppendMessage(transcriptPath, session.Message{
		Role:      "user",
		Content:   "I love cooking pasta with tomato sauce",
		Timestamp: time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("append user message: %v", err)
	}
	if err := session.AppendMessage(transcriptPath, session.Message{
		Role:      "assistant",
		Content:   "Here is a great pasta recipe with tomato sauce",
		Timestamp: time.Date(2026, 3, 20, 10, 1, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("append assistant message: %v", err)
	}

	tl := NewMemorySearchTool(root, nil)

	// With include_sessions=true, should find session content
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"pasta","include_sessions":true}`))
	if err != nil {
		t.Fatalf("execute with sessions: %v", err)
	}
	if !strings.Contains(result.Text(), "pasta") {
		t.Fatalf("expected session match for pasta, got %q", result.Text())
	}
	if !strings.Contains(result.Text(), "session:") {
		t.Fatalf("expected session source prefix, got %q", result.Text())
	}

	// With include_sessions=false (default), should NOT find session content
	result2, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"pasta","include_sessions":false}`))
	if err != nil {
		t.Fatalf("execute without sessions: %v", err)
	}
	if strings.Contains(result2.Text(), "session:") {
		t.Fatalf("did not expect session results when include_sessions=false, got %q", result2.Text())
	}
}

func TestMemorySearchTool_IncludeSessionsSkipsSystemAndTool(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	transcriptPath := store.TranscriptPath(sess.ID)
	// System and tool messages should be skipped
	if err := session.AppendMessage(transcriptPath, session.Message{
		Role:    "system",
		Content: "secretword system prompt",
	}); err != nil {
		t.Fatalf("append system message: %v", err)
	}
	if err := session.AppendMessage(transcriptPath, session.Message{
		Role:    "tool",
		Content: "secretword tool result",
	}); err != nil {
		t.Fatalf("append tool message: %v", err)
	}
	if err := session.AppendMessage(transcriptPath, session.Message{
		Role:    "user",
		Content: "visible user message",
	}); err != nil {
		t.Fatalf("append user message: %v", err)
	}

	tl := NewMemorySearchTool(root, nil)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"secretword","include_sessions":true}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	// Results should not include system/tool message content as snippets
	var payload struct {
		Results []struct {
			Source  string `json:"source"`
			Snippet string `json:"snippet"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.Text()), &payload); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	for _, r := range payload.Results {
		if strings.Contains(r.Snippet, "system prompt") || strings.Contains(r.Snippet, "tool result") {
			t.Fatalf("should not find system/tool messages in results, got snippet=%q", r.Snippet)
		}
	}
}

func TestMemorySearchTool_SearchesKnowledgeBaseNotes(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := memory.NewKnowledgeStore(root, nil)
	if _, err := store.Upsert(memory.KnowledgeNote{
		Slug:    "coffee-preference",
		Title:   "Coffee Preference",
		Kind:    "preference",
		Summary: "User prefers black coffee.",
		Body:    "Knowledge base note for coffee choices.",
	}); err != nil {
		t.Fatalf("upsert knowledge note: %v", err)
	}

	tl := NewMemorySearchTool(root, nil)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"black coffee","include_memory":false,"include_daily":false}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(result.Text(), "memory/wiki/notes/coffee-preference.md") {
		t.Fatalf("expected knowledge note source in output, got %q", result.Text())
	}
}
