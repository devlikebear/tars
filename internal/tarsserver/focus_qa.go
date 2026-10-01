package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
)

// Focus mode's Q&A side channel (docs/decisions/focus-mode.md §8):
//
//	POST /v1/focus/pipelines/{id}/qa  {card_id, question} → 202 {qa_session_id, turn}
//
// Each pipeline has one hidden worker session, in plan mode (read-only),
// working in the pipeline session's folder. A question runs as a turn there
// with the card and the slice of the transcript it came from as console
// context; the console reads the answer on GET /v1/chat/stream and the
// session history. Q&A never touches the pipeline beyond remembering the
// session and which Q&A turns were about which card, and it never enters
// the main agent's context unless the developer promotes an answer.

// focusQASource names Q&A turns in the ops approval queue and the audit.
const focusQASource = "focus_qa"

// focusQACardMarker starts the console-context line naming the card a
// question is about (the console's lib/focus.ts qaThreads reads it).
const focusQACardMarker = "focus-card: "

// Console context budget of a question: the card's payload and the source
// excerpt share the 2000-byte console context.
const (
	focusQAPayloadBytes = 700
	focusQAExcerptBytes = 900
)

var errFocusQABusy = errors.New("a question about this pipeline is still being answered")

type focusQARequest struct {
	CardID   string `json:"card_id"`
	Question string `json:"question"`
}

func (a *focusAPI) qa(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, ok := a.load(w, id)
	if !ok {
		return
	}
	var req focusQARequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	question := strings.TrimSpace(req.Question)
	if question == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "question is required"})
		return
	}
	card, found := focusCard(p, strings.TrimSpace(req.CardID))
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": focuspipeline.ErrCardNotFound.Error()})
		return
	}
	if !a.driver.canAsk() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "focus Q&A is unavailable"})
		return
	}
	parent, err := a.sessions.Get(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pipeline not found"})
		return
	}
	qaID, err := a.qaSession(id, parent)
	if err != nil {
		a.logger.Error().Err(err).Str("session_id", id).Msg("focus: prepare Q&A session failed")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "prepare Q&A session failed"})
		return
	}
	release, err := a.driver.reserveQA(qaID)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "qa_session_id": qaID})
		return
	}
	turn := countUserTurns(a.sessions.TranscriptPath(qaID)) + 1
	if _, _, err := a.store().Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		return focuspipeline.RecordQATurn(p, qaID, card.ID, turn, a.now())
	}); err != nil {
		release()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	context := focusQAContext(card, focusCardExcerpt(a.sessions.TranscriptPath(id), card.Turn))
	a.driver.ask(qaID, question, context, serverauth.RoleFromRequest(r), release)
	writeJSON(w, http.StatusAccepted, map[string]any{"qa_session_id": qaID, "turn": turn})
}

func focusCard(p focuspipeline.Pipeline, cardID string) (focuspipeline.Card, bool) {
	for _, c := range p.Cards {
		if c.ID == cardID {
			return c, true
		}
	}
	return focuspipeline.Card{}, false
}

// qaSession returns the pipeline's Q&A session, creating it on first use,
// and points it at the folder the pipeline session works in now (it may
// have moved into a worktree since the last question). Finding or making
// the session and recording its id is one locked step, so two first
// questions at once share one session instead of orphaning one.
func (a *focusAPI) qaSession(pipelineID string, parent session.Session) (string, error) {
	a.qaMu.Lock()
	defer a.qaMu.Unlock()
	p, _, err := a.store().Get(pipelineID)
	if err != nil {
		return "", err
	}
	qaID := strings.TrimSpace(p.QASessionID)
	if qaID != "" {
		if _, err := a.sessions.Get(qaID); err != nil {
			qaID = ""
		}
	}
	if qaID == "" {
		created, err := a.sessions.CreateWithOptions(focusQATitle(parent.Title), "worker", true)
		if err != nil {
			return "", err
		}
		qaID = created.ID
		// Never isolated and never holding the repository: a read-only
		// question must not push the pipeline's own turns into a worktree.
		if err := a.sessions.SetIsolation(qaID, session.IsolationOff); err != nil {
			return "", err
		}
		// Recorded at once (no session store call inside the update).
		if _, _, err := a.store().Update(pipelineID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
			p.QASessionID = qaID
			return p, nil
		}); err != nil {
			_ = a.sessions.Delete(qaID)
			return "", err
		}
	}
	if err := a.sessions.SetPermissionMode(qaID, chatPermissionModePlan); err != nil {
		return "", err
	}
	if cwd := strings.TrimSpace(parent.CurrentDir); cwd != "" {
		dirs := append([]string(nil), parent.WorkDirs...)
		if !slices.Contains(dirs, cwd) {
			dirs = append(dirs, cwd)
		}
		if err := a.sessions.SetWorkDirs(qaID, dirs, cwd); err != nil {
			return "", err
		}
	}
	return qaID, nil
}

