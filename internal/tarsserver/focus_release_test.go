package tarsserver

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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
	h := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())

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

// releasePipeline is a release pipeline of the repository; finished when
// done, else still running at its plan gate.
func releasePipeline(sessionID string, items []string, since time.Time, done bool, at time.Time) focuspipeline.Pipeline {
	var p focuspipeline.Pipeline
	if done {
		p = finishedPipeline(sessionID, at)
		p.FinishedAt = &at
	} else {
		p = focuspipeline.New(sessionID, "Release", at)
	}
	p.Kind = focuspipeline.KindRelease
	p.ReleaseItems = items
	p.ReleaseSince = &since
	return p
}

// Item 2: work that finished while a release was in flight is not covered by
// it, even though the release tag (made when the release merged) is newer.
func TestFocusReleaseTrainUsesReleaseCoverage(t *testing.T) {
	f := newWorktreeFixture(t)
	tagAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tagRepo(t, f.repo, "v0.1.0", tagAt)

	old := f.session(t, "released by hand before")
	saveFocus(t, f.store, finishedPipeline(old.ID, time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)))
	shipped := f.session(t, "shipped")
	saveFocus(t, f.store, finishedPipeline(shipped.ID, time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)))
	during := f.session(t, "finished during the release")
	saveFocus(t, f.store, finishedPipeline(during.ID, time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC)))
	release := f.session(t, "Release")
	saveFocus(t, f.store, releasePipeline(release.ID, []string{shipped.ID}, tagAt, true, time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC)))
	// CI tags the release when it merges, after "during" finished.
	tagRepo(t, f.repo, "v0.2.0", time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC))

	out := releaseTrainOf(t, f.store)
	if len(out.Groups) != 1 {
		t.Fatalf("groups = %+v", out.Groups)
	}
	if ids := itemIDs(out.Groups[0]); len(ids) != 1 || ids[0] != during.ID {
		t.Fatalf("items = %v, want only the work that finished during the release", ids)
	}
}

// Item 3: a running release is named on its group, and a second one is
// refused with the running one's session.
func TestFocusReleaseActiveReleaseConflict(t *testing.T) {
	f := newWorktreeFixture(t)
	feature := f.session(t, "feature")
	saveFocus(t, f.store, finishedPipeline(feature.ID, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)))
	running := f.session(t, "Release (1 change)")
	saveFocus(t, f.store, releasePipeline(running.ID, []string{feature.ID}, time.Time{}, false, time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)))

	out := releaseTrainOf(t, f.store)
	if len(out.Groups) != 1 || out.Groups[0].ActiveRelease != running.ID || len(out.Groups[0].Items) != 1 {
		t.Fatalf("groups = %+v", out.Groups)
	}

	h := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())
	body := `{"goal":"Release","kind":"release","cwd":` + jsonString(f.repo) + `,"release_items":[` + jsonString(feature.ID) + `]}`
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", body, true)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second release: %d %s", rec.Code, rec.Body.String())
	}
	var conflict struct {
		Error     string `json:"error"`
		SessionID string `json:"session_id"`
	}
	decodeInto(t, rec, &conflict)
	if conflict.SessionID != running.ID || conflict.Error == "" {
		t.Fatalf("409 body = %+v", conflict)
	}
	// A feature task in the same repository is not a release: no conflict.
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", `{"goal":"g","cwd":`+jsonString(f.repo)+`}`, true); rec.Code != http.StatusCreated {
		t.Fatalf("feature create: %d %s", rec.Code, rec.Body.String())
	}

	// Stopping the running release frees the repository; the new release
	// records its list and cut-off.
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+running.ID+"/stop", "", true); rec.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", rec.Code, rec.Body.String())
	}
	body = `{"goal":"Release","kind":"release","cwd":` + jsonString(f.repo) + `,"release_items":[` + jsonString(feature.ID) + `,` + jsonString(feature.ID) + `],"release_since":"2026-01-01T00:00:00Z"}`
	rec = focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", body, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("release after stop: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Pipeline focuspipeline.Pipeline `json:"pipeline"`
	}
	decodeInto(t, rec, &created)
	if len(created.Pipeline.ReleaseItems) != 1 || created.Pipeline.ReleaseItems[0] != feature.ID || created.Pipeline.ReleaseSince == nil {
		t.Fatalf("release fields = %+v / %v", created.Pipeline.ReleaseItems, created.Pipeline.ReleaseSince)
	}
}

// Item 5: only merged work is listed; a plan that skipped merge finishes
// without shipping anything.
func TestFocusReleaseTrainListsOnlyMergedWork(t *testing.T) {
	f := newWorktreeFixture(t)
	merged := f.session(t, "merged")
	saveFocus(t, f.store, finishedPipeline(merged.ID, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)))
	local := f.session(t, "local only")
	p := focuspipeline.New(local.ID, "local", time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC))
	p.Plan = &focuspipeline.Plan{Stages: []focuspipeline.StageID{focuspipeline.StagePlan, focuspipeline.StageBuild}}
	for i := range p.Stages {
		switch p.Stages[i].ID {
		case focuspipeline.StagePlan, focuspipeline.StageBuild:
			p.Stages[i].Status = focuspipeline.StatusDone
		default:
			p.Stages[i].Status = focuspipeline.StatusSkipped
		}
	}
	saveFocus(t, f.store, p)

	out := releaseTrainOf(t, f.store)
	if len(out.Groups) != 1 {
		t.Fatalf("groups = %+v", out.Groups)
	}
	if ids := itemIDs(out.Groups[0]); len(ids) != 1 || ids[0] != merged.ID {
		t.Fatalf("items = %v", ids)
	}
}

