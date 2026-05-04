package tarsserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/pulse"
	"github.com/devlikebear/tars/internal/pulse/autofix"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

const autoResumeEscalationWindow = 30 * time.Minute
const autoResumeEscalationLimit = 3

type autoResumeTurnRunner func(ctx context.Context, candidate pulse.StalledChatCandidate, prompt string) (string, error)

type sessionAutoResumeController struct {
	workspaceDir string
	store        *session.Store
	manager      *ops.Manager
	chatDeps     chatHandlerDeps
	emit         func(ctx context.Context, evt notificationEvent)
	logger       zerolog.Logger
	now          func() time.Time
	runTurn      autoResumeTurnRunner
}

func (c *sessionAutoResumeController) AutoContinueStalledChats(ctx context.Context) (autofix.ChatAutoContinueResult, error) {
	if c == nil || c.store == nil {
		return autofix.ChatAutoContinueResult{}, fmt.Errorf("session auto-resume is not configured")
	}
	now := c.nowTime()
	candidates, err := pulse.DetectStalledChatCandidates(ctx, c.store, now)
	if err != nil {
		return autofix.ChatAutoContinueResult{}, err
	}
	result := autofix.ChatAutoContinueResult{}
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if !candidate.CanAutoResume {
			result.Skipped++
			c.recordAutoResumeAudit(candidate, "blocked", blockedAutoResumeReason(candidate), "")
			continue
		}
		if c.recentAutoResumeCount(candidate.SessionID, now) >= autoResumeEscalationLimit {
			result.Escalated++
			c.recordAutoResumeAudit(candidate, "escalated", "auto-resume limit exceeded", "")
			c.emitAutoResumeNotification(ctx, candidate, "warn", "Auto-resume paused", "stalled chat needs user attention after repeated auto-resumes")
			continue
		}
		prompt := buildAutoResumePrompt(candidate)
		runTurn := c.runTurn
		if runTurn == nil {
			runTurn = c.runAutoResumeTurn
		}
		summary, err := runTurn(ctx, candidate, prompt)
		if err != nil {
			result.Skipped++
			c.recordAutoResumeAudit(candidate, "failed", "auto-resume chat run failed", err.Error())
			continue
		}
		result.Resumed++
		result.SessionIDs = append(result.SessionIDs, candidate.SessionID)
		c.recordAutoResumeAudit(candidate, "success", "stalled chat auto-resumed", summary)
		c.emitAutoResumeNotification(ctx, candidate, "info", "Chat auto-resumed", "pulse continued a stalled chat using "+candidate.ResumeMode)
	}
	return result, nil
}

func (c *sessionAutoResumeController) runAutoResumeTurn(ctx context.Context, candidate pulse.StalledChatCandidate, prompt string) (string, error) {
	if c == nil || c.store == nil {
		return "", fmt.Errorf("session store is not configured")
	}
	requestWorkspaceDir := strings.TrimSpace(c.workspaceDir)
	if requestWorkspaceDir == "" {
		requestWorkspaceDir = c.chatDeps.workspaceDir
	}
	if requestWorkspaceDir == "" {
		return "", fmt.Errorf("workspace dir is not configured")
	}
	if _, err := maybeAutoCompactSession(
		requestWorkspaceDir,
		c.store.TranscriptPath(candidate.SessionID),
		candidate.SessionID,
		c.store,
		c.chatDeps.router,
		c.logger,
		c.chatDeps.tooling.Compaction,
		c.chatDeps.tooling.MemorySemanticConfig,
	); err != nil {
		return "", err
	}
	state, err := buildSessionChatRunState(
		requestWorkspaceDir,
		defaultWorkspaceID,
		c.store,
		candidate.SessionID,
		prompt,
		nil,
		nil,
		nil,
		false,
		serverauth.RoleUser,
		c.chatDeps,
	)
	if err != nil {
		return "", err
	}
	now := c.nowTime().UTC()
	userMsg := session.Message{Role: "user", Content: prompt, Timestamp: now}
	if err := session.AppendMessage(state.transcriptPath, userMsg); err != nil {
		return "", err
	}
	runCtx := usage.WithCallMeta(ctx, usage.CallMeta{Source: "pulse", SessionID: state.sessionID})
	runCtx = tool.WithCurrentSessionInfo(runCtx, state.sessionID, state.sessionKind)
	if c.chatDeps.router != nil {
		if _, resolution, err := c.chatDeps.router.ClientFor(llm.RoleChatMain); err == nil {
			runCtx = llm.WithSelectionMetadata(runCtx, llm.SelectionMetadata{
				Role:      llm.RoleChatMain,
				Tier:      resolution.Tier,
				Provider:  resolution.Provider,
				Model:     resolution.Model,
				Source:    "pulse_auto_resume",
				SessionID: state.sessionID,
			})
		}
	}
	loop, toolCallRecords := setupAgentLoop(
		state.llmClient,
		state.registry,
		state.sessionID,
		len(state.history),
		c.chatDeps.tooling.UsageTracker,
		c.logger,
		func(string, string, string, string, string, string, ...bool) {},
		nil,
	)
	resp, err := loop.Run(runCtx, state.llmMessages, agent.RunOptions{
		MaxIterations: c.chatDeps.maxIters,
		Tools:         state.injectedSchemas,
		BlockedTools:  state.blockedTools,
		ToolChoice:    state.toolChoice,
	})
	if err != nil {
		return "", err
	}
	persistChatResult(state, prompt, resp, *toolCallRecords, c.logger)
	startMemoryPrefetchForNextTurn(
		state.requestWorkspaceDir,
		prompt,
		state.sessionID,
		c.chatDeps.tooling.MemorySemanticConfig,
		c.chatDeps.tooling.MemoryCache,
		c.chatDeps.tooling.PlanClarifyMode,
	)
	return trimForMemory(resp.Message.Content, 240), nil
}