func focusQATitle(parent string) string {
	parent = strings.TrimSpace(parent)
	if parent == "" {
		return "Focus Q&A"
	}
	return "Q&A · " + parent
}

// focusQAContext is the console context of a question: which card it is
// about, the card's payload, and the reply it came from.
func focusQAContext(card focuspipeline.Card, excerpt string) string {
	var b strings.Builder
	// The first line names the card for the console, which threads the Q&A
	// transcript by it (turn numbers shift when the transcript compacts).
	fmt.Fprintf(&b, "%s%s\n", focusQACardMarker, card.ID)
	b.WriteString("The developer is asking about one focus-mode card. Answer the question; read code if you need to, but do not edit anything.\n")
	fmt.Fprintf(&b, "Card %s (%s, stage %s, turn %d): %s\n", card.ID, card.Kind, card.Stage, card.Turn, card.Title)
	if payload := strings.TrimSpace(string(card.Payload)); payload != "" && payload != "null" {
		fmt.Fprintf(&b, "Card payload: %s\n", clipBytes(payload, focusQAPayloadBytes))
	}
	if excerpt = strings.TrimSpace(excerpt); excerpt != "" {
		fmt.Fprintf(&b, "Source reply (excerpt):\n%s\n", excerpt)
	}
	return b.String()
}

// focusCardExcerpt is the end of the assistant reply of transcript turn n
// (1-based user turns), without its focus blocks: the part of a long reply
// a card summarises usually sits just before the block.
func focusCardExcerpt(transcriptPath string, n int) string {
	if n <= 0 {
		return ""
	}
	messages, err := session.ReadMessages(transcriptPath)
	if err != nil {
		return ""
	}
	turn := 0
	reply := ""
	for _, m := range messages {
		switch {
		case m.Role == "user":
			turn++
		case m.Role == "assistant" && turn == n && strings.TrimSpace(m.Content) != "":
			reply = m.Content
		}
	}
	reply = strings.TrimSpace(focuspipeline.StripBlocks(reply))
	if len(reply) <= focusQAExcerptBytes {
		return reply
	}
	tail := reply[len(reply)-focusQAExcerptBytes:]
	// Start on a whole rune.
	for len(tail) > 0 && (tail[0]&0xC0) == 0x80 {
		tail = tail[1:]
	}
	return "…" + tail
}

// canAsk reports whether the driver can run Q&A turns.
func (d *focusDriver) canAsk() bool { return d != nil && d.qaTurn != nil }

// reserveQA marks a Q&A session busy until the returned release runs, so
// two questions never run as overlapping turns of one session.
func (d *focusDriver) reserveQA(qaID string) (func(), error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.qaBusy[qaID] || d.cancels.Running(qaID) {
		return nil, errFocusQABusy
	}
	d.qaBusy[qaID] = true
	return func() {
		d.mu.Lock()
		delete(d.qaBusy, qaID)
		d.mu.Unlock()
	}, nil
}

// ask runs one Q&A turn in the background, bound to the server's lifetime.
func (d *focusDriver) ask(qaID, question, consoleContext, role string, release func()) {
	d.mu.Lock()
	if d.ctx.Err() != nil {
		d.mu.Unlock()
		release()
		return
	}
	d.wg.Add(1)
	d.mu.Unlock()
	go func() {
		defer d.wg.Done()
		defer release()
		ctx := serverauth.WithRoleContext(d.ctx, role)
		if err := d.qaTurn(ctx, qaID, question, consoleContext); err != nil {
			d.logger.Info().Err(err).Str("session_id", qaID).Msg("focus: Q&A turn ended without an answer")
		}
	}()
}

// focusQATurnRunner runs one Q&A turn: read-only, never taking the
// repository lease.
type focusQATurnRunner func(ctx context.Context, qaSessionID, question, consoleContext string) error
