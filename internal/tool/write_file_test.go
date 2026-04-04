package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileTool_WritesWorkspaceFileAtomically(t *testing.T) {
	root := t.TempDir()
	tl := NewWriteFileTool(root)

	result, err := tl.Execute(context.Background(), json.RawMessage(`{
		"path":"profiles/USER.md",
		"content":"# USER.md\n\n- prefers Korean\n"
	}`))
	if err != nil {
		t.Fatalf("execute write_file: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", result.Text())
	}

	var body writeFileResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if body.Path != "profiles/USER.md" {
		t.Fatalf("expected path profiles/USER.md, got %q", body.Path)
	}
	if !body.Created {
		t.Fatalf("expected created=true")
	}

	raw, err := os.ReadFile(filepath.Join(root, "profiles", "USER.md"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(raw) != "# USER.md\n\n- prefers Korean\n" {
		t.Fatalf("unexpected file content: %q", string(raw))
	}
}

func TestWriteFileTool_RejectsTraversal(t *testing.T) {
	root := t.TempDir()
	tl := NewWriteFileTool(root)

	result, err := tl.Execute(context.Background(), json.RawMessage(`{
		"path":"../outside.txt",
		"content":"blocked"
	}`))
	if err != nil {
		t.Fatalf("execute write_file: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected traversal error, got %s", result.Text())
	}

	var body writeFileResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !strings.Contains(strings.ToLower(body.Message), "workspace") {
		t.Fatalf("expected outside workspace message, got %q", body.Message)
	}
}
