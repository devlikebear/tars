package tarsserver

import (
	"context"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func executeChatLoop(
	ctx context.Context,
	deps chatHandlerDeps,
	state chatRunState,
	stream *chatStreamWriter,
) (llm.ChatResponse, bool, []ToolCallRecord, error) {
	streamingAnnounced := false
	deltaSent := false
	var accumulated strings.Builder
	loop, toolCallRecords := setupAgentLoop(deps.client, state.registry, state.sessionID, len(state.history), deps.logger, stream.status)

	deps.logger.Debug().Str("session_id", state.sessionID).Int("messages", len(state.llmMessages)).Msg("llm chat call start")
	chatResp, err := loop.Run(ctx, state.llmMessages, agent.RunOptions{
		MaxIterations: deps.maxIters,
		Tools:         state.injectedSchemas,
		ToolChoice:    state.toolChoice,
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
			Role:       "tool",
			Content:    tc.ToolResult,
			Timestamp:  now,
			ToolName:   tc.ToolName,
			ToolCallID: tc.ToolCallID,
			ToolArgs:   tc.ToolArgs,
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
		ProjectID:        state.projectID,
		UserMessage:      userMessage,
		AssistantMessage: chatResp.Message.Content,
		AssistantTime:    assistantMsg.Timestamp,
		LLMClient:        state.llmClient,
	}); err != nil {
		logger.Error().Err(err).Str("session_id", state.sessionID).Msg("write chat memory failed")
	}
}
