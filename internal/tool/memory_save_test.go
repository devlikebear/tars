package tool

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/memory"
)

func TestMemorySaveTool_AppendsExperience(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	now := time.Date(2026, 2, 22, 9, 0, 0, 0, time.UTC)
	semantic := memory.NewService(root, memory.ServiceOptions{
		Config: memory.SemanticConfig{
			Enabled:         true,
			EmbedProvider:   "gemini",
			EmbedBaseURL:    "https://example.test",
			EmbedAPIKey:     "secret",
			EmbedModel:      "gemini-embedding-2-preview",
			EmbedDimensions: 2,
		},
		Embedder: saveStubEmbedder{vectors: map[string][]float64{
			"RETRIEVAL_DOCUMENT|User prefers concise Korean responses": {0.8, 0.2},
			"RETRIEVAL_QUERY|concise Korean responses":                 {0.8, 0.2},
		}},
	})
	tool := NewMemorySaveTool(root, semantic, func() time.Time { return now })
	result, err := tool.Execute(context.Background(), json.RawMessage(`{
		"category":"preference",
		"summary":"User prefers concise Korean responses",
		"tags":["user","style"],
		"source_session":"sess-1",
		"project_id":"proj_demo",
		"importance":7
	}`))
	if err != nil {
		t.Fatalf("execute memory_save: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %+v", result)
	}
	if !strings.Contains(result.Text(), "saved") {
		t.Fatalf("expected saved message, got %q", result.Text())
	}

	hits, err := memory.SearchExperiences(root, memory.SearchOptions{Query: "concise", Limit: 5})
	if err != nil {
		t.Fatalf("search experiences: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected one hit, got %d", len(hits))
	}
	if hits[0].ProjectID != "proj_demo" {
		t.Fatalf("expected project_id proj_demo, got %q", hits[0].ProjectID)
	}
	if hits[0].SourceSession != "sess-1" {
		t.Fatalf("expected source_session sess-1, got %q", hits[0].SourceSession)
	}
	semanticHits, err := semantic.Search(context.Background(), memory.SearchRequest{
		Query:     "concise Korean responses",
		ProjectID: "proj_demo",
		SessionID: "sess-1",
		Limit:     5,
	})
	if err != nil {
		t.Fatalf("search semantic index: %v", err)
	}
	if len(semanticHits) != 1 {
		t.Fatalf("expected one semantic hit, got %d", len(semanticHits))
	}

	if _, err := filepath.Glob(filepath.Join(root, "memory", "experiences.jsonl")); err != nil {
		t.Fatalf("glob experiences path: %v", err)
	}
}

func TestMemorySaveTool_RejectsEmptySummary(t *testing.T) {
	tool := NewMemorySaveTool(t.TempDir(), nil, time.Now)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"category":"fact","summary":"   "}`))
	if err != nil {
		t.Fatalf("execute memory_save: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for empty summary")
	}
	if !strings.Contains(strings.ToLower(result.Text()), "summary") {
		t.Fatalf("expected summary validation error, got %q", result.Text())
	}
}

type saveStubEmbedder struct {
	vectors map[string][]float64
}

func (s saveStubEmbedder) Embed(_ context.Context, req memory.EmbedRequest) ([]float64, error) {
	vector, ok := s.vectors[req.TaskType+"|"+req.Text]
	if !ok {
		return nil, context.DeadlineExceeded
	}
	out := make([]float64, len(vector))
	copy(out, vector)
	return out, nil
}