func (c *sessionAutoResumeController) recentAutoResumeCount(sessionID string, now time.Time) int {
	if c == nil || c.manager == nil || strings.TrimSpace(sessionID) == "" {
		return 0
	}
	items, err := c.manager.ListAutomationAudit(ops.AutomationAuditListOptions{SessionID: sessionID, Limit: 500})
	if err != nil {
		c.logger.Debug().Err(err).Str("session_id", sessionID).Msg("auto-resume audit read failed")
		return 0
	}
	cutoff := now.Add(-autoResumeEscalationWindow)
	count := 0
	for _, item := range items {
		if item.Action != autofix.AutoContinueChatName || item.Result != "success" {
			continue
		}
		if item.Timestamp.Before(cutoff) {
			continue
		}
		count++
	}
	return count
}

func (c *sessionAutoResumeController) recordAutoResumeAudit(candidate pulse.StalledChatCandidate, result string, reason string, detail string) {
	if c == nil || c.manager == nil {
		return
	}
	cwd := ""
	if sess, err := c.store.Get(candidate.SessionID); err == nil {
		cwd = sess.CurrentDir
	}
	details := map[string]any{
		"resume_mode":         candidate.ResumeMode,
		"last_message_id":     candidate.LastMessageID,
		"age_minutes":         candidate.AgeMinutes,
		"auto_resume_enabled": candidate.AutoResumeEnabled,
	}
	if strings.TrimSpace(candidate.BlockReason) != "" {
		details["block_reason"] = candidate.BlockReason
	}
	if strings.TrimSpace(detail) != "" {
		details["detail"] = trimForMemory(detail, 360)
	}
	_, _ = c.manager.RecordAutomationAudit(ops.AutomationAuditEntry{
		Actor:     "pulse",
		Action:    autofix.AutoContinueChatName,
		Reason:    reason,
		SessionID: candidate.SessionID,
		CWD:       cwd,
		Result:    result,
		Details:   details,
	})
}

func (c *sessionAutoResumeController) emitAutoResumeNotification(ctx context.Context, candidate pulse.StalledChatCandidate, severity string, title string, message string) {
	if c == nil || c.emit == nil {
		return
	}
	evt := newNotificationEvent("pulse", severity, title, message)
	evt.SessionID = candidate.SessionID
	if candidate.SessionID != "" {
		evt.OpenPath = "/console/chat/" + candidate.SessionID
	}
	c.emit(ctx, evt)
}

func (c *sessionAutoResumeController) nowTime() time.Time {
	if c != nil && c.now != nil {
		return c.now().UTC()
	}
	return time.Now().UTC()
}

