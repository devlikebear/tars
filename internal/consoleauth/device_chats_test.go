package consoleauth

import (
	"os"
	"testing"
)

func TestDeviceChatSessionRoundTrip(t *testing.T) {
	store := NewStore(t.TempDir())

	if _, ok, err := store.DeviceChatSession("dev-1"); err != nil || ok {
		t.Fatalf("empty store: ok=%v err=%v", ok, err)
	}

	if err := store.SetDeviceChatSession("dev-1", "sess-a"); err != nil {
		t.Fatalf("SetDeviceChatSession: %v", err)
	}
	sessionID, ok, err := store.DeviceChatSession("dev-1")
	if err != nil || !ok || sessionID != "sess-a" {
		t.Fatalf("DeviceChatSession = %q, %v, %v; want sess-a, true, nil", sessionID, ok, err)
	}

	// A second device gets its own entry; the first is untouched.
	if err := store.SetDeviceChatSession("dev-2", "sess-b"); err != nil {
		t.Fatalf("SetDeviceChatSession dev-2: %v", err)
	}
	if sessionID, ok, _ := store.DeviceChatSession("dev-1"); !ok || sessionID != "sess-a" {
		t.Fatalf("dev-1 entry disturbed: %q, %v", sessionID, ok)
	}

	// Replacing dev-1's session (its old one was gone) overwrites in place.
	if err := store.SetDeviceChatSession("dev-1", "sess-c"); err != nil {
		t.Fatalf("SetDeviceChatSession replace: %v", err)
	}
	if sessionID, ok, _ := store.DeviceChatSession("dev-1"); !ok || sessionID != "sess-c" {
		t.Fatalf("dev-1 replaced = %q, %v; want sess-c, true", sessionID, ok)
	}

	if _, ok, err := store.DeviceChatSession(""); err != nil || ok {
		t.Fatalf("empty device id: ok=%v err=%v", ok, err)
	}
}

func TestDeviceChatSessionCorruptFile(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := os.MkdirAll(store.authDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.deviceChatsPath(), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.DeviceChatSession("dev-1"); err == nil {
		t.Fatalf("a corrupt file must error, not silently answer not-found")
	}
	if err := store.SetDeviceChatSession("dev-1", "sess-a"); err == nil {
		t.Fatalf("a corrupt file must error on write too")
	}
}

func TestDeviceChatSessionUnreadablePath(t *testing.T) {
	store := NewStore(t.TempDir())
	// A directory where the file should be: os.ReadFile fails with
	// something other than "does not exist".
	if err := os.MkdirAll(store.deviceChatsPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.DeviceChatSession("dev-1"); err == nil {
		t.Fatalf("expected a read error")
	}
}

func TestSetDeviceChatSessionRejectsEmpty(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SetDeviceChatSession("", "sess-a"); err == nil {
		t.Fatalf("empty device id must error")
	}
	if err := store.SetDeviceChatSession("dev-1", ""); err == nil {
		t.Fatalf("empty session id must error")
	}
}
