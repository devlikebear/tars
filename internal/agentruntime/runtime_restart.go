package agentruntime

import (
	"context"
	"fmt"
	"strings"
)

func (r *Runtime) RestartFromCheckpoint(ctx context.Context, req RestartRequest) (Run, error) {
	if err := r.requireEnabled(); err != nil {
		return Run{}, err
	}
	sourceRunID := strings.TrimSpace(req.RunID)
	if sourceRunID == "" {
		return Run{}, fmt.Errorf("run_id is required")
	}

	source, checkpoint, err := r.restartSource(sourceRunID, req.CheckpointID, req.WorkspaceID)
	if err != nil {
		return Run{}, err
	}
	mode := req.Mode
	if mode == "" {
		mode = RecoveryModeRetryFromPrompt
	}
	if checkpoint.RecoveryApprovalRequired && !req.ConfirmUnsafeRecovery {
		return Run{}, fmt.Errorf("%w: %s", ErrRecoveryApprovalRequired, checkpoint.RecoveryApprovalReason)
	}
	if !recoveryModeSupported(checkpoint, mode) {
		return Run{}, fmt.Errorf("%w: checkpoint %s does not support %s", ErrRecoveryModeUnsupported, checkpoint.ID, mode)
	}
	recoveryPlan, err := buildRecoveryExecutionPlan(source, checkpoint, mode)
	if err != nil {
		return Run{}, err
	}
	agent := firstNonEmpty(req.Agent, checkpoint.Agent, source.Agent)
	tier := firstNonEmpty(req.Tier, checkpoint.Tier, source.Tier)
	providerOverride := CloneProviderOverride(req.ProviderOverride)
	if providerOverride == nil {
		providerOverride = CloneProviderOverride(checkpoint.ProviderOverride)
	}
	if providerOverride == nil {
		providerOverride = CloneProviderOverride(source.ProviderOverride)
	}
	prompt := buildRestartPrompt(firstNonEmpty(checkpoint.Prompt, source.Prompt), source, checkpoint, mode, recoveryPlan, req.PromptAdjustment)
	rootRunID := strings.TrimSpace(source.RootRunID)
	if rootRunID == "" {
		rootRunID = source.ID
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = recoveryTitle(mode) + " " + source.ID
	}

	return r.Spawn(ctx, SpawnRequest{
		WorkspaceID:               firstNonEmpty(req.WorkspaceID, source.WorkspaceID),
		WorkID:                    source.WorkID,
		SessionID:                 source.SessionID,
		TaskID:                    source.TaskID,
		Title:                     title,
		Prompt:                    prompt,
		Agent:                     agent,
		ParentRunID:               source.ID,
		RootRunID:                 rootRunID,
		ParentSessionID:           source.ParentSessionID,
		Depth:                     source.Depth + 1,
		SessionKind:               source.SessionKind,
		FlowID:                    source.FlowID,
		StepID:                    source.StepID,
		Tier:                      tier,
		ProviderOverride:          providerOverride,
		RestartedFromRunID:        source.ID,
		RestartedFromCheckpointID: checkpoint.ID,
		RestartAttempt:            r.nextRestartAttempt(source.ID),
		RestartReason:             strings.TrimSpace(req.PromptAdjustment),
		RecoveryMode:              mode,
		RecoveryPlan:              recoveryPlan,
	})
}

func (r *Runtime) restartSource(runID, checkpointID, workspaceID string) (Run, RunCheckpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, ok := r.runs[strings.TrimSpace(runID)]
	if !ok || state == nil {
		return Run{}, RunCheckpoint{}, fmt.Errorf("run not found: %s", strings.TrimSpace(runID))
	}
	source := state.run
	if strings.TrimSpace(workspaceID) != "" && normalizeWorkspaceID(source.WorkspaceID) != normalizeWorkspaceID(workspaceID) {
		return Run{}, RunCheckpoint{}, fmt.Errorf("run not found: %s", strings.TrimSpace(runID))
	}
	if source.Status != RunStatusFailed && source.Status != RunStatusCanceled {
		return Run{}, RunCheckpoint{}, fmt.Errorf("run %s is not failed or canceled", source.ID)
	}
	checkpoint, ok := findRunCheckpoint(source, checkpointID)
	if !ok {
		return Run{}, RunCheckpoint{}, fmt.Errorf("checkpoint not found: %s", strings.TrimSpace(checkpointID))
	}
	return source, checkpoint, nil
}

