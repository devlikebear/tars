package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/agent"
	"github.com/devlikebear/tarsncase/internal/extensions"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/project"
	"github.com/devlikebear/tarsncase/internal/serverauth"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/devlikebear/tarsncase/internal/tool"
	"github.com/devlikebear/tarsncase/internal/usage"
	"github.com/rs/zerolog"
)

type chatHandlerDeps struct {
	workspaceDir  string
	store         *session.Store
	client        llm.Client
	logger        zerolog.Logger
	maxIters      int
	chatLimiter   *inflightLimiter
	activity      *runtimeActivity
	mainSessionID string
	tooling       chatToolingOptions
	extraTools    []tool.Tool
}

type chatRequestPayload struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
	ProjectID string `json:"project_id,omitempty"`
}

type chatRunState struct {
	requestWorkspaceDir string
	workspaceID         string
	store               *session.Store
	sessionID           string
	projectID           string
	transcriptPath      string
	history             []session.Message
	registry            *tool.Registry
	toolChoice          string
	llmMessages         []llm.ChatMessage
	injectedSchemas     []llm.ToolSchema
}

type chatStreamWriter struct {
	w         http.ResponseWriter
	flusher   http.Flusher
	sessionID string
	logger    zerolog.Logger
}

func handleChatRequest(w http.ResponseWriter, r *http.Request, deps chatHandlerDeps) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	if deps.chatLimiter != nil {
		release, ok := deps.chatLimiter.tryAcquire()
		if !ok {
			writeError(w, http.StatusTooManyRequests, "overloaded", "overloaded")
			return
		}
		defer release()
	}

	req, status, message := decodeChatRequestPayload(r)
	if status != 0 {
		writeError(w, status, "", message)
		return
	}

	endBusy := deps.activity.beginChat()
	defer endBusy()
	deps.logger.Debug().
		Str("path", r.URL.Path).
		Str("session_id", strings.TrimSpace(req.SessionID)).
		Int("message_len", len(strings.TrimSpace(req.Message))).
		Msg("chat request accepted")

	state, status, errMessage, err := prepareChatRunState(r, req, deps)
	if err != nil {
		writeError(w, status, "", errMessage)
		return
	}

	stream := newChatStreamWriter(w, state.sessionID, deps.logger)
	stream.status("stream_open", "stream connected", "", "", "", "")

	chatCtx := usage.WithCallMeta(r.Context(), usage.CallMeta{
		Source:    "chat",
		SessionID: state.sessionID,
		ProjectID: state.projectID,
	})
	chatResp, deltaSent, err := executeChatLoop(chatCtx, deps, state, stream)
	if err != nil {
		stream.error(err)
		return
	}
	if !deltaSent && chatResp.Message.Content != "" {
		deps.logger.Debug().
			Str("session_id", state.sessionID).
			Int("assistant_len", len(chatResp.Message.Content)).
			Msg("emit fallback delta from non-streaming llm response")
		stream.delta(chatResp.Message.Content)
	}

	persistChatResult(state, req.Message, chatResp, deps.logger)
	stream.done(chatResp.Usage)
	deps.logger.Debug().Str("session_id", state.sessionID).Msg("chat request complete")
}

func decodeChatRequestPayload(r *http.Request) (chatRequestPayload, int, string) {
	var req chatRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return chatRequestPayload{}, http.StatusBadRequest, "invalid request body"
	}
	if strings.TrimSpace(req.Message) == "" {
		return chatRequestPayload{}, http.StatusBadRequest, "message is required"
	}
	return req, 0, ""
}

