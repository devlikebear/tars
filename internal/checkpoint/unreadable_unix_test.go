//go:build !windows

package checkpoint

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// A file the server cannot read does not fail the turn; it is reported as
// unknown so a revert never touches it.
func TestUnreadableFileIsReportedAsUnknown(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads files regardless of mode")
	}
	s := newTestStore(t, Options{})
	root := t.TempDir()
	writeFile(t, root, "secret.txt", "no read access\n")
	writeFile(t, root, "free.txt", "free\n")
	locked := filepath.Join(root, "secret.txt")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })

	entry := runTurn(t, s, "sess", "turn", root, func() { writeFile(t, root, "free.txt", "edited\n") })
	if entry.Skipped != "" || !slices.Contains(entry.Unknown, "secret.txt") {
		t.Fatalf("entry = %+v", entry)
	}
	files := filesByPath(mustDiff(t, s, "sess", "turn", ScopeTurn).Files)
	if _, ok := files["secret.txt"]; ok || files["free.txt"].Status != "modified" {
		t.Fatalf("files = %+v", files)
	}
}
