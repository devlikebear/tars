package tarsserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/devlikebear/tarsncase/internal/agent"
	"github.com/devlikebear/tarsncase/internal/extensions"
	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/prompt"
	"github.com/devlikebear/tarsncase/internal/secrets"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/devlikebear/tarsncase/internal/skill"
	"github.com/devlikebear/tarsncase/internal/tool"
	"github.com/devlikebear/tarsncase/internal/usage"
	"github.com/rs/zerolog"
)

func resolveChatSession(store *session.Store, sessionID string, mainSessionID string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		id := strings.TrimSpace(mainSessionID)
		if id == "" {
			sess, err := store.Create("chat")
			if err != nil {
				return "", err
			}
			return sess.ID, nil
		}
		if _, err := store.Get(id); err != nil {
			return "", err
		}
		return id, nil
	}
	if _, err := store.Get(strings.TrimSpace(sessionID)); err != nil {
		return "", err
	}
	return strings.TrimSpace(sessionID), nil
}

func prepareChatContext(workspaceDir, userMessage string) (systemPrompt string, toolChoice string, err error) {
	return prepareChatContextWithExtensions(workspaceDir, userMessage, extensions.Snapshot{}, nil)
}

func prepareChatContextWithExtensions(
	workspaceDir string,
	userMessage string,
	extSnapshot extensions.Snapshot,
	invokedSkill *skill.Definition,
) (systemPrompt string, toolChoice string, err error) {
	systemPrompt = prompt.Build(prompt.BuildOptions{WorkspaceDir: workspaceDir})
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
	if shouldForceMemoryToolCall(userMessage) {
		toolChoice = "required"
	}
	return systemPrompt, toolChoice, nil
}

func loadSessionHistory(transcriptPath string, maxTokens int) ([]session.Message, error) {
	return session.LoadHistory(transcriptPath, maxTokens)
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

type chatToolingOptions struct {
	ProcessManager              *tool.ProcessManager
	Extensions                  *extensions.Manager
	Gateway                     *gateway.Runtime
	AutomationToolsForWorkspace func(workspaceID string) []tool.Tool
	ToolsDefaultSet             string
	UsageTracker                *usage.Tracker
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
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat", func(w http.ResponseWriter, r *http.Request) {
		handleChatRequest(w, r, chatHandlerDeps{
			workspaceDir:  workspaceDir,
			store:         store,
			client:        client,
			logger:        logger,
			maxIters:      maxIters,
			activity:      activity,
			mainSessionID: strings.TrimSpace(mainSessionID),
			tooling:       tooling,
			extraTools:    extraTools,
		})
	})
	return mux
}