// Item 1: one fetch per repository per window, shared by concurrent
// requests.
func TestFocusReleaseTrainCachesTagFetch(t *testing.T) {
	f := newWorktreeFixture(t)
	sess := f.session(t, "after")
	saveFocus(t, f.store, finishedPipeline(sess.ID, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)))

	var mu sync.Mutex
	calls := 0
	release := make(chan struct{})
	api := newFocusReleaseAPI(f.store, zerolog.Nop(), func(context.Context, string) bool {
		mu.Lock()
		calls++
		mu.Unlock()
		<-release
		return true
	})
	clock := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	api.fetches.now = func() time.Time { return clock }
	h := api.handler()

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = focusRequest(t, h, http.MethodGet, "/v1/focus/release-train", "", false)
		}()
	}
	time.Sleep(200 * time.Millisecond)
	close(release)
	wg.Wait()
	_ = focusRequest(t, h, http.MethodGet, "/v1/focus/release-train", "", false)
	mu.Lock()
	if calls != 1 {
		t.Fatalf("fetches within the window = %d, want 1", calls)
	}
	mu.Unlock()

	clock = clock.Add(releaseFetchWindow + time.Second)
	_ = focusRequest(t, h, http.MethodGet, "/v1/focus/release-train", "", false)
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Fatalf("fetches after the window = %d, want 2", calls)
	}
}

// commitRepo adds an empty commit with subject to repo and returns its id.
func commitRepo(t *testing.T, repo, subject string) string {
	t.Helper()
	gitRun(t, repo, "commit", "-q", "--allow-empty", "-m", subject)
	return strings.TrimSpace(gitRun(t, repo, "rev-parse", "HEAD"))
}

// mergedPipeline is a finished pipeline whose pull request merged.
func mergedPipeline(sessionID string, at time.Time, pr focuspipeline.PRInfo) focuspipeline.Pipeline {
	p := finishedPipeline(sessionID, at)
	p.FinishedAt = &at
	pr.State = focuspipeline.PRStateMerged
	p.PR = &pr
	return p
}

// A release made outside focus (a hand-made release PR, tagged by CI) ships
// work no focus release lists: a pipeline whose merge is in the latest tag is
// released, whatever the last focus release's cut-off says.
func TestFocusReleaseTrainSkipsWorkInLatestTag(t *testing.T) {
	f := newWorktreeFixture(t)
	tagRepo(t, f.repo, "v0.1.0", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	// The only focus release: finished long before the work below.
	release := f.session(t, "Release")
	saveFocus(t, f.store, releasePipeline(release.ID, nil, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), true, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)))

	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	squashed := f.session(t, "squash-merged, tagged by hand")
	commitRepo(t, f.repo, "fix: a (#11)")
	saveFocus(t, f.store, mergedPipeline(squashed.ID, at, focuspipeline.PRInfo{Number: 11}))
	merged := f.session(t, "merge commit, tagged by hand")
	commitRepo(t, f.repo, "Merge pull request #12 from acme/b")
	saveFocus(t, f.store, mergedPipeline(merged.ID, at, focuspipeline.PRInfo{Number: 12}))
	byCommit := f.session(t, "merge commit recorded, tagged by hand")
	saveFocus(t, f.store, mergedPipeline(byCommit.ID, at, focuspipeline.PRInfo{Number: 13, MergeOID: commitRepo(t, f.repo, "fix: c")}))
	// Mentions of another PR do not ship it: only a merge's own subject does.
	commitRepo(t, f.repo, "chore: release\n\n- fix: d (#14)\n- follow-up to (#1)")
	tagRepo(t, f.repo, "v0.2.0", time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC))

	mentioned := f.session(t, "only mentioned in the tag")
	saveFocus(t, f.store, mergedPipeline(mentioned.ID, at.Add(time.Minute), focuspipeline.PRInfo{Number: 14}))
	// Merged after the tag: the next release's work.
	fresh := f.session(t, "after the tag")
	commitRepo(t, f.repo, "fix: e (#15)")
	saveFocus(t, f.store, mergedPipeline(fresh.ID, at.Add(2*time.Minute), focuspipeline.PRInfo{Number: 15}))
	freshByCommit := f.session(t, "after the tag, merge commit recorded")
	// Its subject names a PR the tag holds; the recorded commit decides.
	saveFocus(t, f.store, mergedPipeline(freshByCommit.ID, at.Add(3*time.Minute), focuspipeline.PRInfo{Number: 11, MergeOID: commitRepo(t, f.repo, "fix: f")}))

	out := releaseTrainOf(t, f.store)
	if len(out.Groups) != 1 || out.Groups[0].LastTag != "v0.2.0" {
		t.Fatalf("groups = %+v", out.Groups)
	}
	want := []string{mentioned.ID, fresh.ID, freshByCommit.ID}
	if ids := itemIDs(out.Groups[0]); !slices.Equal(ids, want) {
		t.Fatalf("items = %v, want %v (work in v0.2.0 is released)", ids, want)
	}
}
