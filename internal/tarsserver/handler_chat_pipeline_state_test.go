package tarsserver

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestPersistChatResult_AppendsAssistantMessageAndTouchesSession(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	state := chatRunState{
		requestWorkspaceDir: root,
		store:               store,
		sessionID:           sess.ID,
		transcriptPath:      store.TranscriptPath(sess.ID),
	}

	persistChatResult(state, "hello", llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:    "assistant",
			Content: "reply",
		},
	}, nil, logger)

	msgs, err := session.ReadMessages(store.TranscriptPath(sess.ID))
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 transcript message, got %d", len(msgs))
	}
	if msgs[0].Role != "assistant" || msgs[0].Content != "reply" {
		t.Fatalf("unexpected transcript message: %+v", msgs[0])
	}

	updated, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if !updated.UpdatedAt.Equal(msgs[0].Timestamp) {
		t.Fatalf("expected updated_at %s to match assistant timestamp %s", updated.UpdatedAt, msgs[0].Timestamp)
	}

	dailyLogPath := filepath.Join(root, "memory", msgs[0].Timestamp.UTC().Format("2006-01-02")+".md")
	dailyLog, err := os.ReadFile(dailyLogPath)
	if err != nil {
		t.Fatalf("read daily log: %v", err)
	}
	if !strings.Contains(string(dailyLog), `chat session=`+sess.ID) {
		t.Fatalf("expected daily log entry for session, got %q", string(dailyLog))
	}
	if !strings.Contains(string(dailyLog), `assistant="reply"`) {
		t.Fatalf("expected daily log to include assistant summary, got %q", string(dailyLog))
	}
}

func TestPersistChatResult_PromotesMemoryAndDedupesExperience(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	state := chatRunState{
		requestWorkspaceDir: root,
		store:               store,
		sessionID:           sess.ID,
		transcriptPath:      store.TranscriptPath(sess.ID),
	}

	response := llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:    "assistant",
			Content: "completed the coffee setup and fixed the grinder issue",
		},
	}
	userMessage := "remember I prefer black coffee"

	persistChatResult(state, userMessage, response, nil, logger)
	persistChatResult(state, userMessage, response, nil, logger)

	memoryFile, err := os.ReadFile(filepath.Join(root, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read MEMORY.md: %v", err)
	}
	if !strings.Contains(string(memoryFile), "user preference/fact: remember I prefer black coffee") {
		t.Fatalf("expected durable memory note, got %q", string(memoryFile))
	}

	rows, err := memory.SearchExperiences(root, memory.SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("search experiences: %v", err)
	}
	// Only the explicit "remember …" hot path survives in the per-turn
	// hook; auto-experience derivation (task_completed, error_resolved,
	// etc.) moved to the nightly reflection job and is covered by
	// reflection.TestMemoryJobExtractsExperiences.
	preferenceCount := 0
	for _, row := range rows {
		if row.Category == "preference" && strings.Contains(strings.ToLower(row.Summary), "black coffee") {
			preferenceCount++
		}
	}
	if preferenceCount != 1 {
		t.Fatalf("expected exactly one preference experience, got %d rows=%+v", preferenceCount, rows)
	}
}

// Auto-experience derivation moved to internal/reflection. The
// equivalent coverage now lives in reflection.TestDeriveUserExperience*
// and TestDeriveAssistantExperience*.

// Per-turn knowledge-base compilation was removed along with
// maybeCompileKnowledgeBase; the equivalent coverage now lives in
// reflection.TestMemoryJobLLMKnowledgeCompile which exercises the
// nightly batch form of the same LLM call.
