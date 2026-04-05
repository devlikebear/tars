package tarsserver

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/prompt"
	"github.com/devlikebear/tars/internal/research"
	"github.com/devlikebear/tars/internal/schedule"
	"github.com/devlikebear/tars/internal/secrets"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

func resolveChatSession(store *session.Store, sessionID string, mainSessionID string) (string, error) {
	// The public session API exposes the main session as id="main";
	// translate it back to the real internal ID so store.Get succeeds.
	trimmedID := strings.TrimSpace(sessionID)
	if strings.EqualFold(trimmedID, "main") {
		sessionID = strings.TrimSpace(mainSessionID)
	} else if strings.EqualFold(trimmedID, "new") {
		return createFallbackChatSession(store)
	}
	if strings.TrimSpace(sessionID) == "" {
		id := strings.TrimSpace(mainSessionID)
		if id == "" {
			return createFallbackChatSession(store)
		}
		if _, err := store.Get(id); err != nil {
			return createFallbackChatSession(store)
		}
		return id, nil
	}
	if _, err := store.Get(strings.TrimSpace(sessionID)); err != nil {
		// Requested session is stale; create a fresh session instead of
		// silently attaching the request to the main session.
		return createFallbackChatSession(store)
	}
	return strings.TrimSpace(sessionID), nil
}

func createFallbackChatSession(store *session.Store) (string, error) {
	sess, err := store.Create("chat")
	if err != nil {
		return "", err
	}
	return sess.ID, nil
}

func prepareChatContext(workspaceDir, userMessage string) (systemPrompt string, toolChoice string, err error) {
	return prepareChatContextWithExtensions(workspaceDir, "", userMessage, extensions.Snapshot{}, nil)
}

type preparedChatContext struct {
	SystemPrompt         string
	ToolChoice           string
	SystemPromptTokens   int
	RelevantMemoryCount  int
	RelevantMemoryTokens int
}

func prepareChatContextWithExtensions(
	workspaceDir string,
	sessionID string,
	userMessage string,
	extSnapshot extensions.Snapshot,
	invokedSkill *skill.Definition,
	semanticCfg ...memory.SemanticConfig,
) (systemPrompt string, toolChoice string, err error) {
	details, err := prepareChatContextDetailsWithExtensions(workspaceDir, sessionID, userMessage, extSnapshot, invokedSkill, semanticCfg...)
	if err != nil {
		return "", "", err
	}
	return details.SystemPrompt, details.ToolChoice, nil
}

func prepareChatContextDetailsWithExtensions(
	workspaceDir string,
	sessionID string,
	userMessage string,
	extSnapshot extensions.Snapshot,
	invokedSkill *skill.Definition,
	semanticCfg ...memory.SemanticConfig,
) (preparedChatContext, error) {
	return prepareChatContextDetailsWithCache(workspaceDir, sessionID, userMessage, extSnapshot, invokedSkill, nil, firstSemanticConfig(semanticCfg...), nil, "")
}

func prepareChatContextDetailsWithCache(
	workspaceDir string,
	sessionID string,
	userMessage string,
	extSnapshot extensions.Snapshot,
	invokedSkill *skill.Definition,
	cache *memoryCache,
	semanticCfg memory.SemanticConfig,
	workDirs []string,
	currentDir string,
) (preparedChatContext, error) {
	forceRelevantMemory := shouldForceMemoryToolCall(userMessage)
	extSnapshot = filterSkillSnapshotForProject(extSnapshot, workspaceDir)

	// Cache-first strategy: check cache before expensive memory search
	if cached, ok := cache.Get(userMessage, sessionID); ok {
		return buildContextFromResult(cached, extSnapshot, invokedSkill, forceRelevantMemory), nil
	}

	memService := buildSemanticMemoryService(workspaceDir, semanticCfg)
	buildResult := prompt.BuildResultFor(prompt.BuildOptions{
		WorkspaceDir:        workspaceDir,
		WorkDirs:            workDirs,
		CurrentDir:          currentDir,
		Query:               userMessage,
		SessionID:           sessionID,
		MemorySearcher:      memService,
		ForceRelevantMemory: forceRelevantMemory,
	})

	// Populate cache with search result
	cache.Put(userMessage, sessionID, buildResult)

	return buildContextFromResult(buildResult, extSnapshot, invokedSkill, forceRelevantMemory), nil
}

