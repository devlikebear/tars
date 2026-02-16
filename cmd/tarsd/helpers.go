package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devlikebear/tarsncase/internal/agent"
	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/heartbeat"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/prompt"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/devlikebear/tarsncase/internal/tool"
	"github.com/devlikebear/tarsncase/internal/toolpolicy"
	"github.com/rs/zerolog"
)

type runtimeActivity struct {
	chatInFlight atomic.Int64
}

func (a *runtimeActivity) beginChat() func() {
	if a == nil {
		return func() {}
	}
	a.chatInFlight.Add(1)
	return func() {
		a.chatInFlight.Add(-1)
	}
}

func (a *runtimeActivity) isChatBusy() bool {
	if a == nil {
		return false
	}
	return a.chatInFlight.Load() > 0
}

type heartbeatRuntimeState struct {
	mu        sync.RWMutex
	hasRun    bool
	lastRunAt time.Time
	lastErr   string
	last      heartbeat.RunResult
}

func (s *heartbeatRuntimeState) record(ranAt time.Time, result heartbeat.RunResult, runErr error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hasRun = true
	s.lastRunAt = ranAt.UTC()
	s.last = result
	if runErr != nil {
		s.lastErr = strings.TrimSpace(runErr.Error())
	} else {
		s.lastErr = ""
	}
}

