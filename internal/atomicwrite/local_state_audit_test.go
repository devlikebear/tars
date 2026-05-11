package atomicwrite

import (
	"os"
	"path/filepath"
	"testing"
)

// This file documents the atomicwrite trust contract referenced by the
// CodeQL go/path-injection dismissals in #806. atomicwrite.Write is the
// canonical state-persistence helper for sessions.json, jobs.json, knowledge
// notes, etc. — every caller passes a path composed from an app-owned root
// joined with an identifier the caller has already validated (server-generated
// hex IDs, index-checked session IDs, kebab-case skill names).
//
// The boundary therefore lives at the identifier-generation/lookup site, not
// at this helper. These tests pin the helper's documented contract.

func TestAtomicWrite_RoundTripsBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "state.json")
	want := []byte(`{"ok":true}`)

	if err := Write(path, want); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("payload mismatch: got %q want %q", got, want)
	}
}

func TestAtomicWrite_LeavesNoTempOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := Write(path, []byte("{}")); err != nil {
		t.Fatalf("write: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "state.json" {
			continue
		}
		t.Fatalf("expected only state.json in %s, found %q", dir, name)
	}
}

func TestAtomicWrite_CreatesParentDirIfMissing(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "missing", "child.json")
	if err := Write(target, []byte("{}")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(target)); err != nil {
		t.Fatalf("expected parent dir to exist: %v", err)
	}
}
