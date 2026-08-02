package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/devlikebear/tars/internal/session"
)

func TestRecoveryModesReplayCommittedEffectAndCarrySiblingResults(t *testing.T) {
	callCount := 0
	var replayPlan *RecoveryExecutionPlan
	executor, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
		Name: "worker", CheckpointSupport: ExecutorCheckpointSupport{Capability: CheckpointCapabilityReplay},
		RunPrompt: func(ctx context.Context, _ string, _ string, _ []string, _ string, _ *ProviderOverride) (string, error) {
			callCount++
			if callCount > 1 {
				replayPlan = RecoveryExecutionFromContext(ctx)
				return "recovered", nil
			}
			recorder := RuntimeExecutionRecorderFromContext(ctx)
			first := RuntimeToolCall{Phase: RuntimeToolPhaseBefore, Iteration: 1, ToolName: "write_file", ToolCallID: "sibling-a", ToolArgs: `{"path":"a.txt"}`, ToolEffectClass: "unsafe"}
			if err := recorder(first); err != nil {
				return "", err
			}
			first.Phase = RuntimeToolPhaseAfter
			first.ToolResult = `{"path":"a.txt","written":true}`
			if err := recorder(first); err != nil {
				return "", err
			}
			return "", fmt.Errorf("parallel sibling b failed")
		},
	})
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	rt := NewRuntime(RuntimeOptions{Enabled: true, SessionStore: session.NewStore(t.TempDir()), Executors: []AgentExecutor{executor}, DefaultAgent: "worker"})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })
	first, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "parallel work"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	failed := waitRuntimeEffectRun(t, rt, first.ID)
	checkpoint, ok := latestRunCheckpoint(failed)
	if !ok || checkpoint.Capability != CheckpointCapabilityReplay {
		t.Fatalf("replay checkpoint: %+v", checkpoint)
	}
	retry, err := rt.RestartFromCheckpoint(context.Background(), RestartRequest{
		RunID: failed.ID, CheckpointID: checkpoint.ID, Mode: RecoveryModeReplayFromCheckpoint,
	})
	if err != nil {
		t.Fatalf("replay from checkpoint: %v", err)
	}
	final := waitRuntimeEffectRun(t, rt, retry.ID)
	if final.Status != RunStatusCompleted || final.RecoveryMode != RecoveryModeReplayFromCheckpoint {
		t.Fatalf("replayed run: %+v", final)
	}
	if replayPlan == nil || replayPlan.Mode != RecoveryModeReplayFromCheckpoint || len(replayPlan.ToolResults) != 1 {
		t.Fatalf("replay plan: %+v", replayPlan)
	}
	if len(final.ToolResults) != 1 || final.ToolResults[0].Result != `{"path":"a.txt","written":true}` {
		t.Fatalf("completed sibling result was not inherited: %+v", final.ToolResults)
	}
}

func TestRecoveryRequiresHumanDecisionForPendingUnsafeEffect(t *testing.T) {
	executor := effectTestExecutor(t, func(ctx context.Context) error {
		recorder := RuntimeExecutionRecorderFromContext(ctx)
		if err := recorder(RuntimeToolCall{
			Phase: RuntimeToolPhaseBefore, Iteration: 1, ToolName: "send_message",
			ToolCallID: "call-1", ToolArgs: `{}`, ToolEffectClass: "unsafe",
		}); err != nil {
			return err
		}
		return errors.New("crash after effect")
	})
	rt := NewRuntime(RuntimeOptions{Enabled: true, SessionStore: session.NewStore(t.TempDir()), Executors: []AgentExecutor{executor}, DefaultAgent: "effect-worker"})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })
	first, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "send"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	failed := waitRuntimeEffectRun(t, rt, first.ID)
	checkpoint, _ := latestRunCheckpoint(failed)
	_, err = rt.RestartFromCheckpoint(context.Background(), RestartRequest{
		RunID: failed.ID, CheckpointID: checkpoint.ID, Mode: RecoveryModeRetryFromPrompt,
	})
	if !errors.Is(err, ErrRecoveryApprovalRequired) {
		t.Fatalf("unsafe recovery error = %v", err)
	}
}

