package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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

func TestWriteTool_RejectsSymlinkedParentEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior is not portable on windows")
	}

	root := t.TempDir()
	outside := t.TempDir()
	linkPath := filepath.Join(root, "linked")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	t1 := NewWriteTool(root)
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"path":"linked/pwned.txt","content":"x"}`))
	if err != nil {
		t.Fatalf("execute write: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected symlinked parent escape to fail")
	}
	if _, err := os.Stat(filepath.Join(outside, "pwned.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected no file to be written outside workspace")
	}
}
