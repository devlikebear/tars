package tarsserver

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/session"
)

// saveVersionPipeline makes the goal driver's pipeline one that saves its
// finished work, working in dir.
func saveVersionPipeline(t *testing.T, store *session.Store, id, dir string) {
	t.Helper()
	if err := store.SetWorkDirs(id, []string{dir}, dir); err != nil {
		t.Fatal(err)
	}
	if _, _, err := focusStoreFor(store).Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		p.SaveVersion = true
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// A pipeline that asked for it ends with its work saved as a version, the
// moment its last verification passes: no pull request, no merge stage.
func TestFinishedPipelineSavesItsVersion(t *testing.T) {
	d, _, store, id := goalDriver(t, stageReply, &fakeVerifier{result: passAll})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := easyproject.InitGit(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	saveVersionPipeline(t, store, id, dir)
	// What the build turn changed.
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	startGoal(t, d, store, id, 0)
	waitFor(t, "the saved version", func() bool { return pipelineOf(t, store, id).Saved != nil })
	p := pipelineOf(t, store, id)
	if !focuspipeline.Finished(p) || p.Saved.Error != "" || p.Saved.Commit != gitOut(t, dir, "rev-parse", "HEAD") {
		t.Fatalf("saved = %+v, finished %v", p.Saved, focuspipeline.Finished(p))
	}
	if got := gitOut(t, dir, "log", "--format=%an|%s"); got != "TARS|g\nTARS|Start the project" {
		t.Fatalf("history:\n%s", got)
	}
	if status := gitOut(t, dir, "status", "--porcelain"); status != "" {
		t.Fatalf("left unsaved:\n%s", status)
	}
}

// Saving goes wrong (here: the folder has no version control) and the
// pipeline says so with a fixed code; it is still finished.
func TestFinishedPipelineRecordsAFailedSave(t *testing.T) {
	d, _, store, id := goalDriver(t, stageReply, &fakeVerifier{result: passAll})
	saveVersionPipeline(t, store, id, t.TempDir())

	startGoal(t, d, store, id, 0)
	waitFor(t, "the save to be recorded", func() bool { return pipelineOf(t, store, id).Saved != nil })
	p := pipelineOf(t, store, id)
	if p.Saved.Error != focuspipeline.SaveFailed || p.Saved.Commit != "" || !focuspipeline.Finished(p) {
		t.Fatalf("saved = %+v", p.Saved)
	}
}

// A pipeline that did not ask saves nothing and records nothing.
func TestFinishedPipelineWithoutSaveVersion(t *testing.T) {
	d, _, store, id := goalDriver(t, stageReply, &fakeVerifier{result: passAll})
	dir := t.TempDir()
	if err := easyproject.InitGit(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if err := store.SetWorkDirs(id, []string{dir}, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	startGoal(t, d, store, id, 0)
	waitFor(t, "goal mode to end", func() bool { return !pipelineOf(t, store, id).GoalActive() })
	if p := pipelineOf(t, store, id); p.Saved != nil {
		t.Fatalf("saved without asking: %+v", p.Saved)
	}
	if status := gitOut(t, dir, "status", "--porcelain"); status != "?? new.txt" {
		t.Fatalf("the work was touched:\n%s", status)
	}
}
