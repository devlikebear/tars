package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/agent"
	"github.com/devlikebear/tarsncase/internal/extensions"
	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/prompt"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/devlikebear/tarsncase/internal/skill"
	"github.com/devlikebear/tarsncase/internal/tool"
	"github.com/rs/zerolog"
)

func resolveChatSession(store *session.Store, sessionID string) (string, error) {
	if sessionID == "" {
		sess, err := store.Create("chat")
		if err != nil {
			return "", err
		}
		return sess.ID, nil
	}
	if _, err := store.Get(sessionID); err != nil {
		return "", err
	}
	return sessionID, nil
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

func setupSSEWriter(w http.ResponseWriter, sessionID string, logger zerolog.Logger) (
	sendSSE func(any),
	sendStatus func(string, string, string, string, string, string),
	flusher http.Flusher,
) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ = w.(http.Flusher)
	sendSSE = func(data any) {
		jsonData, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", jsonData)
		if evt, ok := data.(map[string]string); ok {
			logger.Debug().Str("event_type", evt["type"]).Msg("chat sse event")
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
	sendStatus = func(phase, message, toolName, toolCallID, toolArgsPreview, toolResultPreview string) {
		payload := map[string]string{
			"type":       "status",
			"phase":      phase,
			"message":    message,
			"session_id": sessionID,
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
		sendSSE(payload)
	}
	return sendSSE, sendStatus, flusher
}

func statusPreview(value string, maxLen int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	normalized := strings.Join(strings.Fields(trimmed), " ")
	runes := []rune(normalized)
	if len(runes) <= maxLen {
		return normalized
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
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
	tooling chatToolingOptions,
	extraTools ...tool.Tool,
) http.Handler {
	maxIters := resolveAgentMaxIterations(maxIterations)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			SessionID string `json:"session_id"`
			Message   string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if strings.TrimSpace(req.Message) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
			return
		}
		endBusy := activity.beginChat()
		defer endBusy()
		logger.Debug().
			Str("path", r.URL.Path).
			Str("session_id", strings.TrimSpace(req.SessionID)).
			Int("message_len", len(strings.TrimSpace(req.Message))).
			Msg("chat request accepted")

		reqStore, requestWorkspaceDir, workspaceID, err := resolveSessionStoreForRequest(workspaceDir, store, r)
		if err != nil {
			logger.Error().Err(err).Msg("resolve workspace session store failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve workspace failed"})
			return
		}

		sessionID, err := resolveChatSession(reqStore, req.SessionID)
		if err != nil {
			if req.SessionID == "" {
				logger.Error().Err(err).Msg("create session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create session failed"})
				return
			}
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}

		transcriptPath := reqStore.TranscriptPath(sessionID)
		logger.Debug().Str("session_id", sessionID).Str("transcript_path", transcriptPath).Msg("chat session resolved")
		if err := maybeAutoCompactSession(requestWorkspaceDir, transcriptPath, sessionID, client, logger); err != nil {
			logger.Error().Err(err).Str("session_id", sessionID).Msg("auto compaction failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auto compaction failed"})
			return
		}

		history, err := loadSessionHistory(transcriptPath, chatHistoryMaxTokens)
		if err != nil {
			logger.Error().Err(err).Msg("load history failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load history failed"})
			return
		}
		extSnapshot := extensions.Snapshot{}
		if tooling.Extensions != nil {
			extSnapshot = tooling.Extensions.Snapshot()
		}
		registry := newBaseToolRegistryWithProcess(requestWorkspaceDir, tooling.ProcessManager)
		registry.Register(tool.NewSessionsListTool(reqStore))
		registry.Register(tool.NewSessionsHistoryTool(reqStore))
		registry.Register(tool.NewSessionsSendTool(tooling.Gateway))
		registry.Register(tool.NewSessionsSpawnTool(tooling.Gateway))
		registry.Register(tool.NewSessionsRunsTool(tooling.Gateway))
		registry.Register(tool.NewAgentsListTool(tooling.Gateway))
		if tooling.AutomationToolsForWorkspace != nil {
			for _, autoTool := range tooling.AutomationToolsForWorkspace(workspaceID) {
				registry.Register(autoTool)
			}
		}
		for _, extra := range extraTools {
			registry.Register(extra)
		}
		if tooling.Extensions != nil {
			for _, extra := range tooling.Extensions.ChatTools() {
				registry.Register(extra)
			}
		}
		registry.Register(tool.NewSessionStatusTool(func(_ context.Context) (tool.SessionStatus, error) {
			return tool.SessionStatus{
				SessionID:       sessionID,
				HistoryMessages: len(history) + 1,
			}, nil
		}))
		invokedSkill := resolveInvokedSkill(req.Message, tooling.Extensions)
		systemPrompt, toolChoice, _ := prepareChatContextWithExtensions(requestWorkspaceDir, req.Message, extSnapshot, invokedSkill)
		logger.Debug().
			Str("session_id", sessionID).
			Int("history_messages", len(history)).
			Int("system_prompt_len", len(systemPrompt)).
			Str("tool_choice", toolChoice).
			Msg("chat context assembled")

		// Append user message to transcript
		now := time.Now().UTC()
		userMsg := session.Message{Role: "user", Content: req.Message, Timestamp: now}
		if err := session.AppendMessage(transcriptPath, userMsg); err != nil {
			logger.Error().Err(err).Msg("append user message failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save message failed"})
			return
		}

		llmMessages := buildLLMMessages(systemPrompt, history, req.Message)

		injectedSchemas := registry.Schemas()
		injectedNames := toolNamesFromSchemas(injectedSchemas)
		logger.Debug().
			Str("session_id", sessionID).
			Int("tool_count_injected", len(injectedSchemas)).
			Strs("injected_tools", injectedNames).
			Msg("tool injection result")
		sendStatusSink := func(_, _, _, _, _, _ string) {}
		sendStatusProxy := func(phase, message, toolName, toolCallID, toolArgsPreview, toolResultPreview string) {
			sendStatusSink(phase, message, toolName, toolCallID, toolArgsPreview, toolResultPreview)
		}
		loop := setupAgentLoop(client, registry, sessionID, len(history), logger, sendStatusProxy)

		sendSSE, sendStatus, flusher := setupSSEWriter(w, sessionID, logger)
		_ = flusher
		sendStatusSink = sendStatus
		sendStatus("stream_open", "stream connected", "", "", "", "")
		deltaSent := false
		streamingAnnounced := false

		logger.Debug().Str("session_id", sessionID).Int("messages", len(llmMessages)).Msg("llm chat call start")
		chatResp, err := loop.Run(r.Context(), llmMessages, agent.RunOptions{
			MaxIterations: maxIters,
			Tools:         injectedSchemas,
			ToolChoice:    toolChoice,
			OnDelta: func(text string) {
				if text == "" {
					return
				}
				if !streamingAnnounced {
					streamingAnnounced = true
					sendStatus("llm_stream", "streaming response", "", "", "", "")
				}
				deltaSent = true
				logger.Debug().Str("session_id", sessionID).Int("delta_len", len(text)).Msg("llm delta")
				sendSSE(map[string]string{"type": "delta", "text": text})
			},
		})
		if err != nil {
			logger.Debug().Str("session_id", sessionID).Err(err).Msg("llm chat call failed")
			sendSSE(map[string]string{"type": "error", "error": err.Error()})
			return
		}
		logger.Debug().
			Str("session_id", sessionID).
			Int("assistant_len", len(chatResp.Message.Content)).
			Int("input_tokens", chatResp.Usage.InputTokens).
			Int("output_tokens", chatResp.Usage.OutputTokens).
			Str("stop_reason", chatResp.StopReason).
			Msg("llm chat call complete")
		if !deltaSent && chatResp.Message.Content != "" {
			logger.Debug().Str("session_id", sessionID).Int("assistant_len", len(chatResp.Message.Content)).Msg("emit fallback delta from non-streaming llm response")
			sendSSE(map[string]string{"type": "delta", "text": chatResp.Message.Content})
		}

		// Append assistant message to transcript
		assistantMsg := session.Message{Role: "assistant", Content: chatResp.Message.Content, Timestamp: time.Now().UTC()}
		if err := session.AppendMessage(transcriptPath, assistantMsg); err != nil {
			logger.Error().Err(err).Msg("append assistant message failed")
		} else if err := reqStore.Touch(sessionID, assistantMsg.Timestamp); err != nil {
			logger.Error().Err(err).Str("session_id", sessionID).Msg("touch session updated_at failed")
		}
		if err := writeChatMemory(requestWorkspaceDir, sessionID, req.Message, chatResp.Message.Content, assistantMsg.Timestamp); err != nil {
			logger.Error().Err(err).Str("session_id", sessionID).Msg("write chat memory failed")
		}

		// Send done event
		sendSSE(map[string]any{
			"type":       "done",
			"session_id": sessionID,
			"usage": map[string]int{
				"input_tokens":  chatResp.Usage.InputTokens,
				"output_tokens": chatResp.Usage.OutputTokens,
			},
		})
		logger.Debug().Str("session_id", sessionID).Msg("chat request complete")
	})
	return mux
}
