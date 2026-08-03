package agentruntime

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

func TestRuntimePropagatesDurableExecutionRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("canonical root: %v", err)
	}
	observed := make(chan string, 1)
	executor, err := NewPromptExecutor("worker", "", func(ctx context.Context, _ string, _ string) (string, error) {
		observed <- ExecutionRootFromContext(ctx)
		return "done", nil
	})
	if err != nil {
		t.Fatalf("new prompt executor: %v", err)
	}
	runtime := NewRuntime(RuntimeOptions{
		Enabled: true, SessionStore: session.NewStore(t.TempDir()),
		Executors: []AgentExecutor{executor}, DefaultAgent: "worker", WorkspaceDir: t.TempDir(),
	})
	t.Cleanup(func() { closeAgentRuntime(t, runtime) })
	run, err := runtime.Spawn(context.Background(), SpawnRequest{Prompt: "work", ExecutionRoot: root})
	if err != nil {
		t.Fatalf("spawn execution-root run: %v", err)
	}
	if run.ExecutionRoot != filepath.Clean(canonicalRoot) {
		t.Fatalf("accepted execution root = %q", run.ExecutionRoot)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	final, err := runtime.Wait(waitCtx, run.ID)
	if err != nil || final.Status != RunStatusCompleted {
		t.Fatalf("wait run=%#v err=%v", final, err)
	}
	if got := <-observed; got != filepath.Clean(canonicalRoot) {
		t.Fatalf("executor execution root = %q", got)
	}
}

func TestRuntimeRejectsUnavailableExecutionRoot(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime(RuntimeOptions{Enabled: true, SessionStore: session.NewStore(t.TempDir())})
	t.Cleanup(func() { closeAgentRuntime(t, runtime) })
	if _, err := runtime.Spawn(context.Background(), SpawnRequest{Prompt: "work", ExecutionRoot: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("spawn accepted an unavailable execution root")
	}
}
