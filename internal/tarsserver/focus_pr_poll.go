package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

// The PR stages' poller (ADR §4–5, P4). While a pipeline wants PR facts —
// the PR or its merge awaited after a write turn, or pr_review running —
// one goroutine per session runs the read-only gh probe every
// focusPRPollInterval and applies the result through the pipeline store.
// It never probes while a turn holds the session (the write turn is still
// pushing), re-reads the pipeline before each probe, and ends once the
// pipeline wants no more (a gate opened, the stage moved on, stopped,
// finished) or at shutdown. Any driver action wakes it or starts it again.
//
// The console follows the pipeline by GET; changes that need the developer
// (new findings, G4, blocked, finished) are also announced on
// /v1/events/stream. When the pipeline finishes, the session worktree ends:
// discard when the probe saw the PR merged, keep otherwise.

// focusPRPollInterval is how often a PR stage is probed.
const focusPRPollInterval = 60 * time.Second

// focusNotifyCategory is the events-stream category of focus announcements.
const focusNotifyCategory = "focus"

type focusPoller struct {
	wake chan struct{}
	// again is set by watchPR under the driver's mu: the poller re-reads
	// the pipeline once more instead of ending.
	again bool
}

// bindPR connects the PR stages to the server: gh, the session worktrees
// and the events stream.
func (d *focusDriver) bindPR(tooling chatToolingOptions) {
	d.probe = probeFocusPR
	d.localBranch = gitLocalBranch
	d.discardCheck = gitDiscardCheck
	d.notify = tooling.Notify
	d.audit = auditTo(tooling.OpsManager)
	if c := tooling.Worktrees; c != nil {
		d.finishWorktree = func(ctx context.Context, sessionID, action string) error {
			_, err := c.finish(ctx, sessionID, action)
			return err
		}
	}
}

// watchPR starts the session's PR poller when its pipeline wants PR facts,
// or wakes the running one to probe now.
func (d *focusDriver) watchPR(sessionID string) {
	if d == nil || d.probe == nil || d.sessions == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	p, ok, err := focusStoreFor(d.sessions).Get(sessionID)
	if err != nil || !ok || !focuspipeline.WantsPRProbe(p) {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ctx.Err() != nil {
		return
	}
	if poller := d.pollers[sessionID]; poller != nil {
		poller.again = true
		select {
		case poller.wake <- struct{}{}:
		default:
		}
		return
	}
	poller := &focusPoller{wake: make(chan struct{}, 1)}
	d.pollers[sessionID] = poller
	d.wg.Add(1)
	go d.pollPR(sessionID, poller)
}

// resumePRPolls starts the pollers of pipelines a restart left waiting on
// PR facts. It returns how many it started.
func (d *focusDriver) resumePRPolls() int {
	if d == nil || d.sessions == nil {
		return 0
	}
	list, err := focusStoreFor(d.sessions).List()
	if err != nil {
		d.logger.Warn().Err(err).Msg("focus: list pipelines to resume PR polls")
		return 0
	}
	n := 0
	for _, p := range list {
		if focuspipeline.WantsPRProbe(p) {
			d.watchPR(p.SessionID)
			n++
		}
	}
	return n
}

func (d *focusDriver) pollPR(sessionID string, poller *focusPoller) {
	defer d.wg.Done()
	log := d.logger.With().Str("session_id", sessionID).Logger()
	for {
		if d.ctx.Err() != nil {
			d.endPoll(sessionID, poller, true)
			return
		}
		p, ok, err := focusStoreFor(d.sessions).Get(sessionID)
		if err != nil || !ok || !focuspipeline.WantsPRProbe(p) {
			if d.endPoll(sessionID, poller, false) {
				return
			}
			continue
		}
		if d.cancels.Running(sessionID) {
			// The write turn is still running: probe once it ends.
			if !d.sleep(poller, d.idlePoll) {
				d.endPoll(sessionID, poller, true)
				return
			}
			continue
		}
		dir, number := d.sessionDir(sessionID), 0
		if p.PR != nil {
			number = p.PR.Number // pinned: never the branch's latest PR
		}
		probe := d.probe(d.ctx, dir, number)
		if number == 0 && probe.Status == focuspipeline.ProbeFound && d.localBranch != nil {
			probe.LocalBranch = d.localBranch(d.ctx, dir)
		}
		if d.ctx.Err() != nil {
			d.endPoll(sessionID, poller, true)
			return
		}
		d.applyPRProbe(sessionID, probe, log)
		if !d.sleep(poller, d.prPoll) {
			d.endPoll(sessionID, poller, true)
			return
		}
	}
}

// endPoll removes the poller unless watchPR asked for another look since
// the last one; force ends it regardless (shutdown). It reports whether
// the poller ended.
func (d *focusDriver) endPoll(sessionID string, poller *focusPoller, force bool) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if poller.again && !force {
		poller.again = false
		return false
	}
	if d.pollers[sessionID] == poller {
		delete(d.pollers, sessionID)
	}
	return true
}

