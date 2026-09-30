package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func text(s string) fileSnapshot { return fileSnapshot{exists: true, data: []byte(s)} }

func TestComputeFileChangeOps(t *testing.T) {
	if _, ok := computeFileChange("a", fileSnapshot{}, fileSnapshot{}); ok {
		t.Fatal("nothing on either side is no change")
	}
	if _, ok := computeFileChange("a", text("x\n"), text("x\n")); ok {
		t.Fatal("identical content is no change")
	}

	created, ok := computeFileChange("a.txt", fileSnapshot{}, text("one\ntwo\n"))
	if !ok || created.Op != FileChangeCreate || created.Additions != 2 || created.Deletions != 0 {
		t.Fatalf("create: %+v", created)
	}
	want := []FileChangeHunk{{OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 2, Lines: []string{"+one", "+two"}}}
	if !reflect.DeepEqual(created.Hunks, want) {
		t.Fatalf("create hunks: %+v", created.Hunks)
	}

	deleted, ok := computeFileChange("a.txt", text("one\n"), fileSnapshot{})
	if !ok || deleted.Op != FileChangeDelete || deleted.Deletions != 1 || deleted.Hunks[0].OldStart != 1 || deleted.Hunks[0].NewStart != 0 {
		t.Fatalf("delete: %+v", deleted)
	}

	modified, ok := computeFileChange("a.txt", text("a\nb\nc\n"), text("a\nB\nc\nd\n"))
	if !ok || modified.Op != FileChangeModify || modified.Additions != 2 || modified.Deletions != 1 {
		t.Fatalf("modify: %+v", modified)
	}
	want = []FileChangeHunk{{OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 4, Lines: []string{" a", "-b", "+B", " c", "+d"}}}
	if !reflect.DeepEqual(modified.Hunks, want) {
		t.Fatalf("modify hunks: %+v", modified.Hunks)
	}
}

func TestComputeFileChangeNoNewlineAtEOF(t *testing.T) {
	change, ok := computeFileChange("a", text("a\nb"), text("a\nb\n"))
	if !ok || change.Additions != 1 || change.Deletions != 1 {
		t.Fatalf("eol change: %+v", change)
	}
	want := []string{" a", "-b", noNewlineMarker, "+b"}
	if !reflect.DeepEqual(change.Hunks[0].Lines, want) {
		t.Fatalf("lines: %q", change.Hunks[0].Lines)
	}
	same, _ := computeFileChange("a", text("x\nend"), text("y\nend"))
	if got := same.Hunks[0].Lines; !reflect.DeepEqual(got, []string{"-x", "+y", " end", noNewlineMarker}) {
		t.Fatalf("context without eol: %q", got)
	}
}

func TestComputeFileChangeSplitsDistantHunks(t *testing.T) {
	var before, after []string
	for i := 1; i <= 30; i++ {
		before = append(before, fmt.Sprintf("line %d", i))
		after = append(after, fmt.Sprintf("line %d", i))
	}
	after[1] = "changed 2"
	after[4] = "changed 5" // within 2*context of line 2: same hunk
	after[25] = "changed 26"
	change, ok := computeFileChange("f", text(strings.Join(before, "\n")+"\n"), text(strings.Join(after, "\n")+"\n"))
	if !ok || len(change.Hunks) != 2 || change.Additions != 3 || change.Deletions != 3 {
		t.Fatalf("hunks: %+v", change)
	}
	first, second := change.Hunks[0], change.Hunks[1]
	if first.OldStart != 1 || first.OldLines != 8 || second.OldStart != 23 || second.OldLines != 7 || second.NewStart != 23 {
		t.Fatalf("hunk ranges: %+v / %+v", first, second)
	}
}