func (s *heartbeatRuntimeState) snapshot(configured bool, activeHours, timezone string, chatBusy bool) tool.HeartbeatStatus {
	status := tool.HeartbeatStatus{
		Configured:  configured,
		ActiveHours: strings.TrimSpace(activeHours),
		Timezone:    strings.TrimSpace(timezone),
		ChatBusy:    chatBusy,
	}
	if s == nil {
		return status
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.hasRun {
		return status
	}
	status.LastRunAt = s.lastRunAt.Format(time.RFC3339)
	status.LastSkipped = s.last.Skipped
	status.LastSkipReason = s.last.SkipReason
	status.LastLogged = s.last.Logged
	status.LastAcknowledged = s.last.Acknowledged
	status.LastResponse = s.last.Response
	status.LastError = s.lastErr
	return status
}

func newHeartbeatRunner(
	workspaceDir string,
	nowFn func() time.Time,
	ask heartbeat.AskFunc,
	policy heartbeat.Policy,
	state *heartbeatRuntimeState,
) func(ctx context.Context) (heartbeat.RunResult, error) {
	return newHeartbeatRunnerWithNotify(workspaceDir, nowFn, ask, policy, state, nil)
}

func newHeartbeatRunnerWithNotify(
	workspaceDir string,
	nowFn func() time.Time,
	ask heartbeat.AskFunc,
	policy heartbeat.Policy,
	state *heartbeatRuntimeState,
	emit func(ctx context.Context, evt notificationEvent),
) func(ctx context.Context) (heartbeat.RunResult, error) {
	var mu sync.Mutex
	return func(ctx context.Context) (heartbeat.RunResult, error) {
		mu.Lock()
		defer mu.Unlock()
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		ranAt := nowFn().UTC()
		result, err := heartbeat.RunOnceWithLLMResultWithPolicy(callCtx, workspaceDir, ranAt, ask, policy)
		if state != nil {
			state.record(ranAt, result, err)
		}
		if emit != nil {
			if err != nil {
				evt := newNotificationEvent("heartbeat", "error", "Heartbeat failed", trimForMemory(err.Error(), 240))
				emit(ctx, evt)
			} else if result.Skipped {
				evt := newNotificationEvent("heartbeat", "info", "Heartbeat skipped", trimForMemory(result.SkipReason, 240))
				emit(ctx, evt)
			} else if result.Acknowledged {
				evt := newNotificationEvent("heartbeat", "info", "Heartbeat acknowledged", "no follow-up action")
				emit(ctx, evt)
			} else {
				evt := newNotificationEvent("heartbeat", "info", "Heartbeat action", trimForMemory(result.Response, 280))
				emit(ctx, evt)
			}
		}
		return result, err
	}
}

func maybeAutoCompactSession(workspaceDir, transcriptPath, sessionID string, client llm.Client, logger zerolog.Logger) error {
	messages, err := session.ReadMessages(transcriptPath)
	if err != nil {
		return err
	}
	estimated := session.EstimateTokens(messages)
	if estimated < autoCompactTriggerTokens {
		return nil
	}

	now := time.Now().UTC()
	result, err := compactWithMemoryFlush(workspaceDir, transcriptPath, sessionID, autoCompactKeepRecent, autoCompactKeepTokens, client, now)
	if err != nil {
		return err
	}
	logger.Debug().
		Str("session_id", sessionID).
		Int("estimated_tokens", estimated).
		Bool("compacted", result.Compacted).
		Int("original_count", result.OriginalCount).
		Int("final_count", result.FinalCount).
		Msg("auto compaction evaluated")
	return nil
}

func compactWithMemoryFlush(workspaceDir, transcriptPath, sessionID string, keepRecent int, keepRecentTokens int, client llm.Client, now time.Time) (session.CompactResult, error) {
	return session.CompactTranscriptWithOptions(transcriptPath, keepRecent, now, session.CompactOptions{
		KeepRecentTokens: keepRecentTokens,
		SummaryBuilder: func(messages []session.Message) (string, error) {
			if client == nil {
				return session.BuildCompactionSummary(messages), nil
			}
			return buildLLMCompactionSummary(messages, client, now)
		},
		BeforeRewrite: func(summary string, compactedCount int, originalCount int) error {
			note := fmt.Sprintf("session %s compacted %d/%d messages", sessionID, compactedCount, originalCount)
			if err := memory.AppendMemoryNote(workspaceDir, now, note); err != nil {
				return err
			}

			preview := strings.ReplaceAll(strings.TrimSpace(summary), "\n", " ")
			if len(preview) > 240 {
				preview = preview[:240] + "..."
			}
			return memory.AppendDailyLog(workspaceDir, now, fmt.Sprintf("compaction flush: %s | %s", note, preview))
		},
	})
}

func buildLLMCompactionSummary(messages []session.Message, client llm.Client, now time.Time) (string, error) {
	const maxMessages = 80
	msgs := messages
	if len(msgs) > maxMessages {
		msgs = msgs[len(msgs)-maxMessages:]
	}

	var b strings.Builder
	for _, m := range msgs {
		content := strings.TrimSpace(strings.ReplaceAll(m.Content, "\n", " "))
		if len(content) > 240 {
			content = content[:240] + "..."
		}
		_, _ = fmt.Fprintf(&b, "- [%s] %s\n", m.Role, content)
	}

	userPrompt := fmt.Sprintf(
		"Create a compact context summary for old chat messages.\n"+
			"Keep concrete facts, goals, decisions, user preferences, unresolved tasks.\n"+
			"Return plain markdown under 900 characters.\n"+
			"Current UTC: %s\n\nMessages:\n%s",
		now.UTC().Format(time.RFC3339),
		b.String(),
	)

	resp, err := client.Chat(context.Background(), []llm.ChatMessage{
		{
			Role:    "system",
			Content: "You are a precise summarizer for context compaction. Output only the summary text.",
		},
		{
			Role:    "user",
			Content: userPrompt,
		},
	}, llm.ChatOptions{})
	if err != nil {
		return session.BuildCompactionSummary(messages), nil
	}

	summary := strings.TrimSpace(resp.Message.Content)
	if summary == "" {
		return session.BuildCompactionSummary(messages), nil
	}
	return "[COMPACTION SUMMARY]\n" + summary, nil
}

func writeChatMemory(workspaceDir, sessionID, userMessage, assistantMessage string, now time.Time) error {
	dailyEntry := fmt.Sprintf(
		"chat session=%s user=%q assistant=%q",
		sessionID,
		trimForMemory(userMessage, 120),
		trimForMemory(assistantMessage, 160),
	)
	if err := memory.AppendDailyLog(workspaceDir, now, dailyEntry); err != nil {
		return err
	}

	if shouldPromoteToMemory(userMessage) {
		note := fmt.Sprintf("session %s user preference/fact: %s", sessionID, strings.TrimSpace(userMessage))
		if err := memory.AppendMemoryNote(workspaceDir, now, note); err != nil {
			return err
		}
	}
	return nil
}

func shouldPromoteToMemory(userMessage string) bool {
	lower := strings.ToLower(strings.TrimSpace(userMessage))
	return strings.HasPrefix(lower, "remember ") ||
		strings.HasPrefix(lower, "remember:") ||
		strings.HasPrefix(lower, "기억해") ||
		strings.HasPrefix(lower, "메모해")
}

func shouldForceMemoryToolCall(userMessage string) bool {
	v := strings.ToLower(strings.TrimSpace(userMessage))
	if v == "" {
		return false
	}
	keywords := []string{
		"memory_search",
		"memory_get",
		"memory",
		"remember",
		"recall",
		"history",
		"previous",
		"earlier",
		"what did i",
		"what do you remember",
		"preference",
		"기억",
		"메모리",
		"기록",
		"이전",
		"지난",
		"취향",
	}
	for _, kw := range keywords {
		if strings.Contains(v, kw) {
			return true
		}
	}
	return false
}

func trimForMemory(s string, max int) string {
	v := strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if max <= 0 || len(v) <= max {
		return v
	}
	return v[:max] + "..."
}

func newBaseToolRegistry(workspaceDir string) *tool.Registry {
	registry := tool.NewRegistry()
	registry.Register(tool.NewMemorySearchTool(workspaceDir))
	registry.Register(tool.NewMemoryGetTool(workspaceDir))
	registry.Register(tool.NewReadTool(workspaceDir))
	registry.Register(tool.NewReadFileTool(workspaceDir))
	registry.Register(tool.NewWriteTool(workspaceDir))
	registry.Register(tool.NewWriteFileTool(workspaceDir))
	registry.Register(tool.NewEditTool(workspaceDir))
	registry.Register(tool.NewEditFileTool(workspaceDir))
	registry.Register(tool.NewListDirTool(workspaceDir))
	registry.Register(tool.NewGlobTool(workspaceDir))
	registry.Register(tool.NewExecTool(workspaceDir))
	return registry
}

func newAgentAskFunc(workspaceDir string, client llm.Client, maxIterations int, logger zerolog.Logger) heartbeat.AskFunc {
	runner := newAgentPromptRunner(workspaceDir, client, maxIterations, logger)
	if runner == nil {
		return nil
	}
	return func(ctx context.Context, promptText string) (string, error) {
		return runner(ctx, "heartbeat", promptText)
	}
}

func newAgentPromptRunner(
	workspaceDir string,
	client llm.Client,
	maxIterations int,
	logger zerolog.Logger,
) func(ctx context.Context, runLabel string, promptText string) (string, error) {
	if client == nil {
		return nil
	}
	maxIters := resolveAgentMaxIterations(maxIterations)
	return func(ctx context.Context, runLabel string, promptText string) (string, error) {
		label := strings.TrimSpace(runLabel)
		if label == "" {
			label = "agent"
		}
		systemPrompt := prompt.Build(prompt.BuildOptions{WorkspaceDir: workspaceDir})
		systemPrompt += "\n" + strings.TrimSpace(memoryToolSystemRule) + "\n"
		registry := newBaseToolRegistry(workspaceDir)
		loop := setupAgentLoop(client, registry, label, 0, logger, func(string, string, string, string, string, string) {})
		resp, err := loop.Run(ctx, []llm.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: promptText},
		}, agent.RunOptions{
			MaxIterations: maxIters,
			Tools:         registry.Schemas(),
		})
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(resp.Message.Content), nil
	}
}

