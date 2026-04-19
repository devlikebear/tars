package logwatcher

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestFileTail_ReturnsLastNLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteString("line-")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteByte('\n')
	}
	writeFile(t, path, sb.String())

	tl := newFileTailTool()
	res, err := tl.Execute(context.Background(), json.RawMessage(`{"path":"`+path+`","tail":10}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Text())
	}
	var out struct {
		Lines     []string `json:"lines"`
		Truncated bool     `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(res.Text()), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Lines) != 10 {
		t.Fatalf("expected 10 lines got %d", len(out.Lines))
	}
	if out.Lines[len(out.Lines)-1] != "line-499" {
		t.Fatalf("last line mismatch: %q", out.Lines[len(out.Lines)-1])
	}
	if !out.Truncated {
		t.Fatalf("expected truncated=true")
	}
}

func TestFileTail_GrepFilter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	writeFile(t, path, "info hello\nerror boom\nwarn mild\nerror again\n")

	tl := newFileTailTool()
	res, err := tl.Execute(context.Background(), json.RawMessage(`{"path":"`+path+`","tail":10,"grep":"error"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var out struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal([]byte(res.Text()), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Lines) != 2 {
		t.Fatalf("expected 2 error lines, got %d: %v", len(out.Lines), out.Lines)
	}
	for _, line := range out.Lines {
		if !strings.Contains(line, "error") {
			t.Fatalf("line missing grep: %q", line)
		}
	}
}

func TestFileTail_MissingFile(t *testing.T) {
	tl := newFileTailTool()
	res, _ := tl.Execute(context.Background(), json.RawMessage(`{"path":"/does/not/exist.log"}`))
	if !res.IsError {
		t.Fatalf("expected error")
	}
	if !strings.Contains(res.Text(), "file not found") {
		t.Fatalf("unexpected message: %s", res.Text())
	}
}

func TestFileTail_RejectsMissingPath(t *testing.T) {
	tl := newFileTailTool()
	res, _ := tl.Execute(context.Background(), json.RawMessage(`{}`))
	if !res.IsError || !strings.Contains(res.Text(), "path is required") {
		t.Fatalf("unexpected: %s", res.Text())
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
