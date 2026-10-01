package tarsserver

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

func TestFocusCreateReleaseKindAndKickoff(t *testing.T) {
	f := newWorktreeFixture(t)
	h := newFocusPipelineHandler(f.store, f.c, zerolog.Nop())

	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", `{"goal":"g","cwd":`+jsonString(f.repo)+`,"kind":"hotfix"}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown kind: %d %s", rec.Code, rec.Body.String())
	}

	body := `{"goal":"Release: ship 2 changes","kickoff":"Release: ship 2 changes\n- a\n- b","cwd":` + jsonString(f.repo) + `,"kind":"release"}`
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", body, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Pipeline focuspipeline.Pipeline `json:"pipeline"`
	}
	decodeInto(t, rec, &out)
	p := out.Pipeline
	if p.Kind != focuspipeline.KindRelease || p.Goal != "Release: ship 2 changes" || p.Kickoff != "Release: ship 2 changes\n- a\n- b" {
		t.Fatalf("pipeline = %+v", p)
	}
	// The guidance repeats the one-line goal, never the kickoff list.
	guidance := focuspipeline.Guidance(p)
	if !strings.Contains(guidance, "Goal: Release: ship 2 changes\n") || strings.Contains(guidance, "- a") {
		t.Fatalf("guidance = %q", guidance)
	}
}

// tagRepo tags HEAD of repo as name, dated at.
func tagRepo(t *testing.T, repo, name string, at time.Time) {
	t.Helper()
	t.Setenv("GIT_COMMITTER_DATE", at.UTC().Format(time.RFC3339))
	gitRun(t, repo, "tag", "-a", name, "-m", name)
}

func releaseTrainOf(t *testing.T, store *session.Store) releaseTrainResponse {
	t.Helper()
	rec := focusRequest(t, newFocusReleaseHandler(store, zerolog.Nop()), http.MethodGet, "/v1/focus/release-train", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("release train: %d %s", rec.Code, rec.Body.String())
	}
	var out releaseTrainResponse
	decodeInto(t, rec, &out)
	return out
}

func itemIDs(g releaseTrainGroup) []string {
	ids := make([]string, 0, len(g.Items))
	for _, it := range g.Items {
		ids = append(ids, it.SessionID)
	}
	return ids
}

// f1: acknowledging a card after the release moves UpdatedAt, not the
// time the pipeline finished, so it stays released.
func TestFocusReleaseTrainUsesFinishedAt(t *testing.T) {
	f := newWorktreeFixture(t)
	tagRepo(t, f.repo, "v0.2.0", time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC))

	shipped := f.session(t, "shipped")
	p := finishedPipeline(shipped.ID, time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC)) // card acked after the tag
	finished := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	p.FinishedAt = &finished
	saveFocus(t, f.store, p)

	fresh := f.session(t, "fresh")
	q := finishedPipeline(fresh.ID, time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC))
	freshAt := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	q.FinishedAt = &freshAt
	saveFocus(t, f.store, q)

	out := releaseTrainOf(t, f.store)
	if len(out.Groups) != 1 || len(out.Groups[0].Items) != 1 || out.Groups[0].Items[0].SessionID != fresh.ID {
		t.Fatalf("groups = %+v", out.Groups)
	}
	if !out.Groups[0].Items[0].FinishedAt.Equal(freshAt) {
		t.Fatalf("finished_at = %v", out.Groups[0].Items[0].FinishedAt)
	}
}

// f4: a release pipeline never lists itself or another release.
func TestFocusReleaseTrainSkipsReleasePipelines(t *testing.T) {
	f := newWorktreeFixture(t)
	feature := f.session(t, "feature")
	saveFocus(t, f.store, finishedPipeline(feature.ID, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)))
	release := f.session(t, "Release (1 change)")
	r := finishedPipeline(release.ID, time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC))
	r.Kind = focuspipeline.KindRelease
	saveFocus(t, f.store, r)

	out := releaseTrainOf(t, f.store)
	if len(out.Groups) != 1 || len(out.Groups[0].Items) != 1 || out.Groups[0].Items[0].SessionID != feature.ID {
		t.Fatalf("groups = %+v", out.Groups)
	}
}

// f2: the release tag lives on the remote (CI tags it); the train fetches
// tags first, and falls back to local tags with a stale hint when it can't.
func TestFocusReleaseTrainFetchesTags(t *testing.T) {
	f := newWorktreeFixture(t)
	tagRepo(t, f.repo, "v0.1.0", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitRun(t, f.repo, "clone", "-q", "--bare", f.repo, remote)
	// CI tags the release on the remote only.
	tagRepo(t, remote, "v0.2.0", time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC))
	gitRun(t, f.repo, "remote", "add", "origin", remote)

	shipped := f.session(t, "shipped in v0.2.0")
	saveFocus(t, f.store, finishedPipeline(shipped.ID, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)))
	next := f.session(t, "next")
	saveFocus(t, f.store, finishedPipeline(next.ID, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)))

	out := releaseTrainOf(t, f.store)
	if len(out.Groups) != 1 {
		t.Fatalf("groups = %+v", out.Groups)
	}
	g := out.Groups[0]
	if g.LastTag != "v0.2.0" || g.TagsStale || len(g.Items) != 1 || g.Items[0].SessionID != next.ID {
		t.Fatalf("fetched: %+v", g)
	}
}

func TestFocusReleaseTrainStaleTagsWhenFetchFails(t *testing.T) {
	f := newWorktreeFixture(t)
	tagRepo(t, f.repo, "v0.1.0", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	gitRun(t, f.repo, "remote", "add", "origin", filepath.Join(t.TempDir(), "missing.git"))
	sess := f.session(t, "after")
	saveFocus(t, f.store, finishedPipeline(sess.ID, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)))

	out := releaseTrainOf(t, f.store)
	if len(out.Groups) != 1 || out.Groups[0].LastTag != "v0.1.0" || !out.Groups[0].TagsStale || len(out.Groups[0].Items) != 1 {
		t.Fatalf("fallback to local tags: %+v", out.Groups)
	}
}

// f3: a session in a manual git worktree belongs to its repository's main
// checkout, not to a group of its own.
func TestFocusReleaseTrainGroupsManualWorktrees(t *testing.T) {
	f := newWorktreeFixture(t)
	wt := filepath.Join(t.TempDir(), "wt-x")
	gitRun(t, f.repo, "worktree", "add", "-q", "-b", "x", wt)
	if resolved, err := filepath.EvalSymlinks(wt); err == nil {
		wt = resolved
	}

	mainSess := f.session(t, "main checkout")
	saveFocus(t, f.store, finishedPipeline(mainSess.ID, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)))
	inWorktree, err := f.store.Create("manual worktree")
	if err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(wt, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := f.store.SetWorkDirs(inWorktree.ID, []string{sub}, sub); err != nil {
		t.Fatal(err)
	}
	saveFocus(t, f.store, finishedPipeline(inWorktree.ID, time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)))

	out := releaseTrainOf(t, f.store)
	if len(out.Groups) != 1 || out.Groups[0].Repo != f.repo {
		t.Fatalf("groups = %+v", out.Groups)
	}
	if ids := itemIDs(out.Groups[0]); len(ids) != 2 || ids[0] != mainSess.ID || ids[1] != inWorktree.ID {
		t.Fatalf("items = %v", ids)
	}
}
