package tarsserver

// The release train (docs/decisions/focus-mode.md §9 P5): the focus
// pipelines finished since the latest v* tag of their repository, grouped by
// repository, so one release PR can batch them.
//
//	GET /v1/focus/release-train → {groups: [{repo, last_tag, since, tags_stale, items: [{session_id, title, goal, pr?, finished_at, updated_at}]}]}
//
// Release tags are made on the remote by CI, so the train fetches tags
// (short timeout) before reading the latest one; when that fails it uses the
// local tags and marks the group tags_stale. Release pipelines themselves are
// never listed.

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// releaseGitTimeout bounds each local git probe of the release train;
// releaseFetchTimeout bounds the tag fetch from the remote.
const (
	releaseGitTimeout   = 2 * time.Second
	releaseFetchTimeout = 5 * time.Second
)

type releaseTrainItem struct {
	SessionID string                `json:"session_id"`
	Title     string                `json:"title"`
	Goal      string                `json:"goal"`
	PR        *focuspipeline.PRInfo `json:"pr,omitempty"`
	// FinishedAt is when the pipeline finished (focuspipeline.ReleaseTime),
	// the time compared with the latest tag.
	FinishedAt time.Time `json:"finished_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type releaseTrainGroup struct {
	Repo string `json:"repo"`
	// LastTag is the latest v* tag; empty when the repository has none,
	// and then every finished pipeline is listed.
	LastTag string     `json:"last_tag,omitempty"`
	Since   *time.Time `json:"since,omitempty"`
	// TagsStale is set when fetching tags from the remote failed, so a
	// release tag made since may be missing and released work listed.
	TagsStale bool               `json:"tags_stale,omitempty"`
	Items     []releaseTrainItem `json:"items"`
}

type releaseTrainResponse struct {
	Groups []releaseTrainGroup `json:"groups"`
}

// releaseTag is a repository's latest v* tag and its date.
type releaseTag struct {
	name string
	at   time.Time
}

type focusReleaseAPI struct {
	sessions  *session.Store
	logger    zerolog.Logger
	repoRoot  func(ctx context.Context, dir string) string
	latestTag func(ctx context.Context, repo string) (releaseTag, bool)
	// fetchTags updates repo's tags from its remote; false when the
	// repository has a remote and the fetch failed.
	fetchTags func(ctx context.Context, repo string) bool
}

func newFocusReleaseHandler(sessions *session.Store, logger zerolog.Logger) http.Handler {
	api := &focusReleaseAPI{sessions: sessions, logger: logger, repoRoot: gitMainCheckout, latestTag: gitLatestReleaseTag, fetchTags: gitFetchTags}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/focus/release-train", api.list)
	return mux
}

func (a *focusReleaseAPI) list(w http.ResponseWriter, r *http.Request) {
	pipelines, err := focusStoreFor(a.sessions).List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, releaseTrainResponse{Groups: a.groups(r.Context(), pipelines)})
}

// groups keeps the finished feature pipelines of live sessions in a
// repository, finished after that repository's latest v* tag. Tags are
// fetched and looked up once per repository, folders once per request.
func (a *focusReleaseAPI) groups(ctx context.Context, pipelines []focuspipeline.Pipeline) []releaseTrainGroup {
	byRepo := map[string]*releaseTrainGroup{}
	roots := map[string]string{}
	for _, p := range pipelines {
		if !focuspipeline.Finished(p) || p.Kind == focuspipeline.KindRelease {
			continue
		}
		sess, err := a.sessions.Get(p.SessionID)
		if err != nil {
			continue // its session is gone; the sweep removes the file
		}
		repo := a.sessionRepo(ctx, sess, roots)
		if repo == "" {
			continue
		}
		g, ok := byRepo[repo]
		if !ok {
			g = &releaseTrainGroup{Repo: repo, Items: []releaseTrainItem{}}
			g.TagsStale = !a.fetchTags(ctx, repo)
			if tag, found := a.latestTag(ctx, repo); found {
				since := tag.at.UTC()
				g.LastTag, g.Since = tag.name, &since
			}
			byRepo[repo] = g
		}
		finished := focuspipeline.ReleaseTime(p)
		if g.Since != nil && !finished.After(*g.Since) {
			continue
		}
		g.Items = append(g.Items, releaseTrainItem{
			SessionID:  p.SessionID,
			Title:      sess.Title,
			Goal:       p.Goal,
			PR:         p.PR,
			FinishedAt: finished,
			UpdatedAt:  p.UpdatedAt,
		})
	}
	out := make([]releaseTrainGroup, 0, len(byRepo))
	for _, g := range byRepo {
		if len(g.Items) == 0 {
			continue
		}
		// Oldest first: the order a changelog reads in.
		sort.SliceStable(g.Items, func(i, j int) bool { return g.Items[i].FinishedAt.Before(g.Items[j].FinishedAt) })
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Repo < out[j].Repo })
	return out
}

// sessionRepo is the main checkout of the repository a session's work lands
// in: that of the folder an isolated session's worktree came from, else of
// its working folder; "" outside any repository. roots caches folders.
func (a *focusReleaseAPI) sessionRepo(ctx context.Context, sess session.Session, roots map[string]string) string {
	dir := strings.TrimSpace(sess.CurrentDir)
	if sess.Worktree != nil && strings.TrimSpace(sess.Worktree.RepoRoot) != "" {
		dir = sess.Worktree.RepoRoot
	}
	if dir == "" {
		return ""
	}
	if root, ok := roots[dir]; ok {
		return root
	}
	root := a.repoRoot(ctx, dir)
	roots[dir] = root
	return root
}

func runGit(ctx context.Context, timeout time.Duration, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) // NOSONAR: git is the person's own toolchain, resolved from their PATH like every other TARS git call.
	// Never wait on a credential prompt nobody can answer.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// gitMainCheckout is the main working tree of dir's repository — the same
// for the checkout and every linked worktree (they share one common git
// dir) — or "" outside a repository.
func gitMainCheckout(ctx context.Context, dir string) string {
	common, err := runGit(ctx, releaseGitTimeout, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err == nil && filepath.Base(common) == ".git" {
		return filepath.Dir(common)
	}
	// A bare repository's worktree, or an unusual layout: its own top level.
	root, err := runGit(ctx, releaseGitTimeout, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return root
}

// gitFetchTags fetches repo's tags from its default remote. A repository
// without a remote has nothing to fetch and its tags are current.
func gitFetchTags(ctx context.Context, repo string) bool {
	remotes, err := runGit(ctx, releaseGitTimeout, repo, "remote")
	if err != nil {
		return false
	}
	if remotes == "" {
		return true
	}
	_, err = runGit(ctx, releaseFetchTimeout, repo, "fetch", "--tags", "--quiet", "--no-recurse-submodules")
	return err == nil
}

// gitLatestReleaseTag is the most recently created v* tag of repo.
func gitLatestReleaseTag(ctx context.Context, repo string) (releaseTag, bool) {
	out, err := runGit(ctx, releaseGitTimeout, repo, "for-each-ref", "--sort=-creatordate", "--count=1",
		"--format=%(refname:short)%09%(creatordate:iso-strict)", "refs/tags/v*")
	if err != nil || out == "" {
		return releaseTag{}, false
	}
	name, date, ok := strings.Cut(out, "\t")
	if !ok {
		return releaseTag{}, false
	}
	at, err := time.Parse(time.RFC3339, date)
	if err != nil {
		return releaseTag{}, false
	}
	return releaseTag{name: name, at: at}, true
}
