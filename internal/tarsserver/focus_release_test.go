package tarsserver

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// finishedPipeline is a pipeline that ran plan → build → merge, last
// touched at updated.
func finishedPipeline(sessionID string, updated time.Time) focuspipeline.Pipeline {
	p := focuspipeline.New(sessionID, "goal of "+sessionID, updated)
	p.Plan = &focuspipeline.Plan{Stages: []focuspipeline.StageID{focuspipeline.StagePlan, focuspipeline.StageBuild, focuspipeline.StageMerge}}
	for i := range p.Stages {
		switch p.Stages[i].ID {
		case focuspipeline.StagePlan, focuspipeline.StageBuild, focuspipeline.StageMerge:
			p.Stages[i].Status = focuspipeline.StatusDone
		default:
			p.Stages[i].Status = focuspipeline.StatusSkipped
		}
	}
	p.Current = focuspipeline.StageMerge
	p.UpdatedAt = updated.UTC()
	return p
}

func saveFocus(t *testing.T, store *session.Store, p focuspipeline.Pipeline) {
	t.Helper()
	if err := focusStoreFor(store).Save(p); err != nil {
		t.Fatal(err)
	}
}

func TestFocusReleaseTrain(t *testing.T) {
	f := newWorktreeFixture(t)
	// An older v tag, the latest v tag (dated 2026-01-01), and a newer
	// non-release tag that must not count.
	t.Setenv("GIT_COMMITTER_DATE", "2025-06-01T00:00:00Z")
	gitRun(t, f.repo, "tag", "-a", "v0.0.9", "-m", "old")
	t.Setenv("GIT_COMMITTER_DATE", "2026-01-01T00:00:00Z")
	gitRun(t, f.repo, "tag", "-a", "v0.1.0", "-m", "release")
	t.Setenv("GIT_COMMITTER_DATE", "2026-03-01T00:00:00Z")
	gitRun(t, f.repo, "tag", "-a", "nightly", "-m", "not a release")

	before := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	after := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)

	// Merged after the tag, with a PR.
	withPR := f.session(t, "with pr")
	p := finishedPipeline(withPR.ID, later)
	p.PR = &focuspipeline.PRInfo{Number: 42, URL: "https://example.com/pr/42", State: "merged"}
	saveFocus(t, f.store, p)

	// Finished after the tag in a session worktree: grouped under the
	// checkout it came from.
	isolated := f.session(t, "isolated")
	wtDir := filepath.Join(t.TempDir(), "wt")
	if err := os.MkdirAll(wtDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := f.store.SetWorktree(isolated.ID, &session.SessionWorktree{Path: wtDir, Dir: wtDir, RepoRoot: f.repo, SourceDir: f.repo}); err != nil {
		t.Fatal(err)
	}
	saveFocus(t, f.store, finishedPipeline(isolated.ID, after))

	// Finished before the tag: already released.
	old := f.session(t, "released")
	saveFocus(t, f.store, finishedPipeline(old.ID, before))

	// Still running.
	running := f.session(t, "running")
	saveFocus(t, f.store, focuspipeline.New(running.ID, "running", after))

	// Finished, but its session is gone (the sweep has not run yet).
	gone := f.session(t, "gone")
	if err := f.store.Delete(gone.ID); err != nil {
		t.Fatal(err)
	}
	saveFocus(t, f.store, finishedPipeline(gone.ID, after))

	// Finished in a folder outside any repository: no release to join.
	loose, err := f.store.Create("loose")
	if err != nil {
		t.Fatal(err)
	}
	looseDir := t.TempDir()
	if err := f.store.SetWorkDirs(loose.ID, []string{looseDir}, looseDir); err != nil {
		t.Fatal(err)
	}
	saveFocus(t, f.store, finishedPipeline(loose.ID, after))

	h := newFocusReleaseHandler(f.store, zerolog.Nop())
	rec := focusRequest(t, h, http.MethodGet, "/v1/focus/release-train", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("release train: %d %s", rec.Code, rec.Body.String())
	}
	var out releaseTrainResponse
	decodeInto(t, rec, &out)
	if len(out.Groups) != 1 {
		t.Fatalf("groups = %+v", out.Groups)
	}
	g := out.Groups[0]
	if g.Repo != f.repo || g.LastTag != "v0.1.0" || g.Since == nil || !g.Since.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("group = %+v", g)
	}
	if len(g.Items) != 2 {
		t.Fatalf("items = %+v", g.Items)
	}
	// Oldest first: the order a changelog reads in.
	if g.Items[0].SessionID != isolated.ID || g.Items[0].Title != "isolated" || g.Items[0].PR != nil {
		t.Fatalf("first item = %+v", g.Items[0])
	}
	if g.Items[1].SessionID != withPR.ID || g.Items[1].PR == nil || g.Items[1].PR.Number != 42 || g.Items[1].PR.URL != "https://example.com/pr/42" {
		t.Fatalf("second item = %+v", g.Items[1])
	}
	if g.Items[1].Goal != "goal of "+withPR.ID {
		t.Fatalf("goal = %q", g.Items[1].Goal)
	}
}

func TestFocusReleaseTrainWithoutTags(t *testing.T) {
	f := newWorktreeFixture(t)
	sess := f.session(t, "first")
	saveFocus(t, f.store, finishedPipeline(sess.ID, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)))

	h := newFocusReleaseHandler(f.store, zerolog.Nop())
	rec := focusRequest(t, h, http.MethodGet, "/v1/focus/release-train", "", false)
	var out releaseTrainResponse
	decodeInto(t, rec, &out)
	if len(out.Groups) != 1 || out.Groups[0].LastTag != "" || out.Groups[0].Since != nil || len(out.Groups[0].Items) != 1 {
		t.Fatalf("untagged repo lists everything finished: %+v", out.Groups)
	}
}

func TestFocusReleaseTrainEmpty(t *testing.T) {
	f := newWorktreeFixture(t)
	rec := focusRequest(t, newFocusReleaseHandler(f.store, zerolog.Nop()), http.MethodGet, "/v1/focus/release-train", "", false)
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"groups\":[]}\n" {
		t.Fatalf("empty: %d %q", rec.Code, rec.Body.String())
	}
}
