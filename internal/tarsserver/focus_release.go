package tarsserver

// The release train (docs/decisions/focus-mode.md §9 P5): the focus
// pipelines merged since the last release, grouped by repository, so one
// release PR can batch them.
//
//	GET /v1/focus/release-train → {groups: [{repo, last_tag, since, tags_stale, active_release?, items: [{session_id, title, goal, pr?, finished_at, updated_at}]}]}
//
// What counts as released:
//   - A finished release pipeline (kind release) records the sessions it
//     listed (ReleaseItems) and the cut-off it started from (ReleaseSince).
//     Once a repository has one, a merged pipeline is released when a
//     finished release listed it or it finished before the latest such
//     cut-off; work that finished while a release was in flight stays.
//   - A repository without a finished focus release falls back to the date
//     of its latest v* tag. Tags are made on the remote by CI, so they are
//     fetched first (bounded, cached per repository); when that fails the
//     local tags are used and the group is marked tags_stale.
//   - Either way, a pipeline whose pull request's merge is in the latest v*
//     tag shipped with it: releases are also made outside focus, and those
//     leave no release pipeline behind. The merge is the commit the probe
//     recorded (PRInfo.MergeOID), or for a pipeline that finished before it
//     was recorded, a commit in the tag whose subject is that PR's squash or
//     merge subject.
//
// active_release names a release pipeline still running for the repository;
// POST /v1/focus/pipelines refuses a second one.

