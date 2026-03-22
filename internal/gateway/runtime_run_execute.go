package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
)

func (r *Runtime) executeRun(ctx context.Context, runID string) {
	state, ok := r.getRunState(runID)
	if !ok {
		return
	}

	executor, ok := r.startRunExecution(state)
	if !ok {
		return
	}

	resp, err := r.executeRunPrompt(ctx, state, executor)

	r.mu.Lock()
	if state.run.Status == RunStatusCanceled {
		r.closeRunDoneLocked(state)
		r.trimRunHistoryLocked()
		r.stateVersion++
		r.mu.Unlock()
		r.persistSnapshot()
		return
	}
	r.finalizeRunLocked(state, resp, err)
	r.mu.Unlock()
	r.persistSnapshot()
}

func (r *Runtime) startRunExecution(state *runState) (AgentExecutor, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if state.run.Status == RunStatusCanceled {
		r.closeRunDoneLocked(state)
		return nil, false
	}
	executor := state.executor
	now := r.nowFn().UTC().Format(time.RFC3339)
	state.run.Status = RunStatusRunning
	state.run.StartedAt = now
	state.run.UpdatedAt = now
	r.stateVersion++
	return executor, true
}

func (r *Runtime) executeRunPrompt(ctx context.Context, state *runState, executor AgentExecutor) (string, error) {
	_ = r.appendSessionMessage(state.run.WorkspaceID, state.run.SessionID, "user", state.run.Prompt, r.nowFn().UTC())
	if executor == nil {
		return "", fmt.Errorf("agent executor is not configured")
	}

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
	resp, err := executor.Execute(execCtx, ExecuteRequest{
		RunID:        state.run.ID,
		WorkspaceID:  state.run.WorkspaceID,
		SessionID:    state.run.SessionID,
		ProjectID:    state.run.ProjectID,
		Prompt:       state.run.Prompt,
		AllowedTools: allowedTools,
	})
	if err == nil && ctx.Err() == nil {
		assistant := strings.TrimSpace(resp)
		if assistant != "" {
			_ = r.appendSessionMessage(state.run.WorkspaceID, state.run.SessionID, "assistant", assistant, r.nowFn().UTC())
		}
	}
	return resp, err
}

func (r *Runtime) finalizeRunLocked(state *runState, resp string, err error) {
	finishedAt := r.nowFn().UTC().Format(time.RFC3339)
	state.run.CompletedAt = finishedAt
	state.run.UpdatedAt = finishedAt
	if err != nil {
		state.run.Status = RunStatusFailed
		state.run.Error = strings.TrimSpace(err.Error())
		state.run.DiagnosticCode, state.run.DiagnosticReason = classifyRunDiagnostic(err)
		if state.run.DiagnosticCode == "policy_tool_blocked" {
			info := gatewayAgentInfo(state.executor)
			state.run.PolicyBlockedTool = blockedToolNameFromReason(state.run.DiagnosticReason)
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
		r.appendRunSummaryToMain(state.run, "")
		return
	}
	state.run.Status = RunStatusCompleted
	state.run.Response = strings.TrimSpace(resp)
	r.appendRunSummaryToMain(state.run, state.run.Response)
	r.closeRunDoneLocked(state)
	r.trimRunHistoryLocked()
	r.stateVersion++
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

func (r *Runtime) appendRunSummaryToMain(run Run, response string) {
	sessionStore := r.sessionStoreForWorkspace(run.WorkspaceID)
	if r == nil || sessionStore == nil || strings.TrimSpace(run.SessionID) == "" {
		return
	}
	targetSessionID := strings.TrimSpace(run.ParentSessionID)
	if targetSessionID == "" {
		targetSession, err := sessionStore.Get(run.SessionID)
		if err != nil || !targetSession.Hidden || !strings.EqualFold(strings.TrimSpace(targetSession.Kind), "worker") {
			return
		}
		mainSession, err := sessionStore.EnsureMain()
		if err != nil || strings.TrimSpace(mainSession.ID) == "" || strings.TrimSpace(mainSession.ID) == strings.TrimSpace(run.SessionID) {
			return
		}
		targetSessionID = mainSession.ID
	}
	if strings.TrimSpace(targetSessionID) == strings.TrimSpace(run.SessionID) {
		return
	}
	summary := buildRunSummaryMessage(run, response)
	_ = r.appendSessionMessage(run.WorkspaceID, targetSessionID, "system", summary, r.nowFn().UTC())
}

func buildRunSummaryMessage(run Run, response string) string {
	detail := trimGatewaySummary(response, 220)
	if strings.TrimSpace(detail) == "" {
		detail = trimGatewaySummary(run.Error, 220)
	}
	return fmt.Sprintf(
		"[RUN SUMMARY]\nagent: %s\nproject_id: %s\nstatus: %s\nresult: %s",
		strings.TrimSpace(run.Agent),
		strings.TrimSpace(run.ProjectID),
		strings.TrimSpace(string(run.Status)),
		detail,
	)
}

func trimGatewaySummary(text string, max int) string {
	value := strings.TrimSpace(text)
	if max <= 0 || len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}
