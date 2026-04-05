package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

type cronRunTelemetry struct {
	PromptTokens               int
	SystemPromptTokens         int
	UserPromptTokens           int
	TargetSessionContextTokens int
	TargetSessionContextUsed   bool
	ToolCount                  int
	ResponseTokens             int
	ContaminationMarkers       []string
}

type cronExternalReminderSender func(ctx context.Context, job cron.Job, reminderText string) error

func newCronJobRunner(
	workspaceDir string,
	store *session.Store,
	runPrompt gatewayPromptRunner,
	logger zerolog.Logger,
) func(ctx context.Context, job cron.Job) (string, error) {
	return newCronJobRunnerWithNotify(workspaceDir, store, runPrompt, logger, nil, "", 0, nil, nil)
}

func newCronJobRunnerWithNotify(
	workspaceDir string,
	store *session.Store,
	runPrompt gatewayPromptRunner,
	logger zerolog.Logger,
	emit func(ctx context.Context, evt notificationEvent),
	mainSessionID string,
	artifactHistoryLimit int,
	resolveDefaultTelegramChatID func(ctx context.Context) (string, error),
	sendExternalReminder cronExternalReminderSender,
) func(ctx context.Context, job cron.Job) (string, error) {
	if runPrompt == nil {
		return nil
	}
	return func(ctx context.Context, job cron.Job) (string, error) {
		prepared, err := prepareCronJobRun(
			ctx,
			workspaceDir,
			store,
			job,
			mainSessionID,
			resolveDefaultTelegramChatID,
		)
		if err != nil {
			return "", err
		}
		telemetry := prepared.telemetry
		if reminderText := cronReminderMessage(job); reminderText != "" {
			artifactNow := time.Now().UTC()
			if err := deliverCronResult(ctx, prepared.workspaceDir, prepared.targetStore, job, prepared.targetSessionID, prepared.explicitTarget, reminderText, artifactNow, logger, sendExternalReminder); err != nil {
				emitCronRunFailure(ctx, emit, prepared.workspaceDir, job, reminderText, artifactNow, telemetry, artifactHistoryLimit, prepared.targetSessionID, "Cron delivery failed", err)
				return "", err
			}
			emitCronRunSuccess(ctx, emit, prepared.workspaceDir, job, reminderText, artifactNow, telemetry, artifactHistoryLimit, prepared.targetSessionID, logger)
			return reminderText, nil
		}

		runLabel := "cron"
		if strings.TrimSpace(job.ID) != "" {
			runLabel = "cron:" + strings.TrimSpace(job.ID)
		}
		callSessionID := prepared.targetSessionID
		if prepared.executionSessionID != "" {
			callSessionID = prepared.executionSessionID
		}
		runCtx := usage.WithCallMeta(ctx, usage.CallMeta{
			Source:    "cron",
			SessionID: callSessionID,
			RunID:     strings.TrimSpace(job.ID),
		})
		runCtx = withCronExecutionContext(runCtx, cronExecutionContext{SessionID: prepared.executionSessionID})
		agentTelemetry := &agentPromptTelemetry{}
		runCtx = withAgentPromptTelemetry(runCtx, agentTelemetry)
		response, err := runPrompt(runCtx, runLabel, prepared.promptText, prepared.allowedTools)
		artifactNow := time.Now().UTC()
		if err != nil {
			emitCronRunFailure(ctx, emit, prepared.workspaceDir, job, "", artifactNow, telemetry, artifactHistoryLimit, prepared.targetSessionID, "Cron failed", err)
			return "", err
		}
		telemetry.SystemPromptTokens = agentTelemetry.SystemPromptTokens
		telemetry.UserPromptTokens = agentTelemetry.UserPromptTokens
		telemetry.ToolCount = agentTelemetry.ToolCount
		telemetry.ResponseTokens = promptTokenEstimate(response)
		telemetry.ContaminationMarkers = detectPseudoToolContamination(response)
		if len(telemetry.ContaminationMarkers) > 0 {
			err := fmt.Errorf("pseudo-tool contamination detected: %s", strings.Join(telemetry.ContaminationMarkers, ", "))
			emitCronRunFailure(ctx, emit, prepared.workspaceDir, job, response, artifactNow, telemetry, artifactHistoryLimit, prepared.targetSessionID, "Cron failed", err)
			return "", err
		}
		if err := verifyCronClaimedFileUpdates(prepared.workspaceDir, job, response, prepared.projectFileSnapshot); err != nil {
			emitCronRunFailure(ctx, emit, prepared.workspaceDir, job, response, artifactNow, telemetry, artifactHistoryLimit, prepared.targetSessionID, "Cron failed", err)
			return "", err
		}
		if err := deliverCronResult(ctx, prepared.workspaceDir, prepared.targetStore, job, prepared.targetSessionID, prepared.explicitTarget, response, time.Now().UTC(), logger, sendExternalReminder); err != nil {
			emitCronRunFailure(ctx, emit, prepared.workspaceDir, job, response, artifactNow, telemetry, artifactHistoryLimit, prepared.targetSessionID, "Cron delivery failed", err)
			return "", err
		}
		emitCronRunSuccess(ctx, emit, prepared.workspaceDir, job, response, artifactNow, telemetry, artifactHistoryLimit, prepared.targetSessionID, logger)
		return response, nil
	}
}

