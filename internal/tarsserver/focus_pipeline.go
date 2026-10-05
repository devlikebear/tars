package tarsserver

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
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
//	POST /v1/focus/pipelines                      {goal, cwd, isolate, title?, kind?, kickoff?, release_items?, release_since?} → 201 {session_id, pipeline}; 409 {error, session_id} when kind is release and the repository's release is still running
//	GET  /v1/focus/pipelines                      → [{session_id, title, goal, current, open_gate, needs_input, updated_at}]
//	GET  /v1/focus/pipelines/{id}                 → pipeline
//	POST /v1/focus/pipelines/{id}/gates/{gate}    {action, note?, edits?, pr?, card_id?} → {pipeline, next_prompt}; 409 {error, pipeline} when the gate is not open, card_id is not the open gate's card, or approving merge finds the PR head moved (the probe it runs first closes G4)
//	POST /v1/focus/pipelines/{id}/cards/{card}    {state, decision?} → {pipeline, next_prompt}
//	POST /v1/focus/pipelines/{id}/advance         {stage} → {pipeline, next_prompt}; 409 unless stage is the active current one with no gate open
//	POST /v1/focus/pipelines/{id}/stop            → {pipeline, next_prompt: ""}; 409 when already finished or stopped
//	POST /v1/focus/pipelines/{id}/qa              {card_id, question} → 202 {qa_session_id, turn} (focus_qa.go)
//
// Every chat turn of a session with a pipeline gets the stage's guidance
// appended to the user message as a <focus-stage> block, and the reply's
// <focus-*> blocks advance the pipeline after the turn, reported on the
// chat stream as a "pipeline" event. From P2 the server carries out what
// the pipeline asks for next (focusDriver): it runs the plan's verification
// after a build turn and starts the next turn itself; next_prompt is only
// shown.

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

// focusTurnMark records the stage a turn's guidance was built for. Only a
// marked turn feeds the pipeline, and only while the pipeline is still at
// that stage and iteration: a reply that ends after the developer approved
// or passed the stage belongs to no stage that is open any more.
type focusTurnMark struct {
	Stage     focuspipeline.StageID
	Iteration int
	// Gate is the gate open when the guidance was built: a turn started at
	// a question gate completes as a question even after the gate closes.
	Gate string
}

// errFocusStaleTurn aborts an update for a turn whose stage has moved on.
var errFocusStaleTurn = errors.New("focus: turn belongs to an earlier stage")

// focusQuestionGateKey carries, on a server turn's context, the question
// gate its decision answers were given at (Action.QuestionGate).
type focusQuestionGateKey struct{}

func withFocusQuestionGate(ctx context.Context, gate string) context.Context {
	if gate == "" {
		return ctx
	}
	return context.WithValue(ctx, focusQuestionGateKey{}, gate)
}

func focusQuestionGateFrom(ctx context.Context) string {
	gate, _ := ctx.Value(focusQuestionGateKey{}).(string)
	return gate
}

// appendFocusGuidanceAt is appendFocusGuidance for a turn that is the
// developer's question at questionGate (decision answers given there):
// the turn gets that gate's guidance and is marked with it, even when the
// gate was decided before the turn started.
func appendFocusGuidanceAt(message string, sessions *session.Store, sessionID, questionGate string, logger zerolog.Logger) (string, *focusTurnMark) {
	store := focusStoreFor(sessions)
	if store == nil || strings.HasPrefix(strings.TrimSpace(message), "/") {
		return message, nil
	}
	p, ok, err := store.Get(sessionID)
	if err != nil {
		logger.Warn().Err(err).Str("session_id", sessionID).Msg("focus: read pipeline failed")
		return message, nil
	}
	if !ok {
		return message, nil
	}
	p = focusRecordBaseCommit(sessions, sessionID, p, logger)
	guidance := strings.TrimSpace(strings.ReplaceAll(focuspipeline.QuestionGuidance(p, questionGate), focusStageClose, ""))
	if guidance == "" {
		return message, nil
	}
	stage, _ := p.Stage(p.Current)
	mark := &focusTurnMark{Stage: stage.ID, Iteration: stage.Iteration, Gate: p.OpenGate}
	if questionGate != "" {
		mark.Gate = questionGate
	}
	return strings.TrimRight(message, "\n") + "\n\n" + focusStageOpen + "\n" + guidance + "\n" + focusStageClose, mark
}

