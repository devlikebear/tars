package consoleauth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKeepAwakeIsOffUntilSet(t *testing.T) {
	store := NewStore(t.TempDir())
	if on, err := store.KeepAwake(); err != nil || on {
		t.Fatalf("unset: on=%t err=%v", on, err)
	}
	if err := store.SetKeepAwake(true); err != nil {
		t.Fatal(err)
	}
	if on, err := store.KeepAwake(); err != nil || !on {
		t.Fatalf("after on: on=%t err=%v", on, err)
	}
	if err := store.SetKeepAwake(false); err != nil {
		t.Fatal(err)
	}
	if on, _ := store.KeepAwake(); on {
		t.Fatal("still on after off")
	}
}

func TestKeepAwakeFileProblems(t *testing.T) {
	store := NewStore(t.TempDir())
	path := store.keepAwakePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if on, err := store.KeepAwake(); err != nil || on {
		t.Fatalf("empty file: on=%t err=%v", on, err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.KeepAwake(); err == nil {
		t.Fatal("a broken file must be an error")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.KeepAwake(); err == nil {
		t.Fatal("an unreadable file must be an error")
	}
}