type preparedCronJobRun struct {
	workspaceDir        string
	targetStore         *session.Store
	promptText          string
	allowedTools        []string
	telemetry           cronRunTelemetry
	projectFileSnapshot map[string]time.Time
	executionSessionID  string
	targetSessionID     string
	explicitTarget      bool
}

func prepareCronJobRun(
	ctx context.Context,
	workspaceDir string,
	store *session.Store,
	job cron.Job,
	mainSessionID string,
	resolveDefaultTelegramChatID func(context.Context) (string, error),
) (preparedCronJobRun, error) {
	prepared := preparedCronJobRun{
		workspaceDir: strings.TrimSpace(workspaceDir),
		targetStore:  store,
		promptText:   strings.TrimSpace(job.Prompt),
		telemetry:    cronRunTelemetry{},
	}
	if prepared.targetStore == nil && prepared.workspaceDir != "" {
		if err := memory.EnsureWorkspace(prepared.workspaceDir); err != nil {
			return prepared, err
		}
		prepared.targetStore = session.NewStore(prepared.workspaceDir)
	}
	if payload := strings.TrimSpace(string(job.Payload)); payload != "" {
		prepared.promptText += "\n\nCRON_PAYLOAD_JSON:\n" + payload
	}
	prepared.executionSessionID = strings.TrimSpace(job.SessionID)
	prepared.allowedTools = resolveCronAllowedTools(prepared.workspaceDir, job)
	if err := validateCronProjectPrerequisites(prepared.workspaceDir, job, prepared.promptText); err != nil {
		return prepared, err
	}
	telegramPrompt, err := buildCronTelegramPromptSection(ctx, resolveDefaultTelegramChatID)
	if err != nil {
		return prepared, err
	}
	if telegramPrompt != "" {
		prepared.promptText += "\n\n" + telegramPrompt
	}
	prepared.targetSessionID, prepared.explicitTarget, err = resolveCronTargetSessionID(prepared.targetStore, job, mainSessionID)
	if err != nil {
		return prepared, err
	}
	if prepared.targetSessionID != "" && prepared.executionSessionID == "" {
		{
			contextText, err := sessionContextByID(prepared.targetStore, prepared.targetSessionID, 6)
			if err != nil {
				return prepared, err
			}
			if contextText != "" {
				prepared.promptText += "\n\nTARGET_SESSION_CONTEXT:\n" + contextText
				prepared.telemetry.TargetSessionContextUsed = true
				prepared.telemetry.TargetSessionContextTokens = promptTokenEstimate(contextText)
			}
		}
	}
	prepared.telemetry.PromptTokens = promptTokenEstimate(prepared.promptText)
	return prepared, nil
}

func resolveCronAllowedTools(_ string, _ cron.Job) []string {
	// No project-based tool policy after project package removal.
	return nil
}

func emitCronRunFailure(
	ctx context.Context,
	emit func(ctx context.Context, evt notificationEvent),
	workspaceDir string,
	job cron.Job,
	response string,
	now time.Time,
	telemetry cronRunTelemetry,
	artifactHistoryLimit int,
	targetSessionID string,
	title string,
	err error,
) string {
	artifactPath, _ := persistCronProjectArtifact(workspaceDir, job, response, err, now, telemetry, artifactHistoryLimit)
	if emit != nil {
		evt := buildCronNotificationEvent(job, "error", title, err.Error(), artifactPath, strings.TrimSpace(targetSessionID))
		emit(ctx, evt)
	}
	return artifactPath
}

