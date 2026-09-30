package session

import (
	"errors"
	"os"
	"path/filepath"
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

func TestStoreSetTierPin_SameTierLeavesTheRecordAlone(t *testing.T) {
	store := NewStore(t.TempDir())
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetTierPin(sess.ID, "heavy"); err != nil {
		t.Fatalf("set tier pin: %v", err)
	}
	pinned, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if err := store.SetTierPin(sess.ID, " heavy "); err != nil {
		t.Fatalf("set same tier pin: %v", err)
	}
	again, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !again.UpdatedAt.Equal(pinned.UpdatedAt) {
		t.Fatalf("UpdatedAt moved from %v to %v on a no-op pin", pinned.UpdatedAt, again.UpdatedAt)
	}
}

func TestStoreSetTierPin_UnreadableIndex(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := os.WriteFile(filepath.Join(dir, "sessions.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := store.SetTierPin("any", "heavy"); err == nil {
		t.Fatal("SetTierPin on a corrupt index should fail")
	}
}

func TestStoreSetTierPin_ArtifactDirCannotBeCreated(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "sessions"))
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	// A file where the session's artifact directory belongs.
	artifactDir := store.sessionArtifactDir(sess.ID)
	if err := os.RemoveAll(artifactDir); err != nil {
		t.Fatalf("remove artifact dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(artifactDir), 0o755); err != nil {
		t.Fatalf("mkdir artifacts: %v", err)
	}
	if err := os.WriteFile(artifactDir, []byte("x"), 0o600); err != nil {
		t.Fatalf("block artifact dir: %v", err)
	}
	if err := store.SetTierPin(sess.ID, "heavy"); err == nil {
		t.Fatal("SetTierPin should report the artifact dir failure")
	}
}

func TestStoreSetPermissionMode_Persists(t *testing.T) {
	store := NewStore(t.TempDir())
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetPermissionMode(sess.ID, " plan "); err != nil {
		t.Fatalf("set permission mode: %v", err)
	}
	got, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PermissionMode != "plan" {
		t.Fatalf("PermissionMode = %q, want plan", got.PermissionMode)
	}
}
