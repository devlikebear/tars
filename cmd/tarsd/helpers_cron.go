package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/serverauth"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/rs/zerolog"
)

func newCronJobRunner(
	workspaceDir string,
	store *session.Store,
	runPrompt func(ctx context.Context, runLabel string, promptText string) (string, error),
	logger zerolog.Logger,
) func(ctx context.Context, job cron.Job) (string, error) {
	return newCronJobRunnerWithNotify(workspaceDir, store, runPrompt, logger, nil)
}

func newCronJobRunnerWithNotify(
	workspaceDir string,
	store *session.Store,
	runPrompt func(ctx context.Context, runLabel string, promptText string) (string, error),
	logger zerolog.Logger,
	emit func(ctx context.Context, evt notificationEvent),
) func(ctx context.Context, job cron.Job) (string, error) {
	if runPrompt == nil {
		return nil
	}
	return func(ctx context.Context, job cron.Job) (string, error) {
		targetWorkspaceDir := strings.TrimSpace(workspaceDir)
		targetStore := store
		workspaceID := normalizeWorkspaceID(serverauth.WorkspaceIDFromContext(ctx))
		if workspaceID != defaultWorkspaceID && targetWorkspaceDir != "" {
			targetWorkspaceDir = resolveWorkspaceDir(targetWorkspaceDir, workspaceID)
			if err := memory.EnsureWorkspace(targetWorkspaceDir); err != nil {
				return "", err
			}
			targetStore = session.NewStore(targetWorkspaceDir)
		}

		promptText := strings.TrimSpace(job.Prompt)
		if payload := strings.TrimSpace(string(job.Payload)); payload != "" {
			promptText += "\n\nCRON_PAYLOAD_JSON:\n" + payload
		}

		targetSessionID, explicitTarget, err := resolveCronTargetSessionID(targetStore, job.SessionTarget)
		if err != nil {
			return "", err
		}
		if targetSessionID != "" {
			contextText, err := sessionContextByID(targetStore, targetSessionID, 6)
			if err != nil {
				return "", err
			}
			if contextText != "" {
				promptText += "\n\nTARGET_SESSION_CONTEXT:\n" + contextText
			}
		}

		runLabel := "cron"
		if strings.TrimSpace(job.ID) != "" {
			runLabel = "cron:" + strings.TrimSpace(job.ID)
		}
		response, err := runPrompt(ctx, runLabel, promptText)
		if err != nil {
			if emit != nil {
				evt := newNotificationEvent("cron", "error", "Cron failed", trimForMemory(err.Error(), 240))
				evt.JobID = strings.TrimSpace(job.ID)
				emit(ctx, evt)
			}
			return "", err
		}
		if err := deliverCronResult(targetWorkspaceDir, targetStore, job, targetSessionID, explicitTarget, response, time.Now().UTC(), logger); err != nil {
			if emit != nil {
				evt := newNotificationEvent("cron", "error", "Cron delivery failed", trimForMemory(err.Error(), 240))
				evt.JobID = strings.TrimSpace(job.ID)
				emit(ctx, evt)
			}
			return "", err
		}
		if emit != nil {
			title := "Cron completed"
			if strings.TrimSpace(job.Name) != "" {
				title = "Cron completed: " + strings.TrimSpace(job.Name)
			}
			evt := newNotificationEvent("cron", "info", title, trimForMemory(response, 280))
			evt.JobID = strings.TrimSpace(job.ID)
			evt.SessionID = strings.TrimSpace(targetSessionID)
			emit(ctx, evt)
		}
		return response, nil
	}
}

func resolveCronTargetSessionID(store *session.Store, raw string) (sessionID string, explicitTarget bool, err error) {
	if store == nil {
		return "", false, nil
	}
	target := strings.TrimSpace(raw)
	if target == "" || strings.EqualFold(target, "isolated") {
		return "", false, nil
	}
	if strings.EqualFold(target, "main") {
		latest, err := store.Latest()
		if err != nil {
			if strings.Contains(err.Error(), "session not found") {
				return "", false, nil
			}
			return "", false, err
		}
		return latest.ID, false, nil
	}
	if _, err := store.Get(target); err != nil {
		if strings.Contains(err.Error(), "session not found") {
			return "", true, fmt.Errorf("target session not found: %s", target)
		}
		return "", true, err
	}
	return target, true, nil
}