func emitCronRunSuccess(
	ctx context.Context,
	emit func(ctx context.Context, evt notificationEvent),
	workspaceDir string,
	job cron.Job,
	response string,
	now time.Time,
	telemetry cronRunTelemetry,
	artifactHistoryLimit int,
	targetSessionID string,
	logger zerolog.Logger,
) {
	artifactPath, err := persistCronProjectArtifact(workspaceDir, job, response, nil, now, telemetry, artifactHistoryLimit)
	if err != nil {
		logger.Debug().Err(err).Str("job_id", strings.TrimSpace(job.ID)).Msg("persist cron project artifact failed")
	}
	if emit != nil {
		evt := buildCronNotificationEvent(job, "info", "Cron completed", response, artifactPath, strings.TrimSpace(targetSessionID))
		emit(ctx, evt)
	}
}

func validateCronProjectPrerequisites(_ string, _ cron.Job, _ string) error {
	// No project prerequisite validation after project package removal.
	return nil
}

func buildCronTelegramPromptSection(ctx context.Context, resolveDefaultTelegramChatID func(context.Context) (string, error)) (string, error) {
	if resolveDefaultTelegramChatID == nil {
		return "", nil
	}
	chatID, err := resolveDefaultTelegramChatID(ctx)
	if err != nil {
		if strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "multiple paired telegram chats") {
			return "CRON_TELEGRAM_CONTEXT:\n- default_paired_chat_available: false\n- warning: multiple paired telegram chats exist; telegram_send requires an explicit chat_id.", nil
		}
		return "", err
	}
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return "", nil
	}
	return fmt.Sprintf(
		"CRON_TELEGRAM_CONTEXT:\n- default_paired_chat_available: true\n- default_paired_chat_id: %s\n- If you need to send a Telegram notification to the same paired chat, call telegram_send.\n- When the tool is configured with a default paired chat, you may omit chat_id and provide only text unless you intentionally target a different chat.",
		chatID,
	), nil
}

func persistCronProjectArtifact(workspaceDir string, job cron.Job, response string, runErr error, now time.Time, telemetry cronRunTelemetry, _ int) (string, error) {
	path := cronArtifactLogPath(strings.TrimSpace(workspaceDir), job)
	if path == "" {
		return "", nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	entry := map[string]any{
		"job_id":          strings.TrimSpace(job.ID),
		"job_name":        strings.TrimSpace(job.Name),
		"scope":           cronExecutionScope(job),
		"session_id":      strings.TrimSpace(job.SessionID),
		"schedule":        strings.TrimSpace(job.Schedule),
		"status":          "completed",
		"ran_at":          now.UTC(),
		"wake_mode":       strings.TrimSpace(job.WakeMode),
		"delivery_mode":   effectiveCronDeliveryMode(job.DeliveryMode, job.SessionTarget, job.SessionID),
		"prompt_preview":  trimForMemory(job.Prompt, 180),
		"result_summary":  summarizeCronResponse(response),
		"result_preview":  trimForMemory(response, 320),
		"prompt_tokens":   telemetry.PromptTokens,
		"tool_count":      telemetry.ToolCount,
		"response_tokens": telemetry.ResponseTokens,
	}
	if runErr != nil {
		entry["status"] = "error"
		entry["error"] = strings.TrimSpace(runErr.Error())
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()

	line, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return "", err
	}
	return path, nil
}

func detectPseudoToolContamination(response string) []string {
	candidates := []string{
		`{"command":`,
		`"tool_uses":`,
		`assistant to=functions.`,
		`{"background":`,
		`"recipient_name":"functions.`,
	}
	normalized := strings.ToLower(strings.TrimSpace(response))
	if normalized == "" {
		return nil
	}
	markers := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.Contains(normalized, strings.ToLower(candidate)) {
			markers = append(markers, candidate)
		}
	}
	if len(markers) == 0 {
		return nil
	}
	slices.Sort(markers)
	return slices.Compact(markers)
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

func trimCronProjectArtifacts(dir string, historyLimit int) error {
	if strings.TrimSpace(dir) == "" || historyLimit <= 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	files := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}
		files = append(files, entry)
	}
	if len(files) <= historyLimit {
		return nil
	}
	slices.SortFunc(files, func(left, right os.DirEntry) int {
		return strings.Compare(left.Name(), right.Name())
	})
	for _, entry := range files[:len(files)-historyLimit] {
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func snapshotCronProjectFiles(_ string, _ string) (map[string]time.Time, error) {
	return nil, nil
}

func verifyCronClaimedFileUpdates(workspaceDir string, job cron.Job, response string, baseline map[string]time.Time) error {
	claimed := extractClaimedCronFileUpdates(strings.TrimSpace(response))
	if len(claimed) == 0 {
		return nil
	}
	root := strings.TrimSpace(workspaceDir)
	for _, claimedPath := range claimed {
		resolved := resolveClaimedCronPath(root, claimedPath)
		if resolved == "" {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("claimed file update not observed: %s", claimedPath)
			}
			return err
		}
		if before, ok := baseline[resolved]; ok && !info.ModTime().After(before) {
			return fmt.Errorf("claimed file update not observed: %s", claimedPath)
		}
	}
	return nil
}

