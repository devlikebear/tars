package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/session"
)

func TestProjectTaskRunner_StartAndWaitUsesSelectedWorkerKind(t *testing.T) {
	codexExecutor, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
		Name: "codex-cli",
		RunPrompt: func(_ context.Context, _ string, prompt string, _ []string) (string, error) {
			return "codex:" + prompt, nil
		},
	})
	if err != nil {
		t.Fatalf("new codex executor: %v", err)
	}
	claudeExecutor, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
		Name: "claude-code",
		RunPrompt: func(_ context.Context, _ string, prompt string, _ []string) (string, error) {
			return "claude:" + prompt, nil
		},
	})
	if err != nil {
		t.Fatalf("new claude executor: %v", err)
	}

	runtime := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: session.NewStore(t.TempDir()),
		Executors:    []AgentExecutor{codexExecutor, claudeExecutor},
		DefaultAgent: "codex-cli",
		Now: func() time.Time {
			return time.Date(2026, 3, 14, 14, 0, 0, 0, time.UTC)
		},
	})
	t.Cleanup(func() { closeGatewayRuntime(t, runtime) })

	runner := NewProjectTaskRunner(runtime, "")
	run, err := runner.Start(context.Background(), project.TaskRunRequest{
		ProjectID:  "proj_demo",
		TaskID:     "task-1",
		Title:      "Implement worker integration",
		Prompt:     "do the task",
		Role:       "developer",
		WorkerKind: project.WorkerKindCodexCLI,
	})
	if err != nil {
		t.Fatalf("start task run: %v", err)
	}
	if run.WorkerKind != project.WorkerKindCodexCLI {
		t.Fatalf("expected worker kind %q, got %+v", project.WorkerKindCodexCLI, run)
	}

	final, err := runner.Wait(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("wait task run: %v", err)
	}
	if final.Status != project.TaskRunStatusCompleted {
		t.Fatalf("expected completed status, got %+v", final)
	}
	if final.WorkerKind != project.WorkerKindCodexCLI {
		t.Fatalf("expected worker kind %q after wait, got %+v", project.WorkerKindCodexCLI, final)
	}
	if final.Agent != project.WorkerKindCodexCLI {
		t.Fatalf("expected gateway agent name %q, got %+v", project.WorkerKindCodexCLI, final)
	}
}

func TestProjectTaskRunner_StartFallsBackToDefaultAgentWhenWorkerAliasIsMissing(t *testing.T) {
	runtime := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: session.NewStore(t.TempDir()),
		DefaultAgent: "default",
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return "default:" + prompt, nil
		},
		Now: func() time.Time {
			return time.Date(2026, 3, 14, 14, 30, 0, 0, time.UTC)
		},
	})
	t.Cleanup(func() { closeGatewayRuntime(t, runtime) })

	runner := NewProjectTaskRunner(runtime, "")
	run, err := runner.Start(context.Background(), project.TaskRunRequest{
		ProjectID:  "proj_demo",
		TaskID:     "task-1",
		Title:      "Implement worker integration",
		Prompt:     "do the task",
		Role:       "developer",
		WorkerKind: project.WorkerKindCodexCLI,
	})
	if err != nil {
		t.Fatalf("expected default-agent fallback, got %v", err)
	}
	if run.Agent != "default" {
		t.Fatalf("expected default agent fallback, got %+v", run)
	}
	if run.WorkerKind != "default" {
		t.Fatalf("expected fallback worker kind to reflect actual agent, got %+v", run)
	}

	final, err := runner.Wait(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("wait task run: %v", err)
	}
	if final.Agent != "default" || final.WorkerKind != "default" {
		t.Fatalf("expected default agent on wait result, got %+v", final)
	}
}