func blockedAutoResumeReason(candidate pulse.StalledChatCandidate) string {
	if candidate.BlockReason != "" {
		return candidate.BlockReason
	}
	if !candidate.AutoResumeEnabled {
		return "session has not enabled auto-resume"
	}
	return "stalled chat is not safe to continue"
}

func buildAutoResumePrompt(candidate pulse.StalledChatCandidate) string {
	mode := strings.TrimSpace(candidate.ResumeMode)
	if mode == "" {
		mode = session.AutoResumeModeRecordAssumptionAndProceed
	}
	return fmt.Sprintf(`[PULSE AUTO-RESUME]
mode: %s
last_assistant_message_id: %s

The previous assistant turn appears to be waiting for user input while this session still has active work.
Continue only if it is safe. Do not invent user-specific facts, credentials, permissions, preferences, or approvals.
If a decisive answer is required, explicitly record a conservative assumption and move to the next safe task.
If there is no safe next step, summarize the blocker and pause.
`, mode, strings.TrimSpace(candidate.LastMessageID))
}

// ResumeFailedChats retries chat sessions whose last activity is a halted
// turn (tool error from a non-mutating tool, or a user message the LLM never
// answered). It mirrors AutoContinueStalledChats but reads from the failed-
// chat detector and uses a retry-flavored prompt. Per-session escalation
// limits are scoped to this autofix's audit action so the counter is
// independent of the question-resume counter.
func (c *sessionAutoResumeController) ResumeFailedChats(ctx context.Context) (autofix.FailedChatResumeResult, error) {
	if c == nil || c.store == nil {
		return autofix.FailedChatResumeResult{}, fmt.Errorf("session auto-resume is not configured")
	}
	now := c.nowTime()
	candidates, err := pulse.DetectFailedChatCandidates(ctx, c.store, now)
	if err != nil {
		return autofix.FailedChatResumeResult{}, err
	}
	result := autofix.FailedChatResumeResult{}
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if !candidate.CanAutoResume {
			result.Skipped++
			c.recordFailedChatAudit(candidate, "blocked", blockedFailedChatReason(candidate), "")
			continue
		}
		if c.recentFailedChatResumeCount(candidate.SessionID, now) >= autoResumeEscalationLimit {
			result.Escalated++
			c.recordFailedChatAudit(candidate, "escalated", "auto-resume retry limit exceeded", "")
			c.emitFailedChatNotification(ctx, candidate, "warn", "Auto-retry paused", "halted chat needs user attention after repeated retries")
			continue
		}
		prompt := buildFailedChatRetryPrompt(candidate)
		runTurn := c.runTurn
		if runTurn == nil {
			runTurn = c.runAutoResumeTurn
		}
		// Reuse the stalled-chat turn runner — only the prompt and audit
		// action differ. Wrap the candidate as a StalledChatCandidate to fit
		// the existing signature; the runner only reads SessionID and
		// LastMessageID from it.
		shimmed := pulse.StalledChatCandidate{
			SessionID:     candidate.SessionID,
			LastMessageID: candidate.LastMessageID,
		}
		summary, err := runTurn(ctx, shimmed, prompt)
		if err != nil {
			result.Skipped++
			c.recordFailedChatAudit(candidate, "failed", "auto-resume failed-chat run failed", err.Error())
			continue
		}
		result.Resumed++
		result.SessionIDs = append(result.SessionIDs, candidate.SessionID)
		c.recordFailedChatAudit(candidate, "success", "halted chat retried", summary)
		c.emitFailedChatNotification(ctx, candidate, "info", "Chat retry succeeded", "pulse retried a halted chat ("+candidate.FailureKind+")")
	}
	return result, nil
}

func (c *sessionAutoResumeController) recentFailedChatResumeCount(sessionID string, now time.Time) int {
	if c == nil || c.manager == nil || strings.TrimSpace(sessionID) == "" {
		return 0
	}
	items, err := c.manager.ListAutomationAudit(ops.AutomationAuditListOptions{SessionID: sessionID, Limit: 500})
	if err != nil {
		c.logger.Debug().Err(err).Str("session_id", sessionID).Msg("failed-chat resume audit read failed")
		return 0
	}
	cutoff := now.Add(-autoResumeEscalationWindow)
	count := 0
	for _, item := range items {
		if item.Action != autofix.AutoResumeFailedChatName || item.Result != "success" {
			continue
		}
		if item.Timestamp.Before(cutoff) {
			continue
		}
		count++
	}
	return count
}

