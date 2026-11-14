package apihandlers

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/checkpoint"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/httpapi"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// NewFocusReadsHandler serves what a pipeline screen reads besides the
// pipeline itself: its session, its transcript (or its Q&A session's), its
// checkpoints and one turn's diff. They sit under /v1/focus/pipelines/{id}/
// so a paired phone (role focus, docs/decisions/mobile-remote.md §4) loads
// the screen without reaching any /v1/admin route. The pipeline in the path
// is the scope: a session that has no pipeline is not found, and the only
// other session reachable is that pipeline's own Q&A session. Every route is
// a read.
func NewFocusReadsHandler(sessions *session.Store, checkpoints *checkpoint.Store, logger zerolog.Logger) http.Handler {
	h := &focusReads{sessions: sessions, checkpoints: checkpoints, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/focus/pipelines/{id}/session", h.session)
	mux.HandleFunc("GET /v1/focus/pipelines/{id}/history", h.history)
	mux.HandleFunc("GET /v1/focus/pipelines/{id}/checkpoints", h.listCheckpoints)
	mux.HandleFunc("GET /v1/focus/pipelines/{id}/checkpoints/{turn}/diff", h.diff)
	return mux
}

type focusReads struct {
	sessions    *session.Store
	checkpoints *checkpoint.Store
	logger      zerolog.Logger
}

// pipeline loads the pipeline named in the path and its session, answering
// 404 when there is none (or its session is gone).
func (h *focusReads) pipeline(w http.ResponseWriter, r *http.Request) (focuspipeline.Pipeline, session.Session, bool) {
	if h.sessions == nil {
		httpapi.WriteUnavailable(w, "sessions are not available")
		return focuspipeline.Pipeline{}, session.Session{}, false
	}
	store := focuspipeline.NewStore(filepath.Join(h.sessions.WorkspaceDir(), "sessions"))
	p, ok, err := store.Get(r.PathValue("id"))
	if err != nil && !errors.Is(err, focuspipeline.ErrInvalidSessionID) {
		h.logger.Error().Err(err).Msg("read focus pipeline failed")
		httpapi.WriteError(w, http.StatusInternalServerError, "", "failed to read focus pipeline")
		return focuspipeline.Pipeline{}, session.Session{}, false
	}
	var sess session.Session
	if ok {
		sess, err = h.sessions.Get(p.SessionID)
	}
	if !ok || err != nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "focus pipeline not found")
		return focuspipeline.Pipeline{}, session.Session{}, false
	}
	return p, sess, true
}

func (h *focusReads) session(w http.ResponseWriter, r *http.Request) {
	if _, sess, ok := h.pipeline(w, r); ok {
		httpapi.WriteJSON(w, http.StatusOK, sess)
	}
}

// history is the pipeline session's transcript, or with ?session=qa its Q&A
// session's (empty before the first question).
func (h *focusReads) history(w http.ResponseWriter, r *http.Request) {
	p, _, ok := h.pipeline(w, r)
	if !ok {
		return
	}
	id := p.SessionID
	switch which := strings.TrimSpace(r.URL.Query().Get("session")); which {
	case "":
	case "qa":
		id = strings.TrimSpace(p.QASessionID)
		if id == "" {
			httpapi.WriteJSON(w, http.StatusOK, []session.Message{})
			return
		}
		if _, err := h.sessions.Get(id); err != nil {
			httpapi.WriteJSON(w, http.StatusOK, []session.Message{})
			return
		}
	default:
		httpapi.WriteError(w, http.StatusBadRequest, "bad_request", "session must be empty or qa")
		return
	}
	messages, err := session.ReadMessages(h.sessions.TranscriptPath(id))
	if err != nil {
		h.logger.Error().Err(err).Str("session_id", id).Msg("read focus session history failed")
		httpapi.WriteError(w, http.StatusInternalServerError, "", "read session history failed")
		return
	}
	if messages == nil {
		messages = []session.Message{}
	}
	httpapi.WriteJSON(w, http.StatusOK, messages)
}

func (h *focusReads) listCheckpoints(w http.ResponseWriter, r *http.Request) {
	p, _, ok := h.pipeline(w, r)
	if !ok {
		return
	}
	turns := []checkpoint.Entry{}
	reverts := []checkpoint.RevertEntry{}
	if h.checkpoints != nil {
		list, err := h.checkpoints.List(p.SessionID)
		if err != nil {
			h.writeCheckpointError(w, err)
			return
		}
		back, err := h.checkpoints.Reverts(p.SessionID)
		if err != nil {
			h.writeCheckpointError(w, err)
			return
		}
		if list != nil {
			turns = list
		}
		if back != nil {
			reverts = back
		}
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"session_id": p.SessionID, "turns": turns, "reverts": reverts})
}

func (h *focusReads) diff(w http.ResponseWriter, r *http.Request) {
	p, _, ok := h.pipeline(w, r)
	if !ok {
		return
	}
	if h.checkpoints == nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "turn has no checkpoint")
		return
	}
	query := r.URL.Query()
	scope := checkpoint.Scope(strings.TrimSpace(query.Get("scope")))
	result, err := h.checkpoints.Diff(r.Context(), p.SessionID, r.PathValue("turn"), scope, strings.TrimSpace(query.Get("path")))
	if err != nil {
		h.writeCheckpointError(w, err)
		return
	}
	if result.Files == nil {
		result.Files = []checkpoint.FileDiff{}
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
}

func (h *focusReads) writeCheckpointError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, checkpoint.ErrNotFound):
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "turn has no checkpoint")
	case errors.Is(err, checkpoint.ErrSkipped):
		httpapi.WriteError(w, http.StatusConflict, "checkpoint_skipped", err.Error())
	case errors.Is(err, checkpoint.ErrInvalid):
		httpapi.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
	default:
		h.logger.Warn().Err(err).Msg("focus checkpoint read failed")
		httpapi.WriteError(w, http.StatusInternalServerError, "", "checkpoint read failed")
	}
}
