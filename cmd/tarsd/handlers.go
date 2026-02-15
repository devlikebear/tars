package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tarsncase/internal/agent"
	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/heartbeat"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/prompt"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/devlikebear/tarsncase/internal/tool"
	"github.com/rs/zerolog"
)

func newHeartbeatAPIHandler(workspaceDir string, nowFn func() time.Time, ask heartbeat.AskFunc, logger zerolog.Logger) http.Handler {
	var mu sync.Mutex
	runHeartbeat := func(ctx context.Context) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return heartbeat.RunOnceWithLLMResult(callCtx, workspaceDir, nowFn(), ask)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/heartbeat/run-once", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		response, err := runHeartbeat(r.Context())
		if err != nil {
			logger.Error().Err(err).Msg("heartbeat run-once api failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"response": response})
	})

	return mux
}

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
	systemPrompt = prompt.Build(prompt.BuildOptions{WorkspaceDir: workspaceDir})
	systemPrompt += "\n" + strings.TrimSpace(memoryToolSystemRule) + "\n"
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

func setupAgentLoop(
	client llm.Client,
	registry *tool.Registry,
	sessionID string,
	historyLen int,
	logger zerolog.Logger,
	sendStatus func(string, string, string, string, string, string),
) *agent.Loop {
	registry.Register(tool.NewSessionStatusTool(func(_ context.Context) (tool.SessionStatus, error) {
		return tool.SessionStatus{
			SessionID:       sessionID,
			HistoryMessages: historyLen + 1,
		}, nil
	}))

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

func newChatAPIHandler(workspaceDir string, store *session.Store, client llm.Client, logger zerolog.Logger) http.Handler {
	return newChatAPIHandlerWithOptions(workspaceDir, store, client, logger, agent.DefaultMaxLoopIters)
}

func newChatAPIHandlerWithOptions(
	workspaceDir string,
	store *session.Store,
	client llm.Client,
	logger zerolog.Logger,
	maxIterations int,
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
		logger.Debug().
			Str("path", r.URL.Path).
			Str("session_id", strings.TrimSpace(req.SessionID)).
			Int("message_len", len(strings.TrimSpace(req.Message))).
			Msg("chat request accepted")

		sessionID, err := resolveChatSession(store, req.SessionID)
		if err != nil {
			if req.SessionID == "" {
				logger.Error().Err(err).Msg("create session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create session failed"})
				return
			}
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}

		transcriptPath := store.TranscriptPath(sessionID)
		logger.Debug().Str("session_id", sessionID).Str("transcript_path", transcriptPath).Msg("chat session resolved")
		if err := maybeAutoCompactSession(workspaceDir, transcriptPath, sessionID, client, logger); err != nil {
			logger.Error().Err(err).Str("session_id", sessionID).Msg("auto compaction failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auto compaction failed"})
			return
		}

		systemPrompt, toolChoice, _ := prepareChatContext(workspaceDir, req.Message)

		history, err := loadSessionHistory(transcriptPath, chatHistoryMaxTokens)
		if err != nil {
			logger.Error().Err(err).Msg("load history failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load history failed"})
			return
		}
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

		registry := newBaseToolRegistry(workspaceDir)
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
			Tools:         registry.Schemas(),
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
		}
		if err := writeChatMemory(workspaceDir, sessionID, req.Message, chatResp.Message.Content, assistantMsg.Timestamp); err != nil {
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

func newSessionAPIHandler(store *session.Store, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			sessions, err := store.List()
			if err != nil {
				logger.Error().Err(err).Msg("list sessions failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list sessions failed"})
				return
			}
			writeJSON(w, http.StatusOK, sessions)
		case http.MethodPost:
			var req struct {
				Title string `json:"title"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}
			title := strings.TrimSpace(req.Title)
			if title == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
				return
			}
			sess, err := store.Create(title)
			if err != nil {
				logger.Error().Err(err).Msg("create session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create session failed"})
				return
			}
			writeJSON(w, http.StatusOK, sess)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v1/sessions/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		sessions, err := store.List()
		if err != nil {
			logger.Error().Err(err).Msg("search sessions failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "search sessions failed"})
			return
		}

		query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		results := make([]session.Session, 0, len(sessions))
		for _, sess := range sessions {
			if strings.Contains(strings.ToLower(sess.Title), query) {
				results = append(results, sess)
			}
		}

		writeJSON(w, http.StatusOK, results)
	})

	mux.HandleFunc("/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		pathRemainder := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
		pathParts := strings.Split(pathRemainder, "/")
		sessionID := pathParts[0]
		if sessionID == "" {
			http.NotFound(w, r)
			return
		}

		switch {
		case len(pathParts) == 1:
			switch r.Method {
			case http.MethodGet:
				sess, err := store.Get(sessionID)
				if err != nil {
					if strings.Contains(err.Error(), "session not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
						return
					}
					logger.Error().Err(err).Str("session_id", sessionID).Msg("get session failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
					return
				}
				writeJSON(w, http.StatusOK, sess)
			case http.MethodDelete:
				if err := store.Delete(sessionID); err != nil {
					logger.Error().Err(err).Str("session_id", sessionID).Msg("delete session failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete session failed"})
					return
				}
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		case len(pathParts) == 2 && pathParts[1] == "history":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if _, err := store.Get(sessionID); err != nil {
				if strings.Contains(err.Error(), "session not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
					return
				}
				logger.Error().Err(err).Str("session_id", sessionID).Msg("get session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
				return
			}
			messages, err := session.ReadMessages(store.TranscriptPath(sessionID))
			if err != nil {
				logger.Error().Err(err).Str("session_id", sessionID).Msg("read session history failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read session history failed"})
				return
			}
			writeJSON(w, http.StatusOK, messages)
		case len(pathParts) == 2 && pathParts[1] == "export":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}

			sess, err := store.Get(sessionID)
			if err != nil {
				if strings.Contains(err.Error(), "session not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
					return
				}
				logger.Error().Err(err).Str("session_id", sessionID).Msg("get session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
				return
			}

			messages, err := session.ReadMessages(store.TranscriptPath(sessionID))
			if err != nil {
				logger.Error().Err(err).Str("session_id", sessionID).Msg("read session history failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read session history failed"})
				return
			}

			var b strings.Builder
			fmt.Fprintf(&b, "# Session: %s\n", sess.Title)
			fmt.Fprintf(&b, "Created: %s\n\n", sess.CreatedAt.Format(time.RFC3339))
			for _, msg := range messages {
				fmt.Fprintf(&b, "## %s\n", msg.Timestamp.Format(time.RFC3339))
				fmt.Fprintf(&b, "**%s**: %s\n\n", msg.Role, msg.Content)
			}

			w.Header().Set("Content-Type", "text/markdown")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, b.String())
		default:
			http.NotFound(w, r)
		}
	})

	return mux
}

func newStatusAPIHandler(workspaceDir string, store *session.Store, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		sessions, err := store.List()
		if err != nil {
			logger.Error().Err(err).Msg("list sessions failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"workspace_dir": workspaceDir,
			"session_count": len(sessions),
		})
	})
}

// Placeholder - actual implementation in Phase 1-G
func newCompactAPIHandler(workspaceDir string, store *session.Store, client llm.Client, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			SessionID        string `json:"session_id"`
			KeepRecent       int    `json:"keep_recent"`
			KeepRecentTokens int    `json:"keep_recent_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		sessionID := strings.TrimSpace(req.SessionID)
		if sessionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id is required"})
			return
		}

		if _, err := store.Get(sessionID); err != nil {
			if strings.Contains(err.Error(), "session not found") {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			logger.Error().Err(err).Str("session_id", sessionID).Msg("get session failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
			return
		}

		now := time.Now().UTC()
		result, err := compactWithMemoryFlush(workspaceDir, store.TranscriptPath(sessionID), sessionID, req.KeepRecent, req.KeepRecentTokens, client, now)
		if err != nil {
			logger.Error().Err(err).Str("session_id", sessionID).Msg("compact transcript failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "compact failed"})
			return
		}

		message := fmt.Sprintf(
			"compaction complete (session=%s compacted=%d final=%d)",
			sessionID,
			result.CompactedCount,
			result.FinalCount,
		)
		if !result.Compacted {
			message = fmt.Sprintf("compaction skipped (session=%s message_count=%d)", sessionID, result.OriginalCount)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"message":        message,
			"session_id":     sessionID,
			"compacted":      result.Compacted,
			"original_count": result.OriginalCount,
			"final_count":    result.FinalCount,
		})
	})
}

