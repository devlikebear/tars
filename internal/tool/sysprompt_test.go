package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/memory"
)

func TestWorkspaceSyspromptGetTool_ListsWorkspacePromptFiles(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	tl := NewWorkspaceSyspromptGetTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute workspace_sysprompt_get: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", result.Text())
	}
	if !strings.Contains(result.Text(), `"path":"USER.md"`) || !strings.Contains(result.Text(), `"path":"IDENTITY.md"`) {
		t.Fatalf("expected USER.md and IDENTITY.md in result, got %s", result.Text())
	}
}

func TestWorkspaceSyspromptSetTool_UpdatesUserIdentityFile(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	tl := NewWorkspaceSyspromptSetTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{
		"file":"USER.md",
		"content":"# USER.md\n\n- Name: Chris\n- Timezone: Asia/Seoul\n"
	}`))
	if err != nil {
		t.Fatalf("execute workspace_sysprompt_set: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", result.Text())
	}

	raw, err := os.ReadFile(filepath.Join(root, "USER.md"))
	if err != nil {
		t.Fatalf("read USER.md: %v", err)
	}
	if !strings.Contains(string(raw), "Timezone: Asia/Seoul") {
		t.Fatalf("expected updated USER.md, got %q", string(raw))
	}
}

func TestAgentSyspromptSetTool_RejectsWorkspaceOnlyFiles(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	tl := NewAgentSyspromptSetTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{
		"file":"USER.md",
		"content":"blocked"
	}`))
	if err != nil {
		t.Fatalf("execute agent_sysprompt_set: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected agent tool to reject USER.md")
	}
	if !strings.Contains(strings.ToLower(result.Text()), "invalid arguments") && !strings.Contains(strings.ToLower(result.Text()), "not exist") {
		t.Fatalf("expected invalid file error, got %s", result.Text())
	}
}
