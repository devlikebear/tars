package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
)

type blockedToolDiagnostic struct {
	Tool   string
	Rule   string
	Group  string
	Source string
}

func (r *Runtime) executeRun(ctx context.Context, runID string) {
	state, ok := r.getRunState(runID)
	if !ok {
		return
	}
	if err := r.acquireExecutionSlot(ctx); err != nil {
		r.mu.Lock()
		if state.run.Status != RunStatusCanceled {
			r.finalizeRunLocked(state, "", PromptExecutionMetadata{}, err)
		}
		r.mu.Unlock()
		r.persistSnapshot()
		return
	}
	defer r.releaseExecutionSlot()

	executor, ok := r.startRunExecution(state)
	if !ok {
		return
	}

	resp, metadata, err := r.executeRunPrompt(ctx, state, executor)

	r.mu.Lock()
	if state.run.Status == RunStatusCanceled {
		r.closeRunDoneLocked(state)
		r.trimRunHistoryLocked()
		r.stateVersion++
		r.publishRunEvent(state.run.ID, RunEvent{Type: "run_finished", RunID: state.run.ID, Agent: state.run.Agent, Status: string(state.run.Status)})
		r.mu.Unlock()
		r.persistSnapshot()
		return
	}
	r.finalizeRunLocked(state, resp, metadata, err)
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
	r.publishRunEvent(state.run.ID, RunEvent{Type: "run_started", RunID: state.run.ID, Timestamp: now, Agent: state.run.Agent, Status: string(state.run.Status), Tier: state.run.Tier})
	return executor, true
}

func (r *Runtime) executeRunPrompt(ctx context.Context, state *runState, executor AgentExecutor) (string, PromptExecutionMetadata, error) {
	_ = r.appendSessionMessage(state.run.WorkspaceID, state.run.SessionID, "user", state.run.Prompt, r.nowFn().UTC())
	if strings.EqualFold(strings.TrimSpace(state.req.Mode), "consensus") || state.req.Consensus != nil {
		resp, err := r.runConsensus(ctx, state, executor)
		return resp, PromptExecutionMetadata{}, err
	}
	if err := r.subagentPool.Acquire(ctx); err != nil {
		return "", PromptExecutionMetadata{}, err
	}
	defer r.subagentPool.Release()
	if executor == nil {
		return "", PromptExecutionMetadata{}, fmt.Errorf("agent executor is not configured")
	}

	allowedTools := resolveRunAllowedTools(
		r.opts.WorkspaceDir,
		gatewayAgentInfo(executor).ToolsAllow,
	)
	execCtx := serverauth.WithWorkspaceID(ctx, state.run.WorkspaceID)
	execCtx = usage.WithCallMeta(execCtx, usage.CallMeta{
		Source:    "agent_run",
		SessionID: state.run.SessionID,
		RunID:     state.run.ID,
	})
	execCtx = llm.WithSelectionMetadata(execCtx, llm.SelectionMetadata{
		SessionID: state.run.SessionID,
		RunID:     state.run.ID,
		AgentName: state.run.Agent,
		FlowID:    state.run.FlowID,
		StepID:    state.run.StepID,
	})
	metadata := PromptExecutionMetadata{}
	resp, err := executor.Execute(execCtx, ExecuteRequest{
		RunID:            state.run.ID,
		WorkspaceID:      state.run.WorkspaceID,
		SessionID:        state.run.SessionID,
		Prompt:           state.run.Prompt,
		AllowedTools:     allowedTools,
		Tier:             state.run.Tier,
		ProviderOverride: CloneProviderOverride(state.run.ProviderOverride),
		Metadata:         &metadata,
	})
	if err == nil && ctx.Err() == nil {
		assistant := strings.TrimSpace(resp)
		if assistant != "" {
			_ = r.appendSessionMessage(state.run.WorkspaceID, state.run.SessionID, "assistant", assistant, r.nowFn().UTC())
		}
	}
	return resp, metadata, err
}