func sessionContextByID(store *session.Store, sessionID string, maxMessages int) (string, error) {
	if store == nil || strings.TrimSpace(sessionID) == "" {
		return "", nil
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		return "", err
	}
	messages, err := session.ReadMessages(store.TranscriptPath(sessionID))
	if err != nil {
		return "", err
	}
	if len(messages) == 0 {
		return "", nil
	}
	if maxMessages <= 0 {
		maxMessages = 6
	}
	start := len(messages) - maxMessages
	if start < 0 {
		start = 0
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "session_id=%s\nsession_title=%s\n", sess.ID, sess.Title)
	for _, msg := range messages[start:] {
		_, _ = fmt.Fprintf(&b, "- [%s] %s\n", msg.Role, trimForMemory(msg.Content, 180))
	}
	return strings.TrimSpace(b.String()), nil
}

func deliverCronResult(
	workspaceDir string,
	store *session.Store,
	job cron.Job,
	targetSessionID string,
	explicitTarget bool,
	response string,
	now time.Time,
	logger zerolog.Logger,
) error {
	mode := effectiveCronDeliveryMode(job.DeliveryMode, job.SessionTarget)
	if mode == "none" {
		return nil
	}
	entry := fmt.Sprintf(
		"cron job=%s id=%s schedule=%s response=%q",
		strings.TrimSpace(job.Name),
		strings.TrimSpace(job.ID),
		strings.TrimSpace(job.Schedule),
		trimForMemory(response, 280),
	)
	writeDaily := mode == "daily_log" || mode == "both"
	writeSession := mode == "session" || mode == "both"
	if writeDaily {
		if err := memory.AppendDailyLog(workspaceDir, now, entry); err != nil {
			return err
		}
	}
	if writeSession {
		if targetSessionID == "" {
			if explicitTarget {
				return fmt.Errorf("cron delivery session target is not available")
			}
			if mode == "session" {
				return nil
			}
			return nil
		}
		content := fmt.Sprintf(
			"[CRON]\njob: %s\nprompt: %s\nresponse: %s",
			strings.TrimSpace(job.Name),
			trimForMemory(job.Prompt, 180),
			trimForMemory(response, 320),
		)
		if err := session.AppendMessage(store.TranscriptPath(targetSessionID), session.Message{
			Role:      "system",
			Content:   content,
			Timestamp: now.UTC(),
		}); err != nil {
			return err
		}
		if err := store.Touch(targetSessionID, now.UTC()); err != nil {
			return err
		}
		logger.Debug().
			Str("job_id", job.ID).
			Str("session_id", targetSessionID).
			Msg("cron response delivered to session")
	}
	return nil
}

func effectiveCronDeliveryMode(raw string, sessionTarget string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		if strings.EqualFold(strings.TrimSpace(sessionTarget), "main") {
			return "session"
		}
		return "daily_log"
	}
	switch v {
	case "none", "daily_log", "session", "both":
		return v
	default:
		return "daily_log"
	}
}

type workspaceCronManager struct {
	resolver *workspaceCronStoreResolver
	runJob   func(ctx context.Context, job cron.Job) (string, error)
	interval time.Duration
	nowFn    func() time.Time
	logger   zerolog.Logger
}

func newWorkspaceCronManager(
	resolver *workspaceCronStoreResolver,
	runJob func(ctx context.Context, job cron.Job) (string, error),
	interval time.Duration,
	nowFn func() time.Time,
	logger zerolog.Logger,
) *workspaceCronManager {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	return &workspaceCronManager{
		resolver: resolver,
		runJob:   runJob,
		interval: interval,
		nowFn:    nowFn,
		logger:   logger,
	}
}

func (m *workspaceCronManager) Start(ctx context.Context) error {
	if m == nil || m.resolver == nil || m.runJob == nil {
		return nil
	}
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_ = m.Tick(ctx)
		}
	}
}

func (m *workspaceCronManager) Tick(ctx context.Context) error {
	if m == nil || m.resolver == nil || m.runJob == nil {
		return nil
	}
	workspaceIDs, err := m.resolver.WorkspaceIDs()
	if err != nil {
		return err
	}
	var firstErr error
	for _, workspaceID := range workspaceIDs {
		store, err := m.resolver.Resolve(workspaceID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			m.logger.Warn().Err(err).Str("workspace_id", workspaceID).Msg("resolve cron store failed")
			continue
		}
		manager := cron.NewManager(store, m.runJob, m.interval, m.nowFn)
		runCtx := serverauth.WithWorkspaceID(ctx, workspaceID)
		if err := manager.Tick(runCtx); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			m.logger.Warn().Err(err).Str("workspace_id", workspaceID).Msg("cron manager tick failed")
		}
	}
	return firstErr
}