// sleep waits for wait, a wake-up, or shutdown (false).
func (d *focusDriver) sleep(poller *focusPoller, wait time.Duration) bool {
	select {
	case <-d.ctx.Done():
		return false
	case <-poller.wake:
		return true
	case <-time.After(wait):
		return true
	}
}

// sessionDir is the folder gh runs in: the session's working folder (its
// worktree when isolated).
func (d *focusDriver) sessionDir(sessionID string) string {
	sess, err := d.sessions.Get(sessionID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(sess.CurrentDir)
}

func (d *focusDriver) applyPRProbe(sessionID string, probe focuspipeline.PRProbe, log zerolog.Logger) (prev, updated focuspipeline.Pipeline, ok bool) {
	var act focuspipeline.Action
	updated, _, err := focusStoreFor(d.sessions).Update(sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		prev = p
		next, a, err := focuspipeline.Apply(p, focuspipeline.Event{Kind: focuspipeline.EventPRProbe, Probe: &probe}, d.now())
		act = a
		return next, err
	})
	if err != nil {
		log.Warn().Err(err).Msg("focus: apply PR probe")
		return prev, updated, false
	}
	// Outside the store's lock: announcing and finishing the worktree reach
	// the session store.
	d.announce(sessionID, prev, updated)
	d.pipelineFinished(sessionID, prev, updated)
	// Almost always ActionNone: a probe only ever sends a turn when it
	// finds the PR already merged and a template stage follows merge (a
	// release stage), which is a build turn like any other. Guarded on act
	// itself, not just left to d.start's own check: d.start unconditionally
	// calls d.watchPR first, which — called from inside this very poller's
	// goroutine while it still wants more probes — would push a wake into
	// its own channel and collapse the next d.sleep(poller, d.prPoll) to
	// zero, turning the 60s poll interval into a busy loop.
	if act.Kind != focuspipeline.ActionNone {
		d.start(sessionID, act, serverauth.RoleAdmin)
	}
	return prev, updated, true
}

// refreshMergeGate probes the pipeline's pinned PR right before G4's
// approval is applied (#1087): a push since the last poll closes G4, and
// the approval is refused instead of merging on another head's facts. It
// reports the pipeline after the probe and whether the head G4 shows
// changed: G4 closed for pr_review, or — with pr_review skipped — reopened
// on the new head, which the developer has not seen yet. Without gh, a PR
// number, or an open G4 nothing is probed.
func (d *focusDriver) refreshMergeGate(ctx context.Context, sessionID string) (focuspipeline.Pipeline, bool) {
	if d == nil || d.probe == nil || d.sessions == nil {
		return focuspipeline.Pipeline{}, false
	}
	p, found, err := focusStoreFor(d.sessions).Get(sessionID)
	if err != nil || !found || p.OpenGate != focuspipeline.GateMerge || p.PR == nil || p.PR.Number <= 0 {
		return p, false
	}
	probe := d.probe(ctx, d.sessionDir(sessionID), p.PR.Number)
	if ctx.Err() != nil {
		return p, false
	}
	log := d.logger.With().Str("session_id", sessionID).Logger()
	prev, updated, ok := d.applyPRProbe(sessionID, probe, log)
	if !ok {
		return p, false
	}
	head := focuspipeline.MergeGateHead(prev)
	moved := head != "" && updated.Active() && focuspipeline.MergeGateHead(updated) != head
	return updated, moved
}

// pipelineFinished ends the session worktree of a pipeline that just
// finished its merge stage, after the store's lock, and records how on the
// pipeline (the finished card says so). Discard only when the probe saw
// the PR merged, the worktree is clean, its HEAD is the merged head (or an
// ancestor of it), and no turn runs on the session; keep otherwise (a merge
// passed by hand, or work the merged PR does not hold). Pipelines whose
// plan leaves merge out keep their worktree untouched.
func (d *focusDriver) pipelineFinished(sessionID string, prev, next focuspipeline.Pipeline) {
	if d == nil || d.sessions == nil || focuspipeline.Finished(prev) || !focuspipeline.Finished(next) {
		return
	}
	if next.Plan == nil || !slices.Contains(next.Plan.Stages, focuspipeline.StageMerge) {
		return
	}
	end := d.endWorktree(sessionID, next)
	if _, _, err := focusStoreFor(d.sessions).Update(sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		return focuspipeline.RecordWorktreeEnd(p, end, d.now()), nil
	}); err != nil {
		d.logger.Warn().Err(err).Str("session_id", sessionID).Msg("focus: record how the worktree ended")
	}
}

// Worktree end actions recorded on the pipeline.
const (
	worktreeEndDiscard = "discard"
	worktreeEndKeep    = "keep"
	worktreeEndNone    = "none"
	worktreeEndLeft    = "left"
)

