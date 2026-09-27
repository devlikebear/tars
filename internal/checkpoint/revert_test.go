package checkpoint

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// thirty returns lines "1".."30", with edits applied by line number.
func thirty(edits map[int]string) string {
	var b strings.Builder
	for i := 1; i <= 30; i++ {
		line, ok := edits[i]
		if !ok {
			line = "line " + string(rune('0'+i/10)) + string(rune('0'+i%10))
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func readText(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

func mustRevert(t *testing.T, s *Store, sessionID, turnID string, req RevertRequest) RevertResult {
	t.Helper()
	result, err := s.Revert(context.Background(), sessionID, turnID, req)
	if err != nil {
		t.Fatalf("revert %s: %v (%+v)", turnID, err, result)
	}
	return result
}

// Three separate hunks in one file (lines 2, 15, and 28 edited).
func threeHunkTurn(t *testing.T) (*Store, string) {
	t.Helper()
	s := newTestStore(t, Options{})
	root := t.TempDir()
	writeFile(t, root, "a.txt", thirty(nil))
	runTurn(t, s, "sess", "turn", root, func() {
		writeFile(t, root, "a.txt", thirty(map[int]string{2: "two", 15: "fifteen", 28: "twenty-eight"}))
	})
	return s, root
}

func TestSpliceHunks(t *testing.T) {
	start := []byte("a\r\nb\r\nc\r\nd")
	end := []byte("a\r\nB\r\nc\r\nD")
	hunks := []Hunk{
		{ID: "h0", OldStart: 2, OldLines: 1, NewStart: 2, NewLines: 1},
		{ID: "h1", OldStart: 4, OldLines: 1, NewStart: 4, NewLines: 1},
	}
	cases := []struct {
		chosen []int
		want   string
	}{
		{nil, "a\r\nB\r\nc\r\nD"},
		{[]int{0, 1}, "a\r\nb\r\nc\r\nd"},
		{[]int{1}, "a\r\nB\r\nc\r\nd"},
		{[]int{0, 0}, "a\r\nb\r\nc\r\nD"},
	}
	for _, tc := range cases {
		got, err := spliceHunks(start, end, hunks, tc.chosen)
		if err != nil || string(got) != tc.want {
			t.Errorf("chosen %v: %q %v, want %q", tc.chosen, got, err, tc.want)
		}
	}
	// An empty range names the line before it: lines added to an empty file.
	got, err := spliceHunks(nil, []byte("x\ny\n"), []Hunk{{ID: "h0", OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 2}}, []int{0})
	if err != nil || len(got) != 0 {
		t.Fatalf("added lines: %q %v", got, err)
	}
	if _, err := spliceHunks(start, end, hunks, []int{5}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown hunk: %v", err)
	}
	if _, err := spliceHunks(start, end, []Hunk{{ID: "h0", OldStart: 9, OldLines: 1, NewStart: 9, NewLines: 1}}, []int{0}); err == nil {
		t.Fatal("out-of-range hunk accepted")
	}
	if splitLines(nil) != nil || !binaryContent([]byte("a\x00b")) || binaryContent([]byte("text")) {
		t.Fatal("helpers")
	}
}

func TestRevertOneHunk(t *testing.T) {
	s, root := threeHunkTurn(t)
	diff := mustDiff(t, s, "sess", "turn", ScopeTurn)
	if len(diff.Files) != 1 || len(diff.Files[0].Hunks) != 3 {
		t.Fatalf("want one file with three hunks: %+v", diff.Files)
	}

	preview := mustRevert(t, s, "sess", "turn", RevertRequest{Files: []RevertFile{{Path: "a.txt", HunkIDs: []string{"h1"}}}})
	if preview.Applied || preview.RevertID != "" || preview.Files[0].Outcome != RevertWrite {
		t.Fatalf("preview = %+v", preview)
	}
	if !strings.Contains(readText(t, root, "a.txt"), "fifteen") {
		t.Fatal("a preview wrote the file")
	}

	result := mustRevert(t, s, "sess", "turn", RevertRequest{Apply: true, Files: []RevertFile{{Path: "a.txt", HunkIDs: []string{"h1"}}}})
	if !result.Applied || result.RevertID == "" {
		t.Fatalf("result = %+v", result)
	}
	if got, want := readText(t, root, "a.txt"), thirty(map[int]string{2: "two", 28: "twenty-eight"}); got != want {
		t.Fatalf("after reverting h1:\n%s", got)
	}
	reverts, err := s.Reverts("sess")
	if err != nil || len(reverts) != 1 || reverts[0].Targets[0].HunkIDs[0] != "h1" || reverts[0].Files[0] != "a.txt" {
		t.Fatalf("reverts = %+v %v", reverts, err)
	}

	// Choosing every hunk is the whole file.
	all := mustRevert(t, s, "sess", "turn", RevertRequest{Apply: true, Files: []RevertFile{{Path: "a.txt", HunkIDs: []string{"h0", "h1", "h2"}}}})
	if all.Files[0].Outcome != RevertMerge || readText(t, root, "a.txt") != thirty(nil) {
		t.Fatalf("all hunks = %+v\n%s", all, readText(t, root, "a.txt"))
	}
}

// Every kind of change goes back: modified, added, deleted, and renamed.
func TestRevertWholeTurn(t *testing.T) {
	s := newTestStore(t, Options{})
	root := t.TempDir()
	writeFile(t, root, "keep.txt", "keep\n")
	writeFile(t, root, "edit.txt", "before\n")
	writeFile(t, root, "gone.txt", "delete me\n")
	writeFile(t, root, "old/name.txt", thirty(nil))
	before := fingerprint(t, root)
	runTurn(t, s, "sess", "turn", root, func() {
		writeFile(t, root, "edit.txt", "after\n")
		writeFile(t, root, "new/file.txt", "added\n")
		if err := os.Remove(filepath.Join(root, "gone.txt")); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "new"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(root, "old", "name.txt"), filepath.Join(root, "new", "name.txt")); err != nil {
			t.Fatal(err)
		}
	})
	result := mustRevert(t, s, "sess", "turn", RevertRequest{Apply: true})
	if result.Conflicts != 0 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	after := fingerprint(t, root)
	if !maps.Equal(before, after) {
		t.Fatalf("work tree after revert:\n%v\nwant\n%v", after, before)
	}
}

// Edits made after the turn elsewhere in the file survive the revert.
func TestRevertMergesLaterEdits(t *testing.T) {
	s, root := threeHunkTurn(t)
	writeFile(t, root, "a.txt", thirty(map[int]string{2: "two", 15: "fifteen", 28: "twenty-eight", 30: "mine"}))
	result := mustRevert(t, s, "sess", "turn", RevertRequest{Apply: true, Files: []RevertFile{{Path: "a.txt", HunkIDs: []string{"h0"}}}})
	if result.Files[0].Outcome != RevertMerge {
		t.Fatalf("result = %+v", result)
	}
	if got, want := readText(t, root, "a.txt"), thirty(map[int]string{15: "fifteen", 28: "twenty-eight", 30: "mine"}); got != want {
		t.Fatalf("merged:\n%s", got)
	}
}

func TestRevertConflictNeedsForce(t *testing.T) {
	s, root := threeHunkTurn(t)
	writeFile(t, root, "a.txt", thirty(map[int]string{2: "TWO!", 15: "fifteen", 28: "twenty-eight"}))
	req := RevertRequest{Apply: true, Files: []RevertFile{{Path: "a.txt", HunkIDs: []string{"h0"}}}}
	result, err := s.Revert(context.Background(), "sess", "turn", req)
	if !errors.Is(err, ErrConflict) || result.Conflicts != 1 || !strings.Contains(result.Files[0].Merged, "<<<<<<< current") {
		t.Fatalf("result = %+v %v", result, err)
	}
	if !strings.Contains(readText(t, root, "a.txt"), "TWO!") {
		t.Fatal("a refused revert wrote the file")
	}
	req.Force = true
	forced := mustRevert(t, s, "sess", "turn", req)
	if !forced.Files[0].Forced || readText(t, root, "a.txt") != thirty(map[int]string{15: "fifteen", 28: "twenty-eight"}) {
		t.Fatalf("forced = %+v\n%s", forced, readText(t, root, "a.txt"))
	}
}

// A file the user deleted after the turn cannot be merged into.
func TestRevertOfMissingFileConflicts(t *testing.T) {
	s, root := threeHunkTurn(t)
	if err := os.Remove(filepath.Join(root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	result, err := s.Revert(context.Background(), "sess", "turn", RevertRequest{Apply: true})
	if !errors.Is(err, ErrConflict) || result.Files[0].Outcome != RevertConflict || result.Files[0].Merged != "" {
		t.Fatalf("result = %+v %v", result, err)
	}
}

func TestRevertSinceRestoresEverythingAfter(t *testing.T) {
	s := newTestStore(t, Options{})
	root := t.TempDir()
	writeFile(t, root, "a.txt", "a0\n")
	before := fingerprint(t, root)
	runTurn(t, s, "sess", "t1", root, func() { writeFile(t, root, "a.txt", "a1\n") })
	runTurn(t, s, "sess", "t2", root, func() {
		writeFile(t, root, "a.txt", "a2\n")
		writeFile(t, root, "b.txt", "b2\n")
	})
	writeFile(t, root, "c.txt", "by hand\n")

	only := mustRevert(t, s, "sess", "t1", RevertRequest{Scope: ScopeSince, Apply: true, Files: []RevertFile{{Path: "b.txt"}}})
	if len(only.Files) != 1 || fileExists(root, "b.txt") || readText(t, root, "a.txt") != "a2\n" {
		t.Fatalf("since, one file = %+v", only)
	}
	mustRevert(t, s, "sess", "t1", RevertRequest{Scope: ScopeSince, Apply: true})
	if got := fingerprint(t, root); !maps.Equal(got, before) {
		t.Fatalf("after since revert: %v", got)
	}
}

func fileExists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

func TestUndoPutsTheRevertBack(t *testing.T) {
	s, root := threeHunkTurn(t)
	after := readText(t, root, "a.txt")
	result := mustRevert(t, s, "sess", "turn", RevertRequest{Apply: true})
	if readText(t, root, "a.txt") != thirty(nil) {
		t.Fatal("revert did not apply")
	}
	undo, err := s.Undo(context.Background(), "sess", result.RevertID, false)
	if err != nil || !undo.Applied || readText(t, root, "a.txt") != after {
		t.Fatalf("undo = %+v %v", undo, err)
	}
	if _, err := s.Undo(context.Background(), "sess", result.RevertID, false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("second undo: %v", err)
	}
	if _, err := s.Undo(context.Background(), "sess", "nope", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown revert: %v", err)
	}
	reverts, _ := s.Reverts("sess")
	if reverts[0].UndoneAt.IsZero() {
		t.Fatal("undo not recorded")
	}
}

func TestUndoConflictNeedsForce(t *testing.T) {
	s, root := threeHunkTurn(t)
	result := mustRevert(t, s, "sess", "turn", RevertRequest{Apply: true, Files: []RevertFile{{Path: "a.txt", HunkIDs: []string{"h0"}}}})
	writeFile(t, root, "a.txt", thirty(map[int]string{2: "mine", 15: "fifteen", 28: "twenty-eight"}))
	undo, err := s.Undo(context.Background(), "sess", result.RevertID, false)
	if !errors.Is(err, ErrConflict) || undo.Conflicts != 1 {
		t.Fatalf("undo = %+v %v", undo, err)
	}
	if _, err := s.Undo(context.Background(), "sess", result.RevertID, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readText(t, root, "a.txt"), "two\n") {
		t.Fatalf("forced undo:\n%s", readText(t, root, "a.txt"))
	}
}

func TestRevertWhileATurnRunsIsBusy(t *testing.T) {
	s, root := threeHunkTurn(t)
	ctx := context.Background()
	running, err := s.BeginTurn(ctx, "sess", "running", root, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Revert(ctx, "sess", "turn", RevertRequest{Apply: true}); !errors.Is(err, ErrBusy) {
		t.Fatalf("revert during a turn: %v", err)
	}
	if _, err := s.Revert(ctx, "sess", "turn", RevertRequest{}); err != nil {
		t.Fatalf("a preview is fine during a turn: %v", err)
	}
	if _, err := s.Undo(ctx, "sess", "any", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("undo lookup comes first: %v", err)
	}
	if _, err := running.End(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestUndoWhileATurnRunsIsBusy(t *testing.T) {
	s, root := threeHunkTurn(t)
	ctx := context.Background()
	result := mustRevert(t, s, "sess", "turn", RevertRequest{Apply: true})
	running, err := s.BeginTurn(ctx, "sess", "running", root, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = running.End(ctx) }()
	if _, err := s.Undo(ctx, "sess", result.RevertID, false); !errors.Is(err, ErrBusy) {
		t.Fatalf("undo during a turn: %v", err)
	}
}

func TestRevertRejectsBadRequests(t *testing.T) {
	s, root := threeHunkTurn(t)
	runTurn(t, s, "sess", "added", root, func() { writeFile(t, root, "new.txt", "one\ntwo\n") })
	ctx := context.Background()
	cases := []struct {
		name   string
		sessID string
		turnID string
		req    RevertRequest
		want   error
	}{
		{"bad session", "../x", "turn", RevertRequest{}, ErrInvalid},
		{"unknown turn", "sess", "nope", RevertRequest{}, ErrNotFound},
		{"bad scope", "sess", "turn", RevertRequest{Scope: ScopeSession}, ErrInvalid},
		{"escaping path", "sess", "turn", RevertRequest{Files: []RevertFile{{Path: "../a.txt"}}}, ErrInvalid},
		{"unchanged path", "sess", "turn", RevertRequest{Files: []RevertFile{{Path: "other.txt"}}}, ErrInvalid},
		{"unknown hunk", "sess", "turn", RevertRequest{Files: []RevertFile{{Path: "a.txt", HunkIDs: []string{"h9"}}}}, ErrInvalid},
		{"hunk of an added file", "sess", "added", RevertRequest{Files: []RevertFile{{Path: "new.txt", HunkIDs: []string{"h0"}}}}, ErrInvalid},
		{"since with hunks", "sess", "turn", RevertRequest{Scope: ScopeSince, Files: []RevertFile{{Path: "a.txt", HunkIDs: []string{"h0"}}}}, ErrInvalid},
	}
	for _, tc := range cases {
		if _, err := s.Revert(ctx, tc.sessID, tc.turnID, tc.req); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, err, tc.want)
		}
	}
	if _, err := s.Undo(ctx, "../x", "r", false); !errors.Is(err, ErrInvalid) {
		t.Errorf("undo with a bad session: %v", err)
	}
	if _, err := s.Reverts("a/b"); !errors.Is(err, ErrInvalid) {
		t.Errorf("reverts with a bad session: %v", err)
	}
}

func TestRevertOfSkippedTurn(t *testing.T) {
	limits := DefaultLimits
	limits.MaxFiles = 1
	s := newTestStore(t, Options{Limits: limits})
	root := t.TempDir()
	runTurn(t, s, "sess", "turn", root, func() {
		writeFile(t, root, "a.txt", "a\n")
		writeFile(t, root, "b.txt", "b\n")
	})
	if _, err := s.Revert(context.Background(), "sess", "turn", RevertRequest{}); !errors.Is(err, ErrSkipped) {
		t.Fatalf("skipped turn: %v", err)
	}
}

// Reverting reads and writes the work tree only: the user's .git is as it was.
func TestRevertLeavesTheUserRepositoryUntouched(t *testing.T) {
	s := newTestStore(t, Options{})
	repo := initUserRepo(t)
	writeFile(t, repo, "a.txt", thirty(nil))
	userGit(t, repo, "add", ".")
	userGit(t, repo, "commit", "-q", "-m", "initial")
	runTurn(t, s, "sess", "turn", repo, func() { writeFile(t, repo, "a.txt", thirty(map[int]string{3: "three"})) })
	before := fingerprint(t, filepath.Join(repo, ".git"))
	result := mustRevert(t, s, "sess", "turn", RevertRequest{Apply: true})
	if _, err := s.Undo(context.Background(), "sess", result.RevertID, false); err != nil {
		t.Fatal(err)
	}
	if after := fingerprint(t, filepath.Join(repo, ".git")); !maps.Equal(before, after) {
		t.Fatal("the user's .git changed")
	}
}

// Old reverts drop out of the record, and their refs with them.
func TestRevertRecordsAreCapped(t *testing.T) {
	saved := keepReverts
	keepReverts = 2
	t.Cleanup(func() { keepReverts = saved })
	s, root := threeHunkTurn(t)
	var ids []string
	for i := 0; i < 3; i++ {
		result := mustRevert(t, s, "sess", "turn", RevertRequest{Apply: true})
		ids = append(ids, result.RevertID)
		if _, err := s.Undo(context.Background(), "sess", result.RevertID, false); err != nil {
			t.Fatal(err)
		}
	}
	reverts, _ := s.Reverts("sess")
	if len(reverts) != 2 || reverts[0].ID != ids[1] {
		t.Fatalf("reverts = %+v", reverts)
	}
	sh, err := s.openShadow(reverts[0].Shadow)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := s.refsUnder(context.Background(), sh, refNamespace+"sess/reverts/")
	if err != nil || len(refs) != 4 {
		t.Fatalf("refs = %v %v", refs, err)
	}
	_ = root
}

// A file that already has the reverted content is left alone, and a revert
// that writes nothing records nothing.
func TestRevertOfAlreadyRevertedFile(t *testing.T) {
	s, root := threeHunkTurn(t)
	writeFile(t, root, "a.txt", thirty(nil))
	result := mustRevert(t, s, "sess", "turn", RevertRequest{Apply: true})
	if result.Applied || result.RevertID != "" || result.Files[0].Outcome != RevertUnchanged {
		t.Fatalf("result = %+v", result)
	}
}

// A link or directory where the file was is reported, never followed.
func TestRevertRefusesNonRegularFiles(t *testing.T) {
	s, root := threeHunkTurn(t)
	if err := os.Remove(filepath.Join(root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "a.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := mustRevert(t, s, "sess", "turn", RevertRequest{})
	if result.Files[0].Outcome != RevertFailed || !strings.Contains(result.Files[0].Detail, "not a regular file") {
		t.Fatalf("result = %+v", result)
	}
}

func TestRevertLabelsAndClip(t *testing.T) {
	long := strings.Repeat("x\n", MaxPatchBytes)
	if got := clipText(long); len(got) > MaxPatchBytes || !strings.HasSuffix(got, "\n") {
		t.Fatalf("clip = %d bytes", len(got))
	}
	if clipText("short") != "short" || firstNonEmpty("", "b") != "b" || firstNonEmpty() != "" {
		t.Fatal("helpers")
	}
	if id := newRevertID(fixedTime()); !safeID.MatchString(id) {
		t.Fatalf("revert id %q is not a safe id", id)
	}
}

func fixedTime() time.Time { return time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC) }

// A file too large for the undo snapshot is never written: undoing would
// have nothing to put back.
func TestRevertRefusesFilesItCannotBackUp(t *testing.T) {
	limits := DefaultLimits
	limits.MaxFileBytes = 64
	s := newTestStore(t, Options{Limits: limits})
	root := t.TempDir()
	writeFile(t, root, "a.txt", "small\n")
	runTurn(t, s, "sess", "turn", root, func() { writeFile(t, root, "a.txt", "edited\n") })
	writeFile(t, root, "a.txt", strings.Repeat("grown past the limit\n", 10))
	result := mustRevert(t, s, "sess", "turn", RevertRequest{Apply: true, Force: true})
	if result.Failed != 1 || !strings.Contains(result.Files[0].Detail, "back up") || result.Applied {
		t.Fatalf("result = %+v", result)
	}
	if !strings.HasPrefix(readText(t, root, "a.txt"), "grown") {
		t.Fatal("the file was written")
	}
}
