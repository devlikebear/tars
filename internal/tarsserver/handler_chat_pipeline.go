package tarsserver

import (
	"context"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

type chatHandlerDeps struct {
	workspaceDir   string
	store          *session.Store
	client         llm.Client
	logger         zerolog.Logger
	maxIters       int
	chatLimiter    *inflightLimiter
	activity       *runtimeActivity
	mainSessionID  string
	tooling        chatToolingOptions
	extraTools     []tool.Tool
	cancelRegistry *chatCancelRegistry
}

type chatAttachment struct {
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data"` // base64-encoded content
}

type chatRequestPayload struct {
	SessionID   string           `json:"session_id"`
	Message     string           `json:"message"`
	ProjectID   string           `json:"project_id,omitempty"`
	Attachments []chatAttachment `json:"attachments,omitempty"`
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

	req, ok := decodeChatRequestPayload(w, r)
	if !ok {
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
	if state.invokedSkill != nil {
		stream.skillSelected(state.invokedSkill.Name, state.invokedSkillReason)
	}

	// Emit context info for frontend monitoring
	stream.contextInfo(map[string]any{
		"system_prompt_tokens": promptTokenEstimate(state.llmMessages[0].Content),
		"history_tokens":       sumHistoryTokens(state.history),
		"history_messages":     len(state.history),
		"tool_count":           len(state.injectedSchemas),
		"tool_names":           toolNamesFromSchemas(state.injectedSchemas),
	})

	baseCtx := usage.WithCallMeta(r.Context(), usage.CallMeta{
		Source:    "chat",
		SessionID: state.sessionID,
		ProjectID: state.projectID,
	})
	chatCtx, cancelChat := context.WithCancel(baseCtx)
	defer cancelChat()
	if deps.cancelRegistry != nil {
		deps.cancelRegistry.Register(state.sessionID, cancelChat)
		defer deps.cancelRegistry.Unregister(state.sessionID)
	}

	chatResp, deltaSent, toolCalls, err := executeChatLoop(chatCtx, deps, state, stream)
	if err != nil {
		if chatCtx.Err() == context.Canceled {
			stream.cancelled()
			if chatResp.Message.Content != "" {
				persistChatResult(state, req.Message, chatResp, toolCalls, deps.logger)
			}
			deps.logger.Debug().Str("session_id", state.sessionID).Msg("chat request cancelled")
			return
		}
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

	persistChatResult(state, req.Message, chatResp, toolCalls, deps.logger)

	// Fire-and-forget: warm cache for next turn based on current user message
	startMemoryPrefetchForNextTurn(
		state.requestWorkspaceDir,
		req.Message,
		state.projectID,
		state.sessionID,
		deps.tooling.MemorySemanticConfig,
		deps.tooling.MemoryCache,
	)

	stream.done(chatResp.Usage)
	deps.logger.Debug().Str("session_id", state.sessionID).Msg("chat request complete")
}
