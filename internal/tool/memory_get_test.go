package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/memory"
)

func TestMemoryGetTool_DailyDefaultDate(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	path := filepath.Join(root, "memory", "2026-02-14.md")
	if err := os.WriteFile(path, []byte("today memory\n"), 0o644); err != nil {
		t.Fatalf("write daily file: %v", err)
	}

	tl := newMemoryGetTool(root, func() time.Time { return now })
	result, err := tl.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %+v", result)
	}
	if !strings.Contains(result.Text(), "today memory") {
		t.Fatalf("expected daily content, got %q", result.Text())
	}
}

func TestMemoryGetTool_SpecificDateAndMissingFile(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	path := filepath.Join(root, "memory", "2026-02-13.md")
	if err := os.WriteFile(path, []byte("yesterday memory\n"), 0o644); err != nil {
		t.Fatalf("write daily file: %v", err)
	}

	tl := NewMemoryGetTool(root)

	hit, err := tl.Execute(context.Background(), json.RawMessage(`{"target":"daily","date":"2026-02-13"}`))
	if err != nil {
		t.Fatalf("execute existing date: %v", err)
	}
	if !strings.Contains(hit.Text(), "yesterday memory") {
		t.Fatalf("expected specific date content, got %q", hit.Text())
	}

	miss, err := tl.Execute(context.Background(), json.RawMessage(`{"target":"daily","date":"2026-02-12"}`))
	if err != nil {
		t.Fatalf("execute missing date: %v", err)
	}
	if miss.IsError {
		t.Fatalf("missing file should not be error")
	}
	if !strings.Contains(miss.Text(), "no daily memory found for 2026-02-12") {
		t.Fatalf("expected missing file message, got %q", miss.Text())
	}
}

func TestMemoryGetTool_InvalidDateFormat(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	tl := NewMemoryGetTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"target":"daily","date":"2026/02/14"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected is_error=true for invalid date format")
	}
	if !strings.Contains(result.Text(), "invalid date format") {
		t.Fatalf("expected invalid date message, got %q", result.Text())
	}
}

func TestMemoryGetTool_ExperiencesTarget(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := memory.AppendExperience(root, memory.Experience{
		Timestamp:     time.Date(2026, 2, 22, 9, 0, 0, 0, time.UTC),
		Category:      "preference",
		Summary:       "User prefers concise responses",
		Tags:          []string{"user", "style"},
		SourceSession: "sess-1",
		Importance:    7,
	}); err != nil {
		t.Fatalf("append experience: %v", err)
	}
	if err := memory.AppendExperience(root, memory.Experience{
		Timestamp:     time.Date(2026, 2, 22, 9, 1, 0, 0, time.UTC),
		Category:      "task_completed",
		Summary:       "Completed gateway report review",
		Tags:          []string{"gateway"},
		SourceSession: "sess-1",
		Importance:    8,
	}); err != nil {
		t.Fatalf("append experience: %v", err)
	}

	tl := NewMemoryGetTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"target":"experiences","query":"gateway","limit":5}`))
	if err != nil {
		t.Fatalf("execute experiences query: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %+v", result)
	}
	if !strings.Contains(result.Text(), "Completed gateway report review") {
		t.Fatalf("expected matched experience in output, got %q", result.Text())
	}
}
