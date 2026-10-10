package session

import (
	"os"
	"path/filepath"
	"testing"
)

// Backstop tests for the CodeQL go/path-injection dismissals in #806. The
// Store's file operations all anchor on s.dir (an app-owned root) joined with
// a session ID that is either server-generated via generateID (16-char hex)
// or already present in the on-disk index. These tests pin both invariants.

func TestGenerateID_HexOnly(t *testing.T) {
	const trials = 32
	for i := 0; i < trials; i++ {
		id, err := generateID()
		if err != nil {
			t.Fatalf("generateID: %v", err)
		}
		if len(id) != 16 {
			t.Fatalf("expected 16-char hex id, got %q (%d chars)", id, len(id))
		}
		for _, ch := range id {
			if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
				t.Fatalf("non-hex character %q in id %q", ch, id)
			}
		}
	}
}

func TestStore_Delete_TraversalIDIsNoOp(t *testing.T) {
	// Sessions are added to the index only via Create/EnsureMain/... which
	// always use generateID. Delete short-circuits when the id is not in
	// the index, so a synthetic ".." or path-traversal id can never reach
	// os.Remove(s.TranscriptPath(id)).
	root := t.TempDir()
	store := NewStore(root)

	outside := filepath.Join(root, "..", "ghost.jsonl")
	outsideAbs, err := filepath.Abs(outside)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if err := os.WriteFile(outsideAbs, []byte("\n"), 0o644); err != nil {
		t.Fatalf("seed outside file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(outsideAbs) })

	// "../ghost" is not in the index; Delete must do nothing.
	if err := store.Delete("../ghost"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(outsideAbs); err != nil {
		t.Fatalf("expected outside file to survive delete, got %v", err)
	}
}

func TestStore_TranscriptPath_AnchorsUnderStoreDir(t *testing.T) {
	// An id with dot-segments used to be joined as it was, so "../etc/passwd"
	// gave a path one level above s.dir, limited only by its ".jsonl" suffix.
	// It now gives no path at all; a plain id is anchored under s.dir.
	store := NewStore("/var/tars/store")
	if got := store.TranscriptPath("../etc/passwd"); got != "" {
		t.Fatalf("expected no path for an id that leaves the store, got %q", got)
	}
	want := filepath.Join("/var/tars/store", "sessions", "abc.jsonl")
	if got := store.TranscriptPath("abc"); got != want {
		t.Fatalf("TranscriptPath(abc) = %q, want %q", got, want)
	}
}
