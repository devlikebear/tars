package extensions

import (
	"fmt"
	"os"
	"path/filepath"
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
