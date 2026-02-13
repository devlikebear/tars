package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")

	if err := EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	paths := []string{
		root,
		filepath.Join(root, "memory"),
		filepath.Join(root, "_shared"),
		filepath.Join(root, "HEARTBEAT.md"),
		filepath.Join(root, "MEMORY.md"),
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}

	heartbeat, err := os.ReadFile(filepath.Join(root, "HEARTBEAT.md"))
	if err != nil {
		t.Fatalf("read HEARTBEAT.md: %v", err)
	}
	if !strings.Contains(string(heartbeat), "Heartbeat Guidance") {
		t.Fatalf("expected default HEARTBEAT template, got %q", string(heartbeat))
	}

	memoryFile, err := os.ReadFile(filepath.Join(root, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read MEMORY.md: %v", err)
	}
	if !strings.Contains(string(memoryFile), "Long-Term Memory") {
		t.Fatalf("expected default MEMORY template, got %q", string(memoryFile))
	}
}

func TestEnsureWorkspace_DoesNotOverwriteExistingFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	customHeartbeat := "custom heartbeat"
	customMemory := "custom memory"
	if err := os.WriteFile(filepath.Join(root, "HEARTBEAT.md"), []byte(customHeartbeat), 0o644); err != nil {
		t.Fatalf("write HEARTBEAT.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte(customMemory), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}

	if err := EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	heartbeat, err := os.ReadFile(filepath.Join(root, "HEARTBEAT.md"))
	if err != nil {
		t.Fatalf("read HEARTBEAT.md: %v", err)
	}
	if string(heartbeat) != customHeartbeat {
		t.Fatalf("expected existing HEARTBEAT.md to remain unchanged, got %q", string(heartbeat))
	}

	memoryFile, err := os.ReadFile(filepath.Join(root, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read MEMORY.md: %v", err)
	}
	if string(memoryFile) != customMemory {
		t.Fatalf("expected existing MEMORY.md to remain unchanged, got %q", string(memoryFile))
	}
}

func TestAppendDailyLog(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 2, 13, 10, 30, 0, 0, time.UTC)

	if err := AppendDailyLog(root, now, "first"); err != nil {
		t.Fatalf("append first: %v", err)
	}
	if err := AppendDailyLog(root, now, "second"); err != nil {
		t.Fatalf("append second: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "memory", "2026-02-13.md"))
	if err != nil {
		t.Fatalf("read daily log: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "first") || !strings.Contains(content, "second") {
		t.Fatalf("unexpected daily log content: %q", content)
	}
}