// focusAfterTurn feeds a completed turn's reply to the session's pipeline
// and returns the action the pipeline asks for next. ok is false when the
// turn carried no mark, the pipeline has moved past the marked stage, or
// there is no pipeline (or it could not be updated).
func focusAfterTurn(sessions *session.Store, sessionID, transcriptPath, reply string, mark *focusTurnMark, now time.Time, logger zerolog.Logger) (focuspipeline.Pipeline, focuspipeline.Action, bool) {
	none := focuspipeline.Action{Kind: focuspipeline.ActionNone}
	store := focusStoreFor(sessions)
	if store == nil || mark == nil {
		return focuspipeline.Pipeline{}, none, false
	}
	turn := countUserTurns(transcriptPath)
	blocks := focusEnrichFindings(sessions, sessionID, focuspipeline.ParseBlocks(reply))
	next := none
	p, ok, err := store.Update(sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		if stage, _ := p.Stage(p.Current); stage.ID != mark.Stage || stage.Iteration != mark.Iteration {
			return p, errFocusStaleTurn
		}
		updated, act, err := focuspipeline.Apply(p, focuspipeline.Event{
			Kind:         focuspipeline.EventTurnCompleted,
			Turn:         turn,
			Blocks:       blocks,
			QuestionGate: mark.Gate,
		}, now)
		next = act
		return updated, err
	})
	if errors.Is(err, errFocusStaleTurn) {
		logger.Debug().Str("session_id", sessionID).Str("stage", string(mark.Stage)).Msg("focus: reply ignored, its stage has moved on")
		return focuspipeline.Pipeline{}, none, false
	}
	if err != nil {
		logger.Warn().Err(err).Str("session_id", sessionID).Msg("focus: update pipeline after turn failed")
		return focuspipeline.Pipeline{}, none, false
	}
	return p, next, ok
}

// focusNextPrompt is the prompt of a send_turn action, "" otherwise: shown
// to the developer, sent by the server.
func focusNextPrompt(act focuspipeline.Action) string {
	if act.Kind == focuspipeline.ActionSendTurn {
		return act.Prompt
	}
	return ""
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
		p, ok, _ := store.Get(sessionID)
		if err := store.Delete(sessionID); err != nil {
			logger.Warn().Err(err).Str("session_id", sessionID).Msg("focus: delete pipeline of deleted session failed")
		}
		if qa := strings.TrimSpace(p.QASessionID); ok && qa != "" {
			// The hook runs under the session index lock: delete the
			// pipeline's hidden Q&A session after it is released.
			go func() {
				if err := sessions.Delete(qa); err != nil && !isSessionNotFound(err) {
					logger.Warn().Err(err).Str("session_id", qa).Msg("focus: delete Q&A session of deleted session failed")
				}
			}()
		}
	}
}

// interruptFocusPipelines runs at startup, before any driver run: a
// pipeline the previous server left mid-step (verification awaited, a turn
// owed) gets the blocked "interrupted" gate instead of resuming silently,
// so the developer decides — retry resumes the cut-off step.
func interruptFocusPipelines(sessions *session.Store, now time.Time, logger zerolog.Logger) int {
	store := focusStoreFor(sessions)
	if store == nil {
		return 0
	}
	list, err := store.List()
	if err != nil {
		logger.Warn().Err(err).Msg("focus: list pipelines for interrupted runs failed")
		return 0
	}
	count := 0
	for _, listed := range list {
		interrupted := false
		_, _, err := store.Update(listed.SessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
			next, ok := focuspipeline.Interrupt(p, now)
			interrupted = ok
			return next, nil
		})
		if err != nil {
			logger.Warn().Err(err).Str("session_id", listed.SessionID).Msg("focus: mark interrupted pipeline failed")
			continue
		}
		if interrupted {
			count++
		}
	}
	return count
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
	// driver runs the next turn a gate, card or advance action asks for.
	driver *focusDriver
	logger zerolog.Logger
	now    func() time.Time
	// qaMu makes finding or creating a pipeline's Q&A session one step.
	qaMu sync.Mutex
}

func newFocusPipelineHandler(sessions *session.Store, worktrees *chatWorktrees, driver *focusDriver, logger zerolog.Logger) http.Handler {
	api := &focusAPI{sessions: sessions, worktrees: worktrees, driver: driver, logger: logger, now: time.Now}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/focus/pipelines", api.list)
	mux.HandleFunc("POST /v1/focus/pipelines", api.create)
	mux.HandleFunc("GET /v1/focus/pipelines/{id}", api.get)
	mux.HandleFunc("POST /v1/focus/pipelines/{id}/gates/{gate}", api.gate)
	mux.HandleFunc("POST /v1/focus/pipelines/{id}/cards/{card}", api.card)
	mux.HandleFunc("POST /v1/focus/pipelines/{id}/advance", api.advance)
	mux.HandleFunc("POST /v1/focus/pipelines/{id}/stop", api.stop)
	mux.HandleFunc("POST /v1/focus/pipelines/{id}/qa", api.qa)
	return mux
}

