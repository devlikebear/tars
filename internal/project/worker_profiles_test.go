package project

import (
	"strings"
	"testing"
)

func TestResolveWorkerProfile_UsesExplicitWorkerAndRoleDefaults(t *testing.T) {
	t.Run("developer defaults to codex", func(t *testing.T) {
		profile, err := ResolveWorkerProfile(BoardTask{Role: "developer"})
		if err != nil {
			t.Fatalf("resolve worker profile: %v", err)
		}
		if profile.Kind != WorkerKindCodexCLI {
			t.Fatalf("expected %q, got %+v", WorkerKindCodexCLI, profile)
		}
	})

	t.Run("reviewer defaults to claude", func(t *testing.T) {
		profile, err := ResolveWorkerProfile(BoardTask{Role: "reviewer"})
		if err != nil {
			t.Fatalf("resolve worker profile: %v", err)
		}
		if profile.Kind != WorkerKindClaudeCode {
			t.Fatalf("expected %q, got %+v", WorkerKindClaudeCode, profile)
		}
	})

	t.Run("explicit worker kind wins", func(t *testing.T) {
		profile, err := ResolveWorkerProfile(BoardTask{
			Role:       "developer",
			WorkerKind: WorkerKindClaudeCode,
		})
		if err != nil {
			t.Fatalf("resolve worker profile: %v", err)
		}
		if profile.Kind != WorkerKindClaudeCode {
			t.Fatalf("expected explicit worker kind %q, got %+v", WorkerKindClaudeCode, profile)
		}
	})

	t.Run("unknown worker kind fails", func(t *testing.T) {
		if _, err := ResolveWorkerProfile(BoardTask{WorkerKind: "unknown-cli"}); err == nil {
			t.Fatal("expected unknown worker kind error")
		}
	})
}

func TestBuildTaskPrompt_UsesFixedReportContract(t *testing.T) {
	profile, err := ResolveWorkerProfile(BoardTask{Role: "developer"})
	if err != nil {
		t.Fatalf("resolve worker profile: %v", err)
	}

	prompt := BuildTaskPrompt(BoardTask{
		ID:           "task-1",
		Title:        "Implement board sync",
		Role:         "developer",
		WorkerKind:   WorkerKindCodexCLI,
		TestCommand:  "go test ./internal/project",
		BuildCommand: "go test ./internal/tarsserver",
	}, "proj_demo", profile)

	want := []string{
		"Project ID: proj_demo",
		"Task ID: task-1",
		"worker_kind: codex-cli",
		"Return the final result using this exact format:",
		"<task-report>",
		"status: completed|blocked|needs_review",
		"tests: <what ran and what passed/failed>",
		"build: <what ran and what passed/failed>",
		"</task-report>",
	}
	for _, item := range want {
		if !strings.Contains(prompt, item) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", item, prompt)
		}
	}
}
