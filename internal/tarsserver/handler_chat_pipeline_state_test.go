package tarsserver

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/project"
	"github.com/devlikebear/tarsncase/internal/session"
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
	}, logger)

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
}
