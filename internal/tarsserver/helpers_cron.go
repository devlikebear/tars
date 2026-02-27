package tarsserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/project"
	"github.com/devlikebear/tarsncase/internal/research"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/devlikebear/tarsncase/internal/usage"
	"github.com/rs/zerolog"
)

func newCronJobRunner(
	workspaceDir string,
	store *session.Store,
	runPrompt func(ctx context.Context, runLabel string, promptText string) (string, error),
	logger zerolog.Logger,
) func(ctx context.Context, job cron.Job) (string, error) {
	return newCronJobRunnerWithNotify(workspaceDir, store, runPrompt, logger, nil, "")
}

func newCronJobRunnerWithNotify(
	workspaceDir string,
	store *session.Store,
	runPrompt func(ctx context.Context, runLabel string, promptText string) (string, error),
	logger zerolog.Logger,
	emit func(ctx context.Context, evt notificationEvent),
	mainSessionID string,
) func(ctx context.Context, job cron.Job) (string, error) {
	if runPrompt == nil {
		return nil
	}
	return func(ctx context.Context, job cron.Job) (string, error) {
		targetWorkspaceDir := strings.TrimSpace(workspaceDir)
		targetStore := store
		if targetStore == nil && targetWorkspaceDir != "" {
			if err := memory.EnsureWorkspace(targetWorkspaceDir); err != nil {
				return "", err
			}
			targetStore = session.NewStore(targetWorkspaceDir)
		}

		promptText := strings.TrimSpace(job.Prompt)
		if payload := strings.TrimSpace(string(job.Payload)); payload != "" {
			promptText += "\n\nCRON_PAYLOAD_JSON:\n" + payload
		}
		if projectPrompt := buildCronProjectPromptSection(targetWorkspaceDir, job.ProjectID); projectPrompt != "" {
			promptText += "\n\n" + projectPrompt
		}

		targetSessionID, explicitTarget, err := resolveCronTargetSessionID(targetStore, job.SessionTarget, mainSessionID)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(job.ProjectID) != "" && targetStore != nil && targetSessionID != "" {
			_ = targetStore.SetProjectID(targetSessionID, strings.TrimSpace(job.ProjectID))
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
		runCtx := usage.WithCallMeta(ctx, usage.CallMeta{
			Source:    "cron",
			SessionID: targetSessionID,
			ProjectID: strings.TrimSpace(job.ProjectID),
			RunID:     strings.TrimSpace(job.ID),
		})
		response, err := runPrompt(runCtx, runLabel, promptText)
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
		if err := persistCronProjectArtifact(targetWorkspaceDir, job, response, time.Now().UTC()); err != nil {
			logger.Debug().Err(err).Str("job_id", strings.TrimSpace(job.ID)).Str("project_id", strings.TrimSpace(job.ProjectID)).Msg("persist cron project artifact failed")
		}
		return response, nil
	}
}

func buildCronProjectPromptSection(workspaceDir string, projectID string) string {
	root := strings.TrimSpace(workspaceDir)
	id := strings.TrimSpace(projectID)
	if root == "" || id == "" {
		return ""
	}
	store := project.NewStore(root, nil)
	item, err := store.Get(id)
	if err != nil {
		return fmt.Sprintf("CRON_PROJECT_CONTEXT:\n- project_id: %s\n- warning: project metadata not found", id)
	}
	artifactDir := filepath.Join(root, "projects", item.ID)
	var b strings.Builder
	b.WriteString("CRON_PROJECT_CONTEXT:\n")
	_, _ = fmt.Fprintf(&b, "- project_id: %s\n", strings.TrimSpace(item.ID))
	_, _ = fmt.Fprintf(&b, "- project_name: %s\n", strings.TrimSpace(item.Name))
	_, _ = fmt.Fprintf(&b, "- project_type: %s\n", strings.TrimSpace(item.Type))
	_, _ = fmt.Fprintf(&b, "- project_status: %s\n", strings.TrimSpace(item.Status))
	_, _ = fmt.Fprintf(&b, "- artifacts_dir: %s\n", artifactDir)
	if body := strings.TrimSpace(item.Body); body != "" {
		b.WriteString("\nPROJECT_INSTRUCTIONS:\n")
		b.WriteString(body)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func persistCronProjectArtifact(workspaceDir string, job cron.Job, response string, now time.Time) error {
	root := strings.TrimSpace(workspaceDir)
	projectID := strings.TrimSpace(job.ProjectID)
	if root == "" || projectID == "" {
		return nil
	}
	if item, err := project.NewStore(root, nil).Get(projectID); err == nil && strings.EqualFold(strings.TrimSpace(item.Type), "research") {
		_, _ = research.NewService(root, research.Options{Now: func() time.Time { return now }}).Run(research.RunInput{
			ProjectID: projectID,
			Topic:     "cron:" + strings.TrimSpace(job.Name),
			Summary:   trimForMemory(response, 220),
			Body:      strings.TrimSpace(response),
		})
	}
	artifactDir := filepath.Join(root, "projects", projectID, "cron_runs")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}
	fileName := fmt.Sprintf("%s_%s.md", now.UTC().Format("20060102T150405Z"), sanitizeArtifactID(job.ID))
	path := filepath.Join(artifactDir, fileName)
	content := fmt.Sprintf(
		"# Cron Run\n\n- project_id: %s\n- job_id: %s\n- job_name: %s\n- ran_at: %s\n\n## Prompt\n\n%s\n\n## Response\n\n%s\n",
		projectID,
		strings.TrimSpace(job.ID),
		strings.TrimSpace(job.Name),
		now.UTC().Format(time.RFC3339),
		strings.TrimSpace(job.Prompt),
		strings.TrimSpace(response),
	)
	return os.WriteFile(path, []byte(content), 0o644)
}

func sanitizeArtifactID(raw string) string {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "job"
	}
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('_')
	}
	return b.String()
}

func resolveCronTargetSessionID(store *session.Store, raw string, mainSessionID string) (sessionID string, explicitTarget bool, err error) {
	if store == nil {
		return "", false, nil
	}
	target := strings.TrimSpace(raw)
	if target == "" || strings.EqualFold(target, "isolated") {
		return "", false, nil
	}
	if strings.EqualFold(target, "main") {
		configuredMain := strings.TrimSpace(mainSessionID)
		if configuredMain != "" {
			if _, err := store.Get(configuredMain); err != nil {
				if strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "session not found") {
					return "", false, fmt.Errorf("main session not found: %s", configuredMain)
				}
				return "", false, err
			}
			return configuredMain, false, nil
		}
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
	store, err := m.resolver.Resolve(defaultWorkspaceID)
	if err != nil {
		return err
	}
	manager := cron.NewManager(store, m.runJob, m.interval, m.nowFn)
	if err := manager.Tick(ctx); err != nil {
		m.logger.Warn().Err(err).Msg("cron manager tick failed")
		return err
	}
	return nil
}
