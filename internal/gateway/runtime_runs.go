package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/project"
	"github.com/devlikebear/tarsncase/internal/serverauth"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/devlikebear/tarsncase/internal/usage"
)

func (r *Runtime) Spawn(ctx context.Context, req SpawnRequest) (Run, error) {
	if r == nil || !r.opts.Enabled {
		return Run{}, fmt.Errorf("gateway runtime is disabled")
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return Run{}, fmt.Errorf("prompt is required")
	}
	sessionStore := r.sessionStoreForWorkspace(req.WorkspaceID)
	if sessionStore == nil {
		return Run{}, fmt.Errorf("session store is not configured")
	}
	selectedAgent, executor, err := r.resolveExecutor(req.Agent)
	if err != nil {
		return Run{}, err
	}
	executorInfo := gatewayAgentInfo(executor)
	sessionID := strings.TrimSpace(req.SessionID)
	switch normalizeSessionRoutingMode(executorInfo.SessionRoutingMode) {
	case "new":
		sessionID = ""
	case "fixed":
		sessionID = strings.TrimSpace(executorInfo.SessionFixedID)
		if sessionID == "" {
			return Run{}, fmt.Errorf("agent %q is configured with fixed session routing but session_fixed_id is empty", selectedAgent)
		}
	}
	if sessionID == "" {
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = "chat"
		}
		s, err := sessionStore.Create(title)
		if err != nil {
			return Run{}, fmt.Errorf("create session: %w", err)
		}
		sessionID = s.ID
	} else {
		if _, err := sessionStore.Get(sessionID); err != nil {
			return Run{}, fmt.Errorf("get session: %w", err)
		}
	}

	projectID := strings.TrimSpace(req.ProjectID)
	if sessionID != "" {
		if sess, err := sessionStore.Get(sessionID); err == nil {
			if projectID == "" {
				projectID = strings.TrimSpace(sess.ProjectID)
			} else if strings.TrimSpace(sess.ProjectID) != projectID {
				_ = sessionStore.SetProjectID(sessionID, projectID)
			}
		}
	}

	now := r.nowFn().UTC()
	runID := fmt.Sprintf("run_%d", r.runSeq.Add(1))
	runCtx, cancel := context.WithCancel(context.Background())
	workspaceID := normalizeWorkspaceID(req.WorkspaceID)
	run := Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		SessionID:   sessionID,
		ProjectID:   strings.TrimSpace(projectID),
		Agent:       selectedAgent,
		Prompt:      prompt,
		Status:      RunStatusAccepted,
		Accepted:    true,
		CreatedAt:   now.Format(time.RFC3339),
		UpdatedAt:   now.Format(time.RFC3339),
	}
	state := &runState{run: run, executor: executor, cancel: cancel, done: make(chan struct{})}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		cancel()
		return Run{}, fmt.Errorf("gateway runtime is closed")
	}
	r.runs[runID] = state
	r.runOrder = append(r.runOrder, runID)
	r.runWG.Add(1)
	r.trimRunHistoryLocked()
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()

	go func() {
		defer r.runWG.Done()
		r.executeRun(runCtx, runID)
	}()
	return run, nil
}