func (r *Runtime) nextRestartAttempt(sourceRunID string) int {
	sourceRunID = strings.TrimSpace(sourceRunID)
	if sourceRunID == "" {
		return 1
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	attempt := 1
	for _, state := range r.runs {
		if state == nil {
			continue
		}
		if state.run.RestartedFromRunID == sourceRunID && state.run.RestartAttempt >= attempt {
			attempt = state.run.RestartAttempt + 1
		}
	}
	return attempt
}

func buildRestartPrompt(base string, source Run, checkpoint RunCheckpoint, mode RecoveryMode, plan *RecoveryExecutionPlan, adjustment string) string {
	parts := []string{strings.TrimSpace(base)}
	contextLines := []string{
		recoveryPromptHeading(mode),
		"- Source run: " + strings.TrimSpace(source.ID),
		"- Checkpoint: " + strings.TrimSpace(checkpoint.ID),
		"- Recovery mode: " + string(mode),
	}
	if source.Error != "" {
		contextLines = append(contextLines, "- Previous error: "+strings.TrimSpace(source.Error))
	}
	if trimmed := strings.TrimSpace(adjustment); trimmed != "" {
		contextLines = append(contextLines, "- Adjustment: "+trimmed)
	}
	if plan != nil && len(plan.ToolResults) > 0 {
		contextLines = append(contextLines, "- Receipt-backed tool results already completed; do not repeat their external effects:")
		for _, result := range plan.ToolResults {
			contextLines = append(contextLines, "  - "+result.Signature+": "+strings.TrimSpace(result.Result))
		}
	}
	parts = append(parts, strings.Join(contextLines, "\n"))
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func recoveryPromptHeading(mode RecoveryMode) string {
	switch mode {
	case RecoveryModeReplayFromCheckpoint:
		return "Replay context:"
	case RecoveryModeResumeFromCheckpoint:
		return "Resume context:"
	default:
		return "Retry context:"
	}
}

func recoveryTitle(mode RecoveryMode) string {
	switch mode {
	case RecoveryModeReplayFromCheckpoint:
		return "Replay"
	case RecoveryModeResumeFromCheckpoint:
		return "Resume"
	default:
		return "Retry"
	}
}

func buildRecoveryExecutionPlan(source Run, checkpoint RunCheckpoint, mode RecoveryMode) (*RecoveryExecutionPlan, error) {
	if mode == RecoveryModeRetryFromPrompt {
		return nil, nil
	}
	plan := &RecoveryExecutionPlan{
		Mode: mode, SourceRunID: source.ID, CheckpointID: checkpoint.ID,
		ContinuationID: checkpointContinuationID(checkpoint),
	}
	requestRefs := referenceIDs(checkpoint.ToolRequestRefs)
	resultRefs := referenceIDs(checkpoint.ToolResultRefs)
	receiptRefs := referenceIDs(checkpoint.EffectReceiptRefs)
	for _, request := range source.ToolRequests {
		if _, ok := requestRefs[request.ID]; ok {
			plan.Requests = append(plan.Requests, request)
		}
	}
	for _, receipt := range source.EffectReceipts {
		if _, ok := receiptRefs[receipt.ID]; ok {
			plan.EffectReceipts = append(plan.EffectReceipts, receipt)
		}
	}
	for _, result := range source.ToolResults {
		if _, ok := resultRefs[result.ID]; !ok {
			continue
		}
		request, ok := recoveryRequestByID(plan.Requests, result.RequestID)
		if !ok {
			continue
		}
		if request.EffectClass != "read_only" && !hasCommittedRecoveryReceipt(plan.EffectReceipts, request.EffectReceiptID) {
			continue
		}
		plan.RecordedResults = append(plan.RecordedResults, result)
		plan.ToolResults = append(plan.ToolResults, RecoveryToolResult{
			RequestID: request.ID, Signature: request.Signature, Result: result.Result,
			IsError: result.IsError, ReceiptID: request.EffectReceiptID,
		})
	}
	if mode == RecoveryModeResumeFromCheckpoint && strings.TrimSpace(plan.ContinuationID) == "" {
		return nil, fmt.Errorf("%w: checkpoint %s has no provider continuation", ErrRecoveryModeUnsupported, checkpoint.ID)
	}
	return plan, nil
}

func checkpointContinuationID(checkpoint RunCheckpoint) string {
	if checkpoint.Continuation == nil {
		return ""
	}
	return strings.TrimSpace(checkpoint.Continuation.ID)
}

func referenceIDs(refs []CheckpointReference) map[string]struct{} {
	out := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if id := strings.TrimSpace(ref.ID); id != "" {
			out[id] = struct{}{}
		}
	}
	return out
}

func recoveryRequestByID(requests []ToolRequestRecord, id string) (ToolRequestRecord, bool) {
	for _, request := range requests {
		if request.ID == id {
			return request, true
		}
	}
	return ToolRequestRecord{}, false
}

func hasCommittedRecoveryReceipt(receipts []EffectReceipt, id string) bool {
	for _, receipt := range receipts {
		if receipt.ID == id && receipt.Status == EffectReceiptStatusCommitted {
			return true
		}
	}
	return false
}