func buildHeartbeatPolicy(
	store *session.Store,
	activeHours string,
	timezone string,
	activity *runtimeActivity,
) heartbeat.Policy {
	return heartbeat.Policy{
		ActiveHours: strings.TrimSpace(activeHours),
		Timezone:    strings.TrimSpace(timezone),
		ShouldRun: func(_ context.Context, _ time.Time) (bool, string) {
			if activity != nil && activity.isChatBusy() {
				return false, "chat queue is busy"
			}
			return true, ""
		},
		LoadSessionContext: func(_ context.Context, _ time.Time) (string, error) {
			return latestSessionContext(store, 6)
		},
		AppendSessionTurn: func(_ context.Context, turn heartbeat.TurnRecord) error {
			return appendHeartbeatTurnToLatestSession(store, turn)
		},
	}
}

func latestSessionContext(store *session.Store, maxMessages int) (string, error) {
	if store == nil {
		return "", nil
	}
	latest, err := store.Latest()
	if err != nil {
		if strings.Contains(err.Error(), "session not found") {
			return "", nil
		}
		return "", err
	}
	messages, err := session.ReadMessages(store.TranscriptPath(latest.ID))
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
	_, _ = fmt.Fprintf(&b, "session_id=%s\nsession_title=%s\n", latest.ID, latest.Title)
	for _, msg := range messages[start:] {
		_, _ = fmt.Fprintf(&b, "- [%s] %s\n", msg.Role, trimForMemory(msg.Content, 180))
	}
	return strings.TrimSpace(b.String()), nil
}

