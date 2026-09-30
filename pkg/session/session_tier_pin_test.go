package session

import (
	"errors"
	"testing"
)

func TestStoreSetTierPin_PersistsPerSessionAndClears(t *testing.T) {
	store := NewStore(t.TempDir())
	pinned, err := store.Create("pinned")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	other, err := store.Create("other")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := store.SetTierPin(pinned.ID, " heavy "); err != nil {
		t.Fatalf("set tier pin: %v", err)
	}
	reloaded, err := store.Get(pinned.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.TierPin != "heavy" {
		t.Fatalf("TierPin = %q, want heavy", reloaded.TierPin)
	}
	untouched, err := store.Get(other.ID)
	if err != nil {
		t.Fatalf("reload other: %v", err)
	}
	if untouched.TierPin != "" {
		t.Fatalf("another session picked up the pin: %q", untouched.TierPin)
	}

	if err := store.SetTierPin(pinned.ID, ""); err != nil {
		t.Fatalf("clear tier pin: %v", err)
	}
	cleared, err := store.Get(pinned.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cleared.TierPin != "" {
		t.Fatalf("TierPin = %q after clearing", cleared.TierPin)
	}
}

func TestStoreSetTierPin_UnknownSession(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SetTierPin("missing", "heavy"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}
