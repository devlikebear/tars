package tarsserver

import (
	"errors"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/serverauth"
)

// Goal mode's server side (docs/decisions/focus-mode.md §4, "Goal mode").
//
// One watcher goroutine per driver looks at every pipeline in goal mode on
// a short tick. For a pipeline nothing is running on (no turn, no
// verification, no driver run) it asks focuspipeline.NextGoalStep what a
// person at the screen would have to do — approve a gate, decide a card,
// retry a blocked stage — and does it through the same functions the
// console's buttons call (focusApplyGate, focusDecideCard), as the admin
// role that turned goal mode on. A pipeline that sits idle with a step owed
// gets the interrupted gate (and then its retry); one with nothing owed is
// sent a turn. So a failed turn, a server restart, a dropped step or a
// reply without its block all end in the next step being taken, until the
// pipeline finishes or the push budget is spent.
//
// Watching instead of hooking every place a gate can open (a turn's end, a
// verification, a gh probe, a restart) keeps one path that cannot be
// missed, and a person's click and the watcher's decision meet in the
// pipeline store's lock: the loser gets "gate not open" and looks again.
//
// While goal mode is on the session's tool permission mode is auto, so the
// turns are not stopped by permission questions either; the mode the
// session had is put back when goal mode ends, unless it was changed
// meanwhile. POST /v1/chat/cancel (Stop) ends goal mode: it never pushes a
// turn the person stopped.

// focusGoalTick is how often the watcher looks at the pipelines in goal mode.
const focusGoalTick = 2 * time.Second

// focusGoalPermissionMode is the session permission mode goal mode runs in.
const focusGoalPermissionMode = chatPermissionModeAuto

// watchGoal adds a session to the goal watcher and starts the watcher on
// first use.
func (d *focusDriver) watchGoal(sessionID string) {
	if d == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ctx.Err() != nil {
		return
	}
	d.goals[sessionID] = true
	if d.goalWatching {
		return
	}
	d.goalWatching = true
	d.wg.Add(1)
	go d.goalLoop()
}

func (d *focusDriver) unwatchGoal(sessionID string) {
	d.mu.Lock()
	delete(d.goals, sessionID)
	d.mu.Unlock()
}

// resumeGoals watches the pipelines a restart left in goal mode. A step the
// restart cut off already waits at the interrupted gate
// (interruptFocusPipelines); the watcher retries it.
func (d *focusDriver) resumeGoals() int {
	if d == nil || d.sessions == nil {
		return 0
	}
	list, err := focusStoreFor(d.sessions).List()
	if err != nil {
		d.logger.Warn().Err(err).Msg("focus: list pipelines in goal mode failed")
		return 0
	}
	n := 0
	for _, p := range list {
		if p.GoalActive() {
			d.watchGoal(p.SessionID)
			n++
		}
	}
	return n
}

func (d *focusDriver) goalLoop() {
	defer d.wg.Done()
	ticker := time.NewTicker(d.goalTick)
	defer ticker.Stop()
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
		}
		d.mu.Lock()
		ids := make([]string, 0, len(d.goals))
		for id := range d.goals {
			ids = append(ids, id)
		}
		d.mu.Unlock()
		for _, id := range ids {
			if d.ctx.Err() != nil {
				return
			}
			d.goalStep(id)
		}
	}
}

// goalBusy reports whether something is running on the session, or a
// driver run is about to: goal mode decides only for an idle pipeline.
func (d *focusDriver) goalBusy(sessionID string) bool {
	if d.turnRunning(sessionID) {
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.runs[sessionID] != nil
}

// goalStep takes one goal-mode step for a session, if one is due.
func (d *focusDriver) goalStep(sessionID string) {
	store := focusStoreFor(d.sessions)
	if store == nil {
		return
	}
	p, ok, err := store.Get(sessionID)
	if err != nil {
		return
	}
	if !ok || !p.GoalActive() {
		d.unwatchGoal(sessionID)
		return
	}
	if d.goalBusy(sessionID) {
		return
	}
	step := focuspipeline.NextGoalStep(p, d.now())
	log := d.logger.With().Str("session_id", sessionID).Str("step", step.Kind).Logger()
	switch step.Kind {
	case focuspipeline.GoalWait:
		return
	case focuspipeline.GoalEnd:
		d.endGoal(sessionID, step.Reason)
		return
	case focuspipeline.GoalInterrupt:
		if _, _, err := store.Update(sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
			next, _ := focuspipeline.Interrupt(p, d.now())
			return next, nil
		}); err != nil {
			log.Warn().Err(err).Msg("focus goal: raise the interrupted gate")
		}
		return
	case focuspipeline.GoalGate:
		req := focusGateRequest{Action: step.Action, Note: step.Note, CardID: step.CardID}
		if _, _, _, err := focusApplyGate(d.ctx, d.sessions, d, d.logger, d.now, sessionID, step.Gate, req, serverauth.RoleAdmin); err != nil {
			// A person decided it first, or the PR head moved under G4: the
			// next tick sees the pipeline as it is now.
			log.Debug().Err(err).Str("gate", step.Gate).Msg("focus goal: gate not decided")
			return
		}
	case focuspipeline.GoalCard:
		req := focusCardRequest{State: focuspipeline.CardDecided, Decision: step.Decision}
		if _, _, err := focusDecideCard(d.sessions, d, d.now, sessionID, step.CardID, req, serverauth.RoleAdmin); err != nil {
			if !errors.Is(err, focuspipeline.ErrCardDecided) {
				log.Warn().Err(err).Str("card", step.CardID).Msg("focus goal: card not decided")
			}
			return
		}
	case focuspipeline.GoalNudge:
	default:
		return
	}
	if _, _, err := store.Update(sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		return focuspipeline.GoalApplied(p, step, d.now()), nil
	}); err != nil {
		log.Warn().Err(err).Msg("focus goal: record step")
		return
	}
	if step.Kind == focuspipeline.GoalNudge {
		d.start(sessionID, focuspipeline.Action{Kind: focuspipeline.ActionSendTurn, Prompt: step.Prompt}, serverauth.RoleAdmin)
	}
	log.Info().Str("gate", step.Gate).Str("action", step.Action).Str("card", step.CardID).Str("decision", step.Decision).Bool("push", step.Push).Msg("focus goal: decided")
	d.auditGoal(sessionID, "decided", map[string]any{
		"step": step.Kind, "gate": step.Gate, "action": step.Action, "card": step.CardID, "decision": step.Decision, "push": step.Push,
	})
}

