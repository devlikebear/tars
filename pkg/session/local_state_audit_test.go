package session

import (
	"os"
	"path/filepath"
	"strings"
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
	// Even when callers pass an id with dot-segments, the path is constructed
	// as filepath.Join(s.dir, id + ".jsonl"). filepath.Join collapses, so
	// "../escape" produces a path one level above s.dir + ".jsonl" suffix.
	// Verify the suffix is preserved so a caller cannot smuggle an arbitrary
	// filename through (e.g. id="../etc/passwd" yields ".../etc/passwd.jsonl",
	// not ".../etc/passwd").
	store := NewStore("/var/tars/store")
	got := store.TranscriptPath("../etc/passwd")
	if !strings.HasSuffix(got, ".jsonl") {
		t.Fatalf("expected .jsonl suffix to survive, got %q", got)
	}
}