func TestComputeFileChangeCaps(t *testing.T) {
	binary, ok := computeFileChange("img.png", text("a\n"), fileSnapshot{exists: true, data: []byte{0x89, 0, 1}})
	if !ok || !binary.Binary || binary.Additions != 0 || len(binary.Hunks) != 0 {
		t.Fatalf("binary: %+v", binary)
	}
	big, ok := computeFileChange("big", fileSnapshot{exists: true, tooBig: true}, text("x"))
	if !ok || !big.Truncated || big.Op != FileChangeModify {
		t.Fatalf("too big: %+v", big)
	}
	if got := snapshotWritten(make([]byte, maxFileChangeBytes+1)); !got.tooBig {
		t.Fatal("written content over the cap is too big")
	}

	// A changed region too large to align reports whole-region counts.
	var a, b strings.Builder
	for i := 0; i < 2100; i++ {
		fmt.Fprintf(&a, "a%d\n", i)
		fmt.Fprintf(&b, "b%d\n", i)
	}
	wide, ok := computeFileChange("w", text("head\n"+a.String()), text("head\n"+b.String()))
	if !ok || !wide.Truncated || wide.Additions != 2100 || wide.Deletions != 2100 || len(wide.Hunks) != 0 {
		t.Fatalf("wide: additions=%d deletions=%d truncated=%v hunks=%d", wide.Additions, wide.Deletions, wide.Truncated, len(wide.Hunks))
	}

	// Past the hunk budget, later hunks are dropped.
	var lines []string
	for i := 0; i < 1000; i++ {
		lines = append(lines, fmt.Sprintf("l%d", i))
	}
	created, _ := computeFileChange("c", fileSnapshot{}, text(strings.Join(lines, "\n")+"\n"))
	if !created.Truncated || created.Additions != 1000 || len(created.Hunks) != 0 {
		t.Fatalf("budget: truncated=%v additions=%d hunks=%d", created.Truncated, created.Additions, len(created.Hunks))
	}
	var edited []string
	edited = append(edited, lines...)
	for i := 0; i < 1000; i += 10 {
		edited[i] = "x"
	}
	many, _ := computeFileChange("m", text(strings.Join(lines, "\n")), text(strings.Join(edited, "\n")))
	if !many.Truncated || len(many.Hunks) == 0 || many.Additions != 100 {
		t.Fatalf("partial budget: truncated=%v hunks=%d additions=%d", many.Truncated, len(many.Hunks), many.Additions)
	}
}