func TestRecoveryResumeRequiresAndPassesProviderContinuation(t *testing.T) {
	callCount := 0
	var resumed *RecoveryExecutionPlan
	executor, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
		Name: "resumable", CheckpointSupport: ExecutorCheckpointSupport{Capability: CheckpointCapabilityResumableStep},
		RunPrompt: func(ctx context.Context, _ string, _ string, _ []string, _ string, _ *ProviderOverride) (string, error) {
			callCount++
			if callCount == 1 {
				recorder := RuntimeExecutionRecorderFromContext(ctx)
				if err := recorder(RuntimeToolCall{Phase: RuntimeToolPhaseAfterLLM, ContinuationID: "provider-session-7"}); err != nil {
					return "", err
				}
				return "", errors.New("provider disconnected")
			}
			resumed = RecoveryExecutionFromContext(ctx)
			return "resumed", nil
		},
	})
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	rt := NewRuntime(RuntimeOptions{Enabled: true, SessionStore: session.NewStore(t.TempDir()), Executors: []AgentExecutor{executor}, DefaultAgent: "resumable"})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })
	first, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "continue"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	failed := waitRuntimeEffectRun(t, rt, first.ID)
	checkpoint, _ := latestRunCheckpoint(failed)
	if !checkpoint.Resumable || checkpoint.Capability != CheckpointCapabilityResumableStep {
		t.Fatalf("resumable checkpoint: %+v", checkpoint)
	}
	retry, err := rt.RestartFromCheckpoint(context.Background(), RestartRequest{
		RunID: failed.ID, CheckpointID: checkpoint.ID, Mode: RecoveryModeResumeFromCheckpoint,
	})
	if err != nil {
		t.Fatalf("resume from checkpoint: %v", err)
	}
	_ = waitRuntimeEffectRun(t, rt, retry.ID)
	if resumed == nil || resumed.ContinuationID != "provider-session-7" || resumed.Mode != RecoveryModeResumeFromCheckpoint {
		t.Fatalf("resume plan: %+v", resumed)
	}
}

func TestRecoveryPreservesDurableWorkCorrelation(t *testing.T) {
	callCount := 0
	executor, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
		Name: "worker", CheckpointSupport: ExecutorCheckpointSupport{Capability: CheckpointCapabilityReplay},
		RunPrompt: func(context.Context, string, string, []string, string, *ProviderOverride) (string, error) {
			callCount++
			if callCount == 1 {
				return "", errors.New("crash")
			}
			return "recovered", nil
		},
	})
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	rt := NewRuntime(RuntimeOptions{Enabled: true, SessionStore: session.NewStore(t.TempDir()), Executors: []AgentExecutor{executor}, DefaultAgent: "worker"})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })
	first, err := rt.Spawn(context.Background(), SpawnRequest{
		Prompt: "continue durable work", WorkID: "work-7", TaskID: "task-7", FlowID: "flow-7", StepID: "step-7",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	failed := waitRuntimeEffectRun(t, rt, first.ID)
	checkpoint, _ := latestRunCheckpoint(failed)
	recovered, err := rt.RestartFromCheckpoint(context.Background(), RestartRequest{
		RunID: failed.ID, CheckpointID: checkpoint.ID, Mode: RecoveryModeReplayFromCheckpoint,
	})
	if err != nil {
		t.Fatalf("replay from checkpoint: %v", err)
	}
	final := waitRuntimeEffectRun(t, rt, recovered.ID)
	if final.WorkID != "work-7" || final.TaskID != "task-7" || final.FlowID != "flow-7" || final.StepID != "step-7" {
		t.Fatalf("recovery correlation metadata: %+v", final)
	}
}

func TestLegacyCheckpointRejectsReplayButStillRetries(t *testing.T) {
	run := Run{ID: "legacy", Status: RunStatusFailed, Checkpoints: []RunCheckpoint{{ID: "legacy-cp", Kind: "prompt", Prompt: "old"}}}
	normalizeRunCheckpointCompatibility(&run)
	checkpoint := run.Checkpoints[0]
	if recoveryModeSupported(checkpoint, RecoveryModeReplayFromCheckpoint) {
		t.Fatalf("legacy checkpoint unexpectedly supports replay: %+v", checkpoint)
	}
}
