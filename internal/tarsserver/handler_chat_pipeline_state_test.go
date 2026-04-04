package tarsserver

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestResolveChatProjectContext_AssociatesRequestedProjectToSession(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	projectStore := project.NewStore(root, nil)
	item, err := projectStore.Create(project.CreateInput{
		Name:         "Alpha",
		Objective:    "Ship the refactor",
		Instructions: "Keep behavior stable",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	resolvedID, activeProject, prompt, err := resolveChatProjectContext(root, store, sess.ID, item.ID)
	if err != nil {
		t.Fatalf("resolve chat project context: %v", err)
	}
	if resolvedID != item.ID {
		t.Fatalf("expected resolved project %q, got %q", item.ID, resolvedID)
	}
	if activeProject == nil || activeProject.ID != item.ID {
		t.Fatalf("expected active project %q, got %+v", item.ID, activeProject)
	}
	expectedPrompt := project.ProjectPromptContext(item)
	if prompt != expectedPrompt {
		t.Fatalf("expected prompt %q, got %q", expectedPrompt, prompt)
	}

	updated, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if updated.ProjectID != item.ID {
		t.Fatalf("expected session project %q, got %q", item.ID, updated.ProjectID)
	}
}

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
		projectID:           "alpha",
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

	rows, err := memory.SearchExperiences(root, memory.SearchOptions{ProjectID: "alpha", Limit: 10})
	if err != nil {
		t.Fatalf("search experiences: %v", err)
	}
	preferenceCount := 0
	taskCount := 0
	for _, row := range rows {
		switch {
		case row.Category == "preference" && strings.Contains(strings.ToLower(row.Summary), "black coffee"):
			preferenceCount++
		case row.Category == "task_completed" && strings.Contains(strings.ToLower(row.Summary), "completed the coffee setup"):
			taskCount++
		}
	}
	if preferenceCount != 1 {
		t.Fatalf("expected exactly one preference experience, got %d rows=%+v", preferenceCount, rows)
	}
	if taskCount != 1 {
		t.Fatalf("expected exactly one task_completed experience, got %d rows=%+v", taskCount, rows)
	}
}

func TestDeriveAutoExperience_Heuristics(t *testing.T) {
	now := time.Date(2026, 3, 7, 3, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		userMessage   string
		assistantText string
		wantCategory  string
		wantSummary   string
	}{
		{
			name:         "preference from user",
			userMessage:  "I prefer concise replies",
			wantCategory: "preference",
			wantSummary:  "I prefer concise replies",
		},
		{
			name:          "task completed from assistant",
			assistantText: "completed the deployment successfully",
			wantCategory:  "task_completed",
			wantSummary:   "completed the deployment successfully",
		},
		{
			name:          "error resolved from assistant",
			assistantText: "resolved the auth issue",
			wantCategory:  "error_resolved",
			wantSummary:   "resolved the auth issue",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := deriveAutoExperience("sess", "proj", tc.userMessage, tc.assistantText, now)
			if !ok {
				t.Fatalf("expected auto experience")
			}
			if got.Category != tc.wantCategory {
				t.Fatalf("expected category %q, got %q", tc.wantCategory, got.Category)
			}
			if got.Summary != tc.wantSummary {
				t.Fatalf("expected summary %q, got %q", tc.wantSummary, got.Summary)
			}
		})
	}
}

type knowledgeCompileStubClient struct {
	content string
}

func (c *knowledgeCompileStubClient) Ask(_ context.Context, _ string) (string, error) {
	return c.content, nil
}

func (c *knowledgeCompileStubClient) Chat(_ context.Context, _ []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	return llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:    "assistant",
			Content: c.content,
		},
	}, nil
}

func TestPersistChatResult_CompilesKnowledgeBaseNote(t *testing.T) {
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
		projectID:           "alpha",
		transcriptPath:      store.TranscriptPath(sess.ID),
		llmClient: &knowledgeCompileStubClient{
			content: `{"notes":[{"slug":"coffee-preference","title":"Coffee Preference","kind":"preference","summary":"User prefers black coffee.","body":"Keep coffee suggestions unsweetened.","tags":["coffee"],"aliases":["black coffee"]}],"edges":[]}`,
		},
	}

	persistChatResult(state, "remember I prefer black coffee", llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:    "assistant",
			Content: "I will remember that you prefer black coffee.",
		},
	}, nil, logger)

	noteRaw, err := os.ReadFile(filepath.Join(root, "memory", "wiki", "notes", "coffee-preference.md"))
	if err != nil {
		t.Fatalf("read compiled note: %v", err)
	}
	if !strings.Contains(string(noteRaw), "Coffee Preference") || !strings.Contains(string(noteRaw), "black coffee") {
		t.Fatalf("expected compiled knowledge note, got %q", string(noteRaw))
	}
}
