package checkpoint

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrimeRecordsNoTurn(t *testing.T) {
	s := newTestStore(t, Options{})
	root := t.TempDir()
	writeFile(t, root, "a.txt", "a\n")
	if err := s.Prime(context.Background(), root); err != nil {
		t.Fatalf("prime: %v", err)
	}
	if turns, err := s.List("sess"); err != nil || len(turns) != 0 {
		t.Fatalf("prime recorded a turn: %+v %v", turns, err)
	}
	entry := runTurn(t, s, "sess", "turn", root, func() { writeFile(t, root, "a.txt", "b\n") })
	if entry.Files != 1 {
		t.Fatalf("turn after prime = %+v", entry)
	}
}

func TestDiffArgumentsAndFallback(t *testing.T) {
	s := newTestStore(t, Options{})
	ctx := context.Background()
	root := t.TempDir()
	writeFile(t, root, "a.txt", "a\n")
	writeFile(t, root, "b.txt", "b\n")
	entry := runTurn(t, s, "sess", "turn", root, func() {
		writeFile(t, root, "a.txt", "a2\n")
		writeFile(t, root, "b.txt", "b2\n")
	})

	if _, err := s.Diff(ctx, "sess", "missing", ScopeTurn, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing turn: %v", err)
	}
	if _, err := s.Diff(ctx, "sess", "turn", Scope("sideways"), ""); err == nil {
		t.Fatal("unknown scope accepted")
	}
	if _, err := s.Diff(ctx, "../x", "turn", ScopeTurn, ""); err == nil {
		t.Fatal("bad session id accepted")
	}
	one, err := s.Diff(ctx, "sess", "turn", "", "b.txt")
	if err != nil || one.Scope != ScopeTurn || len(one.Files) != 1 || one.Files[0].Path != "b.txt" {
		t.Fatalf("path filter = %+v %v", one, err)
	}

	sh, err := s.openShadow(entry.Shadow)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := s.nameStatus(ctx, sh, entry.Start, entry.End, "")
	if err != nil {
		t.Fatal(err)
	}
	fallback, err := s.diffFilesOneByOne(ctx, sh, entry.Start, entry.End, entries, nil)
	if err != nil {
		t.Fatal(err)
	}
	combined, err := s.diffFiles(ctx, sh, entry.Start, entry.End, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(fallback) != len(combined) {
		t.Fatalf("fallback %d files, combined %d", len(fallback), len(combined))
	}
	for i := range combined {
		if fallback[i].Path != combined[i].Path || fallback[i].Patch != combined[i].Patch || fallback[i].Additions != combined[i].Additions {
			t.Fatalf("fallback[%d] = %+v, combined = %+v", i, fallback[i], combined[i])
		}
	}
}

func TestLongPatchesAreTruncatedWithoutHunks(t *testing.T) {
	var b strings.Builder
	b.WriteString("diff --git a/x b/x\n@@ -1,1 +1,1 @@\n")
	for b.Len() <= MaxPatchBytes {
		b.WriteString("+" + strings.Repeat("y", 100) + "\n")
	}
	f := buildFileDiff(statusEntry{status: "modified", path: "x"}, numstatEntry{additions: 1}, b.String())
	if !f.Truncated || f.Hunks != nil || len(f.Patch) > MaxPatchBytes || !strings.HasSuffix(f.Patch, "\n") {
		t.Fatalf("truncated diff = truncated:%v hunks:%v len:%d", f.Truncated, f.Hunks, len(f.Patch))
	}
}

func TestClipPreview(t *testing.T) {
	if got := clipPreview("  first line\nsecond line"); got != "first line" {
		t.Fatalf("multi-line preview = %q", got)
	}
	long := strings.Repeat("가", 200)
	got := clipPreview(long)
	if len([]rune(got)) != 120 || !strings.HasSuffix(got, "…") {
		t.Fatalf("long preview = %d runes", len([]rune(got)))
	}
}

func TestErrorMessages(t *testing.T) {
	cause := errors.New("exit status 1")
	withStderr := &gitError{args: []string{"add"}, stderr: "boom", err: cause}
	if withStderr.Error() != "checkpoint: git add: boom" || !errors.Is(withStderr, cause) {
		t.Fatalf("gitError = %q", withStderr.Error())
	}
	bare := &gitError{err: cause}
	if bare.Error() != "checkpoint: git: exit status 1" {
		t.Fatalf("bare gitError = %q", bare.Error())
	}
	skip := &skipError{reason: SkipTooLarge, detail: "9 bytes"}
	if reason, detail := skipFields(skip); reason != SkipTooLarge || detail != "9 bytes" || !strings.Contains(skip.Error(), "too_large") {
		t.Fatalf("skip = %s %s %q", reason, detail, skip.Error())
	}
	if reason, _ := skipFields(cause); reason != SkipFailed {
		t.Fatalf("plain error reason = %s", reason)
	}
	if long := trimStderr(strings.Repeat("x", 3000)); len(long) > 2010 {
		t.Fatalf("stderr not trimmed: %d", len(long))
	}
}

func TestShadowRefusesAnotherRootsDirectory(t *testing.T) {
	s := newTestStore(t, Options{})
	root := t.TempDir()
	gitDir := filepath.Join(s.dir, "shadow", shadowKey(root)+".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, shadowRootMarker), []byte("/somewhere/else\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.shadow(context.Background(), root); err == nil {
		t.Fatal("a shadow recorded for another root was reused")
	}
	if _, err := s.openShadow("../escape"); err == nil {
		t.Fatal("bad shadow key accepted")
	}
	entry := runTurn(t, s, "sess", "turn", root, nil)
	if entry.Skipped != SkipFailed {
		t.Fatalf("turn against a foreign shadow = %+v", entry)
	}
	if turn := (&Turn{entry: entry}); turn.Entry().TurnID != "turn" {
		t.Fatal("Entry accessor")
	}
}

func TestRelativeToRoot(t *testing.T) {
	root := t.TempDir()
	if got := relativeToRoot(root, filepath.Join(root, "sub", "f.txt")); got != "sub/f.txt" {
		t.Fatalf("absolute path = %q", got)
	}
	if got := relativeToRoot(root, "sub/f.txt"); got != "sub/f.txt" {
		t.Fatalf("relative path = %q", got)
	}
	if _, ok := relativeInside(root, root); ok {
		t.Fatal("root is not inside itself")
	}
}

func TestListRejectsBadSessionID(t *testing.T) {
	s := newTestStore(t, Options{})
	if _, err := s.List("a/b"); err == nil {
		t.Fatal("bad id accepted")
	}
	if err := s.DropSession(context.Background(), "a/b"); err == nil {
		t.Fatal("bad id accepted by DropSession")
	}
}
