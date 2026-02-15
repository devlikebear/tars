package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileTool_ReadsWorkspaceFile(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(filePath, []byte("hello tool"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tl := NewReadFileTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"path":"notes.txt"}`))
	if err != nil {
		t.Fatalf("execute read_file: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", result.Text())
	}

	var body readFileResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if body.Path != "notes.txt" {
		t.Fatalf("expected path notes.txt, got %q", body.Path)
	}
	if body.Content != "hello tool" {
		t.Fatalf("unexpected content: %q", body.Content)
	}
}

func TestReadFileTool_RejectsTraversal(t *testing.T) {
	root := t.TempDir()
	tl := NewReadFileTool(root)

	result, err := tl.Execute(context.Background(), json.RawMessage(`{"path":"../outside.txt"}`))
	if err != nil {
		t.Fatalf("execute read_file: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for traversal, got %s", result.Text())
	}
}

func TestReadFileTool_TruncatesByMaxBytes(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "long.txt")
	if err := os.WriteFile(filePath, []byte("abcdefghij"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tl := NewReadFileTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"path":"long.txt","max_bytes":4}`))
	if err != nil {
		t.Fatalf("execute read_file: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", result.Text())
	}

	var body readFileResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !body.Truncated {
		t.Fatalf("expected truncated=true")
	}
	if body.Content != "abcd" {
		t.Fatalf("unexpected truncated content: %q", body.Content)
	}
}
