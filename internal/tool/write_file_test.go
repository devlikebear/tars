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

func TestWriteFileTool_NormalizesAllowedRootPrefixedRelativePath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	artifactDir := filepath.Join(root, "artifacts", "sess-1")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("mkdir artifact dir: %v", err)
	}

	tl := NewWriteFileToolWithPolicy(NewPathPolicy(root, []string{artifactDir}, artifactDir))
	result, err := tl.Execute(context.Background(), json.RawMessage(`{
		"path":"workspace/artifacts/sess-1/report.md",
		"content":"saved"
	}`))
	if err != nil {
		t.Fatalf("execute write_file: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", result.Text())
	}

	expectedPath := filepath.Join(root, "artifacts", "sess-1", "report.md")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected normalized file at %s: %v", expectedPath, err)
	}

	wrongNestedPath := filepath.Join(root, "artifacts", "sess-1", "workspace", "artifacts", "sess-1", "report.md")
	if _, err := os.Stat(wrongNestedPath); !os.IsNotExist(err) {
		t.Fatalf("expected no nested workspace path, stat err=%v", err)
	}
}
