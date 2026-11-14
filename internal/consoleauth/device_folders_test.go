package consoleauth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeviceFoldersRoundTrip(t *testing.T) {
	store := NewStore(t.TempDir())
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")

	got, err := store.DeviceFolders()
	if err != nil || len(got) != 0 {
		t.Fatalf("empty store: %v %v", got, err)
	}

	stored, err := store.SetDeviceFolders([]string{" " + a + string(filepath.Separator) + " ", b, filepath.Join(a, ".")})
	if err != nil {
		t.Fatalf("SetDeviceFolders: %v", err)
	}
	if len(stored) != 2 || stored[0] != a || stored[1] != b {
		t.Fatalf("stored = %v, want [%s %s]", stored, a, b)
	}
	got, err = store.DeviceFolders()
	if err != nil || len(got) != 2 || got[0] != a {
		t.Fatalf("DeviceFolders = %v, %v", got, err)
	}

	for path, want := range map[string]bool{
		a:                              true,
		a + string(filepath.Separator): true,
		filepath.Join(a, "sub"):        false,
		root:                           false,
		"relative":                     false,
		"":                             false,
	} {
		ok, err := store.DeviceFolderAllowed(path)
		if err != nil || ok != want {
			t.Errorf("DeviceFolderAllowed(%q) = %v, %v; want %v", path, ok, err, want)
		}
	}

	if stored, err := store.SetDeviceFolders(nil); err != nil || len(stored) != 0 {
		t.Fatalf("clearing: %v %v", stored, err)
	}
	if ok, _ := store.DeviceFolderAllowed(a); ok {
		t.Fatalf("a cleared list allows nothing")
	}
}

func TestSetDeviceFoldersRejectsBadInput(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.SetDeviceFolders([]string{"relative/path"}); err == nil {
		t.Fatalf("a relative path must be rejected")
	}
	if _, err := store.SetDeviceFolders([]string{"  "}); err == nil {
		t.Fatalf("an empty path must be rejected")
	}
	many := make([]string, MaxDeviceFolders+1)
	for i := range many {
		many[i] = filepath.Join(t.TempDir(), "f")
	}
	if _, err := store.SetDeviceFolders(many); err == nil {
		t.Fatalf("more than %d folders must be rejected", MaxDeviceFolders)
	}
	if got, _ := store.DeviceFolders(); len(got) != 0 {
		t.Fatalf("a rejected list must not be stored: %v", got)
	}
}

func TestDeviceFoldersReadsBrokenFileAsError(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := os.MkdirAll(store.authDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.deviceFoldersPath(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeviceFolders(); err == nil {
		t.Fatalf("a broken file must be an error, not an empty list")
	}
	if _, err := store.DeviceFolderAllowed(filepath.Join(t.TempDir(), "x")); err == nil {
		t.Fatalf("an unreadable list must not allow or silently deny")
	}
	if err := os.WriteFile(store.deviceFoldersPath(), []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := store.DeviceFolders(); err != nil || len(got) != 0 {
		t.Fatalf("a blank file is an empty list: %v %v", got, err)
	}
}