func (r *Runtime) executeRun(ctx context.Context, runID string) {
	state, ok := r.getRunState(runID)
	if !ok {
		return
	}

	r.mu.Lock()
	if state.run.Status == RunStatusCanceled {
		r.closeRunDoneLocked(state)
		r.mu.Unlock()
		return
	}
	executor := state.executor
	now := r.nowFn().UTC().Format(time.RFC3339)
	state.run.Status = RunStatusRunning
	state.run.StartedAt = now
	state.run.UpdatedAt = now
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()

	_ = r.appendSessionMessage(state.run.WorkspaceID, state.run.SessionID, "user", state.run.Prompt, r.nowFn().UTC())
	var (
		resp string
		err  error
	)
	if executor == nil {
		err = fmt.Errorf("agent executor is not configured")
	} else {
		allowedTools := resolveRunAllowedTools(
			r.opts.WorkspaceDir,
			strings.TrimSpace(state.run.ProjectID),
			gatewayAgentInfo(executor).ToolsAllow,
		)
		execCtx := serverauth.WithWorkspaceID(ctx, state.run.WorkspaceID)
		execCtx = usage.WithCallMeta(execCtx, usage.CallMeta{
			Source:    "agent_run",
			SessionID: state.run.SessionID,
			ProjectID: state.run.ProjectID,
			RunID:     state.run.ID,
		})
		resp, err = executor.Execute(execCtx, ExecuteRequest{
			RunID:        state.run.ID,
			WorkspaceID:  state.run.WorkspaceID,
			SessionID:    state.run.SessionID,
			ProjectID:    state.run.ProjectID,
			Prompt:       state.run.Prompt,
			AllowedTools: allowedTools,
		})
	}
	if err == nil && ctx.Err() == nil {
		assistant := strings.TrimSpace(resp)
		if assistant != "" {
			_ = r.appendSessionMessage(state.run.WorkspaceID, state.run.SessionID, "assistant", assistant, r.nowFn().UTC())
		}
	}

	r.mu.Lock()
	if state.run.Status == RunStatusCanceled {
		r.closeRunDoneLocked(state)
		r.trimRunHistoryLocked()
		r.stateVersion++
		r.mu.Unlock()
		r.persistSnapshot()
		return
	}
	finishedAt := r.nowFn().UTC().Format(time.RFC3339)
	state.run.CompletedAt = finishedAt
	state.run.UpdatedAt = finishedAt
	if err != nil {
		state.run.Status = RunStatusFailed
		state.run.Error = strings.TrimSpace(err.Error())
		state.run.DiagnosticCode, state.run.DiagnosticReason = classifyRunDiagnostic(err)
		if state.run.DiagnosticCode == "policy_tool_blocked" {
			state.run.PolicyBlockedTool = blockedToolNameFromReason(state.run.DiagnosticReason)
			info := gatewayAgentInfo(executor)
			if len(info.ToolsAllow) > 0 {
				state.run.PolicyAllowedTools = append([]string(nil), info.ToolsAllow...)
			}
			if len(info.ToolsDeny) > 0 {
				state.run.PolicyDeniedTools = append([]string(nil), info.ToolsDeny...)
			}
			if strings.TrimSpace(info.ToolsRiskMax) != "" {
				state.run.PolicyRiskMax = strings.TrimSpace(info.ToolsRiskMax)
			}
		}
		r.closeRunDoneLocked(state)
		r.trimRunHistoryLocked()
		r.stateVersion++
		r.mu.Unlock()
		r.persistSnapshot()
		return
	}
	state.run.Status = RunStatusCompleted
	state.run.Response = strings.TrimSpace(resp)
	r.closeRunDoneLocked(state)
	r.trimRunHistoryLocked()
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
}

func blockedToolNameFromReason(reason string) string {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return ""
	}
	const prefix = "tool not injected for this request:"
	lower := strings.ToLower(trimmed)
	idx := strings.Index(lower, prefix)
	if idx == -1 {
		return ""
	}
	toolName := strings.TrimSpace(trimmed[idx+len(prefix):])
	if toolName == "" {
		return ""
	}
	return toolName
}

func resolveRunAllowedTools(baseWorkspaceDir, projectID string, executorAllowed []string) []string {
	base := sanitizeStringList(executorAllowed)
	if strings.TrimSpace(projectID) == "" {
		return base
	}
	workspaceDir := strings.TrimSpace(baseWorkspaceDir)
	if workspaceDir == "" {
		return base
	}
	store := project.NewStore(workspaceDir, nil)
	item, err := store.Get(strings.TrimSpace(projectID))
	if err != nil {
		return base
	}
	policy := project.NormalizeToolPolicy(project.ToolPolicySpec{
		ToolsAllow:               item.ToolsAllow,
		ToolsAllowExists:         len(item.ToolsAllow) > 0,
		ToolsAllowGroups:         item.ToolsAllowGroups,
		ToolsAllowGroupsExists:   len(item.ToolsAllowGroups) > 0,
		ToolsAllowPatterns:       item.ToolsAllowPatterns,
		ToolsAllowPatternsExists: len(item.ToolsAllowPatterns) > 0,
		ToolsDeny:                item.ToolsDeny,
		ToolsDenyExists:          len(item.ToolsDeny) > 0,
		ToolsRiskMax:             item.ToolsRiskMax,
		ToolsRiskMaxExists:       strings.TrimSpace(item.ToolsRiskMax) != "",
	}, sanitizeStringListAsSet(base), project.ToolPolicyOptions{})
	if !policy.HasPolicy {
		return base
	}
	return project.ApplyToolPolicy(base, policy)
}

func sanitizeStringListAsSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range sanitizeStringList(values) {
		out[value] = struct{}{}
	}
	return out
}

func (r *Runtime) appendSessionMessage(workspaceID, sessionID, role, content string, ts time.Time) error {
	sessionStore := r.sessionStoreForWorkspace(workspaceID)
	if r == nil || sessionStore == nil {
		return nil
	}
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(content) == "" {
		return nil
	}
	path := sessionStore.TranscriptPath(sessionID)
	if err := session.AppendMessage(path, session.Message{Role: role, Content: content, Timestamp: ts.UTC()}); err != nil {
		return err
	}
	return sessionStore.Touch(sessionID, ts.UTC())
}

func (r *Runtime) getRunState(runID string) (*runState, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, ok := r.runs[strings.TrimSpace(runID)]
	return state, ok
}

func (r *Runtime) sessionStoreForWorkspace(workspaceID string) *session.Store {
	if r == nil {
		return nil
	}
	if r.opts.SessionStoreForWorkspace != nil {
		if resolved := r.opts.SessionStoreForWorkspace(normalizeWorkspaceID(workspaceID)); resolved != nil {
			return resolved
		}
	}
	return r.opts.SessionStore
}

