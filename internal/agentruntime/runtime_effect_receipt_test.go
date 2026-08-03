package agentruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/workstore"
)

func TestRuntimeCommitsToolResultAndEffectReceiptAtSafeBoundary(t *testing.T) {
	ctx := context.Background()
	ledger := openRuntimeEffectLedger(t)
	work, step := createRuntimeEffectWork(t, ledger, "committed-effect")
	effects := 0
	executor := effectTestExecutor(t, func(ctx context.Context) error {
		recorder := RuntimeExecutionRecorderFromContext(ctx)
		if recorder == nil {
			t.Fatal("runtime execution recorder missing")
		}
		call := RuntimeToolCall{
			Phase: RuntimeToolPhaseBefore, Iteration: 1, ToolName: "send_message",
			ToolCallID: "call-1", ToolArgs: `{"channel":"ops","text":"hello"}`,
			ToolEffectClass: "unsafe",
		}
		if err := recorder(call); err != nil {
			return err
		}
		effects++
		call.Phase = RuntimeToolPhaseAfter
		call.ToolResult = `{"message_id":"msg-42"}`
		return recorder(call)
	})
	rt := newRuntimeWithEffectLedger(t, ledger, executor)
	run, err := rt.Spawn(ctx, SpawnRequest{Prompt: "send", WorkID: work.ID, StepID: step.ID})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	final := waitRuntimeEffectRun(t, rt, run.ID)
	if effects != 1 || final.Status != RunStatusCompleted {
		t.Fatalf("effects=%d final=%+v", effects, final)
	}
	if len(final.ToolRequests) != 1 || final.ToolRequests[0].Status != ToolRequestStatusCommitted {
		t.Fatalf("tool requests: %+v", final.ToolRequests)
	}
	if len(final.ToolResults) != 1 || final.ToolResults[0].Result != `{"message_id":"msg-42"}` {
		t.Fatalf("tool results: %+v", final.ToolResults)
	}
	if len(final.EffectReceipts) != 1 || final.EffectReceipts[0].Status != EffectReceiptStatusCommitted {
		t.Fatalf("runtime effect receipts: %+v", final.EffectReceipts)
	}
	latest, ok := latestRunCheckpoint(final)
	if !ok || latest.Capability != CheckpointCapabilityReplay || len(latest.EffectReceiptRefs) != 1 || len(latest.ToolResultRefs) != 1 {
		t.Fatalf("latest checkpoint: %+v", latest)
	}
	projection, err := ledger.GetWorkProjection(ctx, work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatalf("projection: %v", err)
	}
	if len(projection.EffectReceipts) != 1 || projection.EffectReceipts[0].Status != workstore.EffectReceiptStatusCommitted {
		t.Fatalf("ledger effect receipts: %+v", projection.EffectReceipts)
	}
}

func TestRuntimeLeavesUnsafeEffectPendingAndRequiresApprovalAfterCrash(t *testing.T) {
	ledger := openRuntimeEffectLedger(t)
	work, step := createRuntimeEffectWork(t, ledger, "pending-effect")
	effects := 0
	executor := effectTestExecutor(t, func(ctx context.Context) error {
		recorder := RuntimeExecutionRecorderFromContext(ctx)
		if err := recorder(RuntimeToolCall{
			Phase: RuntimeToolPhaseBefore, Iteration: 1, ToolName: "send_message",
			ToolCallID: "call-1", ToolArgs: `{"text":"hello"}`, ToolEffectClass: "unsafe",
		}); err != nil {
			return err
		}
		effects++
		return errors.New("process crashed after external effect")
	})
	rt := newRuntimeWithEffectLedger(t, ledger, executor)
	run, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "send", WorkID: work.ID, StepID: step.ID})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	final := waitRuntimeEffectRun(t, rt, run.ID)
	if effects != 1 || len(final.EffectReceipts) != 1 || final.EffectReceipts[0].Status != EffectReceiptStatusPending {
		t.Fatalf("pending runtime effect: effects=%d receipts=%+v", effects, final.EffectReceipts)
	}
	latest, ok := latestRunCheckpoint(final)
	if !ok || !latest.RecoveryApprovalRequired || latest.Capability != CheckpointCapabilityRetryOnly {
		t.Fatalf("unsafe recovery checkpoint: %+v", latest)
	}
	if latest.RecoveryApprovalReason == "" || latest.Resumable {
		t.Fatalf("unsafe recovery explanation: %+v", latest)
	}
	receipt, err := ledger.GetEffectReceipt(context.Background(), work.WorkspaceID, work.ID, final.EffectReceipts[0].IdempotencyKey)
	if err != nil {
		t.Fatalf("get ledger receipt: %v", err)
	}
	if receipt.Status != workstore.EffectReceiptStatusPending {
		t.Fatalf("ledger pending receipt: %+v", receipt)
	}
}

