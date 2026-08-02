package agentruntime

import (
	"fmt"
	"strings"
	"time"
)

func (r *Runtime) appendRunCheckpointLocked(state *runState, kind string, label string, errText string, allowedTools []string) RunCheckpoint {
	if state == nil {
		return RunCheckpoint{}
	}
	now := r.nowFn().UTC().Format(time.RFC3339)
	support := checkpointSupportForExecutor(state.executor)
	capability := support.Capability
	if capability == CheckpointCapabilityResumableStep {
		capability = CheckpointCapabilityReplay
	}
	resumeReason := strings.TrimSpace(support.Limitation)
	if resumeReason == "" {
		resumeReason = "checkpoint does not contain a provider continuation handle"
	}
	checkpoint := RunCheckpoint{
		SchemaVersion: currentCheckpointSchemaVersion,
		ID:            fmt.Sprintf("%s_cp_%d", state.run.ID, len(state.run.Checkpoints)+1),
		RunID:         state.run.ID,
		Format:        CheckpointFormatStepV1,
		Capability:    capability,
		Resumable:     false,
		ResumeReason:  resumeReason,
		RecoveryModes: checkpointRecoveryModes(capability, nil),
		NextAction:    checkpointNextAction(kind),
		StateRefs: []CheckpointReference{
			{Kind: "run", ID: state.run.ID},
			{Kind: "session", ID: state.run.SessionID},
			{Kind: "workspace", ID: state.run.WorkspaceID},
		},
		Kind:             strings.TrimSpace(kind),
		Label:            strings.TrimSpace(label),
		Status:           state.run.Status,
		Agent:            state.run.Agent,
		Prompt:           state.run.Prompt,
		Tier:             state.run.Tier,
		ProviderOverride: CloneProviderOverride(state.run.ProviderOverride),
		AllowedTools:     sanitizeToolsAllow(allowedTools),
		Error:            strings.TrimSpace(errText),
		CreatedAt:        now,
	}
	if checkpoint.Kind == "" {
		checkpoint.Kind = "step"
	}
	state.run.Checkpoints = append(state.run.Checkpoints, checkpoint)
	return checkpoint
}

func checkpointNextAction(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "failure":
		return "recover"
	case "prompt":
		return "dispatch_prompt"
	default:
		return "continue_step"
	}
}

func latestRunCheckpoint(run Run) (RunCheckpoint, bool) {
	for i := len(run.Checkpoints) - 1; i >= 0; i-- {
		if strings.TrimSpace(run.Checkpoints[i].ID) != "" {
			return run.Checkpoints[i], true
		}
	}
	return RunCheckpoint{}, false
}

func findRunCheckpoint(run Run, checkpointID string) (RunCheckpoint, bool) {
	target := strings.TrimSpace(checkpointID)
	if target == "" {
		return latestRunCheckpoint(run)
	}
	for _, checkpoint := range run.Checkpoints {
		if checkpoint.ID == target {
			return checkpoint, true
		}
	}
	return RunCheckpoint{}, false
}