func appendHeartbeatTurnToLatestSession(store *session.Store, turn heartbeat.TurnRecord) error {
	if store == nil {
		return nil
	}
	latest, err := store.Latest()
	if err != nil {
		if strings.Contains(err.Error(), "session not found") {
			return nil
		}
		return err
	}
	if strings.TrimSpace(turn.Response) == "" {
		return nil
	}
	path := store.TranscriptPath(latest.ID)
	content := fmt.Sprintf(
		"[HEARTBEAT]\nprompt: %s\nresponse: %s",
		trimForMemory(turn.Prompt, 220),
		trimForMemory(turn.Response, 320),
	)
	if err := session.AppendMessage(path, session.Message{
		Role:      "system",
		Content:   content,
		Timestamp: turn.OccurredAt.UTC(),
	}); err != nil {
		return err
	}
	return store.Touch(latest.ID, turn.OccurredAt.UTC())
}

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
		promptText := strings.TrimSpace(job.Prompt)
		if payload := strings.TrimSpace(string(job.Payload)); payload != "" {
			promptText += "\n\nCRON_PAYLOAD_JSON:\n" + payload
		}

		targetSessionID, explicitTarget, err := resolveCronTargetSessionID(store, job.SessionTarget)
		if err != nil {
			return "", err
		}
		if targetSessionID != "" {
			contextText, err := sessionContextByID(store, targetSessionID, 6)
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
		if err := deliverCronResult(workspaceDir, store, job, targetSessionID, explicitTarget, response, time.Now().UTC(), logger); err != nil {
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

func buildAutomationTools(
	cronStore *cron.Store,
	cronRunner func(ctx context.Context, job cron.Job) (string, error),
	heartbeatRunner func(ctx context.Context) (heartbeat.RunResult, error),
	heartbeatStatusProvider func(ctx context.Context) (tool.HeartbeatStatus, error),
	nowFn func() time.Time,
) []tool.Tool {
	return []tool.Tool{
		tool.NewCronListTool(cronStore),
		tool.NewCronCreateTool(cronStore),
		tool.NewCronUpdateTool(cronStore),
		tool.NewCronDeleteTool(cronStore),
		tool.NewCronRunTool(cronStore, cronRunner),
		tool.NewHeartbeatStatusTool(heartbeatStatusProvider),
		tool.NewHeartbeatRunOnceTool(func(ctx context.Context) (tool.HeartbeatRunResult, error) {
			if heartbeatRunner == nil {
				return tool.HeartbeatRunResult{}, fmt.Errorf("heartbeat runner is not configured")
			}
			ranAt := nowFn().UTC()
			result, err := heartbeatRunner(ctx)
			return tool.HeartbeatRunResult{
				Response:     result.Response,
				Skipped:      result.Skipped,
				SkipReason:   result.SkipReason,
				Logged:       result.Logged,
				Acknowledged: result.Acknowledged,
				RanAt:        ranAt,
			}, err
		}),
	}
}

func buildChatToolingOptions(cfg config.Config) chatToolingOptions {
	byProvider := make(map[string]toolpolicy.ProviderPolicy, len(cfg.ToolsByProvider))
	for key, p := range cfg.ToolsByProvider {
		byProvider[key] = toolpolicy.ProviderPolicy{
			Profile: strings.TrimSpace(p.Profile),
			Allow:   append([]string(nil), p.Allow...),
			Deny:    append([]string(nil), p.Deny...),
		}
	}
	selector := toolpolicy.NewSelector(
		toolpolicy.Policy{
			Profile:    strings.TrimSpace(cfg.ToolsProfile),
			Allow:      append([]string(nil), cfg.ToolsAllow...),
			Deny:       append([]string(nil), cfg.ToolsDeny...),
			ByProvider: byProvider,
		},
		toolpolicy.SelectorConfig{
			Mode:       strings.TrimSpace(cfg.ToolSelectorMode),
			MaxTools:   cfg.ToolSelectorMaxTools,
			AutoExpand: cfg.ToolSelectorAutoExpand,
		},
	)
	return chatToolingOptions{
		Provider: strings.TrimSpace(cfg.LLMProvider),
		Model:    strings.TrimSpace(cfg.LLMModel),
		Selector: selector,
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

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func requestDebugMiddleware(logger zerolog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		logger.Debug().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", rec.status).
			Int("bytes", rec.bytes).
			Dur("latency", time.Since(start)).
			Msg("http request")
	})
}
