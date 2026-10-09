package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// Session retention: a session nobody has touched for a while is archived
// (hidden from the lists, nothing removed), and a session that has stayed
// archived and untouched for a while longer is deleted with everything kept
// beside it. Both steps are decided from two timestamps on the session, in
// Go — no model is asked which sessions matter.
//
// Without it nothing ever left: sixty sessions had none archived, and each
// one keeps a transcript, a task file, a pipeline, checkpoints, ledger
// records and often a worktree.

const (
	// sessionRetentionInterval is how often the sweep runs after the one at
	// startup. Both thresholds are in days, so nothing is gained by more.
	sessionRetentionInterval = 6 * time.Hour
	// sessionRetentionStartDelay keeps the first sweep out of startup.
	sessionRetentionStartDelay = 2 * time.Minute
	// sessionRetentionWarning is how long before a deletion the user is
	// told it is coming.
	sessionRetentionWarning = 3 * 24 * time.Hour
	// sessionRetentionSlack is how much later than its archiving a session
	// may have been updated and still count as untouched since: archiving
	// stamps both times itself.
	sessionRetentionSlack = time.Minute
)

// sessionRetentionPolicy holds the two ages; zero turns that step off.
type sessionRetentionPolicy struct {
	ArchiveAfter time.Duration
	DeleteAfter  time.Duration
}

func sessionRetentionPolicyFromDays(archiveDays, deleteDays int) sessionRetentionPolicy {
	day := 24 * time.Hour
	return sessionRetentionPolicy{
		ArchiveAfter: time.Duration(max(archiveDays, 0)) * day,
		DeleteAfter:  time.Duration(max(deleteDays, 0)) * day,
	}
}

func (p sessionRetentionPolicy) enabled() bool { return p.ArchiveAfter > 0 || p.DeleteAfter > 0 }

// sessionRetentionPlan is what one sweep would do.
type sessionRetentionPlan struct {
	Archive []session.Session
	Delete  []session.Session
	// Warn are archived sessions whose deletion is within
	// sessionRetentionWarning.
	Warn []session.Session
}

// planSessionRetention decides a sweep from the sessions alone.
//
//   - A pinned session, the main session (the always-on chat, by kind or by
//     the configured default id), and a session keep reports as still in
//     use are never touched.
//   - Archive: not archived, and not updated for ArchiveAfter.
//   - Delete: archived for DeleteAfter and not updated since it was
//     archived. A session used after it was archived is somebody's, whatever
//     its flag says, so it stays.
func planSessionRetention(sessions []session.Session, policy sessionRetentionPolicy, now time.Time, defaultID string, keep func(session.Session) bool) sessionRetentionPlan {
	var plan sessionRetentionPlan
	defaultID = strings.TrimSpace(defaultID)
	for _, sess := range sessions {
		if sess.PinnedAt != nil || (defaultID != "" && sess.ID == defaultID) || strings.EqualFold(strings.TrimSpace(sess.Kind), "main") {
			continue
		}
		if keep != nil && keep(sess) {
			continue
		}
		if sess.ArchivedAt == nil {
			if policy.ArchiveAfter > 0 && !sess.UpdatedAt.IsZero() && now.Sub(sess.UpdatedAt) >= policy.ArchiveAfter {
				plan.Archive = append(plan.Archive, sess)
			}
			continue
		}
		if policy.DeleteAfter <= 0 || sess.UpdatedAt.After(sess.ArchivedAt.Add(sessionRetentionSlack)) {
			continue
		}
		switch age := now.Sub(*sess.ArchivedAt); {
		case age >= policy.DeleteAfter:
			plan.Delete = append(plan.Delete, sess)
		case age >= policy.DeleteAfter-sessionRetentionWarning:
			plan.Warn = append(plan.Warn, sess)
		}
	}
	return plan
}

