package tarsserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/research"
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

var briefIDPattern = regexp.MustCompile(`\bbrief_id=([A-Za-z0-9_-]+)\b`)

func newCronJobRunner(
	workspaceDir string,
	store *session.Store,
	runPrompt gatewayPromptRunner,
	logger zerolog.Logger,
) func(ctx context.Context, job cron.Job) (string, error) {
	return newCronJobRunnerWithNotify(workspaceDir, store, runPrompt, logger, nil, "", 0, nil)
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

		runLabel := "cron"
		if strings.TrimSpace(job.ID) != "" {
			runLabel = "cron:" + strings.TrimSpace(job.ID)
		}
		runCtx := usage.WithCallMeta(ctx, usage.CallMeta{
			Source:    "cron",
			SessionID: prepared.targetSessionID,
			ProjectID: strings.TrimSpace(job.ProjectID),
			RunID:     strings.TrimSpace(job.ID),
		})
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
		if err := deliverCronResult(prepared.workspaceDir, prepared.targetStore, job, prepared.targetSessionID, prepared.explicitTarget, response, time.Now().UTC(), logger); err != nil {
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
	prepared.allowedTools = resolveCronAllowedTools(prepared.workspaceDir, job)
	if projectPrompt := buildCronProjectPromptSection(prepared.workspaceDir, job.ProjectID); projectPrompt != "" {
		prepared.promptText += "\n\n" + projectPrompt
	}
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
	prepared.projectFileSnapshot, err = snapshotCronProjectFiles(prepared.workspaceDir, job.ProjectID)
	if err != nil {
		return prepared, err
	}
	prepared.targetSessionID, prepared.explicitTarget, err = resolveCronTargetSessionID(prepared.targetStore, job, mainSessionID)
	if err != nil {
		return prepared, err
	}
	if strings.TrimSpace(job.ProjectID) != "" && prepared.targetStore != nil && prepared.targetSessionID != "" {
		_ = prepared.targetStore.SetProjectID(prepared.targetSessionID, strings.TrimSpace(job.ProjectID))
	}
	if prepared.targetSessionID != "" {
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

func resolveCronAllowedTools(workspaceDir string, job cron.Job) []string {
	root := strings.TrimSpace(workspaceDir)
	projectID := strings.TrimSpace(job.ProjectID)
	if root == "" || projectID == "" {
		return nil
	}
	item, err := project.NewStore(root, nil).Get(projectID)
	if err != nil {
		return nil
	}
	registry := newBaseToolRegistry(root)
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
	}, knownToolsFromRegistry(registry), project.ToolPolicyOptions{})
	if !policy.HasPolicy {
		return nil
	}
	names := defaultMinimalToolNames()
	if len(policy.AllowedTools) > 0 {
		names = append(names, policy.AllowedTools...)
	}
	names = normalizeToolNames(names)
	names = project.ApplyToolConstraints(names, policy)
	if len(names) == 0 {
		return nil
	}
	return names
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
	artifactPath, _ := persistCronProjectArtifact(workspaceDir, job, response, now, telemetry, artifactHistoryLimit)
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
	artifactPath, err := persistCronProjectArtifact(workspaceDir, job, response, now, telemetry, artifactHistoryLimit)
	if err != nil {
		logger.Debug().Err(err).Str("job_id", strings.TrimSpace(job.ID)).Str("project_id", strings.TrimSpace(job.ProjectID)).Msg("persist cron project artifact failed")
	}
	if emit != nil {
		evt := buildCronNotificationEvent(job, "info", "Cron completed", response, artifactPath, strings.TrimSpace(targetSessionID))
		emit(ctx, evt)
	}
}

func validateCronProjectPrerequisites(workspaceDir string, job cron.Job, promptText string) error {
	if strings.TrimSpace(workspaceDir) == "" {
		return nil
	}
	if strings.TrimSpace(job.ProjectID) != "" {
		return nil
	}
	matches := briefIDPattern.FindStringSubmatch(promptText)
	if len(matches) != 2 {
		return nil
	}
	briefID := strings.TrimSpace(matches[1])
	if briefID == "" {
		return nil
	}
	store := project.NewStore(workspaceDir, nil)
	brief, err := store.GetBrief(briefID)
	if err != nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(brief.Status), "finalized") {
		return fmt.Errorf("brief %s is not finalized; create a project before scheduling autonomous work", briefID)
	}
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
	return project.CronPromptContext(root, item)
}

