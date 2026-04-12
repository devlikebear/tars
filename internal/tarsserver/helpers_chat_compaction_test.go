package tarsserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

type compactionTestRouter struct {
	client llm.Client
	called int
}

func (r *compactionTestRouter) ClientFor(role llm.Role) (llm.Client, llm.TierResolution, error) {
	r.called++
	return r.client, llm.TierResolution{Role: role, Tier: llm.TierLight}, nil
}

func (r *compactionTestRouter) ClientForTier(tier llm.Tier) (llm.Client, llm.TierResolution, error) {
	return r.client, llm.TierResolution{Tier: tier}, nil
}

func (r *compactionTestRouter) DefaultTier() llm.Tier              { return llm.TierStandard }
func (r *compactionTestRouter) TierForRole(role llm.Role) llm.Tier { return llm.TierStandard }
func (r *compactionTestRouter) Close() error                       { return nil }

type slowCompactionClient struct{}

func (slowCompactionClient) Ask(context.Context, string) (string, error) { return "", nil }

func (slowCompactionClient) Chat(ctx context.Context, _ []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	<-ctx.Done()
	return llm.ChatResponse{}, ctx.Err()
}

func TestCompactionClient_DeterministicModeSkipsRouterResolution(t *testing.T) {
	router := &compactionTestRouter{client: &mockLLMClient{}}
	client := compactionClient(router, "deterministic")
	if client != nil {
		t.Fatalf("expected nil client in deterministic mode")
	}
	if router.called != 0 {
		t.Fatalf("expected router not to be touched, got %d calls", router.called)
	}
}

func TestCompactWithMemoryFlush_TimeoutFallsBackToDeterministic(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	path := filepath.Join(root, "sessions", "transcript.jsonl")
	if err := appendCompactionTestMessages(path, 12); err != nil {
		t.Fatalf("append messages: %v", err)
	}

	router := &compactionTestRouter{client: slowCompactionClient{}}
	result, mode, err := compactWithMemoryFlush(root, path, "sess_1", 0, chatCompactionOptions{
		TriggerTokens:      1,
		KeepRecentTokens:   10,
		KeepRecentFraction: 0.20,
		LLMMode:            "auto",
		LLMTimeoutSeconds:  1,
	}, "", router, time.Now().UTC(), nil)
	if err != nil {
		t.Fatalf("compactWithMemoryFlush: %v", err)
	}
	if !result.Compacted {
		t.Fatalf("expected compaction result")
	}
	if mode != "deterministic" {
		t.Fatalf("expected deterministic fallback mode, got %q", mode)
	}
	if router.called == 0 {
		t.Fatalf("expected router to be consulted in auto mode")
	}
	msgs, err := session.ReadMessages(path)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if len(msgs) == 0 || msgs[0].Role != "system" {
		t.Fatalf("expected transcript to be rewritten with summary, got %+v", msgs)
	}
	if msgs[0].Content == "" {
		t.Fatalf("expected deterministic summary content")
	}
}

func TestHandleChatRequest_EmitsCompactionAppliedEvent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := appendCompactionTestMessages(store.TranscriptPath(sess.ID), 20); err != nil {
		t.Fatalf("append messages: %v", err)
	}
	handler := newChatAPIHandlerWithRuntimeConfig(
		root,
		store,
		&mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Content: "done"}}},
		nil,
		zerolog.Nop(),
		4,
		nil,
		"",
		chatToolingOptions{
			Compaction: chatCompactionOptions{
				TriggerTokens:      1,
				KeepRecentTokens:   10,
				KeepRecentFraction: 0.20,
				LLMMode:            "deterministic",
				LLMTimeoutSeconds:  1,
			},
		},
	)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"session_id":"`+sess.ID+`","message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"compaction_applied"`) {
		t.Fatalf("expected compaction_applied event, got %q", body)
	}
	if !strings.Contains(body, `"mode":"deterministic"`) {
		t.Fatalf("expected deterministic compaction mode, got %q", body)
	}
}

func appendCompactionTestMessages(path string, count int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		if err := session.AppendMessage(path, session.Message{
			Role:      role,
			Content:   strings.Repeat("message-", 12),
			Timestamp: time.Date(2026, 4, 1, 12, 0, i, 0, time.UTC),
		}); err != nil {
			return err
		}
	}
	return nil
}

var _ llm.Router = (*compactionTestRouter)(nil)
var _ llm.Client = (*slowCompactionClient)(nil)