func (d *focusDriver) endWorktree(sessionID string, p focuspipeline.Pipeline) focuspipeline.WorktreeEnd {
	sess, err := d.sessions.Get(sessionID)
	if err != nil || sess.Worktree == nil || d.finishWorktree == nil {
		return focuspipeline.WorktreeEnd{Action: worktreeEndNone}
	}
	if d.cancels != nil && d.cancels.Running(sessionID) {
		return focuspipeline.WorktreeEnd{Action: worktreeEndLeft, Reason: "a turn was running on the session"}
	}
	end := focuspipeline.WorktreeEnd{Action: worktreeEndKeep, Reason: "the merge was passed by hand"}
	if p.PR != nil && p.PR.State == focuspipeline.PRStateMerged {
		end.Reason = ""
		check := d.discardCheck
		if check == nil {
			// Nothing can vouch for the worktree: keep it.
			check = func(context.Context, string, string) string { return "the worktree could not be checked" }
		}
		if reason := check(d.ctx, sess.Worktree.Dir, p.PR.HeadOID); reason != "" {
			end.Reason = reason
		} else {
			end.Action = worktreeEndDiscard
		}
	}
	if err := d.finishWorktree(d.ctx, sessionID, end.Action); err != nil {
		if errors.Is(err, errWorktreeMissing) {
			return focuspipeline.WorktreeEnd{Action: worktreeEndNone}
		}
		d.logger.Warn().Err(err).Str("session_id", sessionID).Str("action", end.Action).Msg("focus: finish the session worktree")
		return focuspipeline.WorktreeEnd{Action: worktreeEndLeft, Reason: err.Error()}
	}
	return end
}

// gitDiscardCheck says why the worktree in dir must not be discarded after
// its PR merged at head, or "" when discarding loses nothing: the tree is
// clean and every local commit is in the merged head.
func gitDiscardCheck(ctx context.Context, dir, head string) string {
	if strings.TrimSpace(head) == "" {
		return "the merged pull request's head commit is unknown"
	}
	if !isGitObjectID(head) {
		return "the merged pull request's head is not a commit id"
	}
	status, err := runGit(ctx, releaseGitTimeout, dir, nil, "status", "--porcelain")
	if err != nil {
		return "git status failed in the worktree"
	}
	if status != "" {
		return "the worktree has uncommitted changes"
	}
	local, err := runGit(ctx, releaseGitTimeout, dir, nil, "rev-parse", "HEAD")
	if err != nil {
		return "the worktree's HEAD is unreadable"
	}
	if local == head {
		return ""
	}
	if _, err := runGit(ctx, releaseGitTimeout, dir, nil, "cat-file", "-e", head+"^{commit}"); err != nil {
		// The PR head gained commits elsewhere (GitHub's "Update branch"):
		// nothing local to compare against, so the work may well be in it.
		return "the merged head is not in the local repository; fetch it to compare"
	}
	if _, err := runGit(ctx, releaseGitTimeout, dir, nil, "merge-base", "--is-ancestor", "HEAD", head); err == nil {
		return ""
	}
	return "the worktree has commits that are not in the merged pull request"
}

// gitLocalBranch is the branch checked out in dir, "" when detached or
// unreadable.
func gitLocalBranch(ctx context.Context, dir string) string {
	branch, err := runGit(ctx, releaseGitTimeout, dir, nil, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || branch == "HEAD" {
		return ""
	}
	return branch
}

// turnRunning reports whether a turn holds the session.
func (d *focusDriver) turnRunning(sessionID string) bool {
	return d != nil && d.cancels != nil && d.cancels.Running(sessionID)
}

// announce publishes the changes a probe made that need the developer.
func (d *focusDriver) announce(sessionID string, prev, next focuspipeline.Pipeline) {
	if d.notify == nil {
		return
	}
	title, severity := "", "info"
	switch {
	case focuspipeline.Finished(next) && !focuspipeline.Finished(prev):
		title = "Pipeline finished"
	case next.OpenGate != prev.OpenGate && next.OpenGate == focuspipeline.GateBlocked:
		title, severity = "Pipeline blocked", "warning"
	case next.OpenGate != prev.OpenGate && next.OpenGate == focuspipeline.GateMerge:
		title = "Ready to merge"
	case next.NeedsInput() > prev.NeedsInput():
		title = fmt.Sprintf("%d new pull request finding(s)", next.NeedsInput()-prev.NeedsInput())
	default:
		return
	}
	message := next.Goal
	if next.PR != nil && next.PR.Number > 0 {
		message = fmt.Sprintf("#%d %s", next.PR.Number, next.Goal)
	}
	evt := newNotificationEvent(focusNotifyCategory, severity, title, message)
	evt.SessionID = sessionID
	evt.OpenPath = "/console/focus/" + sessionID
	d.notify(d.ctx, evt)
}
