package extensions

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestDisabledStoreSetDisabledReturnsLoadErrorAndPreservesFile(t *testing.T) {
	workspaceDir := t.TempDir()
	store := newDisabledStore(workspaceDir)
	path := filepath.Join(workspaceDir, disabledFileName)
	corrupt := []byte(`{"skills": [`)
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatalf("write corrupt disabled state: %v", err)
	}

	if err := store.SetDisabled("skill", "deploy", true); err == nil {
		t.Fatalf("expected corrupt disabled state to be returned")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read disabled state: %v", err)
	}
	if string(got) != string(corrupt) {
		t.Fatalf("expected corrupt state to be preserved, got %q", got)
	}
}

func TestDisabledStoreSaveWriteFailurePreservesExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permission failure is not reliable in this environment")
	}

	workspaceDir := t.TempDir()
	store := newDisabledStore(workspaceDir)
	path := filepath.Join(workspaceDir, disabledFileName)
	if err := store.Save(DisabledSet{Skills: []string{"keep"}}); err != nil {
		t.Fatalf("seed disabled state: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seed disabled state: %v", err)
	}

	if err := os.Chmod(workspaceDir, 0o555); err != nil {
		t.Fatalf("chmod workspace: %v", err)
	}
	defer func() {
		_ = os.Chmod(workspaceDir, 0o755)
	}()

	if err := store.Save(DisabledSet{Skills: []string{"replace"}}); err == nil {
		t.Fatalf("expected save failure")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read disabled state after failed save: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("expected existing disabled state to be preserved, got %q want %q", after, before)
	}
}

func TestDisabledStoreSaveUsesPrivateFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file mode bits are not reliable on Windows")
	}

	workspaceDir := t.TempDir()
	store := newDisabledStore(workspaceDir)
	path := filepath.Join(workspaceDir, disabledFileName)
	if err := store.Save(DisabledSet{Skills: []string{"keep"}}); err != nil {
		t.Fatalf("save disabled state: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat disabled state: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("disabled state mode = %v, want 0600", got)
	}
}

func TestDisabledStoreSetDisabledKeepsConcurrentUpdates(t *testing.T) {
	workspaceDir := t.TempDir()
	store := newDisabledStore(workspaceDir)

	const count = 64
	var start sync.WaitGroup
	start.Add(1)
	var done sync.WaitGroup
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("skill-%02d", i)
		done.Add(1)
		go func(name string) {
			defer done.Done()
			start.Wait()
			if err := store.SetDisabled("skill", name, true); err != nil {
				t.Errorf("set disabled %s: %v", name, err)
			}
		}(name)
	}
	start.Done()
	done.Wait()

	ds, err := store.Load()
	if err != nil {
		t.Fatalf("load disabled state: %v", err)
	}
	if len(ds.Skills) != count {
		t.Fatalf("expected %d disabled skills, got %d: %+v", count, len(ds.Skills), ds.Skills)
	}
}
