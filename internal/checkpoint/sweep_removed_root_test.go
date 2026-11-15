package checkpoint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A session's folder can vanish together with its parent (a deleted
// temporary folder). git resolves the shadow's core.worktree at startup and
// died with "Invalid path" on every sweep; the shadow's own refs and objects
// must still be swept and read.
func TestSweepSurvivesRemovedRootFolder(t *testing.T) {
	s := newTestStore(t, Options{})
	scratch := filepath.Join(t.TempDir(), "scratch")
	root := filepath.Join(scratch, "worktree")
	writeFile(t, root, "a.txt", "one\n")
	runTurn(t, s, "live", "turn", root, func() { writeFile(t, root, "a.txt", "two\n") })
	dead := runTurn(t, s, "dead", "turn", root, nil)
	if err := os.RemoveAll(scratch); err != nil {
		t.Fatal(err)
	}

	if err := s.SweepOrphans(context.Background(), func(id string) bool { return id == "live" }); err != nil {
		t.Fatalf("sweep with a removed root folder: %v", err)
	}
	refs := shadowRefs(t, s, dead)
	if len(refs) == 0 {
		t.Fatal("live session lost its refs")
	}
	for _, ref := range refs {
		if strings.Contains(ref, "/dead/") {
			t.Fatalf("ref of a deleted session survived: %s", ref)
		}
	}
	files := filesByPath(mustDiff(t, s, "live", "turn", ScopeTurn).Files)
	if len(files) != 1 || files["a.txt"].Status != "modified" {
		t.Fatalf("diff after the root folder was removed: %+v", files)
	}
}

// One shadow git cannot open must not stop the sweep of the others, and the
// failure is still reported.
func TestSweepContinuesPastBrokenShadow(t *testing.T) {
	s := newTestStore(t, Options{})
	brokenRoot := t.TempDir()
	otherRoot := t.TempDir()
	broken := runTurn(t, s, "keep", "turn", brokenRoot, nil)
	runTurn(t, s, "lost", "turn", brokenRoot, nil)
	runTurn(t, s, "keep", "turn2", otherRoot, nil)
	other := runTurn(t, s, "gone", "turn", otherRoot, nil)
	if err := os.Remove(filepath.Join(s.dir, "shadow", broken.Shadow+".git", "HEAD")); err != nil {
		t.Fatal(err)
	}

	err := s.SweepOrphans(context.Background(), func(id string) bool { return id == "keep" })
	if err == nil || !strings.Contains(err.Error(), "shadow "+broken.Shadow) || !strings.Contains(err.Error(), "session lost") {
		t.Fatalf("broken shadow not reported: %v", err)
	}
	for _, ref := range shadowRefs(t, s, other) {
		if strings.Contains(ref, "/gone/") {
			t.Fatalf("sweep stopped before the other shadow: %s survived", ref)
		}
	}
}