func prepareChatRunState(r *http.Request, req chatRequestPayload, deps chatHandlerDeps) (chatRunState, int, string, error) {
	reqStore, requestWorkspaceDir, workspaceID, err := resolveSessionStoreForRequest(deps.workspaceDir, deps.store, r)
	if err != nil {
		deps.logger.Error().Err(err).Msg("resolve workspace session store failed")
		return chatRunState{}, http.StatusInternalServerError, "resolve workspace failed", err
	}

	sessionID, err := resolveChatSession(reqStore, req.SessionID, deps.mainSessionID)
	if err != nil {
		if strings.TrimSpace(req.SessionID) == "" {
			deps.logger.Error().Err(err).Msg("create session failed")
			return chatRunState{}, http.StatusInternalServerError, "create session failed", err
		}
		return chatRunState{}, http.StatusNotFound, "session not found", err
	}

	transcriptPath := reqStore.TranscriptPath(sessionID)
	deps.logger.Debug().Str("session_id", sessionID).Str("transcript_path", transcriptPath).Msg("chat session resolved")
	if err := maybeAutoCompactSession(requestWorkspaceDir, transcriptPath, sessionID, deps.client, deps.logger); err != nil {
		deps.logger.Error().Err(err).Str("session_id", sessionID).Msg("auto compaction failed")
		return chatRunState{}, http.StatusInternalServerError, "auto compaction failed", err
	}

	history, err := loadSessionHistory(transcriptPath, chatHistoryMaxTokens)
	if err != nil {
		deps.logger.Error().Err(err).Msg("load history failed")
		return chatRunState{}, http.StatusInternalServerError, "load history failed", err
	}

	registry := buildChatToolRegistry(
		reqStore,
		workspaceID,
		sessionID,
		requestWorkspaceDir,
		history,
		deps,
	)
	extSnapshot := extensions.Snapshot{}
	if deps.tooling.Extensions != nil {
		extSnapshot = deps.tooling.Extensions.Snapshot()
	}
	invokedSkill := resolveInvokedSkill(req.Message, deps.tooling.Extensions)
	systemPrompt, toolChoice, _ := prepareChatContextWithExtensions(requestWorkspaceDir, req.Message, extSnapshot, invokedSkill)
	resolvedProjectID, activeProject, projectPrompt, err := resolveChatProjectContext(requestWorkspaceDir, reqStore, sessionID, strings.TrimSpace(req.ProjectID))
	if err != nil {
		return chatRunState{}, http.StatusNotFound, err.Error(), err
	}
	if strings.TrimSpace(projectPrompt) != "" {
		systemPrompt += "\n" + strings.TrimSpace(projectPrompt) + "\n"
	}
	deps.logger.Debug().
		Str("session_id", sessionID).
		Str("project_id", resolvedProjectID).
		Int("history_messages", len(history)).
		Int("system_prompt_len", len(systemPrompt)).
		Str("tool_choice", toolChoice).
		Msg("chat context assembled")

	userMsg := session.Message{Role: "user", Content: req.Message, Timestamp: time.Now().UTC()}
	if err := session.AppendMessage(transcriptPath, userMsg); err != nil {
		deps.logger.Error().Err(err).Msg("append user message failed")
		return chatRunState{}, http.StatusInternalServerError, "save message failed", err
	}

	llmMessages := buildLLMMessages(systemPrompt, history, req.Message)
	authRole := strings.TrimSpace(serverauth.RoleFromRequest(r))
	injectedSchemas := resolveInjectedToolSchemas(
		registry,
		deps.tooling.ToolsDefaultSet,
		activeProject,
		authRole,
		deps.tooling.ToolsAllowHighRiskUser,
	)
	deps.logger.Debug().
		Str("session_id", sessionID).
		Int("tool_count_injected", len(injectedSchemas)).
		Strs("injected_tools", toolNamesFromSchemas(injectedSchemas)).
		Msg("tool injection result")

	return chatRunState{
		requestWorkspaceDir: requestWorkspaceDir,
		workspaceID:         workspaceID,
		store:               reqStore,
		sessionID:           sessionID,
		projectID:           resolvedProjectID,
		transcriptPath:      transcriptPath,
		history:             history,
		registry:            registry,
		toolChoice:          toolChoice,
		llmMessages:         llmMessages,
		injectedSchemas:     injectedSchemas,
	}, 0, "", nil
}