func (c *sessionAutoResumeController) recordFailedChatAudit(candidate pulse.FailedChatCandidate, result string, reason string, detail string) {
	if c == nil || c.manager == nil {
		return
	}
	cwd := ""
	if sess, err := c.store.Get(candidate.SessionID); err == nil {
		cwd = sess.CurrentDir
	}
	details := map[string]any{
		"failure_kind":        candidate.FailureKind,
		"failing_tool":        candidate.FailingToolName,
		"last_message_id":     candidate.LastMessageID,
		"age_minutes":         candidate.AgeMinutes,
		"auto_resume_enabled": candidate.AutoResumeEnabled,
	}
	if strings.TrimSpace(candidate.BlockReason) != "" {
		details["block_reason"] = candidate.BlockReason
	}
	if strings.TrimSpace(detail) != "" {
		details["detail"] = trimForMemory(detail, 360)
	}
	_, _ = c.manager.RecordAutomationAudit(ops.AutomationAuditEntry{
		Actor:     "pulse",
		Action:    autofix.AutoResumeFailedChatName,
		Reason:    reason,
		SessionID: candidate.SessionID,
		CWD:       cwd,
		Result:    result,
		Details:   details,
	})
}

func (c *sessionAutoResumeController) emitFailedChatNotification(ctx context.Context, candidate pulse.FailedChatCandidate, severity string, title string, message string) {
	if c == nil || c.emit == nil {
		return
	}
	evt := newNotificationEvent("pulse", severity, title, message)
	evt.SessionID = candidate.SessionID
	if candidate.SessionID != "" {
		evt.OpenPath = "/console/chat/" + candidate.SessionID
	}
	c.emit(ctx, evt)
}

func blockedFailedChatReason(candidate pulse.FailedChatCandidate) string {
	if candidate.BlockReason != "" {
		return candidate.BlockReason
	}
	if !candidate.AutoResumeEnabled {
		return "session has not enabled auto-resume"
	}
	return "halted chat is not safe to retry"
}

func buildFailedChatRetryPrompt(candidate pulse.FailedChatCandidate) string {
	preview := strings.TrimSpace(candidate.FailurePreview)
	switch candidate.FailureKind {
	case pulse.FailedChatKindToolError:
		return fmt.Sprintf(`[PULSE AUTO-RESUME — RETRY]
failure_kind: tool_error
failing_tool: %s
last_message_id: %s

The previous turn halted because the %q tool returned an error. The error preview is below.
Inspect the error, decide whether to retry the same tool with adjusted arguments, switch to a different tool, or summarise the failure and pause.
Do not invent results or assume the tool succeeded.

Error preview:
%s
`, strings.TrimSpace(candidate.FailingToolName), strings.TrimSpace(candidate.LastMessageID), strings.TrimSpace(candidate.FailingToolName), preview)
	case pulse.FailedChatKindNoResponse:
		return fmt.Sprintf(`[PULSE AUTO-RESUME — RETRY]
failure_kind: no_response
last_message_id: %s

The previous turn halted before any assistant or tool response was produced (likely a transient LLM or network failure).
Read the user's last message in the transcript and respond now. If you cannot make progress, summarise the blocker and pause.
`, strings.TrimSpace(candidate.LastMessageID))
	default:
		return fmt.Sprintf(`[PULSE AUTO-RESUME — RETRY]
last_message_id: %s

The previous turn halted unexpectedly. Inspect the transcript and continue if it is safe; otherwise summarise the blocker and pause.
`, strings.TrimSpace(candidate.LastMessageID))
	}
}

