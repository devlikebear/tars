package tarsserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/project"
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

func resolveChatSession(store *session.Store, sessionID string, mainSessionID string, userMessage string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		if isProjectKickoffMessage(userMessage) {
			return createFallbackChatSession(store)
		}
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
	return prepareChatContextWithExtensions(workspaceDir, "", "", userMessage, extensions.Snapshot{}, nil)
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
	projectID string,
	sessionID string,
	userMessage string,
	extSnapshot extensions.Snapshot,
	invokedSkill *skill.Definition,
) (systemPrompt string, toolChoice string, err error) {
	details, err := prepareChatContextDetailsWithExtensions(workspaceDir, projectID, sessionID, userMessage, extSnapshot, invokedSkill)
	if err != nil {
		return "", "", err
	}
	return details.SystemPrompt, details.ToolChoice, nil
}

func prepareChatContextDetailsWithExtensions(
	workspaceDir string,
	projectID string,
	sessionID string,
	userMessage string,
	extSnapshot extensions.Snapshot,
	invokedSkill *skill.Definition,
) (preparedChatContext, error) {
	forceRelevantMemory := shouldForceMemoryToolCall(userMessage)
	extSnapshot = filterSkillSnapshotForProject(extSnapshot, workspaceDir, projectID)
	buildResult := prompt.BuildResultFor(prompt.BuildOptions{
		WorkspaceDir:        workspaceDir,
		Query:               userMessage,
		ProjectID:           projectID,
		SessionID:           sessionID,
		ForceRelevantMemory: forceRelevantMemory,
	})
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
	}, nil
}

func filterSkillSnapshotForProject(snapshot extensions.Snapshot, workspaceDir, projectID string) extensions.Snapshot {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || strings.TrimSpace(workspaceDir) == "" {
		return snapshot
	}
	store := project.NewStore(workspaceDir, nil)
	item, err := store.Get(projectID)
	if err != nil || len(item.SkillsAllow) == 0 {
		return snapshot
	}
	allowed := map[string]struct{}{}
	for _, name := range item.SkillsAllow {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			continue
		}
		allowed[key] = struct{}{}
	}
	filtered := make([]skill.Definition, 0, len(snapshot.Skills))
	for _, def := range snapshot.Skills {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(def.Name))]; !ok {
			continue
		}
		filtered = append(filtered, def)
	}
	if len(filtered) == 0 {
		snapshot.SkillPrompt = ""
		snapshot.Skills = nil
		return snapshot
	}
	snapshot.Skills = filtered
	snapshot.SkillPrompt = skill.FormatAvailableSkills(filtered)
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
	llmMessages := make([]llm.ChatMessage, 0, len(history)+2)
	llmMessages = append(llmMessages, llm.ChatMessage{Role: "system", Content: systemPrompt})
	for _, m := range history {
		llmMessages = append(llmMessages, llm.ChatMessage{Role: m.Role, Content: m.Content})
	}
	llmMessages = append(llmMessages, llm.ChatMessage{Role: "user", Content: userMessage})
	return llmMessages
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
	if invoked := resolveInvokedSkill(message, manager); invoked != nil {
		return invoked
	}
	projectStart := findProjectStartSkill(manager)
	if projectStart == nil {
		return nil
	}
	if hasActiveProjectBrief(workspaceDir, sessionID) {
		return projectStart
	}
	if isProjectKickoffMessage(message) {
		return projectStart
	}
	return nil
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

func hasActiveProjectBrief(workspaceDir, sessionID string) bool {
	if strings.TrimSpace(workspaceDir) == "" || strings.TrimSpace(sessionID) == "" {
		return false
	}
	store := project.NewStore(workspaceDir, nil)
	item, err := store.GetBrief(strings.TrimSpace(sessionID))
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(item.Status)) {
	case "collecting", "ready":
		return true
	default:
		return false
	}
}

func isProjectKickoffMessage(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" || strings.HasPrefix(lower, "/") {
		return false
	}
	projectHints := []string{"project", "프로젝트"}
	actionHints := []string{"start", "시작", "만들", "build", "create", "개발", "구축"}
	hasProject := false
	for _, hint := range projectHints {
		if strings.Contains(lower, hint) {
			hasProject = true
			break
		}
	}
	if !hasProject {
		return false
	}
	for _, hint := range actionHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

func setupAgentLoop(
	client llm.Client,
	registry *tool.Registry,
	sessionID string,
	historyLen int,
	logger zerolog.Logger,
	sendStatus func(string, string, string, string, string, string),
) *agent.Loop {
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
				"",
				statusPreview(evt.ToolResult, 180),
			)
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
	return agent.NewLoop(client, registry, counterHook, auditHook, logHook)
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

type chatToolingOptions struct {
	ProcessManager              *tool.ProcessManager
	Extensions                  *extensions.Manager
	Gateway                     *gateway.Runtime
	ProjectAutopilot            *project.AutopilotManager
	AutomationToolsForWorkspace func(workspaceID string) []tool.Tool
	ToolsDefaultSet             string
	ToolsAllowHighRiskUser      bool
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

func newChatAPIHandler(workspaceDir string, store *session.Store, client llm.Client, logger zerolog.Logger) http.Handler {
	return newChatAPIHandlerWithRuntimeConfig(
		workspaceDir,
		store,
		client,
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
	logger zerolog.Logger,
	maxIterations int,
	activity *runtimeActivity,
	mainSessionID string,
	tooling chatToolingOptions,
	extraTools ...tool.Tool,
) http.Handler {
	maxIters := resolveAgentMaxIterations(maxIterations)
	chatLimiter := newInflightLimiter(tooling.APIMaxInflightChat, 2)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat", func(w http.ResponseWriter, r *http.Request) {
		handleChatRequest(w, r, chatHandlerDeps{
			workspaceDir:  workspaceDir,
			store:         store,
			client:        client,
			logger:        logger,
			maxIters:      maxIters,
			chatLimiter:   chatLimiter,
			activity:      activity,
			mainSessionID: strings.TrimSpace(mainSessionID),
			tooling:       tooling,
			extraTools:    extraTools,
		})
	})
	return mux
}
