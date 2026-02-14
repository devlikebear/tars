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
		filepath.Join(root, "AGENTS.md"),
		filepath.Join(root, "SOUL.md"),
		filepath.Join(root, "USER.md"),
		filepath.Join(root, "IDENTITY.md"),
		filepath.Join(root, "TOOLS.md"),
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

	agentsFile, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(agentsFile), "Operating Guidelines") {
		t.Fatalf("expected default AGENTS template, got %q", string(agentsFile))
	}

	soulFile, err := os.ReadFile(filepath.Join(root, "SOUL.md"))
	if err != nil {
		t.Fatalf("read SOUL.md: %v", err)
	}
	if !strings.Contains(string(soulFile), "Persona") {
		t.Fatalf("expected default SOUL template, got %q", string(soulFile))
	}

	userFile, err := os.ReadFile(filepath.Join(root, "USER.md"))
	if err != nil {
		t.Fatalf("read USER.md: %v", err)
	}
	if !strings.Contains(string(userFile), "User Profile") {
		t.Fatalf("expected default USER template, got %q", string(userFile))
	}

	identityFile, err := os.ReadFile(filepath.Join(root, "IDENTITY.md"))
	if err != nil {
		t.Fatalf("read IDENTITY.md: %v", err)
	}
	if !strings.Contains(string(identityFile), "Agent Identity") {
		t.Fatalf("expected default IDENTITY template, got %q", string(identityFile))
	}

	toolsFile, err := os.ReadFile(filepath.Join(root, "TOOLS.md"))
	if err != nil {
		t.Fatalf("read TOOLS.md: %v", err)
	}
	if !strings.Contains(string(toolsFile), "Environment Tools") {
		t.Fatalf("expected default TOOLS template, got %q", string(toolsFile))
	}
}

func TestEnsureWorkspace_DoesNotOverwriteExistingFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	customHeartbeat := "custom heartbeat"
	customMemory := "custom memory"
	customAgents := "custom agents"
	customSoul := "custom soul"
	customUser := "custom user"
	customIdentity := "custom identity"
	customTools := "custom tools"
	if err := os.WriteFile(filepath.Join(root, "HEARTBEAT.md"), []byte(customHeartbeat), 0o644); err != nil {
		t.Fatalf("write HEARTBEAT.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte(customMemory), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(customAgents), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "SOUL.md"), []byte(customSoul), 0o644); err != nil {
		t.Fatalf("write SOUL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "USER.md"), []byte(customUser), 0o644); err != nil {
		t.Fatalf("write USER.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "IDENTITY.md"), []byte(customIdentity), 0o644); err != nil {
		t.Fatalf("write IDENTITY.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "TOOLS.md"), []byte(customTools), 0o644); err != nil {
		t.Fatalf("write TOOLS.md: %v", err)
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

	agentsFile, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if string(agentsFile) != customAgents {
		t.Fatalf("expected existing AGENTS.md to remain unchanged, got %q", string(agentsFile))
	}

	soulFile, err := os.ReadFile(filepath.Join(root, "SOUL.md"))
	if err != nil {
		t.Fatalf("read SOUL.md: %v", err)
	}
	if string(soulFile) != customSoul {
		t.Fatalf("expected existing SOUL.md to remain unchanged, got %q", string(soulFile))
	}

	userFile, err := os.ReadFile(filepath.Join(root, "USER.md"))
	if err != nil {
		t.Fatalf("read USER.md: %v", err)
	}
	if string(userFile) != customUser {
		t.Fatalf("expected existing USER.md to remain unchanged, got %q", string(userFile))
	}

	identityFile, err := os.ReadFile(filepath.Join(root, "IDENTITY.md"))
	if err != nil {
		t.Fatalf("read IDENTITY.md: %v", err)
	}
	if string(identityFile) != customIdentity {
		t.Fatalf("expected existing IDENTITY.md to remain unchanged, got %q", string(identityFile))
	}

	toolsFile, err := os.ReadFile(filepath.Join(root, "TOOLS.md"))
	if err != nil {
		t.Fatalf("read TOOLS.md: %v", err)
	}
	if string(toolsFile) != customTools {
		t.Fatalf("expected existing TOOLS.md to remain unchanged, got %q", string(toolsFile))
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