var cronClaimedPathPattern = regexp.MustCompile("`([^`]+\\.md)`|\"([^\"]+\\.md)\"|'([^']+\\.md)'")

func extractClaimedCronFileUpdates(response string) []string {
	if strings.TrimSpace(response) == "" {
		return nil
	}
	updateVerbs := []string{
		"추가", "작성", "갱신", "업데이트", "생성", "저장", "수정",
		"added", "created", "updated", "saved", "wrote", "modified",
	}
	lines := strings.Split(response, "\n")
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		hasVerb := false
		for _, verb := range updateVerbs {
			if strings.Contains(lower, strings.ToLower(verb)) {
				hasVerb = true
				break
			}
		}
		if !hasVerb {
			continue
		}
		matches := cronClaimedPathPattern.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			for _, candidate := range match[1:] {
				candidate = strings.TrimSpace(candidate)
				if candidate == "" {
					continue
				}
				if _, ok := seen[candidate]; ok {
					continue
				}
				seen[candidate] = struct{}{}
				out = append(out, candidate)
			}
		}
	}
	return out
}

func resolveClaimedCronPath(workspaceDir string, claimed string) string {
	candidate := strings.TrimSpace(strings.Trim(claimed, "`\"'"))
	if candidate == "" {
		return ""
	}
	if filepath.IsAbs(candidate) {
		return candidate
	}
	return filepath.Join(workspaceDir, filepath.Clean(candidate))
}

func resolveCronTargetSessionID(store *session.Store, job cron.Job, mainSessionID string) (sessionID string, explicitTarget bool, err error) {
	if store == nil {
		return "", false, nil
	}
	target := strings.TrimSpace(job.SessionTarget)
	if target == "" || strings.EqualFold(target, "isolated") {
		boundSessionID := strings.TrimSpace(job.SessionID)
		if boundSessionID == "" {
			return "", false, nil
		}
		if _, err := store.Get(boundSessionID); err != nil {
			if strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "session not found") {
				return "", false, fmt.Errorf("bound session not found: %s", boundSessionID)
			}
			return "", false, err
		}
		return boundSessionID, false, nil
	}
	if strings.EqualFold(target, "main") || strings.EqualFold(target, "current") {
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
	ctx context.Context,
	workspaceDir string,
	store *session.Store,
	job cron.Job,
	targetSessionID string,
	explicitTarget bool,
	response string,
	now time.Time,
	logger zerolog.Logger,
	sendExternalReminder cronExternalReminderSender,
) error {
	mode := effectiveCronDeliveryMode(job.DeliveryMode, job.SessionTarget, job.SessionID)
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
			return nil
		}
		if err := appendCronSessionMessage(store, targetSessionID, buildCronSessionContent(job, response), now); err != nil {
			return err
		}
		logger.Debug().
			Str("job_id", job.ID).
			Str("session_id", targetSessionID).
			Msg("cron response delivered to session")
	}
	if reminderText := cronReminderMessage(job); reminderText != "" && strings.TrimSpace(job.SessionID) == "" && sendExternalReminder != nil {
		if err := sendExternalReminder(ctx, job, reminderText); err != nil {
			return err
		}
	}
	return nil
}

