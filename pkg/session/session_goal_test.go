package session

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeGoal_EmptyDescriptionReturnsNil(t *testing.T) {
	got := NormalizeGoal(&SessionGoal{Description: "   "})
	if got != nil {
		t.Fatalf("expected nil for whitespace-only description, got %+v", got)
	}
	if NormalizeGoal(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestNormalizeGoal_DefaultsAndClamps(t *testing.T) {
	got := NormalizeGoal(&SessionGoal{Description: " do the thing  "})
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Description != "do the thing" {
		t.Fatalf("description not trimmed: %q", got.Description)
	}
	if got.MaxAutoContinues != DefaultGoalMaxAutoContinues {
		t.Fatalf("max default mismatch: %d", got.MaxAutoContinues)
	}
	if got.Status != SessionGoalStatusActive {
		t.Fatalf("status default mismatch: %q", got.Status)
	}

	long := strings.Repeat("x", MaxGoalDescriptionLen+50)
	got = NormalizeGoal(&SessionGoal{Description: long, MaxAutoContinues: 999})
	if len(got.Description) != MaxGoalDescriptionLen {
		t.Fatalf("description not clamped: len=%d", len(got.Description))
	}
	if got.MaxAutoContinues != MaxGoalMaxAutoContinues {
		t.Fatalf("max not clamped: %d", got.MaxAutoContinues)
	}

	got = NormalizeGoal(&SessionGoal{Description: "x", Status: "bogus", AutoContinueCount: -3})
	if got.Status != SessionGoalStatusActive {
		t.Fatalf("invalid status not reset: %q", got.Status)
	}
	if got.AutoContinueCount != 0 {
		t.Fatalf("negative count not clamped: %d", got.AutoContinueCount)
	}
}

func TestSessionGoalIsActive(t *testing.T) {
	var nilGoal *SessionGoal
	if nilGoal.IsActive() {
		t.Fatal("nil goal must not be active")
	}
	g := &SessionGoal{Status: SessionGoalStatusActive}
	if !g.IsActive() {
		t.Fatal("expected active")
	}
	g.Status = SessionGoalStatusSatisfied
	if g.IsActive() {
		t.Fatal("satisfied must not be active")
	}
}

func TestStoreSetGoal_MainSessionRoundTrip(t *testing.T) {
	store := NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	sess, err := store.SetGoal(main.ID, &SessionGoal{Description: "win the race"})
	if err != nil {
		t.Fatalf("set goal: %v", err)
	}
	if !sess.Goal.IsActive() {
		t.Fatalf("expected active goal, got %+v", sess.Goal)
	}
	if sess.Goal.MaxAutoContinues != DefaultGoalMaxAutoContinues {
		t.Fatalf("max default missing: %d", sess.Goal.MaxAutoContinues)
	}

	loaded, err := store.Get(main.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if loaded.Goal == nil || loaded.Goal.Description != "win the race" {
		t.Fatalf("persisted goal mismatch: %+v", loaded.Goal)
	}
}

func TestStoreSetGoal_ClearsOnEmpty(t *testing.T) {
	store := NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	if _, err := store.SetGoal(main.ID, &SessionGoal{Description: "first"}); err != nil {
		t.Fatalf("set first: %v", err)
	}
	sess, err := store.SetGoal(main.ID, &SessionGoal{Description: "   "})
	if err != nil {
		t.Fatalf("set empty: %v", err)
	}
	if sess.Goal != nil {
		t.Fatalf("expected goal cleared by empty description, got %+v", sess.Goal)
	}
}

func TestStoreSetGoal_WorkerKindsRejected(t *testing.T) {
	store := NewStore(t.TempDir())
	for _, kind := range []string{"worker", "subagent"} {
		sess, err := store.CreateWithOptions(kind+"-like", kind, true)
		if err != nil {
			t.Fatalf("create %s: %v", kind, err)
		}
		_, err = store.SetGoal(sess.ID, &SessionGoal{Description: "x"})
		if !errors.Is(err, ErrSessionKindUnsupported) {
			t.Fatalf("%s: expected ErrSessionKindUnsupported, got %v", kind, err)
		}
	}
}

func TestStoreSetGoal_OrdinaryChatAllowed(t *testing.T) {
	store := NewStore(t.TempDir())
	sess, err := store.Create("chat in a folder")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := store.SetGoal(sess.ID, &SessionGoal{Description: "ship it"})
	if err != nil {
		t.Fatalf("set goal on an ordinary chat: %v", err)
	}
	if updated.Goal == nil || updated.Goal.Description != "ship it" {
		t.Fatalf("expected goal stored, got %+v", updated.Goal)
	}
}

func TestStoreClearGoal_NoopWhenAbsent(t *testing.T) {
	store := NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	sess, err := store.ClearGoal(main.ID)
	if err != nil {
		t.Fatalf("clear when absent: %v", err)
	}
	if sess.Goal != nil {
		t.Fatalf("unexpected goal: %+v", sess.Goal)
	}
}

func TestStoreUpdateGoalProgress_BumpsCount(t *testing.T) {
	store := NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	if _, err := store.SetGoal(main.ID, &SessionGoal{Description: "x"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	sess, err := store.UpdateGoalProgress(main.ID, func(g *SessionGoal) *SessionGoal {
		g.AutoContinueCount++
		return g
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if sess.Goal == nil || sess.Goal.AutoContinueCount != 1 {
		t.Fatalf("count mismatch: %+v", sess.Goal)
	}

	sess, err = store.UpdateGoalProgress(main.ID, func(g *SessionGoal) *SessionGoal { return nil })
	if err != nil {
		t.Fatalf("clear via mutator: %v", err)
	}
	if sess.Goal != nil {
		t.Fatalf("expected nil after mutator returns nil, got %+v", sess.Goal)
	}
}

func TestStoreSetGoal_UnknownSession(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.SetGoal("nope", &SessionGoal{Description: "x"}); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestStoreClearGoal_UnknownSession(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.ClearGoal("nope"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestStoreUpdateGoalProgress_NoopWhenAbsent(t *testing.T) {
	store := NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	called := 0
	sess, err := store.UpdateGoalProgress(main.ID, func(g *SessionGoal) *SessionGoal {
		called++
		return g
	})
	if err != nil {
		t.Fatalf("update no-op: %v", err)
	}
	if called != 0 {
		t.Fatalf("expected mutator not to run when no goal, got %d calls", called)
	}
	if sess.Goal != nil {
		t.Fatalf("expected nil goal: %+v", sess.Goal)
	}
}

func TestStoreUpdateGoalProgress_UnknownSession(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.UpdateGoalProgress("nope", func(g *SessionGoal) *SessionGoal { return g }); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestSessionGoal_JSONOmitemptyWhenAbsent(t *testing.T) {
	sess := Session{Title: "x", Kind: "main"}
	data, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "\"goal\"") {
		t.Fatalf("expected goal field omitted when nil: %s", data)
	}
}

func TestStoreSetGoal_PermissionModeGrantedAndRestored(t *testing.T) {
	store := NewStore(t.TempDir())
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.SetPermissionMode(sess.ID, "manual"); err != nil {
		t.Fatalf("set mode: %v", err)
	}

	updated, err := store.SetGoal(sess.ID, &SessionGoal{Description: "ship", PermissionMode: "auto"})
	if err != nil {
		t.Fatalf("set goal: %v", err)
	}
	if updated.PermissionMode != "auto" || updated.Goal.RestorePermissionMode != "manual" {
		t.Fatalf("expected auto granted with manual to restore, got mode=%q goal=%+v", updated.PermissionMode, updated.Goal)
	}

	// Progress on the same active goal keeps the grant.
	updated, err = store.UpdateGoalProgress(sess.ID, func(g *SessionGoal) *SessionGoal {
		g.AutoContinueCount++
		return g
	})
	if err != nil || updated.PermissionMode != "auto" {
		t.Fatalf("expected grant kept during progress, got mode=%q err=%v", updated.PermissionMode, err)
	}

	// The goal ending hands the previous mode back, once.
	updated, err = store.UpdateGoalProgress(sess.ID, func(g *SessionGoal) *SessionGoal {
		g.Status = SessionGoalStatusExhausted
		return g
	})
	if err != nil || updated.PermissionMode != "manual" || updated.Goal.PermissionMode != "" {
		t.Fatalf("expected manual restored on exhausted, got mode=%q goal=%+v err=%v", updated.PermissionMode, updated.Goal, err)
	}
	if err := store.SetPermissionMode(sess.ID, "plan"); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	updated, err = store.ClearGoal(sess.ID)
	if err != nil || updated.PermissionMode != "plan" {
		t.Fatalf("expected a later mode left alone on clear, got mode=%q err=%v", updated.PermissionMode, err)
	}
}

func TestStoreClearGoal_KeepsModeTheUserChangedDuringTheGoal(t *testing.T) {
	store := NewStore(t.TempDir())
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.SetGoal(sess.ID, &SessionGoal{Description: "ship", PermissionMode: "auto"}); err != nil {
		t.Fatalf("set goal: %v", err)
	}
	if err := store.SetPermissionMode(sess.ID, "plan"); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	updated, err := store.ClearGoal(sess.ID)
	if err != nil || updated.PermissionMode != "plan" {
		t.Fatalf("expected the user's plan mode kept, got mode=%q err=%v", updated.PermissionMode, err)
	}
}

func TestStoreSetGoal_ReplacingAGoalRestoresBeforeGranting(t *testing.T) {
	store := NewStore(t.TempDir())
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.SetGoal(sess.ID, &SessionGoal{Description: "first", PermissionMode: "auto"}); err != nil {
		t.Fatalf("set first: %v", err)
	}
	updated, err := store.SetGoal(sess.ID, &SessionGoal{Description: "second"})
	if err != nil || updated.PermissionMode != "" || updated.Goal.PermissionMode != "" {
		t.Fatalf("expected the grant to end with the first goal, got mode=%q goal=%+v err=%v", updated.PermissionMode, updated.Goal, err)
	}
}