// sessionRetention runs the sweep.
type sessionRetention struct {
	store     *session.Store
	policy    sessionRetentionPolicy
	defaultID string
	// retire keeps a deleted session's worktree changes on its branch
	// (chatWorktrees.retire); nil when worktrees are not wired.
	retire func(context.Context, session.Session)
	notify func(context.Context, notificationEvent)
	audit  func(ops.AutomationAuditEntry)
	logger zerolog.Logger
	now    func() time.Time
	// statePath is where the time deletion was first switched on is kept
	// (firstDeleteAt); empty skips the grace period.
	statePath string

	mu     sync.Mutex
	warned map[string]bool // sessions already announced for deletion
}

// sessionRetentionResult is what one sweep did.
type sessionRetentionResult struct {
	Archived int
	Deleted  int
	Warned   int
}

// pipelineInUse reports a session whose focus pipeline is still being
// driven without a person: goal mode is on, or the server is waiting on its
// pull request. A pipeline waiting at a gate for a week is not in use.
func (r *sessionRetention) pipelineInUse(sess session.Session) bool {
	p, ok, err := focusStoreFor(r.store).Get(sess.ID)
	if err != nil || !ok || focuspipeline.Finished(p) {
		return false
	}
	return p.GoalActive() || p.PRWait != ""
}

func (r *sessionRetention) sweep(ctx context.Context) sessionRetentionResult {
	var result sessionRetentionResult
	if r == nil || r.store == nil || !r.policy.enabled() {
		return result
	}
	sessions, err := r.store.ListAll()
	if err != nil {
		r.logger.Warn().Err(err).Msg("session retention: list sessions failed")
		return result
	}
	now := time.Now()
	if r.now != nil {
		now = r.now()
	}
	plan := planSessionRetention(sessions, r.policy, now, r.defaultID, r.pipelineInUse)
	// Deletion that has only just been switched on (an upgrade, or the
	// setting) warns first: a session archived months ago is due at once,
	// and nobody has been told it would go.
	graceUntil := r.firstDeleteAt(now).Add(sessionRetentionWarning)
	if r.policy.DeleteAfter > 0 && now.Before(graceUntil) {
		plan.Warn = append(plan.Warn, plan.Delete...)
		plan.Delete = nil
	}

	for _, sess := range plan.Archive {
		if ctx.Err() != nil {
			return result
		}
		if _, err := r.store.SetArchived(sess.ID, true); err != nil {
			r.record(sess, "archive", "error", err.Error(), now)
			continue
		}
		result.Archived++
		r.record(sess, "archive", "ok", fmt.Sprintf("not updated since %s", sess.UpdatedAt.UTC().Format(time.RFC3339)), now)
	}
	for _, sess := range plan.Delete {
		if ctx.Err() != nil {
			return result
		}
		// The delete hooks take the transcript's neighbours: checkpoints,
		// the pipeline, the ledger records. The worktree is not theirs.
		if err := r.store.Delete(sess.ID); err != nil {
			r.record(sess, "delete", "error", err.Error(), now)
			continue
		}
		if r.retire != nil {
			r.retire(context.WithoutCancel(ctx), sess)
		}
		result.Deleted++
		r.record(sess, "delete", "ok", fmt.Sprintf("archived since %s", sess.ArchivedAt.UTC().Format(time.RFC3339)), now)
	}
	result.Warned = r.warn(ctx, plan.Warn, graceUntil)
	if result.Archived+result.Deleted > 0 {
		r.logger.Info().Int("archived", result.Archived).Int("deleted", result.Deleted).Msg("session retention sweep")
		r.announce(ctx, result)
	}
	return result
}

