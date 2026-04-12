package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

func TestRuntime_ExecutionSemaphoreKeepsSecondRunAcceptedUntilSlotFree(t *testing.T) {
	store := session.NewStore(t.TempDir())
	started := make(chan string, 2)
	release := make(chan struct{})
	executor, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
		Name:        "worker",
		Description: "worker",
		PolicyMode:  "allowlist",
		ToolsAllow:  []string{"read_file"},
		RunPrompt: func(_ context.Context, runLabel string, _ string, _ []string, _ string, _ *ProviderOverride) (string, error) {
			started <- runLabel
			<-release
			return "ok", nil
		},
	})
	if err != nil {
		t.Fatalf("new prompt executor: %v", err)
	}
	rt := NewRuntime(RuntimeOptions{
		Enabled:                    true,
		SessionStore:               store,
		Executors:                  []AgentExecutor{executor},
		DefaultAgent:               "worker",
		GatewaySubagentsMaxThreads: 1,
	})
	t.Cleanup(func() { closeGatewayRuntime(t, rt) })

	runA, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "first", Agent: "worker"})
	if err != nil {
		t.Fatalf("spawn first: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected first run to start")
	}
	runB, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "second", Agent: "worker"})
	if err != nil {
		t.Fatalf("spawn second: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	currentB, ok := rt.Get(runB.ID)
	if !ok {
		t.Fatalf("expected second run to exist")
	}
	if currentB.Status != RunStatusAccepted {
		t.Fatalf("expected second run to remain accepted while semaphore is held, got %+v", currentB)
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := rt.Wait(ctx, runA.ID); err != nil {
		t.Fatalf("wait first: %v", err)
	}
	if _, err := rt.Wait(ctx, runB.ID); err != nil {
		t.Fatalf("wait second: %v", err)
	}
}