func lookupCronDeliverySession(store *session.Store, sessionID string) (*session.Session, error) {
	if store == nil || strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		if strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "session not found") {
			return nil, nil
		}
		return nil, err
	}
	return &sess, nil
}

func appendCronSessionMessage(store *session.Store, sessionID string, content string, now time.Time) error {
	if store == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	if err := session.AppendMessage(store.TranscriptPath(sessionID), session.Message{
		Role:      "system",
		Content:   content,
		Timestamp: now.UTC(),
	}); err != nil {
		return err
	}
	return store.Touch(sessionID, now.UTC())
}

func buildCronSessionContent(job cron.Job, response string) string {
	if reminderText := cronReminderMessage(job); reminderText != "" {
		return fmt.Sprintf(
			"[REMINDER]\njob: %s\nmessage: %s",
			strings.TrimSpace(job.Name),
			trimForMemory(reminderText, 320),
		)
	}
	return fmt.Sprintf(
		"[CRON]\njob: %s\nprompt: %s\nresponse: %s",
		strings.TrimSpace(job.Name),
		trimForMemory(job.Prompt, 180),
		trimForMemory(response, 320),
	)
}

func cronReminderMessage(job cron.Job) string {
	meta, ok := cron.ExtractPayloadMeta(job.Payload)
	if !ok || !strings.EqualFold(strings.TrimSpace(meta.TaskType), "reminder") {
		return ""
	}
	if strings.TrimSpace(meta.ReminderMessage) != "" {
		return strings.TrimSpace(meta.ReminderMessage)
	}
	if strings.TrimSpace(job.Name) != "" {
		return strings.TrimSpace(job.Name)
	}
	return strings.TrimSpace(job.Prompt)
}

func buildCronSummaryContent(job cron.Job, response string) string {
	status := "completed"
	if strings.TrimSpace(response) == "" {
		status = "completed"
	}
	return fmt.Sprintf(
		"[CRON SUMMARY]\njob: %s\nstatus: %s\nresult: %s",
		strings.TrimSpace(job.Name),
		status,
		trimForMemory(response, 220),
	)
}

func buildCronNotificationEvent(job cron.Job, severity string, baseTitle string, details string, openPath string, sessionID string) notificationEvent {
	title := strings.TrimSpace(baseTitle)
	if jobName := strings.TrimSpace(job.Name); jobName != "" {
		title = title + ": " + jobName
	}
	message := buildCronNotificationMessage(job, details)
	evt := newNotificationEvent("cron", severity, title, message)
	evt.JobID = strings.TrimSpace(job.ID)
	evt.SessionID = strings.TrimSpace(sessionID)
	evt.OpenPath = strings.TrimSpace(openPath)
	return evt
}

func buildCronNotificationMessage(job cron.Job, details string) string {
	summary := summarizeCronResponse(details)
	if summary != "" {
		return summary
	}
	return "cron job finished"
}

func summarizeCronResponse(raw string) string {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(raw), "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.Trim(strings.TrimSpace(line), "-*`")
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(lower, "변경 파일") ||
			strings.HasPrefix(lower, "상태 갱신") ||
			strings.HasPrefix(lower, "open questions") {
			continue
		}
		return trimForMemory(trimmed, 180)
	}
	return trimForMemory(strings.TrimSpace(raw), 180)
}

func effectiveCronDeliveryMode(raw string, sessionTarget string, sessionID string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		if strings.EqualFold(strings.TrimSpace(sessionTarget), "main") || strings.TrimSpace(sessionID) != "" {
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

func cronArtifactLogPath(workspaceDir string, job cron.Job) string {
	if workspaceDir == "" {
		return ""
	}
	if sessionID := strings.TrimSpace(job.SessionID); sessionID != "" {
		return filepath.Join(workspaceDir, "artifacts", sessionID, "cronjob-log.jsonl")
	}
	return filepath.Join(workspaceDir, "artifacts", "_global", "cronjob-log.jsonl")
}

func cronExecutionScope(job cron.Job) string {
	if strings.TrimSpace(job.SessionID) != "" {
		return "session"
	}
	return "global"
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