func TestRuntimeDoesNotInvokeEffectWhenPendingReceiptCannotPersist(t *testing.T) {
	ledger := openRuntimeEffectLedger(t)
	executions := 0
	executor := effectTestExecutor(t, func(ctx context.Context) error {
		recorder := RuntimeExecutionRecorderFromContext(ctx)
		if err := recorder(RuntimeToolCall{
			Phase: RuntimeToolPhaseBefore, Iteration: 1, ToolName: "send_message",
			ToolCallID: "call-1", ToolArgs: `{}`, ToolEffectClass: "unsafe",
		}); err != nil {
			return err
		}
		executions++
		return nil
	})
	rt := newRuntimeWithEffectLedger(t, ledger, executor)
	run, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "send", WorkID: "missing-work", StepID: "missing-step"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	final := waitRuntimeEffectRun(t, rt, run.ID)
	if executions != 0 || final.Status != RunStatusFailed {
		t.Fatalf("executions=%d final=%+v", executions, final)
	}
}

func effectTestExecutor(t *testing.T, execute func(context.Context) error) AgentExecutor {
	t.Helper()
	executor, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
		Name:              "effect-worker",
		CheckpointSupport: ExecutorCheckpointSupport{Capability: CheckpointCapabilityReplay},
		RunPrompt: func(ctx context.Context, _ string, _ string, _ []string, _ string, _ *ProviderOverride) (string, error) {
			if err := execute(ctx); err != nil {
				return "", err
			}
			return "done", nil
		},
	})
	if err != nil {
		t.Fatalf("new effect executor: %v", err)
	}
	return executor
}

func newRuntimeWithEffectLedger(t *testing.T, ledger *workstore.Store, executor AgentExecutor) *Runtime {
	t.Helper()
	rt := NewRuntime(RuntimeOptions{
		Enabled: true, SessionStore: session.NewStore(t.TempDir()),
		Executors: []AgentExecutor{executor}, DefaultAgent: "effect-worker",
	})
	rt.SetEffectReceiptStore(ledger)
	t.Cleanup(func() { closeAgentRuntime(t, rt) })
	return rt
}

func openRuntimeEffectLedger(t *testing.T) *workstore.Store {
	t.Helper()
	ledger, err := workstore.Open(context.Background(), t.TempDir()+"/work-ledger.db", workstore.Options{})
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	t.Cleanup(func() { _ = ledger.Close() })
	return ledger
}

func createRuntimeEffectWork(t *testing.T, ledger *workstore.Store, key string) (workstore.Work, workstore.Step) {
	t.Helper()
	work, err := ledger.CreateWork(context.Background(), workstore.CreateWorkInput{
		WorkspaceID: DefaultWorkspaceID, Kind: "agent-run", IdempotencyKey: key,
		Title: key, InitialState: workstore.WorkStateRunning, ActorID: "test",
	})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	step, err := ledger.CreateStep(context.Background(), workstore.CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: key + ":step",
		Title: "execute", State: workstore.WorkStateRunning, ActorID: "test",
	})
	if err != nil {
		t.Fatalf("create step: %v", err)
	}
	return work, step
}

func waitRuntimeEffectRun(t *testing.T, rt *Runtime, runID string) Run {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	run, err := rt.Wait(ctx, runID)
	if err != nil {
		t.Fatalf("wait run: %v", err)
	}
	return run
}
