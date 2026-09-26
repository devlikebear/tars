package checkpoint

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The contract that matters most: checkpointing reads the user's work tree
// and writes nothing under their .git — not the index, the stash, HEAD, refs,
// objects, or config.
func TestTurnsLeaveTheUserRepositoryUntouched(t *testing.T) {
	s := newTestStore(t, Options{})
	repo := initUserRepo(t)
	userGit(t, repo, "config", "core.autocrlf", "true")
	writeFile(t, repo, "a.txt", "alpha\n")
	writeFile(t, repo, "b.txt", "bravo\n")
	userGit(t, repo, "add", ".")
	userGit(t, repo, "commit", "-q", "-m", "initial")
	writeFile(t, repo, "b.txt", "stashed change\n")
	userGit(t, repo, "stash", "push", "-q", "-m", "keep me")
	writeFile(t, repo, "a.txt", "alpha staged\n")
	userGit(t, repo, "add", "a.txt")
	writeFile(t, repo, "b.txt", "bravo unstaged\n")
	writeFile(t, repo, "c.txt", "untracked\n")

	before := fingerprint(t, filepath.Join(repo, ".git"))
	status := userGit(t, repo, "status", "--porcelain=v2", "-z")
	stash := userGit(t, repo, "stash", "list")

	runTurn(t, s, "sess1", "turn1", repo, nil)
	if got := userGit(t, repo, "status", "--porcelain=v2", "-z"); got != status {
		t.Fatalf("status changed:\n%q\n%q", status, got)
	}
	entry := runTurn(t, s, "sess1", "turn2", repo, func() {
		writeFile(t, repo, "a.txt", "alpha edited by the agent\n")
		writeFile(t, repo, "d.txt", "new\n")
	})
	mustDiff(t, s, "sess1", "turn2", ScopeTurn)
	mustDiff(t, s, "sess1", "turn1", ScopeSession)
	mustDiff(t, s, "sess1", "turn1", ScopeSince)

	if after := fingerprint(t, filepath.Join(repo, ".git")); !maps.Equal(before, after) {
		t.Fatalf("the user's .git changed:\nbefore %v\nafter  %v", before, after)
	}
	if got := userGit(t, repo, "stash", "list"); got != stash {
		t.Fatalf("stash changed: %q -> %q", stash, got)
	}
	if refs := userGit(t, repo, "for-each-ref", "refs/tars"); refs != "" {
		t.Fatalf("refs written to the user's repository: %s", refs)
	}
	if entry.Files != 2 || !samePath(entry.Root, canonicalPath(repo)) {
		t.Fatalf("entry = %+v", entry)
	}
}

// A checkpoint stores the bytes on disk. With autocrlf on, borrowing the
// user's conversion would normalize CRLF away and hide EOL-only edits.
func TestSnapshotsKeepExactBytes(t *testing.T) {
	s := newTestStore(t, Options{})
	repo := initUserRepo(t)
	userGit(t, repo, "config", "core.autocrlf", "true")
	writeFile(t, repo, ".gitattributes", "* text=auto eol=crlf\n")
	writeFile(t, repo, "lf.txt", "one\ntwo\n")
	writeFile(t, repo, "crlf.txt", "one\r\ntwo\r\n")
	writeFile(t, repo, "mixed.txt", "one\r\ntwo\n")

	entry := runTurn(t, s, "sess", "turn", repo, func() {
		writeFile(t, repo, "lf.txt", "one\r\ntwo\r\n")
		writeFile(t, repo, "crlf.txt", "one\ntwo\n")
	})
	for path, want := range map[string]string{"lf.txt": "one\ntwo\n", "crlf.txt": "one\r\ntwo\r\n", "mixed.txt": "one\r\ntwo\n"} {
		if got := string(shadowBlob(t, s, entry, entry.Start, path)); got != want {
			t.Fatalf("start %s = %q, want %q", path, got, want)
		}
	}
	if got := string(shadowBlob(t, s, entry, entry.End, "crlf.txt")); got != "one\ntwo\n" {
		t.Fatalf("end crlf.txt = %q", got)
	}
	files := filesByPath(mustDiff(t, s, "sess", "turn", ScopeTurn).Files)
	if len(files) != 2 || files["lf.txt"].Status != "modified" || files["crlf.txt"].Status != "modified" {
		t.Fatalf("EOL-only edits not seen: %+v", files)
	}
}

