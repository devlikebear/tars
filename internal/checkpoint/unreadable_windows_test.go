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

func lockFile(t *testing.T, path string, share uint32) {
	t.Helper()
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, share, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("lock %s: %v", path, err)
	}
	t.Cleanup(func() { _ = syscall.CloseHandle(handle) })
}

// A file another program holds open is reported; the rest of the revert
// goes ahead. One that cannot be read is not even planned.
func TestRevertReportsLockedFiles(t *testing.T) {
	s := newTestStore(t, Options{})
	root := t.TempDir()
	for _, name := range []string{"unreadable.txt", "unwritable.txt", "free.txt"} {
		writeFile(t, root, name, "before\n")
	}
	runTurn(t, s, "sess", "turn", root, func() {
		for _, name := range []string{"unreadable.txt", "unwritable.txt", "free.txt"} {
			writeFile(t, root, name, "after\n")
		}
	})
	lockFile(t, filepath.Join(root, "unreadable.txt"), 0)
	lockFile(t, filepath.Join(root, "unwritable.txt"), syscall.FILE_SHARE_READ)

	result := mustRevert(t, s, "sess", "turn", RevertRequest{Apply: true})
	outcomes := map[string]string{}
	for _, f := range result.Files {
		outcomes[f.Path] = f.Outcome
	}
	if outcomes["unreadable.txt"] != RevertFailed || outcomes["unwritable.txt"] != RevertFailed || outcomes["free.txt"] != RevertWrite {
		t.Fatalf("outcomes = %v", outcomes)
	}
	if !result.Applied || result.RevertID == "" || readText(t, root, "free.txt") != "before\n" {
		t.Fatalf("result = %+v", result)
	}
}
