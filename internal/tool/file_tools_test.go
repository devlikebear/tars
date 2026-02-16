package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteTool_WritesFileWithinWorkspace(t *testing.T) {
	root := t.TempDir()
	t1 := NewWriteTool(root)
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"path":"notes/a.txt","content":"hello"}`))
	if err != nil {
		t.Fatalf("execute write: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got %s", res.Text())
	}
	body, err := os.ReadFile(filepath.Join(root, "notes", "a.txt"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("unexpected content: %q", string(body))
	}
}

func TestWriteTool_RejectsWorkspaceEscape(t *testing.T) {
	root := t.TempDir()
	t1 := NewWriteTool(root)
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"path":"../escape.txt","content":"x"}`))
	if err != nil {
		t.Fatalf("execute write: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error for escaped path")
	}
}

func TestEditTool_ReplacesSingleOccurrence(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a.txt")
	if err := os.WriteFile(p, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	t1 := NewEditTool(root)
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"path":"a.txt","old_text":"world","new_text":"tars"}`))
	if err != nil {
		t.Fatalf("execute edit: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got %s", res.Text())
	}
	body, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(body) != "hello tars" {
		t.Fatalf("unexpected content: %q", string(body))
	}
}

func TestEditTool_ErrorsWhenOldTextNotFound(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	t1 := NewEditTool(root)
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"path":"a.txt","old_text":"x","new_text":"y"}`))
	if err != nil {
		t.Fatalf("execute edit: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected edit error")
	}
}

func TestGlobTool_ReturnsMatches(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "b.md"), []byte("b"), 0o644)
	t1 := NewGlobTool(root)
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"pattern":"*.txt"}`))
	if err != nil {
		t.Fatalf("execute glob: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got %s", res.Text())
	}
	if !strings.Contains(res.Text(), "a.txt") {
		t.Fatalf("expected txt match, got %s", res.Text())
	}
}