// goalStarted puts the session of a pipeline that just entered goal mode in
// the auto permission mode (remembering what it had) and watches it. It
// returns the pipeline as stored.
func (d *focusDriver) goalStarted(sessionID string, p focuspipeline.Pipeline) focuspipeline.Pipeline {
	if d == nil || d.sessions == nil {
		return p
	}
	if p.GoalMode != nil && !p.GoalMode.PermissionSet {
		if sess, err := d.sessions.Get(sessionID); err == nil && sess.PermissionMode != focusGoalPermissionMode {
			if err := d.sessions.SetPermissionMode(sessionID, focusGoalPermissionMode); err != nil {
				d.logger.Warn().Err(err).Str("session_id", sessionID).Msg("focus goal: set the auto permission mode")
			} else if updated, _, err := focusStoreFor(d.sessions).Update(sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
				return focuspipeline.GoalPermission(p, sess.PermissionMode, d.now()), nil
			}); err == nil {
				p = updated
			}
		}
	}
	d.auditGoal(sessionID, "started", map[string]any{"max_pushes": p.GoalMode.MaxPushes})
	d.watchGoal(sessionID)
	return p
}

// endGoal ends a session's goal mode: the session gets back the permission
// mode it had (unless it was changed meanwhile), and an end the developer
// did not ask for is announced. ok is false when goal mode was not on.
func (d *focusDriver) endGoal(sessionID, reason string) (focuspipeline.Pipeline, bool) {
	if d == nil || d.sessions == nil || strings.TrimSpace(sessionID) == "" {
		return focuspipeline.Pipeline{}, false
	}
	store := focusStoreFor(d.sessions)
	// Restore the permission mode before the Update below ever makes
	// GoalActive() false: Update's own store lock is folder-wide (its own
	// doc comment: fn must never call back into the session store), so the
	// restore cannot happen inside the Update closure — it has to run
	// around it instead. Doing it first, not after, closes the window a
	// concurrent reader (a poll, a console refresh) could catch between
	// "goal mode just ended" and "the permission mode is back to what it
	// was": TestFocusGoalRunsToTheEnd caught exactly that window flaking
	// under CI's slower, coverage-instrumented run.
	if before, ok, err := store.Get(sessionID); err == nil && ok && before.GoalMode != nil && before.GoalMode.PermissionSet {
		if sess, err := d.sessions.Get(sessionID); err == nil && sess.PermissionMode == focusGoalPermissionMode {
			if err := d.sessions.SetPermissionMode(sessionID, before.GoalMode.RestorePermission); err != nil {
				d.logger.Warn().Err(err).Str("session_id", sessionID).Msg("focus goal: restore the permission mode")
			}
		}
	}
	ended := false
	p, found, err := store.Update(sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		next, ok := focuspipeline.EndGoal(p, reason, d.now())
		ended = ok
		return next, nil
	})
	d.unwatchGoal(sessionID)
	if err != nil || !found || !ended {
		return p, false
	}
	d.logger.Info().Str("session_id", sessionID).Str("reason", reason).Msg("focus goal: ended")
	d.auditGoal(sessionID, "ended", map[string]any{"reason": reason, "pushes": p.GoalMode.Pushes})
	d.announceGoalEnd(sessionID, p, reason)
	return p, true
}

func (d *focusDriver) announceGoalEnd(sessionID string, p focuspipeline.Pipeline, reason string) {
	if d.notify == nil {
		return
	}
	title, severity := "", "info"
	switch reason {
	case focuspipeline.GoalEndFinished:
		title = "Goal reached"
	case focuspipeline.GoalEndExhausted, focuspipeline.GoalEndPRClosed:
		title, severity = "Goal mode stopped", "warning"
	default:
		return // the developer's own stop needs no announcement
	}
	evt := newNotificationEvent(focusNotifyCategory, severity, title, p.Goal)
	evt.SessionID = sessionID
	evt.OpenPath = "/console/focus/" + sessionID
	d.notify(d.ctx, evt)
}

// auditGoal records a goal-mode event in the ops automation audit: goal
// mode decides in the developer's place, so every decision leaves a trace.
func (d *focusDriver) auditGoal(sessionID, result string, details map[string]any) {
	if d.audit == nil {
		return
	}
	d.audit(ops.AutomationAuditEntry{
		Actor: "focus_goal", Action: "focus_goal_mode", SessionID: sessionID, Result: result, Details: details,
	})
}