func buildContextFromResult(
	buildResult prompt.BuildResult,
	extSnapshot extensions.Snapshot,
	invokedSkill *skill.Definition,
	forceRelevantMemory bool,
) preparedChatContext {
	systemPrompt := buildResult.Prompt
	systemPrompt += "\n" + strings.TrimSpace(memoryToolSystemRule) + "\n"
	if strings.TrimSpace(extSnapshot.SkillPrompt) != "" {
		systemPrompt += "\n## Skills\n"
		systemPrompt += strings.TrimSpace(extSnapshot.SkillPrompt) + "\n"
		systemPrompt += "\n## Skill Usage Policy\n"
		systemPrompt += "- Skill body content is not preloaded in the prompt.\n"
		systemPrompt += "- If you need a skill, call read_file with the listed skill path first.\n"
	}
	if invokedSkill != nil {
		systemPrompt += "\n## Invoked Skill\n"
		systemPrompt += fmt.Sprintf(
			"User invoked /%s.\nBefore responding, call read_file on path %q to load this skill.\n",
			strings.TrimSpace(invokedSkill.Name),
			strings.TrimSpace(invokedSkill.RuntimePath),
		)
	}
	toolChoice := ""
	if forceRelevantMemory {
		toolChoice = "required"
	}
	return preparedChatContext{
		SystemPrompt:         systemPrompt,
		ToolChoice:           toolChoice,
		SystemPromptTokens:   promptTokenEstimate(systemPrompt),
		RelevantMemoryCount:  buildResult.RelevantMemoryCount,
		RelevantMemoryTokens: buildResult.RelevantTokens,
	}
}

func filterSkillSnapshotForProject(snapshot extensions.Snapshot, _ string) extensions.Snapshot {
	// No project-level skill filtering after project package removal.
	return snapshot
}

func loadSessionHistory(transcriptPath string, maxTokens int) ([]session.Message, error) {
	snapshot, err := loadSessionHistorySnapshot(transcriptPath, maxTokens)
	if err != nil {
		return nil, err
	}
	return snapshot.Messages, nil
}

func loadSessionHistorySnapshot(transcriptPath string, maxTokens int) (session.HistorySnapshot, error) {
	return session.LoadHistorySnapshot(transcriptPath, maxTokens)
}

func buildLLMMessages(systemPrompt string, history []session.Message, userMessage string) []llm.ChatMessage {
	return buildLLMMessagesWithBlocks(systemPrompt, history, userMessage, nil)
}

func buildLLMMessagesWithBlocks(systemPrompt string, history []session.Message, userMessage string, contentBlocks []llm.ContentBlock) []llm.ChatMessage {
	llmMessages := make([]llm.ChatMessage, 0, len(history)+2)
	llmMessages = append(llmMessages, llm.ChatMessage{Role: "system", Content: systemPrompt})
	for _, m := range history {
		llmMessages = append(llmMessages, llm.ChatMessage{Role: m.Role, Content: m.Content})
	}
	msg := llm.ChatMessage{Role: "user", Content: userMessage, ContentBlocks: contentBlocks}
	llmMessages = append(llmMessages, msg)
	return llmMessages
}

// attachmentsToContentBlocks converts chat attachments to LLM content blocks.
// Text files are injected as text blocks, images as image blocks, PDFs as document blocks.
func attachmentsToContentBlocks(attachments []chatAttachment) []llm.ContentBlock {
	if len(attachments) == 0 {
		return nil
	}
	blocks := make([]llm.ContentBlock, 0, len(attachments))
	for _, a := range attachments {
		mime := strings.TrimSpace(a.MimeType)
		data := strings.TrimSpace(a.Data)
		if data == "" {
			continue
		}

		switch {
		case strings.HasPrefix(mime, "text/") || isTextMime(mime):
			// Decode base64 text content and inject as text block
			decoded, err := base64Decode(data)
			if err != nil {
				continue
			}
			label := strings.TrimSpace(a.Name)
			if label == "" {
				label = "attachment"
			}
			blocks = append(blocks, llm.ContentBlock{
				Type: "text",
				Text: fmt.Sprintf("--- File: %s ---\n%s\n--- End of file ---", label, string(decoded)),
			})
		case strings.HasPrefix(mime, "image/"):
			blocks = append(blocks, llm.ContentBlock{
				Type:      "image",
				MediaType: mime,
				Data:      data,
			})
		case mime == "application/pdf":
			blocks = append(blocks, llm.ContentBlock{
				Type:      "document",
				MediaType: mime,
				Data:      data,
			})
		default:
			// Unknown binary — try as text
			decoded, err := base64Decode(data)
			if err != nil {
				continue
			}
			label := strings.TrimSpace(a.Name)
			if label == "" {
				label = "attachment"
			}
			blocks = append(blocks, llm.ContentBlock{
				Type: "text",
				Text: fmt.Sprintf("--- File: %s ---\n%s\n--- End of file ---", label, string(decoded)),
			})
		}
	}
	return blocks
}