func newCronAPIHandler(
	store *cron.Store,
	runPrompt func(ctx context.Context, prompt string) (string, error),
	logger zerolog.Logger,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/cron/jobs", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			jobs, err := store.List()
			if err != nil {
				logger.Error().Err(err).Msg("list cron jobs failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list cron jobs failed"})
				return
			}
			writeJSON(w, http.StatusOK, jobs)
		case http.MethodPost:
			var req struct {
				Name           string `json:"name"`
				Prompt         string `json:"prompt"`
				Schedule       string `json:"schedule"`
				Enabled        *bool  `json:"enabled,omitempty"`
				DeleteAfterRun bool   `json:"delete_after_run,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}
			enabled := true
			hasEnable := false
			if req.Enabled != nil {
				enabled = *req.Enabled
				hasEnable = true
			}
			job, err := store.CreateWithOptions(cron.CreateInput{
				Name:           req.Name,
				Prompt:         req.Prompt,
				Schedule:       req.Schedule,
				Enabled:        enabled,
				HasEnable:      hasEnable,
				DeleteAfterRun: req.DeleteAfterRun,
			})
			if err != nil {
				if strings.Contains(err.Error(), "prompt is required") || strings.Contains(err.Error(), "invalid schedule") {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				logger.Error().Err(err).Msg("create cron job failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create cron job failed"})
				return
			}
			writeJSON(w, http.StatusOK, job)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v1/cron/jobs/", func(w http.ResponseWriter, r *http.Request) {
		pathRemainder := strings.TrimPrefix(r.URL.Path, "/v1/cron/jobs/")
		pathParts := strings.Split(pathRemainder, "/")
		if len(pathParts) < 1 || pathParts[0] == "" {
			http.NotFound(w, r)
			return
		}
		jobID := pathParts[0]
		if len(pathParts) == 1 {
			switch r.Method {
			case http.MethodPut:
				var req struct {
					Name           *string `json:"name,omitempty"`
					Prompt         *string `json:"prompt,omitempty"`
					Schedule       *string `json:"schedule,omitempty"`
					Enabled        *bool   `json:"enabled,omitempty"`
					DeleteAfterRun *bool   `json:"delete_after_run,omitempty"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
					return
				}
				job, err := store.Update(jobID, cron.UpdateInput{
					Name:           req.Name,
					Prompt:         req.Prompt,
					Schedule:       req.Schedule,
					Enabled:        req.Enabled,
					DeleteAfterRun: req.DeleteAfterRun,
				})
				if err != nil {
					switch {
					case strings.Contains(err.Error(), "job not found"):
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
					case strings.Contains(err.Error(), "required"), strings.Contains(err.Error(), "invalid schedule"):
						writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					default:
						logger.Error().Err(err).Str("job_id", jobID).Msg("update cron job failed")
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update cron job failed"})
					}
					return
				}
				writeJSON(w, http.StatusOK, job)
			case http.MethodDelete:
				if err := store.Delete(jobID); err != nil {
					if strings.Contains(err.Error(), "job not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
						return
					}
					logger.Error().Err(err).Str("job_id", jobID).Msg("delete cron job failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete cron job failed"})
					return
				}
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		if len(pathParts) != 2 || pathParts[1] != "run" {
			if len(pathParts) == 2 && pathParts[1] == "runs" {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				if _, err := store.Get(jobID); err != nil {
					if strings.Contains(err.Error(), "job not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
						return
					}
					logger.Error().Err(err).Str("job_id", jobID).Msg("get cron job failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get cron job failed"})
					return
				}
				limit := 50
				if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
					v, err := strconv.Atoi(raw)
					if err != nil || v <= 0 {
						writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
						return
					}
					limit = v
				}
				runs, err := store.ListRuns(jobID, limit)
				if err != nil {
					logger.Error().Err(err).Str("job_id", jobID).Msg("list cron runs failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list cron runs failed"})
					return
				}
				writeJSON(w, http.StatusOK, runs)
				return
			}
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runPrompt == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cron runner is not configured"})
			return
		}
		job, err := store.Get(jobID)
		if err != nil {
			if strings.Contains(err.Error(), "job not found") {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
				return
			}
			logger.Error().Err(err).Str("job_id", jobID).Msg("get cron job failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get cron job failed"})
			return
		}

		response, err := runPrompt(r.Context(), job.Prompt)
		_, _ = store.MarkRunResult(jobID, time.Now().UTC(), err)
		if err != nil {
			logger.Error().Err(err).Str("job_id", jobID).Msg("run cron job failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "run cron job failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"job_id":     job.ID,
			"response":   response,
			"job_name":   job.Name,
			"job_prompt": job.Prompt,
		})
	})

	return mux
}
