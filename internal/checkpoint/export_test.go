package checkpoint

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExportTurn(t *testing.T) {
	s := newTestStore(t, Options{})
	repo := initUserRepo(t)
	writeFile(t, repo, "app/x.go", "defect\n")
	writeFile(t, repo, "keep.txt", "same\n")
	writeFile(t, repo, "gone.txt", "removed by the fix\n")
	entry := runTurn(t, s, "s1", "t1", repo, func() {
		writeFile(t, repo, "app/x.go", "fixed\n")
		writeFile(t, repo, "new/added.txt", "added\n")
		if err := os.Remove(filepath.Join(repo, "gone.txt")); err != nil {
			t.Fatal(err)
		}
	})
	if entry.Start == "" || entry.End == "" {
		t.Fatalf("no checkpoint: %+v", entry)
	}
	ctx := context.Background()
	start, end := t.TempDir(), t.TempDir()
	if err := s.ExportTurn(ctx, "s1", "t1", SideStart, start); err != nil {
		t.Fatal(err)
	}
	if err := s.ExportTurn(ctx, "s1", "t1", SideEnd, end); err != nil {
		t.Fatal(err)
	}
	before, after := fingerprint(t, start), fingerprint(t, end)
	if len(before) != 3 || before["gone.txt"] == "" || before["app/x.go"] == after["app/x.go"] {
		t.Fatalf("start = %v", before)
	}
	if len(after) != 3 || after["new/added.txt"] == "" || after["gone.txt"] != "" || after["keep.txt"] != before["keep.txt"] {
		t.Fatalf("end = %v", after)
	}
	// The work tree itself is untouched by an export.
	if got, _ := os.ReadFile(filepath.Join(repo, "app/x.go")); string(got) != "fixed\n" {
		t.Fatalf("work tree changed: %q", got)
	}

	if err := s.ExportTurn(ctx, "s1", "t1", "middle", t.TempDir()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad side: %v", err)
	}
	if err := s.ExportTurn(ctx, "s1", "nope", SideStart, t.TempDir()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown turn: %v", err)
	}
	if err := s.ExportTurn(ctx, "../x", "t1", SideStart, t.TempDir()); err == nil {
		t.Fatal("bad session id")
	}
	if err := s.ExportTurn(ctx, "s1", "../t", SideStart, t.TempDir()); err == nil {
		t.Fatal("bad turn id")
	}
}

// hostileCommit writes a commit into the shadow of e whose tree holds a
// single entry named name (a tree holding file x), as no git client would
// create but a damaged or tampered shadow could hold, and records it as
// turn "hostile" of session s1.
func hostileCommit(t *testing.T, s *Store, e Entry, name string) {
	t.Helper()
	sh, err := s.openShadow(e.Shadow)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	run := func(stdin []byte, args ...string) string {
		call := sh.objectCall(args...)
		call.stdin = stdin
		out, _, err := s.git.run(ctx, call)
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return strings.TrimSpace(string(out))
	}
	rawOID := func(hexID string) []byte {
		b, err := hex.DecodeString(hexID)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	blob := run([]byte("outside\n"), "hash-object", "-w", "--stdin")
	inner := run(append([]byte("100644 x\x00"), rawOID(blob)...), "hash-object", "-t", "tree", "-w", "--literally", "--stdin")
	outer := run(append([]byte("40000 "+name+"\x00"), rawOID(inner)...), "hash-object", "-t", "tree", "-w", "--literally", "--stdin")
	commit := run(nil, "commit-tree", outer, "-m", "hostile")
	idx, err := s.readIndex("s1")
	if err != nil {
		t.Fatal(err)
	}
	idx.Turns = append(idx.Turns, Entry{TurnID: "hostile", Root: e.Root, Shadow: e.Shadow, Start: commit, End: commit})
	if err := s.writeIndex(idx); err != nil {
		t.Fatal(err)
	}
}

// A snapshot whose tree climbs out of the folder (an entry named "..") or
// into a .git is refused by git itself, and nothing is written outside.
func TestExportTurnRefusesEscapingEntries(t *testing.T) {
	for _, name := range []string{"..", ".git"} {
		t.Run(name, func(t *testing.T) {
			s := newTestStore(t, Options{})
			repo := initUserRepo(t)
			writeFile(t, repo, "a.txt", "a\n")
			entry := runTurn(t, s, "s1", "t1", repo, func() { writeFile(t, repo, "a.txt", "b\n") })
			hostileCommit(t, s, entry, name)
			parent := t.TempDir()
			dest := filepath.Join(parent, "tree")
			if err := os.Mkdir(dest, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := s.ExportTurn(context.Background(), "s1", "hostile", SideStart, dest); err == nil {
				t.Fatal("a tree entry escaping the folder was exported")
			}
			if _, err := os.Stat(filepath.Join(parent, "x")); !os.IsNotExist(err) {
				t.Fatalf("written outside: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dest, ".git", "x")); !os.IsNotExist(err) {
				t.Fatalf("written into .git: %v", err)
			}
		})
	}
}

// Symbolic links recorded in a snapshot come back only when they stay
// inside the folder.
func TestExportTurnDropsEscapingLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	s := newTestStore(t, Options{})
	repo := initUserRepo(t)
	writeFile(t, repo, "dir/run.sh", "echo\n")
	entry := runTurn(t, s, "s1", "t1", repo, func() {
		for link, target := range map[string]string{
			"in": "dir/run.sh", "dir/up": "../dir/run.sh", "out": "../../etc/passwd", "abs": "/etc/passwd", "dir/far": "../../x",
		} {
			if err := os.Symlink(target, filepath.Join(repo, filepath.FromSlash(link))); err != nil {
				t.Fatal(err)
			}
		}
	})
	dest := t.TempDir()
	if err := s.ExportTurn(context.Background(), "s1", entry.TurnID, SideEnd, dest); err != nil {
		t.Fatal(err)
	}
	for _, kept := range []string{"in", "dir/up"} {
		if _, err := os.Readlink(filepath.Join(dest, filepath.FromSlash(kept))); err != nil {
			t.Errorf("inside link %s: %v", kept, err)
		}
	}
	for _, gone := range []string{"out", "abs", "dir/far"} {
		if _, err := os.Lstat(filepath.Join(dest, filepath.FromSlash(gone))); !os.IsNotExist(err) {
			t.Errorf("escaping link %s was kept", gone)
		}
	}
}

func TestLinkStaysInside(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	link := filepath.Join(root, "a", "l")
	for target, want := range map[string]bool{
		"b": true, "../b": true, "..": true, "../..": false, "../../x": false, "": false,
		string(filepath.Separator) + "etc": false,
	} {
		if got := linkStaysInside(root, link, target); got != want {
			t.Errorf("%q: %v, want %v", target, got, want)
		}
	}
	if err := pruneEscapingLinks(filepath.Join(root, "missing")); err == nil {
		t.Fatal("a missing folder is an error")
	}
}