// warn announces, once per server run, the sessions about to be deleted.
func (r *sessionRetention) warn(ctx context.Context, sessions []session.Session, notBefore time.Time) int {
	r.mu.Lock()
	if r.warned == nil {
		r.warned = map[string]bool{}
	}
	var fresh []session.Session
	for _, sess := range sessions {
		if !r.warned[sess.ID] {
			r.warned[sess.ID] = true
			fresh = append(fresh, sess)
		}
	}
	r.mu.Unlock()
	if len(fresh) == 0 || r.notify == nil {
		return len(fresh)
	}
	first := fresh[0].ArchivedAt.Add(r.policy.DeleteAfter)
	titles := make([]string, 0, 3)
	for _, sess := range fresh {
		if at := sess.ArchivedAt.Add(r.policy.DeleteAfter); at.Before(first) {
			first = at
		}
		if len(titles) < 3 {
			titles = append(titles, sessionRetentionTitle(sess))
		}
	}
	if first.Before(notBefore) {
		first = notBefore
	}
	message := fmt.Sprintf("%d archived session(s) will be deleted from %s: %s", len(fresh), first.Local().Format("2006-01-02"), strings.Join(titles, ", "))
	if len(fresh) > len(titles) {
		message += ", …"
	}
	message += ". Unarchive or pin one to keep it."
	r.notify(ctx, newNotificationEvent("ops", "warning", "Archived sessions will be deleted", message))
	return len(fresh)
}

// firstDeleteAt is when this workspace first ran a sweep with deletion on,
// recorded the first time and read back after that. Without a state path,
// or with deletion off, it is the zero time: no grace period.
func (r *sessionRetention) firstDeleteAt(now time.Time) time.Time {
	if r.statePath == "" || r.policy.DeleteAfter <= 0 {
		return time.Time{}
	}
	var state struct {
		DeleteEnabledAt time.Time `json:"delete_enabled_at"`
	}
	if raw, err := os.ReadFile(r.statePath); err == nil && json.Unmarshal(raw, &state) == nil && !state.DeleteEnabledAt.IsZero() {
		return state.DeleteEnabledAt
	}
	state.DeleteEnabledAt = now.UTC()
	if raw, err := json.Marshal(state); err == nil {
		if err := os.MkdirAll(filepath.Dir(r.statePath), 0o755); err == nil {
			_ = os.WriteFile(r.statePath, raw, 0o600)
		}
	}
	return state.DeleteEnabledAt
}

func (r *sessionRetention) announce(ctx context.Context, result sessionRetentionResult) {
	if r.notify == nil {
		return
	}
	var parts []string
	if result.Archived > 0 {
		parts = append(parts, fmt.Sprintf("archived %d session(s) not used for %d days", result.Archived, int(r.policy.ArchiveAfter.Hours()/24)))
	}
	if result.Deleted > 0 {
		parts = append(parts, fmt.Sprintf("deleted %d session(s) archived for %d days", result.Deleted, int(r.policy.DeleteAfter.Hours()/24)))
	}
	r.notify(ctx, newNotificationEvent("ops", "info", "Session cleanup", strings.Join(parts, "; ")))
}

func (r *sessionRetention) record(sess session.Session, action, result, reason string, now time.Time) {
	if result != "ok" {
		r.logger.Warn().Str("session_id", sess.ID).Str("action", action).Str("error", reason).Msg("session retention step failed")
	}
	if r.audit == nil {
		return
	}
	r.audit(ops.AutomationAuditEntry{
		Timestamp: now.UTC(),
		Actor:     "session_retention",
		Action:    "session_retention",
		Reason:    reason,
		SessionID: sess.ID,
		Result:    result,
		Details:   map[string]any{"step": action, "title": sessionRetentionTitle(sess), "kind": sess.Kind},
	})
}

func sessionRetentionTitle(sess session.Session) string {
	if title := strings.TrimSpace(sess.Title); title != "" {
		return truncateRunes(title, 40)
	}
	return sess.ID
}

// start runs a sweep shortly after startup and then every
// sessionRetentionInterval, until ctx ends. wait returns once it has.
func (r *sessionRetention) start(ctx context.Context) (wait func()) {
	if r == nil || r.store == nil || !r.policy.enabled() {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(sessionRetentionStartDelay)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				r.sweep(ctx)
				timer.Reset(sessionRetentionInterval)
			}
		}
	}()
	return func() { <-done }
}
