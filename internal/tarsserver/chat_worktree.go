package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/sessionoverride"
	"github.com/devlikebear/tars/internal/sessionworktree"
)

// Hybrid worktrees (#971, docs/decisions/console-workbench.md §4).
//
// A foreground session works in its current folder and takes the write lease
// for that folder's repository. A second session that starts a turn in the
// same repository while the lease is held gets a worktree of its own, and so
// does an unattended (cron) run, so no two sessions edit one checkout at once.
// A person can also isolate a session by hand, and can turn automatic
// worktrees off for it. An isolated session keeps its worktree until the
// person applies, keeps or discards it.

// repoLeaseGrace keeps the lease with the session that last worked in a
// repository for a while after its turn, so the next session to start a turn
// there right away does not interleave with it.
const repoLeaseGrace = 15 * time.Minute

type repoLease struct {
	sessionID string
	running   int
	until     time.Time
}

// repoLeases tracks which session holds each repository's write lease.
type repoLeases struct {
	mu      sync.Mutex
	holders map[string]*repoLease
	grace   time.Duration
	now     func() time.Time
}

func newRepoLeases() *repoLeases {
	return &repoLeases{holders: map[string]*repoLease{}, grace: repoLeaseGrace, now: time.Now}
}

// acquire takes the lease on repo for sessionID for the length of a turn.
// It fails, naming the holder, while another session holds it.
func (l *repoLeases) acquire(repo, sessionID string) (holder string, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	lease := l.holders[repo]
	if lease != nil && lease.sessionID != sessionID && (lease.running > 0 || l.now().Before(lease.until)) {
		return lease.sessionID, false
	}
	if lease == nil || lease.sessionID != sessionID {
		lease = &repoLease{sessionID: sessionID}
		l.holders[repo] = lease
	}
	lease.running++
	return sessionID, true
}

// finish ends a turn that acquire started; the lease stays with the
// session for the grace period.
func (l *repoLeases) finish(repo, sessionID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	lease := l.holders[repo]
	if lease == nil || lease.sessionID != sessionID {
		return
	}
	if lease.running > 0 {
		lease.running--
	}
	lease.until = l.now().Add(l.grace)
}

// holder returns the session holding repo's lease, or "".
func (l *repoLeases) holder(repo string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	lease := l.holders[repo]
	if lease == nil || (lease.running == 0 && !l.now().Before(lease.until)) {
		return ""
	}
	return lease.sessionID
}

// release gives up whatever lease sessionID holds, as when it moves into a
// worktree or is deleted.
func (l *repoLeases) release(sessionID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for repo, lease := range l.holders {
		if lease.sessionID == sessionID {
			delete(l.holders, repo)
		}
	}
}

// chatWorktrees ties sessions, their worktrees and the repository leases.
type chatWorktrees struct {
	manager   *sessionworktree.Manager
	store     *session.Store
	leases    *repoLeases
	overrides *sessionoverride.Service
	audit     func(ops.AutomationAuditEntry)
	// running reports whether a turn is in progress in a session; worktree
	// changes wait for it to finish.
	running func(sessionID string) bool
	// mu serializes worktree changes so two turns cannot isolate one
	// session twice.
	mu sync.Mutex
}

// worktreeNotice tells the console a turn moved into a worktree.
type worktreeNotice struct {
	Worktree session.SessionWorktree
	// Holder is the session that held the repository, when that was why.
	Holder string
	Copied []string
}

// beginTurn runs before a turn in sessionID. It takes the repository lease,
// or isolates the session when another session holds it or the turn is
// unattended. The returned func ends the turn's hold on the lease.
func (c *chatWorktrees) beginTurn(ctx context.Context, sessionID string, unattended bool) (*worktreeNotice, func()) {
	noop := func() {}
	if c == nil || strings.TrimSpace(sessionID) == "" {
		return nil, noop
	}
	sess, err := c.store.Get(sessionID)
	if err != nil || sess.Worktree != nil || strings.TrimSpace(sess.CurrentDir) == "" {
		return nil, noop
	}
	repo, err := sessionworktree.RepoRoot(ctx, sess.CurrentDir)
	if err != nil {
		return nil, noop
	}
	auto := sess.Isolation != session.IsolationOff
	if unattended && auto {
		notice, err := c.isolate(ctx, sess, "unattended", "")
		if err == nil {
			return notice, noop
		}
		// Without a worktree the run works in the checkout as it always has.
	}
	holder, ok := c.leases.acquire(repo, sessionID)
	if ok {
		return nil, func() { c.leases.finish(repo, sessionID) }
	}
	if !auto {
		return nil, noop
	}
	notice, err := c.isolate(ctx, sess, "lease", holder)
	if err != nil {
		return nil, noop
	}
	return notice, noop
}

