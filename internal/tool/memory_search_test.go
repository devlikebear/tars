package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tarsncase/internal/memory"
)

func TestMemorySearchTool_QueryAndMetadata(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	memPath := filepath.Join(root, "MEMORY.md")
	if err := os.WriteFile(memPath, []byte("# MEMORY\n- Coffee preference is long-term.\n"), 0o644); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	dailyPath := filepath.Join(root, "memory", "2026-02-14.md")
	if err := os.WriteFile(dailyPath, []byte("coffee run note\n"), 0o644); err != nil {
		t.Fatalf("write daily: %v", err)
	}
	_ = os.Chtimes(memPath, time.Date(2026, 2, 13, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 13, 0, 0, 0, 0, time.UTC))
	_ = os.Chtimes(dailyPath, time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC))

	tl := NewMemorySearchTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"coffee","limit":5}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}

	var payload struct {
		Results []struct {
			Source  string `json:"source"`
			Date    string `json:"date"`
			Line    int    `json:"line"`
			Snippet string `json:"snippet"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.Text()), &payload); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(payload.Results) == 0 {
		t.Fatalf("expected search results")
	}
	first := payload.Results[0]
	if first.Source != "memory/2026-02-14.md" {
		t.Fatalf("expected latest file first, got source=%q", first.Source)
	}
	if first.Date == "" || first.Line <= 0 || strings.TrimSpace(first.Snippet) == "" {
		t.Fatalf("expected metadata fields, got %+v", first)
	}
}

func TestMemorySearchTool_LimitCap(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	lines := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		lines = append(lines, "capword line")
	}
	path := filepath.Join(root, "memory", "2026-02-14.md")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write daily file: %v", err)
	}

	tl := NewMemorySearchTool(root)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"capword","limit":100,"include_memory":false,"include_daily":true}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload struct {
		Results []any `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.Text()), &payload); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(payload.Results) != 30 {
		t.Fatalf("expected capped 30 results, got %d", len(payload.Results))
	}
}

func TestMemorySearchTool_IncludeFlags(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte("alpha memory only\n"), 0o644); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory", "2026-02-14.md"), []byte("alpha daily only\n"), 0o644); err != nil {
		t.Fatalf("write daily: %v", err)
	}

	tl := NewMemorySearchTool(root)

	onlyMemory, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"alpha","include_memory":true,"include_daily":false}`))
	if err != nil {
		t.Fatalf("execute memory-only: %v", err)
	}
	if !strings.Contains(onlyMemory.Text(), `"source":"MEMORY.md"`) {
		t.Fatalf("expected memory-only result, got %q", onlyMemory.Text())
	}
	if strings.Contains(onlyMemory.Text(), `"source":"memory/`) {
		t.Fatalf("did not expect daily source in memory-only result, got %q", onlyMemory.Text())
	}

	onlyDaily, err := tl.Execute(context.Background(), json.RawMessage(`{"query":"alpha","include_memory":false,"include_daily":true}`))
	if err != nil {
		t.Fatalf("execute daily-only: %v", err)
	}
	if !strings.Contains(onlyDaily.Text(), `"source":"memory/2026-02-14.md"`) {
		t.Fatalf("expected daily-only result, got %q", onlyDaily.Text())
	}
	if strings.Contains(onlyDaily.Text(), `"source":"MEMORY.md"`) {
		t.Fatalf("did not expect MEMORY.md source in daily-only result, got %q", onlyDaily.Text())
	}
}
