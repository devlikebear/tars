package session

import (
	"errors"
	"testing"
)

func TestStoreSetUpstreamSessionID_PersistsAndRoundTrips(t *testing.T) {
	store := NewStore(t.TempDir())
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sess.UpstreamSessionID != "" {
		t.Fatalf("new session should have empty upstream id, got %q", sess.UpstreamSessionID)
	}

	if err := store.SetUpstreamSessionID(sess.ID, "  sess-upstream-1  "); err != nil {
		t.Fatalf("set upstream: %v", err)
	}
	reloaded, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if reloaded.UpstreamSessionID != "sess-upstream-1" {
		t.Fatalf("expected trimmed upstream id, got %q", reloaded.UpstreamSessionID)
	}

	// Clearing.
	if err := store.SetUpstreamSessionID(sess.ID, ""); err != nil {
		t.Fatalf("clear upstream: %v", err)
	}
	cleared, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if cleared.UpstreamSessionID != "" {
		t.Fatalf("expected cleared upstream id, got %q", cleared.UpstreamSessionID)
	}
}

func TestStoreSetUpstreamSessionID_NoBumpWhenUnchanged(t *testing.T) {
	store := NewStore(t.TempDir())
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetUpstreamSessionID(sess.ID, "sess-X"); err != nil {
		t.Fatalf("first set: %v", err)
	}
	first, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if err := store.SetUpstreamSessionID(sess.ID, "sess-X"); err != nil {
		t.Fatalf("idempotent set: %v", err)
	}
	again, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !again.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("UpdatedAt must not bump when value is unchanged: before=%v after=%v", first.UpdatedAt, again.UpdatedAt)
	}
}

func TestStoreSetUpstreamSessionID_UnknownSession(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SetUpstreamSessionID("nope", "sess-Y"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}
