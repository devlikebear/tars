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
	capability, resumable, resumeReason, approvalRequired, approvalReason := checkpointRecoveryState(state.run, support)
	continuation := cloneCheckpointContinuation(state.run.LatestContinuation)
	checkpoint := RunCheckpoint{
		SchemaVersion:            currentCheckpointSchemaVersion,
		ID:                       fmt.Sprintf("%s_cp_%d", state.run.ID, len(state.run.Checkpoints)+1),
		RunID:                    state.run.ID,
		Format:                   CheckpointFormatStepV1,
		Capability:               capability,
		Resumable:                resumable,
		ResumeReason:             resumeReason,
		RecoveryModes:            checkpointRecoveryModes(capability, continuation),
		RecoveryApprovalRequired: approvalRequired,
		RecoveryApprovalReason:   approvalReason,
		NextAction:               checkpointNextAction(kind),
		StateRefs: []CheckpointReference{
			{Kind: "run", ID: state.run.ID},
			{Kind: "session", ID: state.run.SessionID},
			{Kind: "workspace", ID: state.run.WorkspaceID},
		},
		ToolRequestRefs:   toolRequestCheckpointRefs(state.run.ToolRequests),
		ToolResultRefs:    toolResultCheckpointRefs(state.run.ToolResults),
		EffectReceiptRefs: effectReceiptCheckpointRefs(state.run.EffectReceipts),
		Continuation:      continuation,
		Kind:              strings.TrimSpace(kind),
		Label:             strings.TrimSpace(label),
		Status:            state.run.Status,
		Agent:             state.run.Agent,
		Prompt:            state.run.Prompt,
		Tier:              state.run.Tier,
		ProviderOverride:  CloneProviderOverride(state.run.ProviderOverride),
		AllowedTools:      sanitizeToolsAllow(allowedTools),
		Error:             strings.TrimSpace(errText),
		CreatedAt:         now,
	}
	if checkpoint.Kind == "" {
		checkpoint.Kind = "step"
	}
	state.run.Checkpoints = append(state.run.Checkpoints, checkpoint)
	return checkpoint
}

func checkpointRecoveryState(run Run, support ExecutorCheckpointSupport) (CheckpointCapability, bool, string, bool, string) {
	for _, request := range run.ToolRequests {
		if request.Status == ToolRequestStatusPending && !request.SafeToRetryPending {
			reason := fmt.Sprintf("tool %s may have completed without an effect receipt; human approval is required before recovery", request.ToolName)
			return CheckpointCapabilityRetryOnly, false, reason, true, reason
		}
	}
	for _, result := range run.ToolResults {
		if result.Truncated {
			reason := "a recorded tool result exceeds the replay limit; retry from prompt is required"
			return CheckpointCapabilityRetryOnly, false, reason, false, ""
		}
	}
	capability := support.Capability
	if capability == CheckpointCapabilityResumableStep {
		if run.LatestContinuation != nil && strings.TrimSpace(run.LatestContinuation.ID) != "" {
			return capability, true, "provider continuation and receipt-backed tool results are available", false, ""
		}
		return CheckpointCapabilityReplay, false, strings.TrimSpace(firstNonEmpty(
			support.Limitation,
			"checkpoint has replayable tool results but no provider continuation handle",
		)), false, ""
	}
	if capability == CheckpointCapabilityReplay {
		return capability, false, strings.TrimSpace(firstNonEmpty(
			support.Limitation,
			"executor supports receipt-backed replay but not provider-session resume",
		)), false, ""
	}
	return CheckpointCapabilityRetryOnly, false, strings.TrimSpace(firstNonEmpty(
		support.Limitation,
		"executor does not expose replayable state or a continuation handle",
	)), false, ""
}

func cloneCheckpointContinuation(value *CheckpointContinuation) *CheckpointContinuation {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func toolRequestCheckpointRefs(records []ToolRequestRecord) []CheckpointReference {
	refs := make([]CheckpointReference, 0, len(records))
	for _, record := range records {
		if strings.TrimSpace(record.ID) == "" {
			continue
		}
		refs = append(refs, CheckpointReference{Kind: "tool_request", ID: record.ID, Digest: record.ArgsDigest})
	}
	return refs
}

func toolResultCheckpointRefs(records []ToolResultRecord) []CheckpointReference {
	refs := make([]CheckpointReference, 0, len(records))
	for _, record := range records {
		if strings.TrimSpace(record.ID) == "" {
			continue
		}
		refs = append(refs, CheckpointReference{Kind: "tool_result", ID: record.ID, Digest: record.Digest})
	}
	return refs
}

func effectReceiptCheckpointRefs(records []EffectReceipt) []CheckpointReference {
	refs := make([]CheckpointReference, 0, len(records))
	for _, record := range records {
		if strings.TrimSpace(record.ID) == "" {
			continue
		}
		refs = append(refs, CheckpointReference{Kind: "effect_receipt", ID: record.ID, Digest: record.RequestDigest})
	}
	return refs
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