func TestDiffReportsEveryKindOfChange(t *testing.T) {
	s := newTestStore(t, Options{})
	root := t.TempDir() // not a repository
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, "line "+strings.Repeat("x", i))
	}
	writeFile(t, root, "a.txt", strings.Join(lines, "\n")+"\n")
	writeFile(t, root, "b.txt", strings.Repeat("a long file that is renamed without edits\n", 20))
	writeFile(t, root, "c.txt", "deleted\n")
	writeFile(t, root, "bin.dat", "bin\x00ary")

	entry := runTurn(t, s, "sess", "turn", root, func() {
		edited := slices.Clone(lines)
		edited[1] = "second line changed"
		edited[17] = "eighteenth line changed"
		writeFile(t, root, "a.txt", strings.Join(edited, "\n")+"\n")
		if err := os.Rename(filepath.Join(root, "b.txt"), filepath.Join(root, "d.txt")); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(root, "c.txt")); err != nil {
			t.Fatal(err)
		}
		writeFile(t, root, "e.txt", "added\n")
		writeFile(t, root, "bin.dat", "bin\x00ary changed")
	})
	if entry.Files != 5 {
		t.Fatalf("entry files = %d, want 5 (%+v)", entry.Files, entry)
	}
	files := filesByPath(mustDiff(t, s, "sess", "turn", ScopeTurn).Files)
	a := files["a.txt"]
	if a.Status != "modified" || len(a.Hunks) != 2 || a.Hunks[0].OldStart != 1 || a.Hunks[1].NewStart != 15 || a.Additions != 2 || a.Deletions != 2 {
		t.Fatalf("a.txt = %+v", a)
	}
	if d := files["d.txt"]; d.Status != "renamed" || d.OldPath != "b.txt" {
		t.Fatalf("d.txt = %+v", d)
	}
	if c := files["c.txt"]; c.Status != "deleted" {
		t.Fatalf("c.txt = %+v", c)
	}
	if e := files["e.txt"]; e.Status != "added" || e.Additions != 1 {
		t.Fatalf("e.txt = %+v", e)
	}
	if b := files["bin.dat"]; !b.Binary || b.Patch != "" || b.Hunks != nil {
		t.Fatalf("bin.dat = %+v", b)
	}
}

func TestPathsKeepTheirSpelling(t *testing.T) {
	s := newTestStore(t, Options{})
	root := t.TempDir()
	names := []string{"한글 폴더/파일 이름.txt", "a b.txt", "[br]acket.txt", "#hash.txt", "!bang.txt"}
	runTurn(t, s, "sess", "turn", root, func() {
		for _, name := range names {
			writeFile(t, root, name, "x\n")
		}
	})
	var got []string
	for _, f := range mustDiff(t, s, "sess", "turn", ScopeTurn).Files {
		got = append(got, f.Path)
	}
	slices.Sort(got)
	want := slices.Clone(names)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("paths = %q, want %q", got, want)
	}
}

func TestResolveRoot(t *testing.T) {
	s := newTestStore(t, Options{})
	ctx := context.Background()
	repo := initUserRepo(t)
	writeFile(t, repo, ".gitignore", "workspace/\n")
	writeFile(t, repo, "src/main.go", "package main\n")
	writeFile(t, repo, "workspace/artifacts/s1/note.txt", "x\n")

	check := func(cwd, want string) {
		t.Helper()
		got, err := s.ResolveRoot(ctx, cwd)
		if err != nil {
			t.Fatalf("resolve %s: %v", cwd, err)
		}
		gotReal, _ := filepath.EvalSymlinks(got)
		wantReal, _ := filepath.EvalSymlinks(want)
		if !samePath(gotReal, wantReal) {
			t.Fatalf("resolve %s = %s, want %s", cwd, got, want)
		}
	}
	check(filepath.Join(repo, "src"), repo)
	ignored := filepath.Join(repo, "workspace", "artifacts", "s1")
	check(ignored, ignored)
	plain := t.TempDir()
	check(plain, plain)
}