func (a *focusAPI) store() *focuspipeline.Store { return focusStoreFor(a.sessions) }

type focusCreateRequest struct {
	Goal    string `json:"goal"`
	Cwd     string `json:"cwd"`
	Isolate bool   `json:"isolate,omitempty"`
	Title   string `json:"title,omitempty"`
	// Kind is "" (feature work) or "release" (started from the release
	// train, which never lists release pipelines).
	Kind string `json:"kind,omitempty"`
	// Kickoff is the first turn when it says more than the goal; stage
	// guidance repeats only the goal.
	Kickoff string `json:"kickoff,omitempty"`
	// ReleaseItems and ReleaseSince (kind release only) are the release
	// train's list and cut-off the release ships.
	ReleaseItems []string   `json:"release_items,omitempty"`
	ReleaseSince *time.Time `json:"release_since,omitempty"`
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
	kind := strings.TrimSpace(req.Kind)
	if kind != "" && kind != focuspipeline.KindRelease {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown pipeline kind"})
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
	if kind == focuspipeline.KindRelease {
		// One release at a time per repository: a second Start release
		// (another tab, a double click) opens the running one instead.
		releaseCreateMu.Lock()
		defer releaseCreateMu.Unlock()
		active, err := activeReleaseIn(r.Context(), a.sessions, a.repoRoot(), expandCwdHome(strings.TrimSpace(req.Cwd)))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if active != "" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "a release is already running for this repository", "session_id": active})
			return
		}
	}
	sess, err := a.worktrees.createIn(r.Context(), newSessionRequest{Title: title, Cwd: req.Cwd, Isolate: req.Isolate})
	if err != nil {
		writeJSON(w, folderErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	p := focuspipeline.New(sess.ID, goal, a.now())
	p.Kind = kind
	p.Kickoff = strings.TrimSpace(req.Kickoff)
	if kind == focuspipeline.KindRelease {
		p.ReleaseItems = releaseItemsOf(req.ReleaseItems)
		if req.ReleaseSince != nil {
			since := req.ReleaseSince.UTC()
			p.ReleaseSince = &since
		}
	}
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
	// PR is G3's edited title and body.
	PR *focuspipeline.PRDraft `json:"pr,omitempty"`
	// CardID is the gate card the console showed: a gate replaced since
	// (G4 reopened on a new head) refuses the action with 409.
	CardID string `json:"card_id,omitempty"`
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
	action := strings.TrimSpace(req.Action)
	if gate == focuspipeline.GateMerge && action == focuspipeline.GateApprove {
		p, moved := a.driver.refreshMergeGate(r.Context(), id)
		if r.Context().Err() != nil {
			// The head was not checked: never approve unchecked.
			return
		}
		if moved {
			writeJSON(w, http.StatusConflict, map[string]any{"error": focuspipeline.ErrHeadMoved.Error(), "pipeline": p})
			return
		}
	}
	now := a.now()
	var act focuspipeline.Action
	p, _, err := a.store().Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		next, result, err := focuspipeline.Apply(p, focuspipeline.Event{
			Kind:   focuspipeline.EventGate,
			Gate:   gate,
			Action: action,
			Edits:  req.Edits,
			PR:     req.PR,
			Note:   req.Note,
			CardID: strings.TrimSpace(req.CardID),
		}, now)
		if err != nil {
			return p, err
		}
		act = result
		return next, nil
	})
	switch {
	case errors.Is(err, focuspipeline.ErrGateNotOpen), errors.Is(err, focuspipeline.ErrHeadMoved):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "pipeline": p})
		return
	case errors.Is(err, focuspipeline.ErrInvalidAction), errors.Is(err, focuspipeline.ErrInvalidEdits):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "pipeline": p})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp := focusActionResponse(p, act)
	if gate == focuspipeline.GatePlan && action == focuspipeline.GateApprove && p.Plan != nil {
		if warning, updated, failed := a.writePlanTasks(id, *p.Plan, now); failed {
			resp = focusActionResponse(updated, act)
			resp["warning"] = warning
		}
	}
	// After the tasks are written: the build turn the approval starts
	// reads them.
	a.driver.start(id, act, serverauth.RoleFromRequest(r))
	writeJSON(w, http.StatusOK, resp)
}

