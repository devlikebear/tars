package tarsserver

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// Focus mode's server surface (docs/decisions/focus-mode.md, P1a).
//
// A pipeline is a sidecar of an ordinary chat session,
// <workspace>/sessions/<id>.pipeline.json:
//
//	POST /v1/focus/pipelines                      {goal, cwd, isolate, title?} → 201 {session_id, pipeline}
//	GET  /v1/focus/pipelines                      → [{session_id, title, goal, current, open_gate, needs_input, updated_at}]
//	GET  /v1/focus/pipelines/{id}                 → pipeline
//	POST /v1/focus/pipelines/{id}/gates/{gate}    {action, note?, edits?} → {pipeline, next_prompt}
//	POST /v1/focus/pipelines/{id}/cards/{card}    {state, decision?} → {pipeline, next_prompt}
//
// Every chat turn of a session with a pipeline gets the stage's guidance
// appended to the user message as a <focus-stage> block, and the reply's
// <focus-*> blocks advance the pipeline after the turn, reported on the
// chat stream as a "pipeline" event. The server never sends next_prompt by
// itself in P1a; the console sends it as the next turn.

const (
	focusStageOpen  = "<focus-stage>"
	focusStageClose = "</focus-stage>"
	// focusTitleRunes caps a session title made from the goal.
	focusTitleRunes = 60
)

// focusStoreFor is the pipeline store next to a session store's sessions.
func focusStoreFor(sessions *session.Store) *focuspipeline.Store {
	if sessions == nil {
		return nil
	}
	return focuspipeline.NewStore(filepath.Join(sessions.WorkspaceDir(), "sessions"))
}

// appendFocusGuidance puts the current stage's instructions after the user's
// message as one <focus-stage> block when the session has an active
// pipeline. Unlike <console-context> it has no size cap: the block format
// it quotes must arrive whole. Slash commands keep their arguments clean.
func appendFocusGuidance(message string, sessions *session.Store, sessionID string, logger zerolog.Logger) string {
	store := focusStoreFor(sessions)
	if store == nil || strings.HasPrefix(strings.TrimSpace(message), "/") {
		return message
	}
	p, ok, err := store.Get(sessionID)
	if err != nil {
		logger.Warn().Err(err).Str("session_id", sessionID).Msg("focus: read pipeline failed")
		return message
	}
	if !ok {
		return message
	}
	guidance := strings.TrimSpace(strings.ReplaceAll(focuspipeline.Guidance(p), focusStageClose, ""))
	if guidance == "" {
		return message
	}
	return strings.TrimRight(message, "\n") + "\n\n" + focusStageOpen + "\n" + guidance + "\n" + focusStageClose
}

// focusAfterTurn feeds a completed turn's reply to the session's pipeline.
// ok is false when the session has none (or it could not be updated).
func focusAfterTurn(sessions *session.Store, sessionID, transcriptPath, reply string, now time.Time, logger zerolog.Logger) (focuspipeline.Pipeline, string, bool) {
	store := focusStoreFor(sessions)
	if store == nil {
		return focuspipeline.Pipeline{}, "", false
	}
	var next string
	p, ok, err := store.Update(sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		updated, act, err := focuspipeline.Apply(p, focuspipeline.Event{
			Kind:   focuspipeline.EventTurnCompleted,
			Turn:   countUserTurns(transcriptPath),
			Blocks: focuspipeline.ParseBlocks(reply),
		}, now)
		if act.Kind == focuspipeline.ActionSendTurn {
			next = act.Prompt
		}
		return updated, err
	})
	if err != nil {
		logger.Warn().Err(err).Str("session_id", sessionID).Msg("focus: update pipeline after turn failed")
		return focuspipeline.Pipeline{}, "", false
	}
	return p, next, ok
}

// countUserTurns is the 1-based number of the transcript's latest turn.
func countUserTurns(transcriptPath string) int {
	messages, err := session.ReadMessages(transcriptPath)
	if err != nil {
		return 0
	}
	n := 0
	for _, m := range messages {
		if m.Role == "user" {
			n++
		}
	}
	return n
}