func isTextMime(mime string) bool {
	textTypes := []string{
		"application/json", "application/xml", "application/yaml",
		"application/x-yaml", "application/javascript", "application/typescript",
		"application/toml", "application/x-sh",
	}
	for _, t := range textTypes {
		if mime == t {
			return true
		}
	}
	return false
}

func base64Decode(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}

func statusPreview(value string, maxLen int) string {
	return secrets.RedactPreview(value, maxLen)
}

func resolveInvokedSkill(message string, manager *extensions.Manager) *skill.Definition {
	if manager == nil {
		return nil
	}
	trimmed := strings.TrimSpace(message)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		return nil
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return nil
	}
	name := strings.TrimPrefix(fields[0], "/")
	if strings.TrimSpace(name) == "" || strings.Contains(name, "/") {
		return nil
	}
	s, ok := manager.FindSkill(name)
	if !ok || !s.UserInvocable {
		return nil
	}
	copySkill := s
	return &copySkill
}

func resolveSkillForMessage(message string, manager *extensions.Manager, workspaceDir, sessionID string) *skill.Definition {
	resolved := resolveSkillSelection(message, manager, workspaceDir, sessionID)
	return resolved.Definition
}

type skillSelection struct {
	Definition *skill.Definition
	Reason     string
}

func resolveSkillSelection(message string, manager *extensions.Manager, workspaceDir, sessionID string, sessionConfig ...session.SessionToolConfig) skillSelection {
	if invoked := resolveInvokedSkill(message, manager); invoked != nil {
		if len(sessionConfig) > 0 {
			filtered := applySessionSkillConfig([]skill.Definition{*invoked}, sessionConfig[0])
			if len(filtered) == 0 {
				return skillSelection{}
			}
		}
		return skillSelection{Definition: invoked, Reason: "explicit_command"}
	}
	projectStart := findProjectStartSkill(manager)
	if projectStart == nil {
		return skillSelection{}
	}
	if hasActiveProjectBrief(workspaceDir, sessionID) {
		if len(sessionConfig) > 0 {
			filtered := applySessionSkillConfig([]skill.Definition{*projectStart}, sessionConfig[0])
			if len(filtered) == 0 {
				return skillSelection{}
			}
		}
		return skillSelection{Definition: projectStart, Reason: "active_brief"}
	}
	return skillSelection{}
}

func findProjectStartSkill(manager *extensions.Manager) *skill.Definition {
	if manager == nil {
		return nil
	}
	for _, name := range []string{"project-start", "project_start"} {
		if skillDef, ok := manager.FindSkill(name); ok {
			copySkill := skillDef
			return &copySkill
		}
	}
	return nil
}

func hasActiveProjectBrief(_, _ string) bool {
	// Project briefs are no longer available after project package removal.
	return false
}

