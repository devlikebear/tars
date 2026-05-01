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