func TestLimitsLeaveOutLargeFilesAndSkipLargeTrees(t *testing.T) {
	s := newTestStore(t, Options{Limits: Limits{MaxFiles: 5, MaxBytes: 1 << 20, MaxFileBytes: 100}})
	root := t.TempDir()
	writeFile(t, root, "small.txt", "small\n")
	writeFile(t, root, "big.bin", strings.Repeat("b", 200))

	entry := runTurn(t, s, "sess", "turn1", root, func() {
		writeFile(t, root, "big.bin", strings.Repeat("c", 300))
		writeFile(t, root, "small.txt", "small edited\n")
	})
	if !slices.Equal(entry.Unknown, []string{"big.bin"}) {
		t.Fatalf("unknown = %q", entry.Unknown)
	}
	files := filesByPath(mustDiff(t, s, "sess", "turn1", ScopeTurn).Files)
	if _, ok := files["big.bin"]; ok || len(files) != 1 {
		t.Fatalf("diff should hide the oversize file: %+v", files)
	}

	for i := range 10 {
		writeFile(t, root, filepath.Join("many", strings.Repeat("f", i+1)+".txt"), "x\n")
	}
	skipped := runTurn(t, s, "sess", "turn2", root, nil)
	if skipped.Skipped != SkipTooManyFiles || skipped.Start != "" {
		t.Fatalf("expected a skipped turn, got %+v", skipped)
	}
	if _, err := s.Diff(context.Background(), "sess", "turn2", ScopeTurn, ""); err == nil {
		t.Fatal("diff of a skipped turn should fail")
	}
}

func TestNestedRepositoriesAreLeftOut(t *testing.T) {
	s := newTestStore(t, Options{})
	root := t.TempDir()
	nested := filepath.Join(root, "vendor", "lib")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	userGit(t, nested, "init", "-q")
	writeFile(t, nested, "lib.go", "package lib\n")
	writeFile(t, root, "main.txt", "x\n")

	entry := runTurn(t, s, "sess", "turn", root, func() {
		writeFile(t, nested, "lib.go", "package lib // edited\n")
		writeFile(t, root, "main.txt", "y\n")
	})
	if !slices.Contains(entry.Unknown, "vendor/lib/") {
		t.Fatalf("unknown = %q", entry.Unknown)
	}
	for _, f := range mustDiff(t, s, "sess", "turn", ScopeTurn).Files {
		if strings.HasPrefix(f.Path, "vendor/") {
			t.Fatalf("nested repository content leaked into the diff: %s", f.Path)
		}
	}
}