// isolate moves sess into a new worktree of its current folder.
func (c *chatWorktrees) isolate(ctx context.Context, sess session.Session, reason, holder string) (*worktreeNotice, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	current, err := c.store.Get(sess.ID)
	if err != nil {
		return nil, err
	}
	if current.Worktree != nil {
		return &worktreeNotice{Worktree: *current.Worktree}, nil
	}
	source := strings.TrimSpace(current.CurrentDir)
	if source == "" {
		return nil, errors.New("the session has no working folder to isolate")
	}
	created, err := c.manager.Create(ctx, sessionworktree.CreateRequest{
		SessionID: current.ID,
		SourceDir: source,
		Include:   c.include(current),
		Reason:    reason,
	})
	if err != nil {
		return nil, err
	}
	if err := c.store.SetWorktree(current.ID, &created.Worktree); err != nil {
		_ = c.manager.Discard(ctx, created.Worktree)
		return nil, err
	}
	c.leases.release(current.ID)
	details := map[string]any{"branch": created.Worktree.Branch, "path": created.Worktree.Path, "reason": reason}
	if holder != "" {
		details["lease_holder"] = holder
	}
	if len(created.Copied) > 0 {
		details["copied"] = created.Copied
	}
	if len(created.Skipped) > 0 {
		details["skipped"] = created.Skipped
	}
	c.record(current.ID, source, "isolated", details)
	return &worktreeNotice{Worktree: created.Worktree, Holder: holder, Copied: created.Copied}, nil
}

func (c *chatWorktrees) include(sess session.Session) []string {
	if c.overrides == nil {
		return nil
	}
	res, _, err := c.overrides.Resolve(sess.ID)
	if err != nil {
		return nil
	}
	return res.Effective.WorktreeInclude
}

// finish applies, keeps or discards the session's worktree and moves the
// session back to the folder the worktree came from.
func (c *chatWorktrees) finish(ctx context.Context, sessionID, action string) (map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sess, err := c.store.Get(sessionID)
	if err != nil {
		return nil, err
	}
	if sess.Worktree == nil {
		return nil, errWorktreeMissing
	}
	wt := *sess.Worktree
	result := map[string]any{"branch": wt.Branch}
	switch action {
	case "apply":
		files, err := c.manager.Apply(ctx, wt)
		if err != nil {
			c.record(sessionID, wt.SourceDir, "apply_failed", map[string]any{"branch": wt.Branch, "error": err.Error()})
			return nil, err
		}
		result["files"] = files
	case "keep":
		branch, err := c.manager.Keep(ctx, wt, "TARS session: "+strings.TrimSpace(sess.Title))
		if err != nil {
			return nil, err
		}
		result["branch"] = branch
	case "discard":
		if err := c.manager.Discard(ctx, wt); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown action %q", action)
	}
	if err := c.store.SetWorktree(sessionID, nil); err != nil {
		return nil, err
	}
	c.record(sessionID, wt.SourceDir, worktreeOutcomes[action], result)
	return result, nil
}

var errWorktreeMissing = errors.New("the session has no worktree")

var worktreeOutcomes = map[string]string{"apply": "applied", "keep": "kept", "discard": "discarded"}

func (c *chatWorktrees) record(sessionID, cwd, result string, details map[string]any) {
	if c.audit == nil {
		return
	}
	c.audit(ops.AutomationAuditEntry{
		Timestamp: time.Now().UTC(),
		Actor:     "tars",
		Action:    "session_worktree",
		SessionID: sessionID,
		CWD:       cwd,
		Result:    result,
		Details:   details,
	})
}

// retire keeps the work of a session that is going away on its branch.
func (c *chatWorktrees) retire(ctx context.Context, sess session.Session) {
	if c == nil {
		return
	}
	c.leases.release(sess.ID)
	if sess.Worktree == nil {
		return
	}
	if _, err := c.manager.Keep(ctx, *sess.Worktree, "TARS session: "+strings.TrimSpace(sess.Title)); err == nil {
		c.record(sess.ID, sess.Worktree.SourceDir, "kept", map[string]any{"branch": sess.Worktree.Branch, "why": "session deleted"})
	}
}

