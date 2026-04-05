package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/memory"
)

func TestMemoryKnowledgeTools_CRUD(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	listTool := NewMemoryKBListTool(root)
	getTool := NewMemoryKBGetTool(root)
	upsertTool := NewMemoryKBUpsertTool(root, nil)
	deleteTool := NewMemoryKBDeleteTool(root, nil)

	createResult, err := upsertTool.Execute(context.Background(), json.RawMessage(`{
		"title":"Coffee Preference",
		"kind":"preference",
		"summary":"User prefers black coffee.",
		"body":"Keep coffee suggestions unsweetened.",
		"tags":["coffee","preference"],
		"aliases":["black coffee"],
		"links":[{"target":"morning-routine","relation":"related_to"}]
	}`))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if createResult.IsError || !strings.Contains(createResult.Text(), `"slug":"coffee-preference"`) {
		t.Fatalf("unexpected create result: %+v", createResult)
	}

	listResult, err := listTool.Execute(context.Background(), json.RawMessage(`{"query":"coffee"}`))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(listResult.Text(), "Coffee Preference") {
		t.Fatalf("expected list result to include note title, got %q", listResult.Text())
	}

	getResult, err := getTool.Execute(context.Background(), json.RawMessage(`{"slug":"coffee-preference"}`))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(getResult.Text(), "unsweetened") {
		t.Fatalf("expected get result to include note body, got %q", getResult.Text())
	}

	deleteResult, err := deleteTool.Execute(context.Background(), json.RawMessage(`{"slug":"coffee-preference"}`))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleteResult.IsError || !strings.Contains(deleteResult.Text(), `"deleted":true`) {
		t.Fatalf("unexpected delete result: %+v", deleteResult)
	}
}
