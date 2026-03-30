package project

import (
	"strings"
	"testing"
	"time"
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

	t.Run("default worker alias resolves", func(t *testing.T) {
		profile, err := ResolveWorkerProfile(BoardTask{
			Role:       "developer",
			WorkerKind: "default",
		})
		if err != nil {
			t.Fatalf("resolve worker profile: %v", err)
		}
		if profile.Kind != "default" || profile.ExecutorName != "default" {
			t.Fatalf("expected default alias profile, got %+v", profile)
		}
	})

	t.Run("unknown worker kind fails", func(t *testing.T) {
		if _, err := ResolveWorkerProfile(BoardTask{WorkerKind: "unknown-cli"}); err == nil {
			t.Fatal("expected unknown worker kind error")
		}
	})
}

func TestResolveWorkerProfileForProject_UsesWorkflowProfileDefaultsAndOverrides(t *testing.T) {
	t.Run("research defaults to runtime worker", func(t *testing.T) {
		project := Project{WorkflowProfile: "research"}
		profile, err := ResolveWorkerProfileForProject(project, BoardTask{Role: "developer"})
		if err != nil {
			t.Fatalf("resolve worker profile: %v", err)
		}
		if profile.Kind != WorkerKindDefault {
			t.Fatalf("expected %q, got %+v", WorkerKindDefault, profile)
		}
	})

	t.Run("research reviewer also defaults to runtime worker", func(t *testing.T) {
		project := Project{WorkflowProfile: "research"}
		profile, err := ResolveWorkerProfileForProject(project, BoardTask{Role: "reviewer"})
		if err != nil {
			t.Fatalf("resolve worker profile: %v", err)
		}
		if profile.Kind != WorkerKindDefault {
			t.Fatalf("expected %q, got %+v", WorkerKindDefault, profile)
		}
	})

	t.Run("workflow rule overrides default worker kind", func(t *testing.T) {
		project := Project{
			WorkflowProfile: "research",
			WorkflowRules: []WorkflowRule{
				{Name: "worker_kind", Params: map[string]string{"role": "developer", "kind": WorkerKindClaudeCode}},
			},
		}
		profile, err := ResolveWorkerProfileForProject(project, BoardTask{Role: "developer"})
		if err != nil {
			t.Fatalf("resolve worker profile: %v", err)
		}
		if profile.Kind != WorkerKindClaudeCode {
			t.Fatalf("expected %q, got %+v", WorkerKindClaudeCode, profile)
		}
	})
}

func TestResolveWorkflowRuntimePolicy_UsesDefaultsAndOverrides(t *testing.T) {
	t.Run("defaults remain unchanged without rules", func(t *testing.T) {
		policy := ResolveWorkflowRuntimePolicy(Project{})
		if policy.PlanningBlockTimeout != defaultPlanningBlockTimeout {
			t.Fatalf("expected default planning timeout %s, got %s", defaultPlanningBlockTimeout, policy.PlanningBlockTimeout)
		}
		if policy.RunRetention != defaultRunRetention {
			t.Fatalf("expected default run retention %s, got %s", defaultRunRetention, policy.RunRetention)
		}
	})

	t.Run("workflow rules override runtime settings", func(t *testing.T) {
		policy := ResolveWorkflowRuntimePolicy(Project{
			WorkflowRules: []WorkflowRule{
				{Name: "planning_block_timeout", Params: map[string]string{"duration": "45m"}},
				{Name: "run_retention", Params: map[string]string{"duration": "72h"}},
			},
		})
		if policy.PlanningBlockTimeout != 45*time.Minute {
			t.Fatalf("expected planning timeout override, got %s", policy.PlanningBlockTimeout)
		}
		if policy.RunRetention != 72*time.Hour {
			t.Fatalf("expected run retention override, got %s", policy.RunRetention)
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
