package tarsserver

import (
	"context"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

func executeChatLoop(
	ctx context.Context,
	deps chatHandlerDeps,
	state chatRunState,
	stream *chatStreamWriter,
) (llm.ChatResponse, bool, []ToolCallRecord, error) {
	if state.llmResolution.Tier != "" {
		ctx = llm.WithSelectionMetadata(ctx, llm.SelectionMetadata{
			Role:      llm.RoleChatMain,
			Tier:      state.llmResolution.Tier,
			Provider:  state.llmResolution.Provider,
			Model:     state.llmResolution.Model,
			Source:    state.llmResolution.Source,
			SessionID: state.sessionID,
		})
	}
	streamingAnnounced := false
	deltaSent := false
	var accumulated strings.Builder
	chatClient := state.llmClient
	if chatClient == nil {
		var err error
		chatClient, _, err = deps.resolveChatClient()
		if err != nil {
			return llm.ChatResponse{}, false, nil, err
		}
	}
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{Source: "chat", SessionID: state.sessionID})
	afterToolHook := func(_ context.Context, evt agent.Event) {
		if evt.ToolName != "tasks" {
			return
		}
		// The console keeps the chat pulse-bar Tasks badge in sync via this
		// event; failing to read tasks is non-fatal — the panel falls back
		// to its own poll on toggle.
		tasks, err := state.store.GetTasks(state.sessionID)
		if err != nil {
			deps.logger.Debug().Err(err).Str("session_id", state.sessionID).Msg("tasks_changed: read failed")
			return
		}
		stream.tasksChanged(tasks)
	}
	loop, toolCallRecords := setupAgentLoop(chatClient, state.registry, state.sessionID, len(state.history), deps.tooling.UsageTracker, deps.logger, stream.status, afterToolHook)
	ctx = tool.WithCurrentSessionInfo(ctx, state.sessionID, state.sessionKind)
	ctx = tool.WithLineEmitter(ctx, stream)

	deps.logger.Debug().Str("session_id", state.sessionID).Int("messages", len(state.llmMessages)).Msg("llm chat call start")
	onTurnEnd := buildChatTurnEndHook(deps, state, stream)

	// Resume the upstream provider session (claude-code-cli only today) when
	// one was captured on a previous turn so we don't replay history.
	resumeID := ""
	if state.store != nil {
		if priorSess, lookupErr := state.store.Get(state.sessionID); lookupErr == nil {
			resumeID = strings.TrimSpace(priorSess.UpstreamSessionID)
		}
	}

	chatResp, err := loop.Run(ctx, state.llmMessages, agent.RunOptions{
		MaxIterations:        deps.maxIters,
		Tools:                state.injectedSchemas,
		BlockedTools:         state.blockedTools,
		ToolChoice:           state.toolChoice,
		OnTurnEnd:            onTurnEnd,
		ResumeSessionID:      resumeID,
		ClaudeCodeMCPServers: state.claudeCodeMCPServers,
		OnDelta: func(text string) {
			if text == "" {
				return
			}
			accumulated.WriteString(text)
			if !streamingAnnounced {
				streamingAnnounced = true
				stream.status("llm_stream", "streaming response", "", "", "", "")
			}
			deltaSent = true
			deps.logger.Debug().Str("session_id", state.sessionID).Int("delta_len", len(text)).Msg("llm delta")
			stream.delta(text)
		},
		OnReasoningDelta: func(text string) {
			if text == "" {
				return
			}
			if !streamingAnnounced {
				streamingAnnounced = true
				stream.status("llm_stream", "streaming response", "", "", "", "")
			}
			deps.logger.Debug().Str("session_id", state.sessionID).Int("delta_len", len(text)).Msg("llm reasoning delta")
			stream.reasoning(text)
		},
	})
	if err != nil {
		if ctx.Err() == context.Canceled {
			// Return partial content on cancellation
			partial := accumulated.String()
			deps.logger.Debug().Str("session_id", state.sessionID).Int("partial_len", len(partial)).Msg("chat cancelled, returning partial")
			return llm.ChatResponse{Message: llm.ChatMessage{Content: partial}}, deltaSent, *toolCallRecords, err
		}
		deps.logger.Debug().Str("session_id", state.sessionID).Err(err).Msg("llm chat call failed")
		return llm.ChatResponse{}, false, nil, err
	}
	if state.store != nil {
		if upstream := strings.TrimSpace(chatResp.SessionID); upstream != "" && upstream != resumeID {
			if persistErr := state.store.SetUpstreamSessionID(state.sessionID, upstream); persistErr != nil {
				// Non-fatal: the next turn will just start a fresh upstream
				// session instead of resuming. Log and continue.
				deps.logger.Debug().Str("session_id", state.sessionID).Str("upstream_session_id", upstream).Err(persistErr).Msg("persist upstream session id failed")
			}
		}
	}

	deps.logger.Debug().
		Str("session_id", state.sessionID).
		Int("assistant_len", len(chatResp.Message.Content)).
		Int("input_tokens", chatResp.Usage.InputTokens).
		Int("output_tokens", chatResp.Usage.OutputTokens).
		Str("stop_reason", chatResp.StopReason).
		Msg("llm chat call complete")

	return chatResp, deltaSent, *toolCallRecords, nil
}

func persistChatResult(state chatRunState, userMessage string, chatResp llm.ChatResponse, toolCalls []ToolCallRecord, logger zerolog.Logger) {
	now := time.Now().UTC()
	// Persist tool call messages before the assistant response
	for _, tc := range toolCalls {
		toolMsg := session.Message{
			Role:        "tool",
			Content:     tc.ToolResult,
			Timestamp:   now,
			ToolName:    tc.ToolName,
			ToolCallID:  tc.ToolCallID,
			ToolArgs:    tc.ToolArgs,
			ToolIsError: tc.ToolIsError,
		}
		if err := session.AppendMessage(state.transcriptPath, toolMsg); err != nil {
			logger.Error().Err(err).Str("tool", tc.ToolName).Msg("append tool message failed")
		}
	}
	assistantMsg := session.Message{Role: "assistant", Content: chatResp.Message.Content, Timestamp: now}
	if err := session.AppendMessage(state.transcriptPath, assistantMsg); err != nil {
		logger.Error().Err(err).Msg("append assistant message failed")
	} else if err := state.store.Touch(state.sessionID, assistantMsg.Timestamp); err != nil {
		logger.Error().Err(err).Str("session_id", state.sessionID).Msg("touch session updated_at failed")
	}
	if err := applyPostChatMemoryHooks(chatMemoryHookInput{
		WorkspaceDir:     state.requestWorkspaceDir,
		SessionID:        state.sessionID,
		UserMessage:      userMessage,
		AssistantMessage: chatResp.Message.Content,
		AssistantTime:    assistantMsg.Timestamp,
		LLMClient:        state.llmClient,
	}); err != nil {
		logger.Error().Err(err).Str("session_id", state.sessionID).Msg("write chat memory failed")
	}
}
