package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
)

// focusTurnSource names server-started focus turns in the ops approval
// queue and the automation audit.
const focusTurnSource = "focus"

// runServerChatTurn runs one chat turn the server starts itself (the focus
// driver's next turn, a Q&A question) through the same path as a console
// request: prepared state, turn feed, cancel registry, worktree lease, chat
// activity, checkpoint, persistence and the post-turn focus hook. Nobody
// reads the response; consoles follow it on GET /v1/chat/stream. ctx bounds
// the turn (the server lifetime) and carries the role the turn runs as
// (serverauth.WithRoleContext).
func runServerChatTurn(ctx context.Context, deps chatHandlerDeps, sessionID, message, consoleContext string) (llm.ChatResponse, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		// An empty or stale id would start a new session.
		return llm.ChatResponse{}, errors.New("server turn needs a session id")
	}
	if deps.store == nil {
		return llm.ChatResponse{}, errors.New("server turn needs a session store")
	}
	if _, err := deps.store.Get(sessionID); err != nil {
		return llm.ChatResponse{}, fmt.Errorf("server turn: %w", err)
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "/v1/chat", http.NoBody)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	req := chatRequestPayload{SessionID: sessionID, Message: message, ConsoleContext: consoleContext}
	return runChatTurn(discardResponseWriter{header: http.Header{}}, r, req, deps, chatTurnOrigin{unattended: focusTurnSource})
}

// discardResponseWriter is the response of a turn nobody requested: its
// events still reach the turn feed.
type discardResponseWriter struct{ header http.Header }

func (d discardResponseWriter) Header() http.Header       { return d.header }
func (discardResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (discardResponseWriter) WriteHeader(int)             {}
