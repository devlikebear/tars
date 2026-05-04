package agentruntime

import (
	"context"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

// TestSpawn_PreservesTaskID confirms that SpawnRequest.TaskID is forwarded
// to the resulting Run.TaskID. This is the read-only metadata link that lets
// UI consumers correlate live run state with the task that triggered it.
func TestSpawn_PreservesTaskID(t *testing.T) {
	store := session.NewStore(t.TempDir())
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return "ok: " + prompt, nil
		},
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	run, err := rt.Spawn(context.Background(), SpawnRequest{
		Prompt: "do thing",
		TaskID: "task_abc",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if run.TaskID != "task_abc" {
		t.Fatalf("Run.TaskID = %q, want %q", run.TaskID, "task_abc")
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	final, err := rt.Wait(waitCtx, run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if final.TaskID != "task_abc" {
		t.Fatalf("final Run.TaskID = %q (must persist through completion), want %q", final.TaskID, "task_abc")
	}
}

// TestSpawn_EmptyTaskID covers the legacy / unspecified case so existing
// callers that omit TaskID continue to produce runs with no link.
func TestSpawn_EmptyTaskID(t *testing.T) {
	store := session.NewStore(t.TempDir())
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return "ok: " + prompt, nil
		},
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	run, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "no link"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if run.TaskID != "" {
		t.Fatalf("Run.TaskID should be empty when SpawnRequest.TaskID is unset, got %q", run.TaskID)
	}
}