func buildChatToolRegistry(
	reqStore *session.Store,
	workspaceID string,
	sessionID string,
	requestWorkspaceDir string,
	history []session.Message,
	deps chatHandlerDeps,
) *tool.Registry {
	registry := newBaseToolRegistryWithProcess(requestWorkspaceDir, deps.tooling.ProcessManager)
	projectStore := project.NewStore(requestWorkspaceDir, nil)
	registry.Register(tool.NewProjectCreateTool(projectStore))
	registry.Register(tool.NewProjectListTool(projectStore))
	registry.Register(tool.NewProjectGetTool(projectStore))
	registry.Register(tool.NewProjectUpdateTool(projectStore))
	registry.Register(tool.NewProjectDeleteTool(projectStore))
	registry.Register(tool.NewProjectActivateTool(projectStore, reqStore, deps.mainSessionID))
	registry.Register(tool.NewOpsStatusTool(deps.tooling.OpsManager))
	registry.Register(tool.NewOpsCleanupPlanTool(deps.tooling.OpsManager))
	registry.Register(tool.NewOpsCleanupApplyTool(deps.tooling.OpsManager))
	registry.Register(tool.NewScheduleCreateTool(deps.tooling.ScheduleStore))
	registry.Register(tool.NewScheduleListTool(deps.tooling.ScheduleStore))
	registry.Register(tool.NewScheduleUpdateTool(deps.tooling.ScheduleStore))
	registry.Register(tool.NewScheduleDeleteTool(deps.tooling.ScheduleStore))
	registry.Register(tool.NewScheduleCompleteTool(deps.tooling.ScheduleStore))
	registry.Register(tool.NewResearchReportTool(deps.tooling.ResearchService))
	if deps.tooling.UsageTracker != nil {
		registry.Register(tool.NewUsageReportTool(deps.tooling.UsageTracker))
	}
	registry.Register(tool.NewSessionsListTool(reqStore))
	registry.Register(tool.NewSessionsHistoryTool(reqStore))
	registry.Register(tool.NewSessionsSendTool(deps.tooling.Gateway))
	registry.Register(tool.NewSessionsSpawnTool(deps.tooling.Gateway))
	registry.Register(tool.NewSessionsRunsTool(deps.tooling.Gateway))
	registry.Register(tool.NewAgentsListTool(deps.tooling.Gateway))
	if deps.tooling.AutomationToolsForWorkspace != nil {
		for _, autoTool := range deps.tooling.AutomationToolsForWorkspace(workspaceID) {
			registry.Register(autoTool)
		}
	}
	for _, extra := range deps.extraTools {
		registry.Register(extra)
	}
	if deps.tooling.Extensions != nil {
		for _, extra := range deps.tooling.Extensions.ChatTools() {
			registry.Register(extra)
		}
	}
	registry.Register(tool.NewSessionStatusTool(func(_ context.Context) (tool.SessionStatus, error) {
		return tool.SessionStatus{
			SessionID:       sessionID,
			HistoryMessages: len(history) + 1,
		}, nil
	}))
	return registry
}

func executeChatLoop(
	ctx context.Context,
	deps chatHandlerDeps,
	state chatRunState,
	stream *chatStreamWriter,
) (llm.ChatResponse, bool, error) {
	streamingAnnounced := false
	deltaSent := false
	loop := setupAgentLoop(deps.client, state.registry, state.sessionID, len(state.history), deps.logger, stream.status)

	deps.logger.Debug().Str("session_id", state.sessionID).Int("messages", len(state.llmMessages)).Msg("llm chat call start")
	chatResp, err := loop.Run(ctx, state.llmMessages, agent.RunOptions{
		MaxIterations: deps.maxIters,
		Tools:         state.injectedSchemas,
		ToolChoice:    state.toolChoice,
		OnDelta: func(text string) {
			if text == "" {
				return
			}
			if !streamingAnnounced {
				streamingAnnounced = true
				stream.status("llm_stream", "streaming response", "", "", "", "")
			}
			deltaSent = true
			deps.logger.Debug().Str("session_id", state.sessionID).Int("delta_len", len(text)).Msg("llm delta")
			stream.delta(text)
		},
	})
	if err != nil {
		deps.logger.Debug().Str("session_id", state.sessionID).Err(err).Msg("llm chat call failed")
		return llm.ChatResponse{}, false, err
	}
	deps.logger.Debug().
		Str("session_id", state.sessionID).
		Int("assistant_len", len(chatResp.Message.Content)).
		Int("input_tokens", chatResp.Usage.InputTokens).
		Int("output_tokens", chatResp.Usage.OutputTokens).
		Str("stop_reason", chatResp.StopReason).
		Msg("llm chat call complete")

	return chatResp, deltaSent, nil
}

