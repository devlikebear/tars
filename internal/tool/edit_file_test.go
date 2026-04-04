package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditFileTool_ReplacesExactTextInWorkspaceFile(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "IDENTITY.md")
	if err := os.WriteFile(filePath, []byte("# IDENTITY.md\n\n- Tone: neutral\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tl := NewEditFileTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{
		"path":"IDENTITY.md",
		"old_text":"neutral",
		"new_text":"direct"
	}`))
	if err != nil {
		t.Fatalf("execute edit_file: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", result.Text())
	}

	var body editFileResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if body.Path != "IDENTITY.md" {
		t.Fatalf("expected path IDENTITY.md, got %q", body.Path)
	}
	if body.Replacements != 1 {
		t.Fatalf("expected 1 replacement, got %d", body.Replacements)
	}

	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read edited file: %v", err)
	}
	if !strings.Contains(string(raw), "direct") {
		t.Fatalf("expected edited content, got %q", string(raw))
	}
}

func TestEditFileTool_RequiresUniqueMatchUnlessReplaceAll(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "TOOLS.md")
	if err := os.WriteFile(filePath, []byte("write_file\nwrite_file\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tl := NewEditFileTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{
		"path":"TOOLS.md",
		"old_text":"write_file",
		"new_text":"edit_file"
	}`))
	if err != nil {
		t.Fatalf("execute edit_file: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected non-unique replacement to fail")
	}

	var body editFileResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !strings.Contains(body.Message, "replace_all=true") {
		t.Fatalf("expected replace_all guidance, got %q", body.Message)
	}
}
