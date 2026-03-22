package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/secrets"
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

func TestReadFileTool_DefaultsToLinePagination(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "long.txt")
	lines := make([]string, 0, 5000)
	for i := 1; i <= 5000; i++ {
		lines = append(lines, "line-"+strconv.Itoa(i))
	}
	if err := os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tl := NewReadFileTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"path":"long.txt"}`))
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
	if body.TotalLines != 5000 {
		t.Fatalf("expected total_lines=5000, got %d", body.TotalLines)
	}
	if body.StartLine != 1 || body.EndLine != 2000 {
		t.Fatalf("expected lines 1-2000, got %d-%d", body.StartLine, body.EndLine)
	}
	if body.NextOffset != 2001 {
		t.Fatalf("expected next_offset=2001, got %d", body.NextOffset)
	}
	if !strings.Contains(body.Content, "line-1") || !strings.Contains(body.Content, "line-2000") {
		t.Fatalf("expected paged content to include first 2000 lines")
	}
	if strings.Contains(body.Content, "line-2001") {
		t.Fatalf("expected paged content to stop before line-2001")
	}
	if !strings.Contains(body.Message, "Showing lines 1-2000 of 5000 total lines") {
		t.Fatalf("expected truncation guidance, got %q", body.Message)
	}
}

func TestReadFileTool_SupportsExplicitLineRanges(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "range.txt")
	if err := os.WriteFile(filePath, []byte("a\nb\nc\nd\ne\nf\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tl := NewReadFileTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"path":"range.txt","start_line":2,"end_line":4}`))
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
	if body.StartLine != 2 || body.EndLine != 4 {
		t.Fatalf("expected lines 2-4, got %d-%d", body.StartLine, body.EndLine)
	}
	if strings.TrimSpace(body.Content) != "b\nc\nd" {
		t.Fatalf("unexpected ranged content: %q", body.Content)
	}
}

func TestReadFileTool_RejectsOversizedFiles(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "huge.txt")
	fh, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := fh.Truncate(21 * 1024 * 1024); err != nil {
		_ = fh.Close()
		t.Fatalf("truncate file: %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	tl := NewReadFileTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"path":"huge.txt"}`))
	if err != nil {
		t.Fatalf("execute read_file: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected oversize read to fail")
	}

	var body readFileResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !strings.Contains(body.Message, "20MB") {
		t.Fatalf("expected oversize message, got %q", body.Message)
	}
}

func TestReadFileTool_ShortensLongLines(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "wide.txt")
	longLine := strings.Repeat("a", 2500)
	if err := os.WriteFile(filePath, []byte(longLine), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tl := NewReadFileTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"path":"wide.txt"}`))
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
	if !strings.Contains(body.Content, strings.Repeat("a", 2000)+"... [truncated]") {
		t.Fatalf("expected long line to be shortened, got %q", body.Content)
	}
	if !strings.Contains(body.Message, "Some lines were shortened to 2000 characters") {
		t.Fatalf("expected long-line message, got %q", body.Message)
	}
}

func TestReadFileTool_RedactsSecretLikeContent(t *testing.T) {
	secrets.ResetForTests()
	root := t.TempDir()
	filePath := filepath.Join(root, ".env")
	secretValue := "sk_test_secret_value_1234567890"
	if err := os.WriteFile(filePath, []byte("OPENAI_API_KEY="+secretValue+"\n"), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	secrets.RegisterNamed("OPENAI_API_KEY", secretValue)

	tl := NewReadFileTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"path":".env"}`))
	if err != nil {
		t.Fatalf("execute read_file: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", result.Text())
	}
	if got := result.Text(); got == "" || got == "{}" {
		t.Fatalf("unexpected empty result: %q", got)
	}
	if body := result.Text(); filepath.Base(filePath) == ".env" && strings.Contains(body, secretValue) {
		t.Fatalf("expected secret to be redacted, got %q", body)
	}
}
