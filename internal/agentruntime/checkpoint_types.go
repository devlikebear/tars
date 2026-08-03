package agentruntime

import (
	"errors"
	"strings"
)

var (
	ErrRecoveryApprovalRequired = errors.New("agent runtime: recovery approval required")
	ErrRecoveryModeUnsupported  = errors.New("agent runtime: recovery mode unsupported")
)

const currentCheckpointSchemaVersion = 1

type CheckpointFormat string

const (
	CheckpointFormatPromptV0 CheckpointFormat = "prompt_checkpoint_v0"
	CheckpointFormatStepV1   CheckpointFormat = "step_checkpoint_v1"
)

type CheckpointCapability string

const (
	CheckpointCapabilityRetryOnly               CheckpointCapability = "retry_only"
	CheckpointCapabilityReplay                  CheckpointCapability = "replay"
	CheckpointCapabilityResumableStep           CheckpointCapability = "resumable_step"
	CheckpointCapabilityEnvironmentRehydratable CheckpointCapability = "environment_rehydratable"
)

type RecoveryMode string

const (
	RecoveryModeRetryFromPrompt      RecoveryMode = "retry_from_prompt"
	RecoveryModeReplayFromCheckpoint RecoveryMode = "replay_from_checkpoint"
	RecoveryModeResumeFromCheckpoint RecoveryMode = "resume_from_checkpoint"
)

type CheckpointReference struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Digest string `json:"digest,omitempty"`
	URI    string `json:"uri,omitempty"`
}

type CheckpointContinuation struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	Executor   string `json:"executor,omitempty"`
	RecordedAt string `json:"recorded_at,omitempty"`
}

type ExecutorCheckpointSupport struct {
	Capability CheckpointCapability `json:"capability"`
	Limitation string               `json:"limitation,omitempty"`
}

type checkpointSupportProvider interface {
	CheckpointSupport() ExecutorCheckpointSupport
}

func checkpointSupportForExecutor(executor AgentExecutor) ExecutorCheckpointSupport {
	fallback := ExecutorCheckpointSupport{
		Capability: CheckpointCapabilityRetryOnly,
		Limitation: "executor does not expose replayable state or a continuation handle",
	}
	provider, ok := executor.(checkpointSupportProvider)
	if !ok || provider == nil {
		return fallback
	}
	support := provider.CheckpointSupport()
	support.Limitation = strings.TrimSpace(support.Limitation)
	switch support.Capability {
	case CheckpointCapabilityRetryOnly:
		if support.Limitation == "" {
			support.Limitation = fallback.Limitation
		}
		return support
	case CheckpointCapabilityReplay:
		if support.Limitation == "" {
			support.Limitation = "executor can replay recorded tool results but cannot resume an in-flight provider session"
		}
		return support
	case CheckpointCapabilityResumableStep:
		if support.Limitation == "" {
			support.Limitation = "resume requires a recorded provider continuation handle"
		}
		return support
	default:
		fallback.Limitation = "executor requested an unsupported checkpoint capability; only prompt retry is enabled"
		return fallback
	}
}

func checkpointRecoveryModes(capability CheckpointCapability, continuation *CheckpointContinuation) []RecoveryMode {
	modes := []RecoveryMode{RecoveryModeRetryFromPrompt}
	if capability == CheckpointCapabilityReplay || capability == CheckpointCapabilityResumableStep || capability == CheckpointCapabilityEnvironmentRehydratable {
		modes = append(modes, RecoveryModeReplayFromCheckpoint)
	}
	if (capability == CheckpointCapabilityResumableStep || capability == CheckpointCapabilityEnvironmentRehydratable) && continuation != nil && strings.TrimSpace(continuation.ID) != "" {
		modes = append(modes, RecoveryModeResumeFromCheckpoint)
	}
	return modes
}

func recoveryModeSupported(checkpoint RunCheckpoint, mode RecoveryMode) bool {
	for _, supported := range checkpoint.RecoveryModes {
		if supported == mode {
			return true
		}
	}
	return false
}

func normalizeRunCheckpointCompatibility(run *Run) {
	if run == nil {
		return
	}
	for i := range run.Checkpoints {
		checkpoint := &run.Checkpoints[i]
		if checkpoint.SchemaVersion > 0 || checkpoint.Format != "" {
			if checkpoint.Format == "" {
				checkpoint.Format = CheckpointFormatStepV1
			}
			if checkpoint.Capability == "" {
				checkpoint.Capability = CheckpointCapabilityRetryOnly
			}
			if checkpoint.ResumeReason == "" {
				checkpoint.ResumeReason = "checkpoint does not contain a provider continuation handle"
			}
			if len(checkpoint.RecoveryModes) == 0 {
				checkpoint.RecoveryModes = checkpointRecoveryModes(checkpoint.Capability, checkpoint.Continuation)
			}
			continue
		}
		checkpoint.SchemaVersion = 0
		checkpoint.Format = CheckpointFormatPromptV0
		checkpoint.Capability = CheckpointCapabilityRetryOnly
		checkpoint.Resumable = false
		checkpoint.ResumeReason = "legacy prompt checkpoint stores no execution position; retry from prompt is the only safe mode"
		checkpoint.RecoveryModes = []RecoveryMode{RecoveryModeRetryFromPrompt}
		if strings.TrimSpace(checkpoint.NextAction) == "" {
			checkpoint.NextAction = "retry_prompt"
		}
	}
}

func NormalizeRunCompatibility(run Run) Run {
	normalizeRunCheckpointCompatibility(&run)
	return run
}