func (r *Runtime) finalizeRunLocked(state *runState, resp string, metadata PromptExecutionMetadata, err error) {
	finishedAt := r.nowFn().UTC().Format(time.RFC3339)
	state.run.CompletedAt = finishedAt
	state.run.UpdatedAt = finishedAt
	state.run.ResolvedAlias = strings.TrimSpace(metadata.ResolvedAlias)
	state.run.ResolvedKind = strings.TrimSpace(metadata.ResolvedKind)
	state.run.ResolvedModel = strings.TrimSpace(metadata.ResolvedModel)
	state.run.OverrideSource = strings.TrimSpace(metadata.OverrideSource)
	if err != nil {
		state.run.Status = RunStatusFailed
		state.run.Error = strings.TrimSpace(err.Error())
		state.run.DiagnosticCode, state.run.DiagnosticReason = classifyRunDiagnostic(err)
		if state.run.DiagnosticCode == "policy_tool_blocked" {
			info := gatewayAgentInfo(state.executor)
			state.run.PolicyBlockedTool = blockedToolNameFromReason(state.run.DiagnosticReason)
			if blocked, ok := blockedToolErrorFromReason(state.run.DiagnosticReason); ok {
				state.run.PolicyBlockedTool = blocked.Tool
				state.run.PolicyBlockedRule = blocked.Rule
				state.run.PolicyBlockedGroup = blocked.Group
				state.run.PolicyBlockedSource = blocked.Source
			}
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
		r.publishRunEvent(state.run.ID, RunEvent{
			Type:          "run_failed",
			RunID:         state.run.ID,
			Agent:         state.run.Agent,
			Status:        string(state.run.Status),
			ResolvedAlias: state.run.ResolvedAlias,
			ResolvedKind:  state.run.ResolvedKind,
			ResolvedModel: state.run.ResolvedModel,
			Error:         state.run.Error,
		})
		r.closeRunDoneLocked(state)
		r.trimRunHistoryLocked()
		r.stateVersion++
		r.appendRunSummaryToMain(state.run, "")
		r.publishRunEvent(state.run.ID, RunEvent{Type: "run_failed", RunID: state.run.ID, Timestamp: finishedAt, Status: string(state.run.Status), Error: state.run.Error})
		return
	}
	state.run.Status = RunStatusCompleted
	state.run.Response = strings.TrimSpace(resp)
	r.publishRunEvent(state.run.ID, RunEvent{
		Type:          "run_finished",
		RunID:         state.run.ID,
		Agent:         state.run.Agent,
		Status:        string(state.run.Status),
		ResolvedAlias: state.run.ResolvedAlias,
		ResolvedKind:  state.run.ResolvedKind,
		ResolvedModel: state.run.ResolvedModel,
		Response:      state.run.Response,
	})
	r.appendRunSummaryToMain(state.run, state.run.Response)
	r.publishRunEvent(state.run.ID, RunEvent{Type: "run_finished", RunID: state.run.ID, Timestamp: finishedAt, Status: string(state.run.Status), Message: trimGatewaySummary(state.run.Response, 220)})
	r.closeRunDoneLocked(state)
	r.trimRunHistoryLocked()
	r.stateVersion++
}

func blockedToolNameFromReason(reason string) string {
	if blocked, ok := blockedToolErrorFromReason(reason); ok {
		return strings.TrimSpace(blocked.Tool)
	}
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

func blockedToolErrorFromReason(reason string) (blockedToolDiagnostic, bool) {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return blockedToolDiagnostic{}, false
	}
	const prefix = "tool not injected for this request:"
	if !strings.Contains(strings.ToLower(trimmed), prefix) {
		return blockedToolDiagnostic{}, false
	}
	toolName := blockedToolNameFromReasonFallback(trimmed)
	if toolName == "" {
		return blockedToolDiagnostic{}, false
	}
	blocked := blockedToolDiagnostic{Tool: toolName}
	if idx := strings.Index(trimmed, "["); idx >= 0 && strings.HasSuffix(trimmed, "]") {
		meta := strings.TrimSpace(strings.TrimSuffix(trimmed[idx+1:], "]"))
		for _, part := range strings.Fields(meta) {
			key, value, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			switch strings.TrimSpace(key) {
			case "rule":
				blocked.Rule = strings.TrimSpace(value)
			case "group":
				blocked.Group = strings.TrimSpace(value)
			case "source":
				blocked.Source = strings.TrimSpace(value)
			}
		}
	}
	return blocked, true
}

func blockedToolNameFromReasonFallback(reason string) string {
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
	if marker := strings.Index(toolName, "["); marker >= 0 {
		toolName = strings.TrimSpace(toolName[:marker])
	}
	return toolName
}

func (r *Runtime) acquireExecutionSlot(ctx context.Context) error {
	if r == nil || r.executionSem == nil {
		return nil
	}
	return r.executionSem.Acquire(ctx)
}

func (r *Runtime) releaseExecutionSlot() {
	if r == nil || r.executionSem == nil {
		return
	}
	r.executionSem.Release()
}

func resolveRunAllowedTools(_ string, executorAllowed []string) []string {
	return sanitizeStringList(executorAllowed)
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
		"[RUN SUMMARY]\nagent: %s\nstatus: %s\nresult: %s",
		strings.TrimSpace(run.Agent),
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