func TestSnapshotFile(t *testing.T) {
	dir := t.TempDir()
	if got := snapshotFile(filepath.Join(dir, "missing")); got.exists {
		t.Fatal("missing file exists")
	}
	if got := snapshotFile(dir); got.exists {
		t.Fatal("a directory is not a file")
	}
	dotted := filepath.Join(dir, "a..b.txt")
	if err := os.WriteFile(dotted, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := snapshotFile(dotted); got.exists || got.data != nil {
		t.Fatalf("a path containing .. is not read: %+v", got)
	}
	if got := snapshotFile("relative.txt"); got.exists || got.data != nil {
		t.Fatalf("a relative path is not read: %+v", got)
	}
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := snapshotFile(path); !got.exists || string(got.data) != "hi\n" {
		t.Fatalf("snapshot: %+v", got)
	}
	if err := os.WriteFile(path, make([]byte, maxFileChangeBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := snapshotFile(path); !got.exists || !got.tooBig {
		t.Fatalf("big snapshot: exists=%v tooBig=%v", got.exists, got.tooBig)
	}
}

// runTool executes a tool that must succeed.
func runTool(t *testing.T, tl Tool, args string) Result {
	t.Helper()
	res, err := tl.Execute(context.Background(), json.RawMessage(args))
	if err != nil || res.IsError {
		t.Fatalf("%s: %v %s", tl.Name, err, res.Text())
	}
	return res
}

// singleChange returns a result's only change.
func singleChange(t *testing.T, res Result) FileChange {
	t.Helper()
	if len(res.FileChanges) != 1 {
		t.Fatalf("expected one change, got %+v", res.FileChanges)
	}
	return res.FileChanges[0]
}

func TestWriteFileReportsChangeOutsideModelText(t *testing.T) {
	write := NewWriteFileTool(t.TempDir())
	res := runTool(t, write, `{"path":"d/a.txt","content":"one\n"}`)
	if got := singleChange(t, res); got.Path != "d/a.txt" || got.Op != FileChangeCreate {
		t.Fatalf("write changes: %+v", got)
	}
	if strings.Contains(res.Text(), "additions") {
		t.Fatalf("the model's text must not carry the change: %s", res.Text())
	}
	encoded, _ := json.Marshal(res)
	if strings.Contains(string(encoded), "FileChanges") || strings.Contains(string(encoded), "hunks") {
		t.Fatalf("FileChanges must not serialize: %s", encoded)
	}
}

func TestWriteFileReportsOverwrites(t *testing.T) {
	write := NewWriteFileTool(t.TempDir())
	runTool(t, write, `{"path":"d/a.txt","content":"one\n"}`)
	if res := runTool(t, write, `{"path":"d/a.txt","content":"one\n"}`); len(res.FileChanges) != 0 {
		t.Fatalf("rewriting the same content: %+v", res.FileChanges)
	}
	got := singleChange(t, runTool(t, write, `{"path":"d/a.txt","content":"two\n"}`))
	if got.Op != FileChangeModify || got.Additions != 1 || got.Deletions != 1 {
		t.Fatalf("overwrite changes: %+v", got)
	}
}

func TestEditFileReportsChange(t *testing.T) {
	root := t.TempDir()
	runTool(t, NewWriteFileTool(root), `{"path":"d/a.txt","content":"two\n"}`)
	edit := NewEditFileTool(root)
	got := singleChange(t, runTool(t, edit, `{"path":"d/a.txt","old_text":"two","new_text":"two\nthree"}`))
	if got.Additions != 1 || got.Deletions != 0 || got.Path != "d/a.txt" {
		t.Fatalf("edit changes: %+v", got)
	}
	res, _ := edit.Execute(context.Background(), json.RawMessage(`{"path":"d/a.txt","old_text":"missing","new_text":"x"}`))
	if !res.IsError || len(res.FileChanges) != 0 {
		t.Fatalf("failed edit: %+v", res)
	}
}

func TestApplyPatchReportsChanges(t *testing.T) {
	if !hasPatchBinary() {
		t.Skip("patch binary is not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := "--- hello.txt\n+++ hello.txt\n@@ -1 +1,2 @@\n-hello\n+world\n+again\n"
	tl := NewApplyPatchTool(root, true)

	dry, err := tl.Execute(context.Background(), json.RawMessage(`{"dry_run":true,"patch":`+jsonQuote(patch)+`}`))
	if err != nil || dry.IsError || len(dry.FileChanges) != 0 {
		t.Fatalf("dry run: %v %+v", err, dry)
	}
	res, err := tl.Execute(context.Background(), json.RawMessage(`{"patch":`+jsonQuote(patch)+`}`))
	if err != nil || res.IsError {
		t.Fatalf("apply: %v %s", err, res.Text())
	}
	if len(res.FileChanges) != 1 {
		t.Fatalf("changes: %+v", res.FileChanges)
	}
	got := res.FileChanges[0]
	if got.Path != "hello.txt" || got.Op != FileChangeModify || got.Additions != 2 || got.Deletions != 1 {
		t.Fatalf("change: %+v", got)
	}
	res, _ = tl.Execute(context.Background(), json.RawMessage(`{"patch":`+jsonQuote(patch)+`}`))
	if !res.IsError || len(res.FileChanges) != 0 {
		t.Fatalf("re-applied patch changes nothing: %+v", res)
	}
}

func TestSnapshotPatchFileStaysInWorkspace(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "inside.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := snapshotPatchFile(root, "inside.txt"); !got.exists || string(got.data) != "ok\n" {
		t.Fatalf("inside = %+v", got)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got := snapshotPatchFile(root, "link/secret.txt"); got.exists || got.data != nil {
		t.Fatalf("a symlink out of the workspace must not be read: %+v", got)
	}
}