func persistCronProjectArtifact(workspaceDir string, job cron.Job, response string, now time.Time, telemetry cronRunTelemetry, historyLimit int) (string, error) {
	root := strings.TrimSpace(workspaceDir)
	projectID := strings.TrimSpace(job.ProjectID)
	if root == "" || projectID == "" {
		return "", nil
	}
	projectItem, getErr := project.NewStore(root, nil).Get(projectID)
	if getErr == nil && strings.EqualFold(strings.TrimSpace(projectItem.Type), "research") {
		_, _ = research.NewService(root, research.Options{Now: func() time.Time { return now }}).Run(research.RunInput{
			ProjectID: projectID,
			Topic:     "cron:" + strings.TrimSpace(job.Name),
			Summary:   trimForMemory(response, 220),
			Body:      strings.TrimSpace(response),
		})
	}
	artifactBase := filepath.Join(root, "projects", projectID)
	if getErr == nil {
		artifactBase = projectItem.WorkingDir()
	}
	artifactDir := filepath.Join(artifactBase, "cron_runs")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return "", err
	}
	fileName := fmt.Sprintf("%s_%s.md", now.UTC().Format("20060102T150405Z"), sanitizeArtifactID(job.ID))
	path := filepath.Join(artifactDir, fileName)
	contaminationMarkers := "none"
	if len(telemetry.ContaminationMarkers) > 0 {
		contaminationMarkers = strings.Join(telemetry.ContaminationMarkers, ", ")
	}
	content := fmt.Sprintf(
		"# Cron Run\n\n- project_id: %s\n- job_id: %s\n- job_name: %s\n- ran_at: %s\n\n## Telemetry\n\n- prompt_tokens: %d\n- system_prompt_tokens: %d\n- user_prompt_tokens: %d\n- target_session_context_used: %t\n- target_session_context_tokens: %d\n- tool_count: %d\n- response_tokens: %d\n- contamination_markers: %s\n\n## Prompt\n\n%s\n\n## Response\n\n%s\n",
		projectID,
		strings.TrimSpace(job.ID),
		strings.TrimSpace(job.Name),
		now.UTC().Format(time.RFC3339),
		telemetry.PromptTokens,
		telemetry.SystemPromptTokens,
		telemetry.UserPromptTokens,
		telemetry.TargetSessionContextUsed,
		telemetry.TargetSessionContextTokens,
		telemetry.ToolCount,
		telemetry.ResponseTokens,
		contaminationMarkers,
		strings.TrimSpace(job.Prompt),
		strings.TrimSpace(response),
	)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	if err := trimCronProjectArtifacts(artifactDir, historyLimit); err != nil {
		return path, err
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

func snapshotCronProjectFiles(workspaceDir string, projectID string) (map[string]time.Time, error) {
	root := strings.TrimSpace(workspaceDir)
	id := strings.TrimSpace(projectID)
	if root == "" || id == "" {
		return nil, nil
	}
	projectDir := filepath.Join(root, "projects", id)
	entries := map[string]time.Time{}
	err := filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info == nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}
		entries[path] = info.ModTime()
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return entries, nil
}

func verifyCronClaimedFileUpdates(workspaceDir string, job cron.Job, response string, baseline map[string]time.Time) error {
	claimed := extractClaimedCronFileUpdates(strings.TrimSpace(response))
	if len(claimed) == 0 {
		return nil
	}
	root := strings.TrimSpace(workspaceDir)
	projectID := strings.TrimSpace(job.ProjectID)
	for _, claimedPath := range claimed {
		resolved := resolveClaimedCronPath(root, projectID, claimedPath)
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

func resolveClaimedCronPath(workspaceDir string, projectID string, claimed string) string {
	candidate := strings.TrimSpace(strings.Trim(claimed, "`\"'"))
	if candidate == "" {
		return ""
	}
	if filepath.IsAbs(candidate) {
		return candidate
	}
	if strings.HasPrefix(candidate, "projects/") || strings.HasPrefix(candidate, "_shared/") {
		return filepath.Join(workspaceDir, filepath.Clean(candidate))
	}
	if strings.Contains(candidate, "/") {
		return filepath.Join(workspaceDir, filepath.Clean(candidate))
	}
	if strings.TrimSpace(projectID) == "" {
		return filepath.Join(workspaceDir, filepath.Clean(candidate))
	}
	return filepath.Join(workspaceDir, "projects", projectID, filepath.Clean(candidate))
}

func resolveCronTargetSessionID(store *session.Store, job cron.Job, mainSessionID string) (sessionID string, explicitTarget bool, err error) {
	if store == nil {
		return "", false, nil
	}
	target := strings.TrimSpace(job.SessionTarget)
	if target == "" || strings.EqualFold(target, "isolated") {
		return "", false, nil
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
	return fmt.Sprintf(
		"[CRON]\njob: %s\nprompt: %s\nresponse: %s",
		strings.TrimSpace(job.Name),
		trimForMemory(job.Prompt, 180),
		trimForMemory(response, 320),
	)
}

func buildCronSummaryContent(job cron.Job, response string) string {
	status := "completed"
	if strings.TrimSpace(response) == "" {
		status = "completed"
	}
	return fmt.Sprintf(
		"[CRON SUMMARY]\njob: %s\nproject_id: %s\nstatus: %s\nresult: %s",
		strings.TrimSpace(job.Name),
		strings.TrimSpace(job.ProjectID),
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
	evt.ProjectID = strings.TrimSpace(job.ProjectID)
	evt.SessionID = strings.TrimSpace(sessionID)
	evt.OpenPath = strings.TrimSpace(openPath)
	return evt
}

func buildCronNotificationMessage(job cron.Job, details string) string {
	summary := summarizeCronResponse(details)
	switch {
	case strings.TrimSpace(job.ProjectID) != "" && summary != "":
		return fmt.Sprintf("%s · %s", strings.TrimSpace(job.ProjectID), summary)
	case strings.TrimSpace(job.ProjectID) != "":
		return strings.TrimSpace(job.ProjectID)
	case summary != "":
		return summary
	default:
		return "cron job finished"
	}
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