func TestStoreDirectoryInsideTheRootIsNotRecorded(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	s, err := Open(filepath.Join(root, "_shared", "checkpoints"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "notes.txt", "x\n")
	runTurn(t, s, "sess", "turn1", root, nil)
	runTurn(t, s, "sess", "turn2", root, func() { writeFile(t, root, "notes.txt", "y\n") })
	for _, f := range mustDiff(t, s, "sess", "turn1", ScopeSince).Files {
		if strings.HasPrefix(f.Path, "_shared/") {
			t.Fatalf("store files recorded: %s", f.Path)
		}
	}
}

func TestTrackedButIgnoredFilesAreRecorded(t *testing.T) {
	s := newTestStore(t, Options{})
	repo := initUserRepo(t)
	writeFile(t, repo, ".gitignore", "*.local\n")
	writeFile(t, repo, "app.local", "setting=1\n")
	userGit(t, repo, "add", ".gitignore")
	userGit(t, repo, "add", "-f", "app.local")
	userGit(t, repo, "commit", "-q", "-m", "initial")

	runTurn(t, s, "sess", "turn", repo, func() { writeFile(t, repo, "app.local", "setting=2\n") })
	files := filesByPath(mustDiff(t, s, "sess", "turn", ScopeTurn).Files)
	if files["app.local"].Status != "modified" {
		t.Fatalf("tracked-but-ignored file missed: %+v", files)
	}
}

func TestScopesSpanTurnsAndLaterEdits(t *testing.T) {
	s := newTestStore(t, Options{})
	root := t.TempDir()
	writeFile(t, root, "one.txt", "1\n")
	runTurn(t, s, "sess", "turn1", root, func() { writeFile(t, root, "one.txt", "1 edited\n") })
	runTurn(t, s, "sess", "turn2", root, func() { writeFile(t, root, "two.txt", "2\n") })
	writeFile(t, root, "three.txt", "edited outside any turn\n")

	paths := func(r DiffResult) []string {
		var out []string
		for _, f := range r.Files {
			out = append(out, f.Path)
		}
		slices.Sort(out)
		return out
	}
	if got := paths(mustDiff(t, s, "sess", "turn1", ScopeTurn)); !slices.Equal(got, []string{"one.txt"}) {
		t.Fatalf("turn scope = %q", got)
	}
	if got := paths(mustDiff(t, s, "sess", "turn2", ScopeSession)); !slices.Equal(got, []string{"one.txt", "two.txt"}) {
		t.Fatalf("session scope = %q", got)
	}
	if got := paths(mustDiff(t, s, "sess", "turn1", ScopeSince)); !slices.Equal(got, []string{"one.txt", "three.txt", "two.txt"}) {
		t.Fatalf("since scope = %q", got)
	}
}

func TestStaleIndexLockDoesNotBlockTheNextTurn(t *testing.T) {
	s := newTestStore(t, Options{})
	root := t.TempDir()
	first := runTurn(t, s, "sess", "turn1", root, nil)
	if err := os.WriteFile(filepath.Join(s.dir, "shadow", first.Shadow+".git", "index.lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if next := runTurn(t, s, "sess", "turn2", root, nil); next.Skipped != "" {
		t.Fatalf("turn after a stale lock was skipped: %+v", next)
	}
}

func TestRetentionDropSessionAndSweep(t *testing.T) {
	s := newTestStore(t, Options{KeepTurns: 2})
	root := t.TempDir()
	other := t.TempDir()
	var first Entry
	for i, id := range []string{"turn1", "turn2", "turn3"} {
		e := runTurn(t, s, "keep", id, root, func() { writeFile(t, root, "f.txt", strings.Repeat("x", i+1)) })
		if i == 0 {
			first = e
		}
	}
	turns, err := s.List("keep")
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].TurnID != "turn2" {
		t.Fatalf("kept %+v", turns)
	}
	for _, ref := range shadowRefs(t, s, first) {
		if strings.Contains(ref, "/turn1/") {
			t.Fatalf("dropped turn still has ref %s", ref)
		}
	}

	gone := runTurn(t, s, "gone", "turn1", root, nil)
	lonely := runTurn(t, s, "gone", "turn2", other, nil)
	if err := s.SweepOrphans(context.Background(), func(id string) bool { return id == "keep" }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.indexPath("gone")); !os.IsNotExist(err) {
		t.Fatalf("index of a deleted session survived: %v", err)
	}
	for _, ref := range shadowRefs(t, s, gone) {
		if strings.Contains(ref, "/gone/") {
			t.Fatalf("ref of a deleted session survived: %s", ref)
		}
	}
	if _, err := os.Stat(filepath.Join(s.dir, "shadow", lonely.Shadow+".git")); !os.IsNotExist(err) {
		t.Fatalf("shadow with no sessions left survived: %v", err)
	}
	if turns, _ := s.List("keep"); len(turns) != 2 {
		t.Fatalf("live session lost turns: %+v", turns)
	}

	if err := s.DropSession(context.Background(), "keep"); err != nil {
		t.Fatal(err)
	}
	if refs := shadowRefs(t, s, first); len(refs) != 0 {
		t.Fatalf("refs left after DropSession: %q", refs)
	}
}

func TestIDsAreValidated(t *testing.T) {
	s := newTestStore(t, Options{})
	ctx := context.Background()
	for _, bad := range []string{"", "..", "../x", "a/b", "a b", `a\b`} {
		if _, err := s.BeginTurn(ctx, bad, "turn", t.TempDir(), ""); err == nil {
			t.Fatalf("session id %q accepted", bad)
		}
		if _, err := s.BeginTurn(ctx, "sess", bad, t.TempDir(), ""); err == nil {
			t.Fatalf("turn id %q accepted", bad)
		}
	}
}

func TestExcludeLiteralEscapesPatternCharacters(t *testing.T) {
	cases := map[string]string{
		"a.txt":       "/a.txt",
		"[x]*?.txt":   `/\[x]\*\?.txt`,
		"dir/trail  ": `/dir/trail\ \ `,
		`back\slash`:  `/back\\slash`,
		"!bang":       "/!bang",
	}
	for in, want := range cases {
		if got := excludeLiteral(in); got != want {
			t.Fatalf("excludeLiteral(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseHunks(t *testing.T) {
	patch := "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1,3 +1,4 @@ func\n a\n+b\n@@ -10 +11,0 @@\n-z\n"
	hunks := parseHunks(patch)
	if len(hunks) != 2 {
		t.Fatalf("hunks = %+v", hunks)
	}
	if h := hunks[0]; h.ID != "h0" || h.OldStart != 1 || h.OldLines != 3 || h.NewStart != 1 || h.NewLines != 4 {
		t.Fatalf("first = %+v", h)
	}
	if h := hunks[1]; h.ID != "h1" || h.OldStart != 10 || h.OldLines != 1 || h.NewStart != 11 || h.NewLines != 0 {
		t.Fatalf("second = %+v", h)
	}
}
