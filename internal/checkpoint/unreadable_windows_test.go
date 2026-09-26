//go:build windows

package checkpoint

import (
	"path/filepath"
	"slices"
	"syscall"
	"testing"
)

// A file another program holds open without sharing cannot be read. The turn
// is still recorded, and the file is reported as unknown so a revert never
// touches it.
func TestLockedFileIsReportedAsUnknown(t *testing.T) {
	s := newTestStore(t, Options{})
	root := t.TempDir()
	writeFile(t, root, "locked.txt", "held open\n")
	writeFile(t, root, "free.txt", "free\n")
	path, err := syscall.UTF16PtrFromString(filepath.Join(root, "locked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(path, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("lock file: %v", err)
	}
	defer syscall.CloseHandle(handle)

	entry := runTurn(t, s, "sess", "turn", root, func() { writeFile(t, root, "free.txt", "edited\n") })
	if entry.Skipped != "" || !slices.Contains(entry.Unknown, "locked.txt") {
		t.Fatalf("entry = %+v", entry)
	}
	files := filesByPath(mustDiff(t, s, "sess", "turn", ScopeTurn).Files)
	if _, ok := files["locked.txt"]; ok || files["free.txt"].Status != "modified" {
		t.Fatalf("files = %+v", files)
	}
}