// focusPipelineCleanup is the session delete hook that removes the
// session's pipeline file. It runs under the session index lock and only
// removes one file.
func focusPipelineCleanup(sessions *session.Store, logger zerolog.Logger) func(string) {
	store := focusStoreFor(sessions)
	return func(sessionID string) {
		if err := store.Delete(sessionID); err != nil {
			logger.Warn().Err(err).Str("session_id", sessionID).Msg("focus: delete pipeline of deleted session failed")
		}
	}
}

// sweepOrphanFocusPipelines removes pipelines whose session is gone (deleted
// while the server was down, or before this cleanup existed).
func sweepOrphanFocusPipelines(sessions *session.Store, logger zerolog.Logger) int {
	store := focusStoreFor(sessions)
	if store == nil {
		return 0
	}
	list, err := store.List()
	if err != nil {
		logger.Warn().Err(err).Msg("focus: list pipelines for sweep failed")
		return 0
	}
	removed := 0
	for _, p := range list {
		if _, err := sessions.Get(p.SessionID); err == nil || !isSessionNotFound(err) {
			continue
		}
		if err := store.Delete(p.SessionID); err == nil {
			removed++
		}
	}
	return removed
}

// focusAPI serves /v1/focus/pipelines.
type focusAPI struct {
	sessions  *session.Store
	worktrees *chatWorktrees
	logger    zerolog.Logger
	now       func() time.Time
}

func newFocusPipelineHandler(sessions *session.Store, worktrees *chatWorktrees, logger zerolog.Logger) http.Handler {
	api := &focusAPI{sessions: sessions, worktrees: worktrees, logger: logger, now: time.Now}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/focus/pipelines", api.list)
	mux.HandleFunc("POST /v1/focus/pipelines", api.create)
	mux.HandleFunc("GET /v1/focus/pipelines/{id}", api.get)
	mux.HandleFunc("POST /v1/focus/pipelines/{id}/gates/{gate}", api.gate)
	mux.HandleFunc("POST /v1/focus/pipelines/{id}/cards/{card}", api.card)
	return mux
}

func (a *focusAPI) store() *focuspipeline.Store { return focusStoreFor(a.sessions) }

type focusCreateRequest struct {
	Goal    string `json:"goal"`
	Cwd     string `json:"cwd"`
	Isolate bool   `json:"isolate,omitempty"`
	Title   string `json:"title,omitempty"`
}

