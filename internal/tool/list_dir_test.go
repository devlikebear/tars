package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestListDirTool_ListsEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tl := NewListDirTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"path":".","max_entries":10}`))
	if err != nil {
		t.Fatalf("execute list_dir: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got %s", result.Text())
	}

	var body listDirResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if body.Count != 2 {
		t.Fatalf("expected 2 entries, got %d", body.Count)
	}
}

func TestListDirTool_RecursiveAndLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a", "b"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "a", "b", "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write nested file: %v", err)
	}

	tl := NewListDirTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"path":"a","recursive":true,"max_entries":1}`))
	if err != nil {
		t.Fatalf("execute list_dir: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got %s", result.Text())
	}

	var body listDirResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !body.Truncated {
		t.Fatalf("expected truncated=true")
	}
	if body.Count != 1 {
		t.Fatalf("expected count=1, got %d", body.Count)
	}
}

func TestListDirTool_RejectsTraversal(t *testing.T) {
	root := t.TempDir()
	tl := NewListDirTool(root)

	result, err := tl.Execute(context.Background(), json.RawMessage(`{"path":"../"}`))
	if err != nil {
		t.Fatalf("execute list_dir: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected traversal error, got %s", result.Text())
	}
}