// sweep keeps, on their branches, the worktrees whose session no longer
// exists. The server runs it at startup.
func (c *chatWorktrees) sweep(ctx context.Context) int {
	if c == nil {
		return 0
	}
	records, err := c.manager.Records()
	if err != nil {
		return 0
	}
	swept := 0
	for _, record := range records {
		if sess, err := c.store.Get(record.SessionID); err == nil && sess.Worktree != nil && sess.Worktree.Path == record.Worktree.Path {
			continue
		}
		if _, err := c.manager.Keep(ctx, record.Worktree, "TARS session "+record.SessionID); err == nil {
			swept++
			c.record(record.SessionID, record.Worktree.SourceDir, "kept", map[string]any{"branch": record.Worktree.Branch, "why": "session gone"})
		}
	}
	return swept
}

// worktreeView is GET /v1/admin/sessions/{id}/worktree.
type worktreeView struct {
	Worktree  *session.SessionWorktree `json:"worktree"`
	Isolation string                   `json:"isolation"`
	// Status lists what the session changed in its worktree.
	Status *sessionworktree.Status `json:"status,omitempty"`
	// RepoRoot is the repository of the session's folder, when it is in one.
	RepoRoot string `json:"repo_root,omitempty"`
	// LeaseHolder is the session holding that repository's write lease.
	LeaseHolder      string `json:"lease_holder,omitempty"`
	LeaseHolderTitle string `json:"lease_holder_title,omitempty"`
	Running          bool   `json:"running"`
}

func (c *chatWorktrees) view(ctx context.Context, sess session.Session) worktreeView {
	view := worktreeView{Worktree: sess.Worktree, Isolation: sess.Isolation, Running: c.running != nil && c.running(sess.ID)}
	if sess.Worktree != nil {
		if status, err := c.manager.Status(ctx, *sess.Worktree); err == nil {
			view.Status = &status
		}
		view.RepoRoot = sess.Worktree.RepoRoot
		return view
	}
	if dir := strings.TrimSpace(sess.CurrentDir); dir != "" {
		if repo, err := sessionworktree.RepoRoot(ctx, dir); err == nil {
			view.RepoRoot = repo
			view.LeaseHolder = c.leases.holder(repo)
			if view.LeaseHolder != "" {
				if holder, err := c.store.Get(view.LeaseHolder); err == nil {
					view.LeaseHolderTitle = holder.Title
				}
			}
		}
	}
	return view
}

// newSessionWorktreeHandler serves /v1/admin/sessions/{id}/worktree:
// GET the view, POST {"action": "isolate"|"apply"|"keep"|"discard"}, and
// PUT {"isolation": ""|"off"}.
func newSessionWorktreeHandler(c *chatWorktrees) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := strings.TrimSpace(r.PathValue("id"))
		sess, err := c.store.Get(sessionID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, c.view(r.Context(), sess))
		case http.MethodPut:
			var body struct {
				Isolation string `json:"isolation"`
			}
			if !decodeJSONBody(w, r, &body) {
				return
			}
			if err := c.store.SetIsolation(sessionID, body.Isolation); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			updated, _ := c.store.Get(sessionID)
			writeJSON(w, http.StatusOK, c.view(r.Context(), updated))
		case http.MethodPost:
			var body struct {
				Action string `json:"action"`
			}
			if !decodeJSONBody(w, r, &body) {
				return
			}
			if c.running != nil && c.running(sessionID) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "a turn is running in this session; wait for it to finish"})
				return
			}
			var result map[string]any
			switch body.Action {
			case "isolate":
				notice, isoErr := c.isolate(r.Context(), sess, "manual", "")
				err = isoErr
				if notice != nil {
					result = map[string]any{"branch": notice.Worktree.Branch, "copied": notice.Copied}
				}
			case "apply", "keep", "discard":
				result, err = c.finish(r.Context(), sessionID, body.Action)
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be isolate, apply, keep or discard"})
				return
			}
			if err != nil {
				status := http.StatusBadRequest
				var conflict *sessionworktree.ConflictError
				if errors.As(err, &conflict) || errors.Is(err, errWorktreeMissing) {
					status = http.StatusConflict
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			updated, _ := c.store.Get(sessionID)
			writeJSON(w, http.StatusOK, map[string]any{"result": result, "view": c.view(r.Context(), updated)})
		default:
			writeMethodNotAllowed(w)
		}
	})
}

// withWorktreeRetire keeps a deleted session's worktree changes on its
// branch and removes the worktree folder, then drops its leases.
func withWorktreeRetire(next http.Handler, c *chatWorktrees) http.Handler {
	if c == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/admin/sessions/")
		if r.Method != http.MethodDelete || id == "" || strings.Contains(id, "/") {
			next.ServeHTTP(w, r)
			return
		}
		sess, err := c.store.Get(id)
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		if err == nil && recorder.status < 300 {
			c.retire(context.WithoutCancel(r.Context()), sess)
		}
	})
}