func persistChatResult(state chatRunState, userMessage string, chatResp llm.ChatResponse, logger zerolog.Logger) {
	assistantMsg := session.Message{Role: "assistant", Content: chatResp.Message.Content, Timestamp: time.Now().UTC()}
	if err := session.AppendMessage(state.transcriptPath, assistantMsg); err != nil {
		logger.Error().Err(err).Msg("append assistant message failed")
	} else if err := state.store.Touch(state.sessionID, assistantMsg.Timestamp); err != nil {
		logger.Error().Err(err).Str("session_id", state.sessionID).Msg("touch session updated_at failed")
	}
	if err := writeChatMemory(state.requestWorkspaceDir, state.sessionID, state.projectID, userMessage, chatResp.Message.Content, assistantMsg.Timestamp); err != nil {
		logger.Error().Err(err).Str("session_id", state.sessionID).Msg("write chat memory failed")
	}
}

func resolveChatProjectContext(
	workspaceDir string,
	store *session.Store,
	sessionID string,
	requestProjectID string,
) (string, *project.Project, string, error) {
	var sessionProjectID string
	if store != nil && strings.TrimSpace(sessionID) != "" {
		sess, err := store.Get(strings.TrimSpace(sessionID))
		if err == nil {
			sessionProjectID = strings.TrimSpace(sess.ProjectID)
		}
	}
	resolvedID := strings.TrimSpace(requestProjectID)
	if resolvedID == "" {
		resolvedID = sessionProjectID
	}
	if resolvedID == "" {
		return "", nil, "", nil
	}

	projectStore := project.NewStore(workspaceDir, nil)
	item, err := projectStore.Get(resolvedID)
	if err != nil {
		return "", nil, "", fmt.Errorf("project not found: %s", resolvedID)
	}
	if store != nil && strings.TrimSpace(sessionID) != "" && strings.TrimSpace(requestProjectID) != "" {
		_ = store.SetProjectID(strings.TrimSpace(sessionID), item.ID)
	}
	return item.ID, &item, formatProjectPromptSection(item), nil
}

func formatProjectPromptSection(item project.Project) string {
	return project.ProjectPromptContext(item)
}

func resolveInjectedToolSchemas(
	registry *tool.Registry,
	toolsDefaultSet string,
	activeProject *project.Project,
	authRole string,
	allowHighRiskUser bool,
) []llm.ToolSchema {
	if registry == nil {
		return nil
	}
	mode := strings.TrimSpace(strings.ToLower(toolsDefaultSet))
	if activeProject == nil {
		if mode == "minimal" {
			names := filterHighRiskToolNamesForRole(defaultMinimalToolNames(), authRole, allowHighRiskUser)
			return registry.SchemasForNames(names)
		}
		if shouldFilterHighRiskTools(authRole, allowHighRiskUser) {
			names := filterHighRiskToolNamesForRole(toolNamesFromSchemas(registry.Schemas()), authRole, allowHighRiskUser)
			return registry.SchemasForNames(names)
		}
		return registry.Schemas()
	}

	names := defaultMinimalToolNames()
	policy := project.NormalizeToolPolicy(project.ToolPolicySpec{
		ToolsAllow:               activeProject.ToolsAllow,
		ToolsAllowExists:         len(activeProject.ToolsAllow) > 0,
		ToolsAllowGroups:         activeProject.ToolsAllowGroups,
		ToolsAllowGroupsExists:   len(activeProject.ToolsAllowGroups) > 0,
		ToolsAllowPatterns:       activeProject.ToolsAllowPatterns,
		ToolsAllowPatternsExists: len(activeProject.ToolsAllowPatterns) > 0,
		ToolsDeny:                activeProject.ToolsDeny,
		ToolsDenyExists:          len(activeProject.ToolsDeny) > 0,
		ToolsRiskMax:             activeProject.ToolsRiskMax,
		ToolsRiskMaxExists:       strings.TrimSpace(activeProject.ToolsRiskMax) != "",
	}, knownToolsFromRegistry(registry), project.ToolPolicyOptions{})
	if len(policy.AllowedTools) > 0 {
		names = append(names, policy.AllowedTools...)
	}
	names = normalizeToolNames(names)
	names = project.ApplyToolConstraints(names, policy)
	names = filterHighRiskToolNamesForRole(names, authRole, allowHighRiskUser)
	if len(names) == 0 {
		return nil
	}
	return registry.SchemasForNames(names)
}

