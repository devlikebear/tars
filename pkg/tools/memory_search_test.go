package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/memory"
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

	tl := NewMemorySearchTool(root, memory.NewFileBackend(root, nil))
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

	tl := NewMemorySearchTool(root, memory.NewFileBackend(root, nil))
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

	tl := NewMemorySearchTool(root, memory.NewFileBackend(root, nil))

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
		SourceSession: "sess-alpha",
		Importance:    8,
	}); err != nil {
		t.Fatalf("index experience: %v", err)
	}

	tl := NewMemorySearchTool(root, memory.NewFileBackend(root, semantic))
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"what coffee should I order without caffeine tonight?","limit":5}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(result.Text(), "decaf espresso") {
		t.Fatalf("expected semantic result in output, got %q", result.Text())
	}
}

// fakeTranscripts is the in-package stand-in for a session store. pkg/tools
// must not import the session package (that is the whole point of
// TranscriptSource), and an internal test cannot import the adapter
// sub-package without a cycle, so the contract is exercised through a fake
// here and against a real store in sessiontranscripts' own test.
type fakeTranscripts struct {
	refs     []TranscriptRef
	messages map[string][]TranscriptMessage
}

func (f fakeTranscripts) ListTranscripts() ([]TranscriptRef, error) { return f.refs, nil }
func (f fakeTranscripts) ReadTranscript(id string) ([]TranscriptMessage, error) {
	return f.messages[id], nil
}

func TestMemorySearchTool_IncludeSessions(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	updated := time.Date(2026, 3, 20, 10, 1, 0, 0, time.UTC)
	transcripts := fakeTranscripts{
		refs: []TranscriptRef{{ID: "sess-1", UpdatedAt: updated}},
		messages: map[string][]TranscriptMessage{"sess-1": {
			{Role: "user", Content: "I love cooking pasta with tomato sauce", Timestamp: time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)},
			{Role: "assistant", Content: "Here is a great pasta recipe with tomato sauce", Timestamp: updated},
		}},
	}

	tl := NewMemorySearchToolWithTranscripts(root, memory.NewFileBackend(root, nil), transcripts)

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

	// System and tool messages should be skipped
	transcripts := fakeTranscripts{
		refs: []TranscriptRef{{ID: "sess-1", UpdatedAt: time.Now()}},
		messages: map[string][]TranscriptMessage{"sess-1": {
			{Role: "system", Content: "secretword system prompt"},
			{Role: "tool", Content: "secretword tool result"},
			{Role: "user", Content: "visible user message"},
		}},
	}

	tl := NewMemorySearchToolWithTranscripts(root, memory.NewFileBackend(root, nil), transcripts)
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

func TestMemorySearchTool_SearchesExperienceLogByTerms(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := memory.AppendExperience(root, memory.Experience{
		Timestamp:     time.Date(2026, 4, 4, 7, 30, 0, 0, time.UTC),
		Category:      "fact",
		Summary:       "나는 삼성전자와 SK하이닉스 주식을 보유하고 있어.",
		Tags:          []string{"주식", "보유종목", "반도체"},
		SourceSession: "sess-korea",
	}); err != nil {
		t.Fatalf("append experience: %v", err)
	}

	tl := NewMemorySearchTool(root, memory.NewFileBackend(root, nil))
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"관심있는 주식","include_memory":false,"include_daily":false,"include_sessions":false}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(result.Text(), "삼성전자와 SK하이닉스") {
		t.Fatalf("expected experience log match in output, got %q", result.Text())
	}
	if !strings.Contains(result.Text(), `"source":"experience:fact"`) {
		t.Fatalf("expected experience source in output, got %q", result.Text())
	}
}

// The plain constructor has no transcripts. include_sessions must then find
// nothing -- and must not claim "no matches" for a search that never ran, so
// with every other source switched off the message is "no memory sources".
func TestMemorySearchTool_NoTranscriptSourceMeansNoSessionSearch(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	tl := NewMemorySearchTool(root, memory.NewFileBackend(root, nil))
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"anything","include_memory":false,"include_daily":false,"include_sessions":true}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if strings.Contains(result.Text(), "session:") {
		t.Fatalf("no source was supplied, yet a session result came back: %s", result.Text())
	}
	if !strings.Contains(result.Text(), "no memory sources found") {
		t.Fatalf("want the honest 'no memory sources found', got %s", result.Text())
	}
}
