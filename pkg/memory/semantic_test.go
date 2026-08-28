package memory

import (
	"context"
	"fmt"
	"reflect"
	"strings"
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

func TestSupportedEmbedProviders(t *testing.T) {
	got := SupportedEmbedProviders()
	want := []string{"gemini"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected supported providers %v, got %v", want, got)
	}
}

func TestValidateSemanticConfigRejectsUnsupportedProvider(t *testing.T) {
	err := ValidateSemanticConfig(SemanticConfig{
		Enabled:         true,
		EmbedProvider:   "openai",
		EmbedBaseURL:    "https://api.openai.com/v1",
		EmbedAPIKey:     "test-key",
		EmbedModel:      "text-embedding-3-small",
		EmbedDimensions: 1536,
	})
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
	if !strings.Contains(err.Error(), "supported providers: gemini") {
		t.Fatalf("expected supported provider guidance, got %v", err)
	}
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
		SourceSession: "sess-alpha",
		Importance:    8,
	})
	if err != nil {
		t.Fatalf("index experience: %v", err)
	}

	hits, err := service.Search(context.Background(), SearchRequest{
		Query:     "what coffee should I order without caffeine tonight?",
		SessionID: "sess-alpha",
		Limit:     3,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected one hit, got %d", len(hits))
	}
	if hits[0].Snippet == "" {
		t.Fatalf("expected snippet for search hit")
	}
}

// TestSemanticService_ReindexesProjectDocsOnlyWhenChanged was removed
// along with the project system (EnsureProjectDocuments no longer exists).