func shouldFilterHighRiskTools(authRole string, allowHighRiskUser bool) bool {
	if allowHighRiskUser {
		return false
	}
	return strings.TrimSpace(strings.ToLower(authRole)) != serverauth.RoleAdmin
}

func filterHighRiskToolNamesForRole(names []string, authRole string, allowHighRiskUser bool) []string {
	if !shouldFilterHighRiskTools(authRole, allowHighRiskUser) {
		return names
	}
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if isHighRiskToolName(name) {
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered
}

func isHighRiskToolName(name string) bool {
	canonical := tool.CanonicalToolName(name)
	if canonical == "" {
		return false
	}
	switch canonical {
	case "exec", "process", "write", "write_file", "edit", "edit_file", "apply_patch":
		return true
	}
	return strings.HasPrefix(canonical, "write_") || strings.HasPrefix(canonical, "edit_")
}

func knownToolsFromRegistry(registry *tool.Registry) map[string]struct{} {
	out := map[string]struct{}{}
	if registry == nil {
		return out
	}
	for _, schema := range registry.Schemas() {
		name := tool.CanonicalToolName(schema.Function.Name)
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	return out
}

func normalizeToolNames(names []string) []string {
	out := make([]string, 0, len(names))
	seen := map[string]struct{}{}
	for _, item := range names {
		name := tool.CanonicalToolName(item)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func defaultMinimalToolNames() []string {
	return []string{
		"memory_get",
		"memory_search",
		"memory_save",
		"project_get",
		"project_list",
		"project_update",
		"project_activate",
		"ops_status",
		"ops_cleanup_plan",
		"schedule_list",
		"schedule_create",
		"research_report",
		"usage_report",
		"session_status",
		"sessions_list",
		"sessions_history",
	}
}

func newChatStreamWriter(w http.ResponseWriter, sessionID string, logger zerolog.Logger) *chatStreamWriter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	return &chatStreamWriter{
		w:         w,
		flusher:   flusher,
		sessionID: sessionID,
		logger:    logger,
	}
}

func (s *chatStreamWriter) send(data any) {
	if s == nil {
		return
	}
	jsonData, _ := json.Marshal(data)
	_, _ = fmt.Fprintf(s.w, "data: %s\n\n", jsonData)
	if evt, ok := data.(map[string]string); ok {
		s.logger.Debug().Str("event_type", evt["type"]).Msg("chat sse event")
	}
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

func (s *chatStreamWriter) status(phase, message, toolName, toolCallID, toolArgsPreview, toolResultPreview string) {
	payload := map[string]string{
		"type":       "status",
		"phase":      phase,
		"message":    message,
		"session_id": s.sessionID,
	}
	if strings.TrimSpace(toolName) != "" {
		payload["tool_name"] = strings.TrimSpace(toolName)
	}
	if strings.TrimSpace(toolCallID) != "" {
		payload["tool_call_id"] = strings.TrimSpace(toolCallID)
	}
	if strings.TrimSpace(toolArgsPreview) != "" {
		payload["tool_args_preview"] = strings.TrimSpace(toolArgsPreview)
	}
	if strings.TrimSpace(toolResultPreview) != "" {
		payload["tool_result_preview"] = strings.TrimSpace(toolResultPreview)
	}
	s.send(payload)
}

func (s *chatStreamWriter) delta(text string) {
	s.send(map[string]string{"type": "delta", "text": text})
}

func (s *chatStreamWriter) error(err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	s.send(map[string]string{"type": "error", "error": msg})
}

func (s *chatStreamWriter) done(usage llm.Usage) {
	s.send(map[string]any{
		"type":       "done",
		"session_id": s.sessionID,
		"usage": map[string]int{
			"input_tokens":       usage.InputTokens,
			"output_tokens":      usage.OutputTokens,
			"cached_tokens":      usage.CachedTokens,
			"cache_read_tokens":  usage.CacheReadTokens,
			"cache_write_tokens": usage.CacheWriteTokens,
		},
	})
}