// focusNoticeTasksNotSaved titles the card raised when an approved plan's
// session tasks could not be written.
const focusNoticeTasksNotSaved = "plan approved, but session tasks were not saved"

// writePlanTasks writes the approved plan's session tasks and contract. It
// runs after the pipeline update committed and released the folder lock:
// saving tasks fires the session store's hooks, which take the session index
// lock, while a session delete holds that lock and calls into the pipeline
// store, so the two locks are never held together. On failure the approval
// stands and a notice card says the tasks are missing.
func (a *focusAPI) writePlanTasks(id string, plan focuspipeline.Plan, now time.Time) (string, focuspipeline.Pipeline, bool) {
	err := a.sessions.SaveTasks(id, focuspipeline.SessionTasks(plan, now))
	if err == nil {
		return "", focuspipeline.Pipeline{}, false
	}
	a.logger.Error().Err(err).Str("session_id", id).Msg("focus: save tasks of approved plan failed")
	updated, _, uerr := a.store().Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		return focuspipeline.AddNotice(p, focusNoticeTasksNotSaved, map[string]any{"errors": []string{err.Error()}}, a.now()), nil
	})
	if uerr != nil {
		a.logger.Error().Err(uerr).Str("session_id", id).Msg("focus: record tasks notice failed")
	}
	return focusNoticeTasksNotSaved + ": " + err.Error(), updated, true
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
	a.driver.start(id, act, serverauth.RoleFromRequest(r))
	writeJSON(w, http.StatusOK, focusActionResponse(p, act))
}

type focusAdvanceRequest struct {
	Stage focuspipeline.StageID `json:"stage"`
}

// advance passes the current stage by hand: the developer's call until a
// stage has fact-based completion (build before P2, PR stages without gh).
func (a *focusAPI) advance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := a.load(w, id); !ok {
		return
	}
	var req focusAdvanceRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	stage := focuspipeline.StageID(strings.TrimSpace(string(req.Stage)))
	if a.driver.turnRunning(id) {
		// A pass by hand never races a turn: passing merge ends the session
		// worktree, which must not move under a running turn. Refused even
		// when the pipeline cannot be re-read for the answer.
		resp := map[string]any{"error": "a turn is running on the session"}
		if p, ok, err := a.store().Get(id); err == nil && ok {
			resp["pipeline"] = p
		}
		writeJSON(w, http.StatusConflict, resp)
		return
	}
	a.applyEvent(w, r, id, focuspipeline.Event{Kind: focuspipeline.EventAdvance, Stage: stage}, focuspipeline.ErrCannotAdvance)
}

// stop stops the pipeline whether or not a gate is open.
func (a *focusAPI) stop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := a.load(w, id); !ok {
		return
	}
	// A stopped pipeline starts nothing more: end the driver's run (and
	// with it a turn it started) first.
	a.driver.cancel(id)
	a.applyEvent(w, r, id, focuspipeline.Event{Kind: focuspipeline.EventStop}, focuspipeline.ErrNotActive)
}

// applyEvent applies ev to a session's pipeline and writes the result; the
// conflict error answers 409 with the unchanged pipeline.
func (a *focusAPI) applyEvent(w http.ResponseWriter, r *http.Request, id string, ev focuspipeline.Event, conflict error) {
	var act focuspipeline.Action
	var prev focuspipeline.Pipeline
	p, _, err := a.store().Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		prev = p
		next, result, err := focuspipeline.Apply(p, ev, a.now())
		act = result
		return next, err
	})
	if err == nil {
		// A manual pass of the merge stage finishes the pipeline: its
		// worktree is kept (focus_pr_poll.go), after the store's lock.
		a.driver.pipelineFinished(id, prev, p)
	}
	switch {
	case errors.Is(err, conflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "pipeline": p})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.driver.start(id, act, serverauth.RoleFromRequest(r))
	writeJSON(w, http.StatusOK, focusActionResponse(p, act))
}

// focusActionResponse answers a gate, card or advance action. next_prompt
// is the turn the server has started for it, for display only: the console
// no longer sends it (P2).
func focusActionResponse(p focuspipeline.Pipeline, act focuspipeline.Action) map[string]any {
	next := ""
	if act.Kind == focuspipeline.ActionSendTurn {
		next = act.Prompt
	}
	return map[string]any{"pipeline": p, "next_prompt": next}
}