func latestTurnUsedTools(messages []session.Message) []string {
	if len(messages) == 0 {
		return nil
	}
	start := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.TrimSpace(messages[i].Role) == "user" {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	used := make([]string, 0)
	seen := map[string]struct{}{}
	for i := start + 1; i < len(messages); i++ {
		if strings.TrimSpace(messages[i].Role) == "user" {
			break
		}
		if strings.TrimSpace(messages[i].Role) != "tool" {
			continue
		}
		name := strings.TrimSpace(messages[i].ToolName)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		used = append(used, name)
	}
	return used
}

// ToolCallRecord holds a tool invocation for transcript persistence.
type ToolCallRecord struct {
	ToolName   string
	ToolCallID string
	ToolArgs   string
	ToolResult string
}

func setupAgentLoop(
	client llm.Client,
	registry *tool.Registry,
	sessionID string,
	historyLen int,
	logger zerolog.Logger,
	sendStatus func(string, string, string, string, string, string),
) (*agent.Loop, *[]ToolCallRecord) {
	toolCalls := &[]ToolCallRecord{}
	if _, ok := registry.Get("session_status"); !ok {
		registry.Register(tool.NewSessionStatusTool(func(_ context.Context) (tool.SessionStatus, error) {
			return tool.SessionStatus{
				SessionID:       sessionID,
				HistoryMessages: historyLen + 1,
			}, nil
		}))
	}

	counterHook := agent.NewCounterHook()
	auditHook := agent.NewAuditHook(64)
	logHook := agent.HookFunc(func(_ context.Context, evt agent.Event) {
		logger.Debug().
			Str("event", string(evt.Type)).
			Int("iteration", evt.Iteration).
			Int("message_count", evt.MessageCount).
			Str("tool_name", evt.ToolName).
			Str("tool_call_id", evt.ToolCallID).
			Msg("agent loop event")
		switch evt.Type {
		case agent.EventLoopStart:
			sendStatus("loop_start", "agent loop started", "", "", "", "")
		case agent.EventBeforeLLM:
			sendStatus("before_llm", "calling llm", "", "", "", "")
		case agent.EventAfterLLM:
			sendStatus("after_llm", "llm response received", "", "", "", "")
		case agent.EventBeforeTool:
			sendStatus(
				"before_tool_call",
				"executing tool",
				evt.ToolName,
				evt.ToolCallID,
				statusPreview(evt.ToolArgs, 180),
				"",
			)
		case agent.EventAfterTool:
			sendStatus(
				"after_tool_call",
				"tool completed",
				evt.ToolName,
				evt.ToolCallID,
				statusPreview(evt.ToolArgs, 180),
				statusPreview(evt.ToolResult, 180),
			)
			*toolCalls = append(*toolCalls, ToolCallRecord{
				ToolName:   evt.ToolName,
				ToolCallID: evt.ToolCallID,
				ToolArgs:   statusPreview(evt.ToolArgs, 500),
				ToolResult: statusPreview(evt.ToolResult, 500),
			})
		case agent.EventLoopEnd:
			sendStatus("loop_end", "agent loop completed", "", "", "", "")
			logger.Debug().
				Str("session_id", sessionID).
				Any("event_counts", counterHook.Snapshot()).
				Int("audit_entries", len(auditHook.Entries())).
				Msg("agent loop summary")
		case agent.EventLoopError:
			msg := "agent loop error"
			if evt.Err != nil {
				msg = evt.Err.Error()
			}
			sendStatus("error", msg, evt.ToolName, evt.ToolCallID, "", "")
		}
	})
	return agent.NewLoop(client, registry, counterHook, auditHook, logHook), toolCalls
}

func resolveAgentMaxIterations(value int) int {
	if value <= 0 {
		return agent.DefaultMaxLoopIters
	}
	return value
}

func promptTokenEstimate(content string) int {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return 0
	}
	tokens := len(trimmed) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func sumHistoryTokens(messages []session.Message) int {
	total := 0
	for _, m := range messages {
		total += promptTokenEstimate(m.Content)
	}
	return total
}

type chatToolingOptions struct {
	ProcessManager              *tool.ProcessManager
	Extensions                  *extensions.Manager
	Gateway                     *gateway.Runtime
	AutomationToolsForWorkspace func(workspaceID string) []tool.Tool
	ToolsDefaultSet             string
	ToolsAllowHighRiskUser      bool
	MemorySemanticConfig        memory.SemanticConfig
	MemoryCache                 *memoryCache
	APIMaxInflightChat          int
	UsageTracker                *usage.Tracker
	OpsManager                  *ops.Manager
	ScheduleStore               *schedule.Store
	ResearchService             *research.Service
}

func defaultChatToolingOptions() chatToolingOptions {
	return chatToolingOptions{}
}

func toolNamesFromSchemas(schemas []llm.ToolSchema) []string {
	out := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		name := strings.TrimSpace(schema.Function.Name)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func skillNamesFromDefinitions(defs []skill.Definition) []string {
	out := make([]string, 0, len(defs))
	for _, def := range defs {
		name := strings.TrimSpace(def.Name)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func skillNameOrEmpty(def *skill.Definition) string {
	if def == nil {
		return ""
	}
	return strings.TrimSpace(def.Name)
}

func newChatAPIHandler(workspaceDir string, store *session.Store, client llm.Client, logger zerolog.Logger) http.Handler {
	return newChatAPIHandlerWithRuntimeConfig(
		workspaceDir,
		store,
		client,
		nil,
		logger,
		agent.DefaultMaxLoopIters,
		nil,
		"",
		defaultChatToolingOptions(),
	)
}

func newChatAPIHandlerWithOptions(
	workspaceDir string,
	store *session.Store,
	client llm.Client,
	logger zerolog.Logger,
	maxIterations int,
	extraTools ...tool.Tool,
) http.Handler {
	return newChatAPIHandlerWithRuntimeConfig(
		workspaceDir,
		store,
		client,
		nil,
		logger,
		maxIterations,
		nil,
		"",
		defaultChatToolingOptions(),
		extraTools...,
	)
}

func newChatAPIHandlerWithRuntime(
	workspaceDir string,
	store *session.Store,
	client llm.Client,
	logger zerolog.Logger,
	maxIterations int,
	activity *runtimeActivity,
	extraTools ...tool.Tool,
) http.Handler {
	return newChatAPIHandlerWithRuntimeConfig(
		workspaceDir,
		store,
		client,
		nil,
		logger,
		maxIterations,
		activity,
		"",
		defaultChatToolingOptions(),
		extraTools...,
	)
}

func newChatAPIHandlerWithRuntimeConfig(
	workspaceDir string,
	store *session.Store,
	client llm.Client,
	router llm.Router,
	logger zerolog.Logger,
	maxIterations int,
	activity *runtimeActivity,
	mainSessionID string,
	tooling chatToolingOptions,
	extraTools ...tool.Tool,
) http.Handler {
	maxIters := resolveAgentMaxIterations(maxIterations)
	chatLimiter := newInflightLimiter(tooling.APIMaxInflightChat, 2)
	cancelRegistry := newChatCancelRegistry()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat", func(w http.ResponseWriter, r *http.Request) {
		handleChatRequest(w, r, chatHandlerDeps{
			workspaceDir:   workspaceDir,
			store:          store,
			client:         client,
			router:         router,
			logger:         logger,
			maxIters:       maxIters,
			chatLimiter:    chatLimiter,
			activity:       activity,
			mainSessionID:  strings.TrimSpace(mainSessionID),
			tooling:        tooling,
			extraTools:     extraTools,
			cancelRegistry: cancelRegistry,
		})
	})
	mux.HandleFunc("/v1/chat/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w)
			return
		}
		sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
		if sessionID == "" {
			writeError(w, http.StatusBadRequest, "", "session_id is required")
			return
		}
		if cancelRegistry.Cancel(sessionID) {
			writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
		} else {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no active chat for session"})
		}
	})
	mux.HandleFunc("/v1/chat/tools", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w)
			return
		}
		reqStore, requestWorkspaceDir, _, err := resolveSessionStoreForRequest(workspaceDir, store, r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "", "resolve workspace failed")
			return
		}
		registry := buildChatToolRegistry(
			reqStore, "", "", requestWorkspaceDir, tool.SingleDirPolicy(requestWorkspaceDir), nil, chatHandlerDeps{
				workspaceDir:  workspaceDir,
				store:         store,
				client:        client,
				router:        router,
				logger:        logger,
				tooling:       tooling,
				extraTools:    extraTools,
				mainSessionID: strings.TrimSpace(mainSessionID),
			},
		)
		type toolInfo struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			HighRisk    bool   `json:"high_risk"`
		}
		schemas := registry.Schemas()
		tools := make([]toolInfo, 0, len(schemas))
		for _, s := range schemas {
			tools = append(tools, toolInfo{
				Name:        s.Function.Name,
				Description: s.Function.Description,
				HighRisk:    isHighRiskToolName(s.Function.Name),
			})
		}
		// Include skills and MCP info if available
		type chatToolsResponse struct {
			Tools  []toolInfo `json:"tools"`
			Skills []string   `json:"skills,omitempty"`
			MCP    []string   `json:"mcp_servers,omitempty"`
		}
		resp := chatToolsResponse{Tools: tools}
		if tooling.Extensions != nil {
			snap := tooling.Extensions.Snapshot()
			for _, sk := range snap.Skills {
				resp.Skills = append(resp.Skills, sk.Name)
			}
		}
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("/v1/chat/context", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w)
			return
		}
		sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
		if sessionID == "" {
			writeError(w, http.StatusBadRequest, "", "session_id is required")
			return
		}
		reqStore, requestWorkspaceDir, _, err := resolveSessionStoreForRequest(workspaceDir, store, r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "", "resolve workspace failed")
			return
		}
		sess, err := reqStore.Get(sessionID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		transcriptPath := reqStore.TranscriptPath(sessionID)
		historySnapshot, err := loadSessionHistorySnapshot(transcriptPath, chatHistoryMaxTokens)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "", "load history failed")
			return
		}
		extSnapshot := extensions.Snapshot{}
		if tooling.Extensions != nil {
			extSnapshot = tooling.Extensions.Snapshot()
		}
		var sessionToolConfigs []session.SessionToolConfig
		if sess.ToolConfig != nil {
			sessionToolConfigs = append(sessionToolConfigs, *sess.ToolConfig)
		}
		extSnapshot = filterExtensionsSnapshotForSession(extSnapshot, sessionToolConfigs...)
		// Build PathPolicy from session work_dirs for context preview
		var previewPolicy tool.PathPolicy
		if len(sess.WorkDirs) > 0 {
			previewPolicy = tool.NewPathPolicy(requestWorkspaceDir, sess.WorkDirs, sess.CurrentDir)
		} else {
			previewPolicy = tool.SingleDirPolicy(requestWorkspaceDir)
		}
		contextDetails, err := prepareChatContextDetailsWithCache(
			requestWorkspaceDir, sessionID, "(context preview)",
			extSnapshot, nil, tooling.MemoryCache, tooling.MemorySemanticConfig,
			sess.WorkDirs, sess.CurrentDir,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "", "prepare context failed")
			return
		}
		systemPrompt := contextDetails.SystemPrompt
		if strings.TrimSpace(sess.PromptOverride) != "" {
			systemPrompt += "\n\n## Session Prompt Override\n" + strings.TrimSpace(sess.PromptOverride) + "\n"
		}
		registry := buildChatToolRegistry(
			reqStore, "", sessionID, requestWorkspaceDir, previewPolicy, historySnapshot.Messages, chatHandlerDeps{
				workspaceDir:  workspaceDir,
				store:         store,
				client:        client,
				router:        router,
				logger:        logger,
				tooling:       tooling,
				extraTools:    extraTools,
				mainSessionID: strings.TrimSpace(mainSessionID),
			},
		)
		injectedSchemas := resolveInjectedToolSchemas(
			registry,
			tooling.ToolsDefaultSet,
			nil,
			"admin",
			tooling.ToolsAllowHighRiskUser,
			sessionToolConfigs...,
		)
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id":           sessionID,
			"system_prompt":        systemPrompt,
			"system_prompt_tokens": promptTokenEstimate(systemPrompt),
			"history_tokens":       sumHistoryTokens(historySnapshot.Messages),
			"history_messages":     len(historySnapshot.Messages),
			"tool_count":           len(injectedSchemas),
			"tool_names":           toolNamesFromSchemas(injectedSchemas),
			"skill_count":          len(extSnapshot.Skills),
			"skill_names":          skillNamesFromDefinitions(extSnapshot.Skills),
			"memory_count":         contextDetails.RelevantMemoryCount,
			"memory_tokens":        contextDetails.RelevantMemoryTokens,
			"used_tool_names":      latestTurnUsedTools(historySnapshot.Messages),
			"prompt_override":      sess.PromptOverride,
		})
	})
	return mux
}