// create starts a session in a folder exactly as POST /v1/admin/sessions
// with a cwd does (same admin-only rule), then attaches a new pipeline.
func (a *focusAPI) create(w http.ResponseWriter, r *http.Request) {
	var req focusCreateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if serverauth.RoleFromRequest(r) != serverauth.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "starting a focus task in a folder needs the admin token"})
		return
	}
	goal := strings.TrimSpace(req.Goal)
	if goal == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "goal is required"})
		return
	}
	if a.worktrees == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sessions in folders are unavailable"})
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = focusTitleFromGoal(goal)
	}
	sess, err := a.worktrees.createIn(r.Context(), newSessionRequest{Title: title, Cwd: req.Cwd, Isolate: req.Isolate})
	if err != nil {
		writeJSON(w, folderErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	p := focuspipeline.New(sess.ID, goal, a.now())
	if err := a.store().Save(p); err != nil {
		// Never leave a focus session without its pipeline.
		a.worktrees.retire(context.WithoutCancel(r.Context()), sess)
		_ = a.sessions.Delete(sess.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save pipeline failed"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"session_id": sess.ID, "pipeline": p})
}

func focusTitleFromGoal(goal string) string {
	line := strings.TrimSpace(strings.SplitN(goal, "\n", 2)[0])
	if runes := []rune(line); len(runes) > focusTitleRunes {
		return strings.TrimSpace(string(runes[:focusTitleRunes-1])) + "…"
	}
	return line
}

type focusListItem struct {
	SessionID  string                `json:"session_id"`
	Title      string                `json:"title"`
	Goal       string                `json:"goal"`
	Current    focuspipeline.StageID `json:"current"`
	OpenGate   string                `json:"open_gate"`
	NeedsInput int                   `json:"needs_input"`
	UpdatedAt  time.Time             `json:"updated_at"`
}

func (a *focusAPI) list(w http.ResponseWriter, _ *http.Request) {
	pipelines, err := a.store().List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]focusListItem, 0, len(pipelines))
	for _, p := range pipelines {
		sess, err := a.sessions.Get(p.SessionID)
		if err != nil {
			continue // its session is gone; the sweep removes the file
		}
		out = append(out, focusListItem{
			SessionID:  p.SessionID,
			Title:      sess.Title,
			Goal:       p.Goal,
			Current:    p.Current,
			OpenGate:   p.OpenGate,
			NeedsInput: p.NeedsInput(),
			UpdatedAt:  p.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// load reads the pipeline of a live session, writing 404 otherwise.
func (a *focusAPI) load(w http.ResponseWriter, sessionID string) (focuspipeline.Pipeline, bool) {
	if _, err := a.sessions.Get(sessionID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pipeline not found"})
		return focuspipeline.Pipeline{}, false
	}
	p, ok, err := a.store().Get(sessionID)
	switch {
	case errors.Is(err, focuspipeline.ErrInvalidSessionID), err == nil && !ok:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pipeline not found"})
		return focuspipeline.Pipeline{}, false
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return focuspipeline.Pipeline{}, false
	}
	return p, true
}

func (a *focusAPI) get(w http.ResponseWriter, r *http.Request) {
	if p, ok := a.load(w, r.PathValue("id")); ok {
		writeJSON(w, http.StatusOK, p)
	}
}

type focusGateRequest struct {
	Action string              `json:"action"`
	Note   string              `json:"note,omitempty"`
	Edits  *focuspipeline.Plan `json:"edits,omitempty"`
}

// gate applies a gate action. Approving the plan also writes the session's
// tasks and an approved contract, in the same update, so the two never
// disagree.
func (a *focusAPI) gate(w http.ResponseWriter, r *http.Request) {
	id, gate := r.PathValue("id"), r.PathValue("gate")
	if _, ok := a.load(w, id); !ok {
		return
	}
	var req focusGateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	now := a.now()
	action := strings.TrimSpace(req.Action)
	var act focuspipeline.Action
	p, _, err := a.store().Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		next, result, err := focuspipeline.Apply(p, focuspipeline.Event{
			Kind:   focuspipeline.EventGate,
			Gate:   gate,
			Action: action,
			Edits:  req.Edits,
			Note:   req.Note,
		}, now)
		if err != nil {
			return p, err
		}
		if gate == focuspipeline.GatePlan && action == focuspipeline.GateApprove && next.Plan != nil {
			if err := a.sessions.SaveTasks(id, focuspipeline.SessionTasks(*next.Plan, now)); err != nil {
				return p, err
			}
		}
		act = result
		return next, nil
	})
	switch {
	case errors.Is(err, focuspipeline.ErrGateNotOpen):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "pipeline": p})
		return
	case errors.Is(err, focuspipeline.ErrInvalidAction), errors.Is(err, focuspipeline.ErrInvalidEdits):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "pipeline": p})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, focusActionResponse(p, act))
}

type focusCardRequest struct {
	State    string `json:"state"`
	Decision string `json:"decision,omitempty"`
}

func (a *focusAPI) card(w http.ResponseWriter, r *http.Request) {
	id, cardID := r.PathValue("id"), r.PathValue("card")
	if _, ok := a.load(w, id); !ok {
		return
	}
	var req focusCardRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	var act focuspipeline.Action
	p, _, err := a.store().Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		next, action, err := focuspipeline.SetCardState(p, cardID, strings.TrimSpace(req.State), req.Decision, a.now())
		act = action
		return next, err
	})
	switch {
	case errors.Is(err, focuspipeline.ErrCardNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	case errors.Is(err, focuspipeline.ErrCardDecided):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "pipeline": p})
		return
	case errors.Is(err, focuspipeline.ErrInvalidCardState):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, focusActionResponse(p, act))
}

func focusActionResponse(p focuspipeline.Pipeline, act focuspipeline.Action) map[string]any {
	next := ""
	if act.Kind == focuspipeline.ActionSendTurn {
		next = act.Prompt
	}
	return map[string]any{"pipeline": p, "next_prompt": next}
}
