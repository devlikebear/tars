package tarsserver

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// stoppedCleanupFixture gives a driver whose session is stopped and holds
// a worktree, wired to a recordedFinish for cleanupWorktree to call.
func stoppedCleanupFixture(t *testing.T) (*focusDriver, *session.Store, string, *recordedFinish) {
	t.Helper()
	d, _, store, id := testFocusDriver(t, func(int, string) string { return focusReport("ok", false) }, &fakeVerifier{result: passAll})
	finished := &recordedFinish{}
	d.finishWorktree = finished.finish
	wt := t.TempDir()
	if err := store.SetWorktree(id, &session.SessionWorktree{Path: wt, Dir: wt, Branch: "tars/session-" + id, SourceDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	applyFocus(t, store, id, focuspipeline.Event{Kind: focuspipeline.EventStop})
	return d, store, id, finished
}

func TestFocusCleanupKeepsByDefault(t *testing.T) {
	d, store, id, finished := stoppedCleanupFixture(t)
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())

	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+id+"/cleanup", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("cleanup: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Pipeline    focuspipeline.Pipeline    `json:"pipeline"`
		WorktreeEnd focuspipeline.WorktreeEnd `json:"worktree_end"`
	}
	decodeInto(t, rec, &out)
	if out.WorktreeEnd.Action != "keep" || out.Pipeline.WorktreeEnd == nil || out.Pipeline.WorktreeEnd.Action != "keep" {
		t.Fatalf("worktree end = %+v pipeline = %+v", out.WorktreeEnd, out.Pipeline.WorktreeEnd)
	}
	if seen := finished.seen(); len(seen) != 1 || seen[0] != "keep" {
		t.Fatalf("finished = %v", seen)
	}
}

func TestFocusCleanupDiscard(t *testing.T) {
	d, store, id, finished := stoppedCleanupFixture(t)
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())

	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+id+"/cleanup", `{"action":"discard"}`, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("cleanup: %d %s", rec.Code, rec.Body.String())
	}
	if seen := finished.seen(); len(seen) != 1 || seen[0] != "discard" {
		t.Fatalf("finished = %v", seen)
	}
	if saved := pipelineOf(t, store, id); saved.WorktreeEnd == nil || saved.WorktreeEnd.Action != "discard" {
		t.Fatalf("saved worktree end = %+v", saved.WorktreeEnd)
	}
}

func TestFocusCleanupRejectsBadAction(t *testing.T) {
	d, store, id, finished := stoppedCleanupFixture(t)
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())

	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+id+"/cleanup", `{"action":"apply"}`, false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad action: %d %s", rec.Code, rec.Body.String())
	}
	if len(finished.seen()) != 0 {
		t.Fatal("a rejected action must not finish the worktree")
	}
}

func TestFocusCleanupRefusedWhileActive(t *testing.T) {
	// Fresh driver/session from testFocusDriver is in the build stage
	// (buildingFocusSession): active, not stopped or finished.
	d, _, store, id := testFocusDriver(t, func(int, string) string { return focusReport("ok", false) }, &fakeVerifier{result: passAll})
	finished := &recordedFinish{}
	d.finishWorktree = finished.finish
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())

	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+id+"/cleanup", "", false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("cleanup while active: %d %s", rec.Code, rec.Body.String())
	}
	if len(finished.seen()) != 0 {
		t.Fatal("a refused cleanup must not finish the worktree")
	}
}

func TestFocusCleanupMissingPipeline(t *testing.T) {
	d, store, _, _ := stoppedCleanupFixture(t)
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/nope/cleanup", "", false); rec.Code != http.StatusNotFound {
		t.Fatalf("missing: %d", rec.Code)
	}
}

func TestFocusCleanupNoWorktree(t *testing.T) {
	d, _, store, id := testFocusDriver(t, func(int, string) string { return focusReport("ok", false) }, &fakeVerifier{result: passAll})
	finished := &recordedFinish{}
	d.finishWorktree = finished.finish
	applyFocus(t, store, id, focuspipeline.Event{Kind: focuspipeline.EventStop})
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())

	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+id+"/cleanup", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("cleanup: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		WorktreeEnd focuspipeline.WorktreeEnd `json:"worktree_end"`
	}
	decodeInto(t, rec, &out)
	if out.WorktreeEnd.Action != "none" {
		t.Fatalf("worktree end = %+v", out.WorktreeEnd)
	}
	if len(finished.seen()) != 0 {
		t.Fatal("finish must not run without a worktree")
	}
}

func TestFocusCleanupWorktreeLeftWhileATurnRuns(t *testing.T) {
	d, _, id, finished := stoppedCleanupFixture(t)
	_, release, ok := d.cancels.Claim(id)
	if !ok {
		t.Fatal("claim")
	}
	defer release()
	end := d.cleanupWorktree(id, "keep")
	if end.Action != "left" || !strings.Contains(end.Reason, "running") {
		t.Fatalf("end = %+v", end)
	}
	if len(finished.seen()) != 0 {
		t.Fatalf("finished = %v", finished.seen())
	}
}

func TestFocusCleanupWorktreeNilDriver(t *testing.T) {
	var d *focusDriver
	if end := d.cleanupWorktree("s1", "keep"); end.Action != "none" {
		t.Fatalf("end = %+v", end)
	}
}

func TestFocusCleanupWorktreeFinishFails(t *testing.T) {
	d, _, id, _ := stoppedCleanupFixture(t)
	d.finishWorktree = func(context.Context, string, string) error { return errWorktreeMissing }
	if end := d.cleanupWorktree(id, "keep"); end.Action != "none" {
		t.Fatalf("missing worktree: end = %+v", end)
	}
	d.finishWorktree = func(context.Context, string, string) error { return errors.New("git apply failed") }
	if end := d.cleanupWorktree(id, "discard"); end.Action != "left" || end.Reason != "git apply failed" {
		t.Fatalf("failed finish: end = %+v", end)
	}
}