func (r *Runtime) closeRunDoneLocked(state *runState) {
	if state.closed {
		return
	}
	close(state.done)
	state.closed = true
}

func (r *Runtime) trimRunHistoryLocked() {
	max := r.opts.GatewayRunsMaxRecords
	if max <= 0 {
		return
	}
	for len(r.runOrder) > max {
		id := r.runOrder[0]
		state := r.runs[id]
		if state != nil {
			if state.run.Status == RunStatusAccepted || state.run.Status == RunStatusRunning {
				return
			}
		}
		delete(r.runs, id)
		r.runOrder = r.runOrder[1:]
	}
}

func (r *Runtime) Wait(ctx context.Context, runID string) (Run, error) {
	state, ok := r.getRunState(runID)
	if !ok {
		return Run{}, fmt.Errorf("run not found: %s", strings.TrimSpace(runID))
	}
	select {
	case <-ctx.Done():
		return Run{}, ctx.Err()
	case <-state.done:
		return r.GetOrZero(runID), nil
	}
}

func (r *Runtime) GetOrZero(runID string) Run {
	run, _ := r.Get(runID)
	return run
}

func (r *Runtime) Get(runID string) (Run, bool) {
	if r == nil {
		return Run{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, ok := r.runs[strings.TrimSpace(runID)]
	if !ok {
		return Run{}, false
	}
	return state.run, true
}

func (r *Runtime) GetByWorkspace(workspaceID, runID string) (Run, bool) {
	if r == nil {
		return Run{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, ok := r.runs[strings.TrimSpace(runID)]
	if !ok {
		return Run{}, false
	}
	if normalizeWorkspaceID(state.run.WorkspaceID) != normalizeWorkspaceID(workspaceID) {
		return Run{}, false
	}
	return state.run, true
}

func (r *Runtime) List(limit int) []Run {
	return r.listRunsByWorkspace("", limit)
}

func (r *Runtime) ListByWorkspace(workspaceID string, limit int) []Run {
	return r.listRunsByWorkspace(workspaceID, limit)
}

func (r *Runtime) listRunsByWorkspace(workspaceID string, limit int) []Run {
	if r == nil {
		return []Run{}
	}
	if limit <= 0 {
		limit = 50
	}
	targetWorkspaceID := strings.TrimSpace(workspaceID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Run, 0, len(r.runOrder))
	for i := len(r.runOrder) - 1; i >= 0; i-- {
		id := r.runOrder[i]
		state := r.runs[id]
		if state == nil {
			continue
		}
		if targetWorkspaceID != "" && normalizeWorkspaceID(state.run.WorkspaceID) != normalizeWorkspaceID(targetWorkspaceID) {
			continue
		}
		out = append(out, state.run)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (r *Runtime) Cancel(runID string) (Run, error) {
	return r.cancelRun(runID, "")
}

func (r *Runtime) CancelByWorkspace(workspaceID, runID string) (Run, error) {
	return r.cancelRun(runID, workspaceID)
}

func (r *Runtime) cancelRun(runID, workspaceID string) (Run, error) {
	if r == nil {
		return Run{}, fmt.Errorf("gateway runtime is disabled")
	}
	runID = strings.TrimSpace(runID)
	targetWorkspaceID := strings.TrimSpace(workspaceID)
	r.mu.Lock()
	state, ok := r.runs[runID]
	if !ok {
		r.mu.Unlock()
		return Run{}, fmt.Errorf("run not found: %s", runID)
	}
	if targetWorkspaceID != "" && normalizeWorkspaceID(state.run.WorkspaceID) != normalizeWorkspaceID(targetWorkspaceID) {
		r.mu.Unlock()
		return Run{}, fmt.Errorf("run not found: %s", runID)
	}
	if state.run.Status == RunStatusCompleted || state.run.Status == RunStatusFailed || state.run.Status == RunStatusCanceled {
		run := state.run
		r.mu.Unlock()
		return run, nil
	}
	if state.cancel != nil {
		state.cancel()
	}
	now := r.nowFn().UTC().Format(time.RFC3339)
	state.run.Status = RunStatusCanceled
	state.run.Error = "canceled by user"
	state.run.CompletedAt = now
	state.run.UpdatedAt = now
	r.closeRunDoneLocked(state)
	r.trimRunHistoryLocked()
	r.stateVersion++
	run := state.run
	r.mu.Unlock()
	r.persistSnapshot()
	return run, nil
}

func classifyRunDiagnostic(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	reason := strings.TrimSpace(err.Error())
	if reason == "" {
		return "", ""
	}
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "tool not injected for this request"):
		return "policy_tool_blocked", reason
	case strings.Contains(lower, "repeated tool call pattern"):
		return "agent_loop_guard", reason
	case strings.Contains(lower, "context canceled"), strings.Contains(lower, "canceled"):
		return "run_canceled", reason
	default:
		return "run_failed", reason
	}
}
