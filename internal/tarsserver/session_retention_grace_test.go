package tarsserver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// A session archived long before deletion was switched on is due the moment
// it is: the first sweeps warn instead, and deletion starts once the warning
// period has passed — also across a restart.
func TestSessionRetentionWarnsBeforeTheFirstDeletions(t *testing.T) {
	dir := t.TempDir()
	store := session.NewStore(filepath.Join(dir, "sessions"))
	sess, err := store.Create("archived by hand, months ago")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetArchived(sess.ID, true); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(dir, "_shared", "session-retention.json")
	var notices []notificationEvent
	clock := time.Now().Add(90 * 24 * time.Hour) // the archive is 90 days old
	build := func() *sessionRetention {
		return &sessionRetention{
			store: store, policy: sessionRetentionPolicyFromDays(7, 30), logger: zerolog.Nop(),
			statePath: statePath,
			now:       func() time.Time { return clock },
			notify:    func(_ context.Context, e notificationEvent) { notices = append(notices, e) },
		}
	}

	r := build()
	if got := r.sweep(context.Background()); got.Deleted != 0 || got.Warned != 1 {
		t.Fatalf("first sweep with deletion on = %+v, want a warning and no deletion", got)
	}
	wantDate := clock.Add(sessionRetentionWarning).Local().Format("2006-01-02")
	if len(notices) != 1 || !strings.Contains(notices[0].Message, wantDate) {
		t.Fatalf("warning = %+v, want the date the grace period ends (%s)", notices, wantDate)
	}

	// A restart inside the grace period reads the same start time.
	clock = clock.Add(2 * 24 * time.Hour)
	r = build()
	if got := r.sweep(context.Background()); got.Deleted != 0 {
		t.Fatalf("sweep on day 2 = %+v, want no deletion yet", got)
	}
	if _, err := store.Get(sess.ID); err != nil {
		t.Fatalf("the session was deleted inside the grace period: %v", err)
	}

	clock = clock.Add(2 * 24 * time.Hour)
	if got := r.sweep(context.Background()); got.Deleted != 1 {
		t.Fatalf("sweep on day 4 = %+v, want the deletion", got)
	}

	// With deletion off nothing is recorded and nothing is deleted.
	off := &sessionRetention{store: store, policy: sessionRetentionPolicyFromDays(7, 0), logger: zerolog.Nop(),
		statePath: filepath.Join(dir, "unused", "state.json"), now: func() time.Time { return clock }}
	if at := off.firstDeleteAt(clock); !at.IsZero() {
		t.Fatalf("grace start with deletion off = %v", at)
	}
}
