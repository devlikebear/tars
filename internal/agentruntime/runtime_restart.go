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
	agent := firstNonEmpty(req.Agent, checkpoint.Agent, source.Agent)
	tier := firstNonEmpty(req.Tier, checkpoint.Tier, source.Tier)
	providerOverride := CloneProviderOverride(req.ProviderOverride)
	if providerOverride == nil {
		providerOverride = CloneProviderOverride(checkpoint.ProviderOverride)
	}
	if providerOverride == nil {
		providerOverride = CloneProviderOverride(source.ProviderOverride)
	}
	prompt := buildRestartPrompt(firstNonEmpty(checkpoint.Prompt, source.Prompt), source, checkpoint, req.PromptAdjustment)
	rootRunID := strings.TrimSpace(source.RootRunID)
	if rootRunID == "" {
		rootRunID = source.ID
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Retry " + source.ID
	}

	return r.Spawn(ctx, SpawnRequest{
		WorkspaceID:               firstNonEmpty(req.WorkspaceID, source.WorkspaceID),
		SessionID:                 source.SessionID,
		Title:                     title,
		Prompt:                    prompt,
		Agent:                     agent,
		ParentRunID:               source.ID,
		RootRunID:                 rootRunID,
		ParentSessionID:           source.ParentSessionID,
		Depth:                     source.Depth + 1,
		SessionKind:               source.SessionKind,
		Tier:                      tier,
		ProviderOverride:          providerOverride,
		RestartedFromRunID:        source.ID,
		RestartedFromCheckpointID: checkpoint.ID,
		RestartAttempt:            r.nextRestartAttempt(source.ID),
		RestartReason:             strings.TrimSpace(req.PromptAdjustment),
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
	if source.Status != RunStatusFailed {
		return Run{}, RunCheckpoint{}, fmt.Errorf("run %s is not failed", source.ID)
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

func buildRestartPrompt(base string, source Run, checkpoint RunCheckpoint, adjustment string) string {
	parts := []string{strings.TrimSpace(base)}
	contextLines := []string{
		"Retry context:",
		"- Source run: " + strings.TrimSpace(source.ID),
		"- Checkpoint: " + strings.TrimSpace(checkpoint.ID),
	}
	if source.Error != "" {
		contextLines = append(contextLines, "- Previous error: "+strings.TrimSpace(source.Error))
	}
	if trimmed := strings.TrimSpace(adjustment); trimmed != "" {
		contextLines = append(contextLines, "- Adjustment: "+trimmed)
	}
	parts = append(parts, strings.Join(contextLines, "\n"))
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