import (
	"context"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/focusprobe"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

type releaseTrainItem struct {
	SessionID string                `json:"session_id"`
	Title     string                `json:"title"`
	Goal      string                `json:"goal"`
	PR        *focuspipeline.PRInfo `json:"pr,omitempty"`
	// FinishedAt is when the pipeline finished (focuspipeline.ReleaseTime).
	FinishedAt time.Time `json:"finished_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type releaseTrainGroup struct {
	Repo string `json:"repo"`
	// LastTag is the latest v* tag; empty when the repository has none.
	LastTag string     `json:"last_tag,omitempty"`
	Since   *time.Time `json:"since,omitempty"`
	// TagsStale is set when fetching tags from the remote failed, so a
	// release tag made since may be missing.
	TagsStale bool `json:"tags_stale,omitempty"`
	// ActiveRelease is the session of a release pipeline still running.
	ActiveRelease string             `json:"active_release,omitempty"`
	Items         []releaseTrainItem `json:"items"`
	// taggedPRs are the pull requests merged in LastTag, read when first
	// asked for.
	taggedPRs map[int]bool
}

type releaseTrainResponse struct {
	Groups []releaseTrainGroup `json:"groups"`
}

// releaseRepos resolves the repository a session's work lands in, caching
// folders for the life of one request.
type releaseRepos struct {
	sessions *session.Store
	repoRoot func(ctx context.Context, dir string) string
	roots    map[string]string
}

func newReleaseRepos(sessions *session.Store, repoRoot func(ctx context.Context, dir string) string) *releaseRepos {
	return &releaseRepos{sessions: sessions, repoRoot: repoRoot, roots: map[string]string{}}
}

// of is the main checkout of the repository a session works in: that of the
// folder an isolated session's worktree came from, else of its working
// folder; "" outside any repository.
func (r *releaseRepos) of(ctx context.Context, sess session.Session) string {
	dir := strings.TrimSpace(sess.CurrentDir)
	if sess.Worktree != nil && strings.TrimSpace(sess.Worktree.RepoRoot) != "" {
		dir = sess.Worktree.RepoRoot
	}
	return r.dir(ctx, dir)
}

func (r *releaseRepos) dir(ctx context.Context, dir string) string {
	if dir == "" {
		return ""
	}
	if root, ok := r.roots[dir]; ok {
		return root
	}
	root := r.repoRoot(ctx, dir)
	r.roots[dir] = root
	return root
}

// livePipeline is a pipeline whose session still exists, with that session
// and its repository.
type livePipeline struct {
	p    focuspipeline.Pipeline
	sess session.Session
	repo string
}

// repoReleases is what a repository's release pipelines say.
type repoReleases struct {
	finished bool
	covered  map[string]bool
	cutoff   time.Time
	active   string
}

// releaseInFlight reports whether a release pipeline still runs: not
// finished and not stopped (a stop leaves the stage blocked with no gate).
func releaseInFlight(p focuspipeline.Pipeline) bool {
	return !focuspipeline.Finished(p) && (p.Active() || p.OpenGate != focuspipeline.GateNone)
}

// scanReleases reads every pipeline of a live session in a repository and
// what that repository's release pipelines cover.
func scanReleases(ctx context.Context, store *focuspipeline.Store, repos *releaseRepos) ([]livePipeline, map[string]*repoReleases, error) {
	pipelines, err := store.List()
	if err != nil {
		return nil, nil, err
	}
	live := make([]livePipeline, 0, len(pipelines))
	releases := map[string]*repoReleases{}
	for _, p := range pipelines {
		sess, err := repos.sessions.Get(p.SessionID)
		if err != nil {
			continue // its session is gone; the sweep removes the file
		}
		repo := repos.of(ctx, sess)
		if repo == "" {
			continue
		}
		live = append(live, livePipeline{p: p, sess: sess, repo: repo})
		if p.Kind != focuspipeline.KindRelease {
			continue
		}
		rel := releases[repo]
		if rel == nil {
			rel = &repoReleases{covered: map[string]bool{}}
			releases[repo] = rel
		}
		switch {
		case focuspipeline.Finished(p):
			rel.finished = true
			for _, id := range p.ReleaseItems {
				rel.covered[id] = true
			}
			if p.ReleaseSince != nil && p.ReleaseSince.After(rel.cutoff) {
				rel.cutoff = *p.ReleaseSince
			}
		case releaseInFlight(p):
			rel.active = p.SessionID
		}
	}
	return live, releases, nil
}

type focusReleaseAPI struct {
	sessions  *session.Store
	logger    zerolog.Logger
	repoRoot  func(ctx context.Context, dir string) string
	latestTag func(ctx context.Context, repo string) (focusprobe.ReleaseTag, bool)
	fetches   *focusprobe.TagFetchCache
}

func newFocusReleaseHandler(sessions *session.Store, logger zerolog.Logger) http.Handler {
	return newFocusReleaseAPI(sessions, logger, focusprobe.FetchTags).handler()
}

func newFocusReleaseAPI(sessions *session.Store, logger zerolog.Logger, fetch func(ctx context.Context, repo string) bool) *focusReleaseAPI {
	return &focusReleaseAPI{sessions: sessions, logger: logger, repoRoot: focusprobe.MainCheckout, latestTag: focusprobe.LatestReleaseTag, fetches: focusprobe.NewTagFetchCache(fetch)}
}

func (a *focusReleaseAPI) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/focus/release-train", a.list)
	return mux
}

func (a *focusReleaseAPI) list(w http.ResponseWriter, r *http.Request) {
	groups, err := a.groups(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, releaseTrainResponse{Groups: groups})
}

// groups lists, per repository, the merged feature pipelines no release has
// shipped yet. Tags are fetched and read once per repository.
func (a *focusReleaseAPI) groups(ctx context.Context) ([]releaseTrainGroup, error) {
	live, releases, err := scanReleases(ctx, focusStoreFor(a.sessions), newReleaseRepos(a.sessions, a.repoRoot))
	if err != nil {
		return nil, err
	}
	byRepo := map[string]*releaseTrainGroup{}
	group := func(repo string) *releaseTrainGroup {
		g, ok := byRepo[repo]
		if !ok {
			g = a.newGroup(ctx, repo, releases[repo])
			byRepo[repo] = g
		}
		return g
	}
	for repo, rel := range releases {
		if rel.active != "" {
			group(repo)
		}
	}
	for _, lp := range live {
		p := lp.p
		if p.Kind == focuspipeline.KindRelease || !focuspipeline.Releasable(p) {
			continue
		}
		g := group(lp.repo)
		finished := focuspipeline.ReleaseTime(p)
		if released(p.SessionID, finished, g, releases[lp.repo]) || a.inLastTag(ctx, g, p.PR) {
			continue
		}
		g.Items = append(g.Items, releaseTrainItem{
			SessionID:  p.SessionID,
			Title:      lp.sess.Title,
			Goal:       p.Goal,
			PR:         p.PR,
			FinishedAt: finished,
			UpdatedAt:  p.UpdatedAt,
		})
	}
	out := make([]releaseTrainGroup, 0, len(byRepo))
	for _, g := range byRepo {
		if len(g.Items) == 0 && g.ActiveRelease == "" {
			continue
		}
		// Oldest first: the order a changelog reads in.
		sort.SliceStable(g.Items, func(i, j int) bool { return g.Items[i].FinishedAt.Before(g.Items[j].FinishedAt) })
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Repo < out[j].Repo })
	return out, nil
}

func (a *focusReleaseAPI) newGroup(ctx context.Context, repo string, rel *repoReleases) *releaseTrainGroup {
	g := &releaseTrainGroup{Repo: repo, Items: []releaseTrainItem{}}
	if rel != nil {
		g.ActiveRelease = rel.active
	}
	g.TagsStale = !a.fetches.Fetched(ctx, repo)
	if tag, found := a.latestTag(ctx, repo); found {
		since := tag.At.UTC()
		g.LastTag, g.Since = tag.Name, &since
	}
	return g
}

// released reports whether a merged pipeline already shipped: listed by a
// finished focus release or finished before its cut-off, or — for a
// repository with no finished focus release — finished by the latest tag.
func released(sessionID string, finished time.Time, g *releaseTrainGroup, rel *repoReleases) bool {
	if rel != nil && rel.finished {
		return rel.covered[sessionID] || !finished.After(rel.cutoff)
	}
	return g.Since != nil && !finished.After(*g.Since)
}

// inLastTag reports whether pr's merge is in the group's latest tag.
func (a *focusReleaseAPI) inLastTag(ctx context.Context, g *releaseTrainGroup, pr *focuspipeline.PRInfo) bool {
	if g.LastTag == "" || pr == nil {
		return false
	}
	if pr.MergeOID != "" {
		return focusprobe.TagHasCommit(ctx, g.Repo, g.LastTag, pr.MergeOID)
	}
	if g.taggedPRs == nil {
		g.taggedPRs = focusprobe.TagMergedPRs(ctx, g.Repo, g.LastTag)
	}
	return g.taggedPRs[pr.Number]
}

// releaseCreateMu serializes the active-release check with the create, so
// two clicks cannot both start a release for one repository.
var releaseCreateMu sync.Mutex

// activeReleaseIn is the session of a release pipeline still running in the
// repository of dir, or "".
func activeReleaseIn(ctx context.Context, sessions *session.Store, repoRoot func(ctx context.Context, dir string) string, dir string) (string, error) {
	repos := newReleaseRepos(sessions, repoRoot)
	repo := repos.dir(ctx, dir)
	if repo == "" {
		return "", nil
	}
	_, releases, err := scanReleases(ctx, focusStoreFor(sessions), repos)
	if err != nil {
		return "", err
	}
	if rel := releases[repo]; rel != nil {
		return rel.active, nil
	}
	return "", nil
}

// releaseItemsOf keeps the session ids a release request lists, trimmed and
// without duplicates.
func releaseItemsOf(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// repoRoot is how the create path resolves a folder's repository for its
// one-release-at-a-time check.
func (a *focusAPI) repoRoot() func(ctx context.Context, dir string) string {
	return focusprobe.MainCheckout
}
