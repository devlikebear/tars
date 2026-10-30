package memory

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
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
	want := []string{"gemini", "local", "openai"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected supported providers %v, got %v", want, got)
	}
}

func TestValidateSemanticConfigRejectsUnsupportedProvider(t *testing.T) {
	err := ValidateSemanticConfig(SemanticConfig{
		Enabled:         true,
		EmbedProvider:   "cohere",
		EmbedBaseURL:    "https://api.cohere.com/v2",
		EmbedAPIKey:     "test-key",
		EmbedModel:      "embed-v4.0",
		EmbedDimensions: 1536,
	})
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
	if !strings.Contains(err.Error(), "supported providers: gemini, local, openai") {
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

func reindexTestService(t *testing.T, root, model string, embedder Embedder) *Service {
	t.Helper()
	return NewService(root, ServiceOptions{
		Config: SemanticConfig{
			Enabled: true, EmbedProvider: "local", EmbedBaseURL: "http://127.0.0.1:1/v1",
			EmbedModel: model, EmbedDimensions: 2,
		},
		Embedder: embedder,
	})
}

type constantEmbedder struct {
	calls   []string
	failOn  string
	failErr error
}

func (c *constantEmbedder) Embed(_ context.Context, req EmbedRequest) ([]float64, error) {
	if c.failOn != "" && req.Text == c.failOn {
		return nil, c.failErr
	}
	c.calls = append(c.calls, req.TaskType+"|"+req.Text)
	return []float64{1, 0}, nil
}

func TestReindex_EmbedsStaleEntriesAndUnindexedExperiences(t *testing.T) {
	root := t.TempDir()
	if err := EnsureWorkspace(root); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	// Indexed with the old model, and logged, so it must become one entry.
	old := reindexTestService(t, root, "old/model", &constantEmbedder{})
	indexed := Experience{Timestamp: at, Category: "fact", Summary: "already indexed", SourceSession: "s1"}
	if err := AppendExperience(root, indexed); err != nil {
		t.Fatal(err)
	}
	if err := old.IndexExperience(ctx, indexed); err != nil {
		t.Fatal(err)
	}
	if err := old.IndexCompactionSummary(ctx, "s2", "compaction text", at); err != nil {
		t.Fatal(err)
	}
	// Written by a path that never embedded it.
	if err := AppendExperience(root, Experience{Timestamp: at.Add(time.Hour), Category: "fact", Summary: "never indexed"}); err != nil {
		t.Fatal(err)
	}

	embedder := &constantEmbedder{}
	svc := reindexTestService(t, root, "new/model", embedder)
	pending, err := svc.ReindexPending()
	if err != nil || pending != 3 {
		t.Fatalf("expected 3 pending, got %d %v", pending, err)
	}
	var seen [][2]int
	result, err := svc.Reindex(ctx, func(done, total int) { seen = append(seen, [2]int{done, total}) })
	if err != nil {
		t.Fatalf("reindex: %v", err)
	}
	if result.Embedded != 3 || len(seen) != 3 || seen[2] != [2]int{3, 3} {
		t.Fatalf("unexpected result %+v progress %v", result, seen)
	}
	for _, call := range embedder.calls {
		if !strings.HasPrefix(call, taskTypeRetrievalDoc+"|") {
			t.Fatalf("expected document task type, got %q", call)
		}
	}
	entries, err := svc.LoadEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected the logged experience to reuse its entry, got %d entries", len(entries))
	}
	for _, entry := range entries {
		if entry.EmbeddingModel != "new/model" || entry.EmbeddingDimensions != 2 {
			t.Fatalf("expected entry on the new model, got %+v", entry)
		}
	}

	embedder.calls = nil
	if pending, _ := svc.ReindexPending(); pending != 0 {
		t.Fatalf("expected nothing pending, got %d", pending)
	}
	if result, err := svc.Reindex(ctx, nil); err != nil || result.Embedded != 0 || len(embedder.calls) != 0 {
		t.Fatalf("expected a no-op second run, got %+v %v %v", result, err, embedder.calls)
	}
}

func TestReindex_StopsOnErrorAndResumes(t *testing.T) {
	root := t.TempDir()
	if err := EnsureWorkspace(root); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for i, summary := range []string{"first", "second"} {
		if err := AppendExperience(root, Experience{Timestamp: at.Add(time.Duration(i) * time.Hour), Summary: summary}); err != nil {
			t.Fatal(err)
		}
	}
	embedder := &constantEmbedder{failOn: "second", failErr: fmt.Errorf("sidecar down")}
	svc := reindexTestService(t, root, "new/model", embedder)
	result, err := svc.Reindex(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "sidecar down") || result.Embedded != 1 {
		t.Fatalf("expected partial result and error, got %+v %v", result, err)
	}
	embedder.failOn = ""
	if result, err = svc.Reindex(context.Background(), nil); err != nil || result.Embedded != 1 {
		t.Fatalf("expected the rest on the next run, got %+v %v", result, err)
	}
}

func TestReindex_RespectsCancelAndDisabledService(t *testing.T) {
	root := t.TempDir()
	if err := EnsureWorkspace(root); err != nil {
		t.Fatal(err)
	}
	if err := AppendExperience(root, Experience{Timestamp: time.Now(), Summary: "x"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reindexTestService(t, root, "m/m", &constantEmbedder{}).Reindex(ctx, nil); err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	disabled := NewService(root, ServiceOptions{})
	if _, err := disabled.Reindex(context.Background(), nil); err != ErrSemanticUnavailable {
		t.Fatalf("expected ErrSemanticUnavailable, got %v", err)
	}
	if pending, err := disabled.ReindexPending(); pending != 0 || err != nil {
		t.Fatalf("expected no pending on a disabled service, got %d %v", pending, err)
	}
}

func TestUpsert_ConcurrentWritersKeepEveryEntry(t *testing.T) {
	root := t.TempDir()
	if err := EnsureWorkspace(root); err != nil {
		t.Fatal(err)
	}
	// Two Service values, as the server builds one per use.
	a := reindexTestService(t, root, "m/m", &constantEmbedder{})
	b := reindexTestService(t, root, "m/m", &constantEmbedder{})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		svc := a
		if i%2 == 1 {
			svc = b
		}
		wg.Add(1)
		go func(i int, svc *Service) {
			defer wg.Done()
			_ = svc.IndexExperience(context.Background(), Experience{Timestamp: time.Unix(int64(i), 0), Summary: fmt.Sprintf("fact %d", i)})
		}(i, svc)
	}
	wg.Wait()
	entries, err := a.LoadEntries()
	if err != nil || len(entries) != 20 {
		t.Fatalf("expected 20 entries, got %d %v", len(entries), err)
	}
}
