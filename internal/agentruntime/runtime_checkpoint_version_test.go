package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

func TestLegacyCheckpointNormalizesToPromptV0RetryOnly(t *testing.T) {
	persistenceDir := t.TempDir()
	payload, err := json.Marshal(map[string]any{
		"runs": []Run{{
			ID: "run_41", WorkspaceID: DefaultWorkspaceID, SessionID: "session-1",
			Agent: "default", Prompt: "legacy prompt", Status: RunStatusFailed,
			CreatedAt: "2026-08-02T00:00:00Z", UpdatedAt: "2026-08-02T00:00:01Z",
			Checkpoints: []RunCheckpoint{{
				ID: "run_41_cp_1", RunID: "run_41", Kind: "prompt",
				Prompt: "legacy prompt", CreatedAt: "2026-08-02T00:00:00Z",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("marshal legacy snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(persistenceDir, runsSnapshotFilename), payload, 0o600); err != nil {
		t.Fatalf("write legacy snapshot: %v", err)
	}

	rt := NewRuntime(RuntimeOptions{
		Enabled: true, SessionStore: session.NewStore(t.TempDir()),
		AgentRuntimePersistenceEnabled: true, AgentRuntimeRunsPersistenceEnabled: true,
		AgentRuntimePersistenceDir: persistenceDir, AgentRuntimeRestoreOnStartup: true,
		RunPrompt: func(context.Context, string, string) (string, error) { return "", nil },
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })
	run, ok := rt.Get("run_41")
	if !ok || len(run.Checkpoints) != 1 {
		t.Fatalf("restored legacy run: %+v found=%v", run, ok)
	}
	checkpoint := run.Checkpoints[0]
	if checkpoint.SchemaVersion != 0 || checkpoint.Format != CheckpointFormatPromptV0 {
		t.Fatalf("legacy checkpoint version: %+v", checkpoint)
	}
	if checkpoint.Capability != CheckpointCapabilityRetryOnly || checkpoint.Resumable {
		t.Fatalf("legacy checkpoint capability: %+v", checkpoint)
	}
	if checkpoint.ResumeReason == "" || len(checkpoint.RecoveryModes) != 1 || checkpoint.RecoveryModes[0] != RecoveryModeRetryFromPrompt {
		t.Fatalf("legacy checkpoint recovery declaration: %+v", checkpoint)
	}
}

func TestNewCheckpointsDeclareVersionCapabilityAndReason(t *testing.T) {
	executor, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
		Name: "recoverable",
		CheckpointSupport: ExecutorCheckpointSupport{
			Capability: CheckpointCapabilityResumableStep,
			Limitation: "resume requires a provider continuation handle",
		},
		RunPrompt: func(context.Context, string, string, []string, string, *ProviderOverride) (string, error) {
			return "", fmt.Errorf("boom")
		},
	})
	if err != nil {
		t.Fatalf("new prompt executor: %v", err)
	}
	rt := NewRuntime(RuntimeOptions{
		Enabled: true, SessionStore: session.NewStore(t.TempDir()), Executors: []AgentExecutor{executor}, DefaultAgent: "recoverable",
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })
	run, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "do work"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	final, err := rt.Wait(waitCtx, run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if len(final.Checkpoints) < 2 {
		t.Fatalf("checkpoints: %+v", final.Checkpoints)
	}
	for _, checkpoint := range final.Checkpoints {
		if checkpoint.SchemaVersion != currentCheckpointSchemaVersion || checkpoint.Format != CheckpointFormatStepV1 {
			t.Errorf("checkpoint version: %+v", checkpoint)
		}
		if checkpoint.Capability != CheckpointCapabilityReplay {
			t.Errorf("checkpoint without continuation must stop at replay capability: %+v", checkpoint)
		}
		if checkpoint.Resumable || checkpoint.ResumeReason == "" {
			t.Errorf("checkpoint must explicitly explain non-resumability: %+v", checkpoint)
		}
		if !containsRecoveryMode(checkpoint.RecoveryModes, RecoveryModeRetryFromPrompt) || !containsRecoveryMode(checkpoint.RecoveryModes, RecoveryModeReplayFromCheckpoint) {
			t.Errorf("checkpoint recovery modes: %+v", checkpoint)
		}
	}
}

func TestExecutorCheckpointCapabilityNegotiationDoesNotOverclaim(t *testing.T) {
	command, err := NewCommandExecutor(CommandExecutorOptions{Name: "command", Command: "true"})
	if err != nil {
		t.Fatalf("new command executor: %v", err)
	}
	commandSupport := checkpointSupportForExecutor(command)
	if commandSupport.Capability != CheckpointCapabilityRetryOnly || commandSupport.Limitation == "" {
		t.Fatalf("command support: %+v", commandSupport)
	}

	prompt, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
		Name: "prompt", RunPrompt: func(context.Context, string, string, []string, string, *ProviderOverride) (string, error) {
			return "ok", nil
		},
		CheckpointSupport: ExecutorCheckpointSupport{Capability: CheckpointCapabilityEnvironmentRehydratable},
	})
	if err != nil {
		t.Fatalf("new prompt executor: %v", err)
	}
	promptSupport := checkpointSupportForExecutor(prompt)
	if promptSupport.Capability != CheckpointCapabilityRetryOnly {
		t.Fatalf("unsupported environment capability must fail closed, got %+v", promptSupport)
	}
}

func containsRecoveryMode(modes []RecoveryMode, target RecoveryMode) bool {
	for _, mode := range modes {
		if mode == target {
			return true
		}
	}
	return false
}
