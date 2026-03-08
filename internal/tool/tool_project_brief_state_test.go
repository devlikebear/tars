package tool

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
)

func TestProjectBriefTools_DefaultToSessionIDAndFinalize(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	store := project.NewStore(root, nil)
	sessionStore := session.NewStore(root)
	sess, err := sessionStore.Create("writer")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	ctx := usage.WithCallMeta(context.Background(), usage.CallMeta{Source: "chat", SessionID: sess.ID})

	update := NewProjectBriefUpdateTool(store)
	result, err := update.Execute(ctx, json.RawMessage(`{
		"title":"Orbit Hearts",
		"goal":"Write a serialized space opera",
		"kind":"serial",
		"premise":"Two rival navigators chase a dead-star map.",
		"open_questions":["Who betrays the crew?"],
		"status":"ready"
	}`))
	if err != nil {
		t.Fatalf("execute project_brief_update: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %s", result.Text())
	}

	get := NewProjectBriefGetTool(store)
	got, err := get.Execute(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute project_brief_get: %v", err)
	}
	if !strings.Contains(got.Text(), "Orbit Hearts") {
		t.Fatalf("expected brief content in get result, got %s", got.Text())
	}

	finalize := NewProjectBriefFinalizeTool(store, sessionStore)
	finalized, err := finalize.Execute(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute project_brief_finalize: %v", err)
	}
	if finalized.IsError {
		t.Fatalf("expected finalize success, got %s", finalized.Text())
	}

	sessAfter, err := sessionStore.Get(sess.ID)
	if err != nil {
		t.Fatalf("get session after finalize: %v", err)
	}
	if strings.TrimSpace(sessAfter.ProjectID) == "" {
		t.Fatalf("expected session project binding after finalize")
	}
}

func TestProjectBriefTools_CurrentAliasUsesSessionID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	store := project.NewStore(root, nil)
	sessionStore := session.NewStore(root)
	sess, err := sessionStore.Create("writer")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	ctx := usage.WithCallMeta(context.Background(), usage.CallMeta{Source: "chat", SessionID: sess.ID})

	update := NewProjectBriefUpdateTool(store)
	result, err := update.Execute(ctx, json.RawMessage(`{
		"brief_id":"current",
		"title":"Current Alias Test",
		"goal":"Verify alias resolution"
	}`))
	if err != nil {
		t.Fatalf("execute project_brief_update: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %s", result.Text())
	}

	if _, err := store.GetBrief(sess.ID); err != nil {
		t.Fatalf("expected brief stored under session id, got err=%v", err)
	}
	if _, err := store.GetBrief("current"); err == nil {
		t.Fatalf("expected no literal current brief to be created")
	}
}

func TestProjectStateUpdateTool_CreatesStateDocument(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	store := project.NewStore(root, nil)
	created, err := store.Create(project.CreateInput{Name: "Ops A"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	tl := NewProjectStateUpdateTool(store)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{
		"project_id":"`+created.ID+`",
		"goal":"Keep the release train moving",
		"phase":"executing",
		"status":"active",
		"next_action":"Review latest checkpoint",
		"remaining_tasks":["summarize blockers","plan next run"]
	}`))
	if err != nil {
		t.Fatalf("execute project_state_update: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %s", result.Text())
	}

	get := NewProjectStateGetTool(store)
	stateRes, err := get.Execute(context.Background(), json.RawMessage(`{"project_id":"`+created.ID+`"}`))
	if err != nil {
		t.Fatalf("execute project_state_get: %v", err)
	}
	if !strings.Contains(stateRes.Text(), "Review latest checkpoint") {
		t.Fatalf("expected next_action in state get result, got %s", stateRes.Text())
	}
}