// ContinueGoalPlans runs one chat turn per session whose plan has just
// completed with auto-continue opted in. The turn asks the LLM to either
// declare the session goal achieved (which terminates the loop by clearing
// the auto-continue flag) or propose a follow-up plan via the tasks tool.
//
// The iteration cap is enforced by counting successful audit-log entries in
// the rolling AutoContinueIterationWindow — this survives plan replacement
// when the LLM proposes a new plan to keep working.
func (c *sessionAutoResumeController) ContinueGoalPlans(ctx context.Context) (autofix.GoalPlanAutoContinueResult, error) {
	if c == nil || c.store == nil {
		return autofix.GoalPlanAutoContinueResult{}, fmt.Errorf("goal-plan auto-continuer is not configured")
	}
	now := c.nowTime()
	candidates, err := pulse.DetectAutoContinueGoalCandidates(ctx, c.store, now)
	if err != nil {
		return autofix.GoalPlanAutoContinueResult{}, err
	}
	result := autofix.GoalPlanAutoContinueResult{}
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if !candidate.CanAutoContinue {
			result.Skipped++
			c.recordGoalPlanAudit(candidate, "blocked", blockedGoalPlanReason(candidate), "")
			continue
		}
		used := c.recentGoalPlanContinueCount(candidate.SessionID, now)
		cap := candidate.MaxIterations
		if cap <= 0 {
			cap = session.DefaultAutoContinueMaxIterations
		}
		if used >= cap {
			result.Escalated++
			c.recordGoalPlanAudit(candidate, "escalated", "goal-plan iteration cap reached", "")
			c.emitGoalPlanNotification(ctx, candidate, "warn", "Goal auto-continue paused", "iteration cap reached; goal session needs user attention")
			c.disableAutoContinue(candidate.SessionID, "iteration_cap_reached")
			continue
		}
		prompt := buildGoalContinuePrompt(candidate, used, cap)
		runTurn := c.runTurn
		if runTurn == nil {
			runTurn = c.runAutoResumeTurn
		}
		shimmed := pulse.StalledChatCandidate{SessionID: candidate.SessionID}
		summary, err := runTurn(ctx, shimmed, prompt)
		if err != nil {
			result.Skipped++
			c.recordGoalPlanAudit(candidate, "failed", "goal-plan auto-continue run failed", err.Error())
			continue
		}
		// Re-read the plan after the turn — the LLM may have replaced it,
		// flipped AutoContinueEnabled to false (goal achieved), or left it
		// unchanged. Use that to classify the outcome and decide whether to
		// emit a "goal completed" notification.
		outcome, _ := c.classifyGoalPlanOutcome(candidate.SessionID)
		switch outcome {
		case "goal_completed":
			result.GoalsCompleted++
			result.SessionIDs = append(result.SessionIDs, candidate.SessionID)
			c.recordGoalPlanAudit(candidate, "success", "goal achieved by auto-continue", summary)
			c.emitGoalPlanNotification(ctx, candidate, "info", "Goal achieved", "auto-continue ended after the LLM declared the goal complete")
		default:
			result.Continued++
			result.SessionIDs = append(result.SessionIDs, candidate.SessionID)
			c.recordGoalPlanAudit(candidate, "success", "auto-continued goal plan", summary)
			c.emitGoalPlanNotification(ctx, candidate, "info", "Plan auto-continued", fmt.Sprintf("pulse continued the goal session (iter %d/%d)", used+1, cap))
		}
	}
	return result, nil
}

func (c *sessionAutoResumeController) recentGoalPlanContinueCount(sessionID string, now time.Time) int {
	if c == nil || c.manager == nil || strings.TrimSpace(sessionID) == "" {
		return 0
	}
	items, err := c.manager.ListAutomationAudit(ops.AutomationAuditListOptions{SessionID: sessionID, Limit: 500})
	if err != nil {
		c.logger.Debug().Err(err).Str("session_id", sessionID).Msg("goal-plan audit read failed")
		return 0
	}
	cutoff := now.Add(-session.AutoContinueIterationWindow)
	count := 0
	for _, item := range items {
		if item.Action != autofix.AutoContinueGoalPlanName || item.Result != "success" {
			continue
		}
		if item.Timestamp.Before(cutoff) {
			continue
		}
		count++
	}
	return count
}

func (c *sessionAutoResumeController) recordGoalPlanAudit(candidate pulse.AutoContinueGoalCandidate, result string, reason string, detail string) {
	if c == nil || c.manager == nil {
		return
	}
	cwd := ""
	if sess, err := c.store.Get(candidate.SessionID); err == nil {
		cwd = sess.CurrentDir
	}
	details := map[string]any{
		"plan_goal":           candidate.PlanGoal,
		"max_iterations":      candidate.MaxIterations,
		"auto_resume_enabled": candidate.AutoResumeEnabled,
	}
	if strings.TrimSpace(candidate.BlockReason) != "" {
		details["block_reason"] = candidate.BlockReason
	}
	if strings.TrimSpace(detail) != "" {
		details["detail"] = trimForMemory(detail, 360)
	}
	_, _ = c.manager.RecordAutomationAudit(ops.AutomationAuditEntry{
		Actor:     "pulse",
		Action:    autofix.AutoContinueGoalPlanName,
		Reason:    reason,
		SessionID: candidate.SessionID,
		CWD:       cwd,
		Result:    result,
		Details:   details,
	})
}

