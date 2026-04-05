package reflection

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

// SessionDeleter is the subset of session.Store that KBCleanupJob needs
// in addition to the SessionSource interface it inherits from the
// memory job. Split into its own interface so tests can provide a fake
// that implements only what's exercised.
type SessionDeleter interface {
	SessionSource
	Delete(id string) error
}

// KBCleanupJob runs the "knowledge base cleanup" half of reflection.
// Phase 1 scope is intentionally minimal: remove sessions whose
// transcript contains zero messages AND whose UpdatedAt is older than
// EmptySessionAge. This is safe because:
//
//   - Zero-message sessions have nothing the user would mourn;
//   - The age threshold protects fresh sessions that the user is in
//     the middle of composing a first turn for;
//   - Main sessions (kind="main") are never touched, so the always-on
//     main chat is never deleted out from under a running UI;
//
// Session compression (gzip old transcripts) is a deliberate follow-up
// — it requires read-side decompression changes that widen the blast
// radius beyond PR2's scope.
type KBCleanupJob struct {
	Sessions        SessionDeleter
	EmptySessionAge time.Duration
	Now             func() time.Time
}

// Name implements Job.
func (k *KBCleanupJob) Name() string { return "kb_cleanup" }

// Run implements Job.
func (k *KBCleanupJob) Run(ctx context.Context) (JobResult, error) {
	if k == nil {
		return JobResult{Name: "kb_cleanup"}, nil
	}
	if k.Sessions == nil {
		return JobResult{Name: "kb_cleanup", Success: false, Err: "no session deleter"}, nil
	}

	now := k.now()
	minAge := k.EmptySessionAge
	if minAge <= 0 {
		minAge = 24 * time.Hour
	}
	cutoff := now.Add(-minAge)

	sessions, err := k.Sessions.ListAll()
	if err != nil {
		return JobResult{Name: "kb_cleanup"}, fmt.Errorf("list sessions: %w", err)
	}

	var (
		removed int
		skipped int
		errs    []string
	)

	for _, sess := range sessions {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err.Error())
			break
		}
		if strings.EqualFold(strings.TrimSpace(sess.Kind), "main") {
			skipped++
			continue
		}
		if sess.UpdatedAt.After(cutoff) {
			skipped++
			continue
		}
		path := k.Sessions.TranscriptPath(sess.ID)
		messages, err := session.ReadMessages(path)
		if err != nil {
			errs = append(errs, fmt.Sprintf("read %s: %s", sess.ID, err.Error()))
			skipped++
			continue
		}
		if len(messages) > 0 {
			skipped++
			continue
		}
		if err := k.Sessions.Delete(sess.ID); err != nil {
			errs = append(errs, fmt.Sprintf("delete %s: %s", sess.ID, err.Error()))
			skipped++
			continue
		}
		removed++
	}

	result := JobResult{
		Name:    "kb_cleanup",
		Success: true,
		Summary: fmt.Sprintf("removed %d empty sessions, skipped %d", removed, skipped),
		Changed: removed > 0,
		Details: map[string]any{
			"removed_count":             removed,
			"skipped_count":             skipped,
			"empty_session_age_seconds": int64(minAge.Seconds()),
		},
	}
	if len(errs) > 0 {
		result.Details["errors"] = errs
	}
	return result, nil
}

func (k *KBCleanupJob) now() time.Time {
	if k.Now != nil {
		return k.Now()
	}
	return time.Now()
}
