package tarsserver

import (
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/usage"
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
