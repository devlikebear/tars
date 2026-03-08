package tarsserver

import (
	"context"
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
	if err := applyPostChatMemoryHooks(chatMemoryHookInput{
		WorkspaceDir:     state.requestWorkspaceDir,
		SessionID:        state.sessionID,
		ProjectID:        state.projectID,
		UserMessage:      userMessage,
		AssistantMessage: chatResp.Message.Content,
		AssistantTime:    assistantMsg.Timestamp,
	}); err != nil {
		logger.Error().Err(err).Str("session_id", state.sessionID).Msg("write chat memory failed")
	}
}
