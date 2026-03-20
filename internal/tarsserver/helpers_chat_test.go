package tarsserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
)

type compactionStubEmbedder struct {
	vectors map[string][]float64
}

func (s compactionStubEmbedder) Embed(_ context.Context, req memory.EmbedRequest) ([]float64, error) {
	vector, ok := s.vectors[req.TaskType+"|"+req.Text]
	if !ok {
		return nil, context.DeadlineExceeded
	}
	out := make([]float64, len(vector))
	copy(out, vector)
	return out, nil
}

type compactionLLMClient struct {
	summary string
	extract string
}

func (c *compactionLLMClient) Ask(_ context.Context, _ string) (string, error) {
	return c.summary, nil
}

func (c *compactionLLMClient) Chat(_ context.Context, messages []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	last := messages[len(messages)-1].Content
	content := c.summary
	if strings.Contains(last, "Return strict JSON") {
		content = c.extract
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: content}}, nil
}

func TestCompactWithMemoryFlush_IndexesSummaryAndExtractedMemories(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	transcriptPath := store.TranscriptPath(sess.ID)
	for i := 0; i < 8; i++ {
		if err := session.AppendMessage(transcriptPath, session.Message{
			Role:      "user",
			Content:   strings.Repeat("User prefers decaf espresso and concise replies. ", 12),
			Timestamp: time.Date(2026, 3, 20, 8, 0, i, 0, time.UTC),
		}); err != nil {
			t.Fatalf("append message %d: %v", i, err)
		}
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
		Embedder: compactionStubEmbedder{
			vectors: map[string][]float64{
				"RETRIEVAL_DOCUMENT|[COMPACTION SUMMARY]\nUser prefers decaf espresso and concise replies.": {0.9, 0.1, 0.0},
				"RETRIEVAL_DOCUMENT|User prefers decaf espresso.":                                           {0.9, 0.1, 0.0},
			},
		},
	})

	client := &compactionLLMClient{
		summary: "[COMPACTION SUMMARY]\nUser prefers decaf espresso and concise replies.",
		extract: `{"memories":[{"category":"preference","summary":"User prefers decaf espresso.","importance":8}]}`,
	}

	if _, err := compactWithMemoryFlush(root, transcriptPath, sess.ID, 2, 20, client, time.Date(2026, 3, 20, 8, 30, 0, 0, time.UTC), semantic); err != nil {
		t.Fatalf("compact with memory flush: %v", err)
	}

	entries, err := semantic.LoadEntries()
	if err != nil {
		t.Fatalf("load semantic entries: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected summary and extracted memory entries, got %d", len(entries))
	}
}