func (c *sessionAutoResumeController) emitGoalPlanNotification(ctx context.Context, candidate pulse.AutoContinueGoalCandidate, severity string, title string, message string) {
	if c == nil || c.emit == nil {
		return
	}
	evt := newNotificationEvent("pulse", severity, title, message)
	evt.SessionID = candidate.SessionID
	if candidate.SessionID != "" {
		evt.OpenPath = "/console/chat/" + candidate.SessionID
	}
	c.emit(ctx, evt)
}

// classifyGoalPlanOutcome inspects the session's current tasks file after an
// auto-continue turn ran. Three observable outcomes:
//   - "goal_completed": plan exists but AutoContinueEnabled is now false (the
//     LLM disabled it to signal the goal is achieved).
//   - "next_plan": a fresh plan exists with status != completed (the LLM
//     proposed a follow-up).
//   - "no_change": the plan is still completed AND AutoContinueEnabled is
//     still true; the LLM didn't make a clean decision. The next pulse tick
//     will re-attempt up to the cap.
func (c *sessionAutoResumeController) classifyGoalPlanOutcome(sessionID string) (string, error) {
	if c == nil || c.store == nil {
		return "no_change", nil
	}
	tasks, err := c.store.GetTasks(sessionID)
	if err != nil {
		return "no_change", err
	}
	if tasks.Plan == nil {
		return "no_change", nil
	}
	if !tasks.Plan.AutoContinueEnabled {
		return "goal_completed", nil
	}
	if strings.TrimSpace(tasks.Plan.Status) != session.PlanStatusCompleted {
		return "next_plan", nil
	}
	return "no_change", nil
}

// disableAutoContinue clears the AutoContinueEnabled flag on the session's
// active plan after the iteration cap has been reached. This prevents the
// detector from re-triggering on every pulse tick once the cap escalates.
func (c *sessionAutoResumeController) disableAutoContinue(sessionID string, reason string) {
	if c == nil || c.store == nil {
		return
	}
	tasks, err := c.store.GetTasks(sessionID)
	if err != nil || tasks.Plan == nil {
		return
	}
	if !tasks.Plan.AutoContinueEnabled {
		return
	}
	tasks.Plan.AutoContinueEnabled = false
	if err := c.store.SaveTasks(sessionID, tasks); err != nil {
		c.logger.Debug().Err(err).Str("session_id", sessionID).Str("reason", reason).Msg("failed to disable auto-continue after cap")
	}
}

func blockedGoalPlanReason(candidate pulse.AutoContinueGoalCandidate) string {
	if candidate.BlockReason != "" {
		return candidate.BlockReason
	}
	if !candidate.AutoResumeEnabled {
		return "session has not enabled auto-resume"
	}
	return "goal plan is not safe to auto-continue"
}

func buildGoalContinuePrompt(candidate pulse.AutoContinueGoalCandidate, used int, cap int) string {
	goal := strings.TrimSpace(candidate.PlanGoal)
	if goal == "" {
		goal = "(unset — please re-state the goal in your reply before continuing)"
	}
	return fmt.Sprintf(`[PULSE AUTO-CONTINUE — GOAL REVIEW]
session_goal: %s
iteration: %d / %d (cap)

The active plan just transitioned to "completed" and this session is opted
in to auto-continue. Decide ONE of:

1. Goal achieved → call the tasks tool to set the active plan's
   "auto_continue_enabled" to false, summarise the outcome, and stop.
2. Goal not yet achieved → propose the next plan (use the tasks tool to
   create the new plan + tasks). Keep "auto_continue_enabled" true only if
   you genuinely expect another short loop to finish the goal.

Hard rules:
- Do not invent user-specific facts, credentials, permissions, or approvals.
- If you cannot decide cleanly, set auto_continue_enabled=false and
  summarise the blocker. Pulse will not re-trigger.
- The iteration counter above is a safety bound; the cap will trip
  automatically and disable auto-continue.
`, goal, used+1, cap)
}
