package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type stubEmbedder struct {
	vectors map[string][]float64
}

func (s stubEmbedder) Embed(_ context.Context, req EmbedRequest) ([]float64, error) {
	key := req.TaskType + "|" + req.Text
	vector, ok := s.vectors[key]
	if !ok {
		return nil, fmt.Errorf("missing vector for %s", key)
	}
	out := make([]float64, len(vector))
	copy(out, vector)
	return out, nil
}

func TestSemanticService_SearchesParaphrasesWithinProject(t *testing.T) {
	root := t.TempDir()
	if err := EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	service := NewService(root, ServiceOptions{
		Config: SemanticConfig{
			Enabled:         true,
			EmbedProvider:   "gemini",
			EmbedBaseURL:    "https://example.test",
			EmbedAPIKey:     "secret",
			EmbedModel:      "gemini-embedding-2-preview",
			EmbedDimensions: 3,
		},
		Embedder: stubEmbedder{
			vectors: map[string][]float64{
				"RETRIEVAL_DOCUMENT|User prefers decaf espresso during late-night sessions.": {0.9, 0.1, 0.0},
				"RETRIEVAL_QUERY|what coffee should I order without caffeine tonight?":       {0.88, 0.12, 0.0},
			},
		},
	})

	err := service.IndexExperience(context.Background(), Experience{
		Timestamp:     time.Date(2026, 3, 20, 8, 0, 0, 0, time.UTC),
		Category:      "preference",
		Summary:       "User prefers decaf espresso during late-night sessions.",
		ProjectID:     "alpha",
		SourceSession: "sess-alpha",
		Importance:    8,
	})
	if err != nil {
		t.Fatalf("index experience: %v", err)
	}

	hits, err := service.Search(context.Background(), SearchRequest{
		Query:     "what coffee should I order without caffeine tonight?",
		ProjectID: "alpha",
		SessionID: "sess-alpha",
		Limit:     3,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected one hit, got %d", len(hits))
	}
	if hits[0].Entry.ProjectID != "alpha" {
		t.Fatalf("expected project scoped hit, got %#v", hits[0].Entry)
	}
	if hits[0].Snippet == "" {
		t.Fatalf("expected snippet for search hit")
	}
}

func TestSemanticService_ReindexesProjectDocsOnlyWhenChanged(t *testing.T) {
	root := t.TempDir()
	if err := EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	service := NewService(root, ServiceOptions{
		Config: SemanticConfig{
			Enabled:         true,
			EmbedProvider:   "gemini",
			EmbedBaseURL:    "https://example.test",
			EmbedAPIKey:     "secret",
			EmbedModel:      "gemini-embedding-2-preview",
			EmbedDimensions: 2,
		},
		Embedder: stubEmbedder{
			vectors: map[string][]float64{
				"RETRIEVAL_DOCUMENT|Write chapter five next.": {0.7, 0.3},
				"RETRIEVAL_DOCUMENT|Write chapter six next.":  {0.7, 0.3},
			},
		},
	})

	projectDir := filepath.Join(root, "projects", "proj-1")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	statePath := filepath.Join(projectDir, "STATE.md")
	if err := os.WriteFile(statePath, []byte("Write chapter five next.\n"), 0o644); err != nil {
		t.Fatalf("write state doc: %v", err)
	}

	if err := service.EnsureProjectDocuments(context.Background(), "proj-1", ""); err != nil {
		t.Fatalf("ensure docs first pass: %v", err)
	}
	entries, err := service.LoadEntries()
	if err != nil {
		t.Fatalf("load entries first pass: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one indexed doc, got %d", len(entries))
	}
	firstHash := entries[0].ContentHash

	if err := service.EnsureProjectDocuments(context.Background(), "proj-1", ""); err != nil {
		t.Fatalf("ensure docs second pass: %v", err)
	}
	entries, err = service.LoadEntries()
	if err != nil {
		t.Fatalf("load entries second pass: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one indexed doc after no-op reindex, got %d", len(entries))
	}
	if entries[0].ContentHash != firstHash {
		t.Fatalf("expected unchanged doc hash on no-op reindex, got %q -> %q", firstHash, entries[0].ContentHash)
	}

	if err := os.WriteFile(statePath, []byte("Write chapter six next.\n"), 0o644); err != nil {
		t.Fatalf("rewrite state doc: %v", err)
	}
	if err := service.EnsureProjectDocuments(context.Background(), "proj-1", ""); err != nil {
		t.Fatalf("ensure docs third pass: %v", err)
	}
	entries, err = service.LoadEntries()
	if err != nil {
		t.Fatalf("load entries third pass: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one indexed doc after rewrite, got %d", len(entries))
	}
	if entries[0].ContentHash == firstHash {
		t.Fatalf("expected content hash to change after rewrite")
	}
}
