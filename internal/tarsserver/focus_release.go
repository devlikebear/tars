package tarsserver

// The release train (docs/decisions/focus-mode.md §9 P5): the focus
// pipelines finished since the latest v* tag of their repository, grouped by
// repository, so one release PR can batch them.
//
//	GET /v1/focus/release-train → {groups: [{repo, last_tag, since, items: [{session_id, title, goal, pr?, updated_at}]}]}

import (
	"context"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// releaseGitTimeout bounds each git probe of the release train.
const releaseGitTimeout = 2 * time.Second

type releaseTrainItem struct {
	SessionID string                `json:"session_id"`
	Title     string                `json:"title"`
	Goal      string                `json:"goal"`
	PR        *focuspipeline.PRInfo `json:"pr,omitempty"`
	UpdatedAt time.Time             `json:"updated_at"`
}

type releaseTrainGroup struct {
	Repo string `json:"repo"`
	// LastTag is the latest v* tag; empty when the repository has none,
	// and then every finished pipeline is listed.
	LastTag string             `json:"last_tag,omitempty"`
	Since   *time.Time         `json:"since,omitempty"`
	Items   []releaseTrainItem `json:"items"`
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
}

func newFocusReleaseHandler(sessions *session.Store, logger zerolog.Logger) http.Handler {
	api := &focusReleaseAPI{sessions: sessions, logger: logger, repoRoot: gitTopLevel, latestTag: gitLatestReleaseTag}
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

// groups keeps the finished pipelines of live sessions in a repository,
// updated after that repository's latest v* tag. Tags are looked up once
// per repository.
func (a *focusReleaseAPI) groups(ctx context.Context, pipelines []focuspipeline.Pipeline) []releaseTrainGroup {
	byRepo := map[string]*releaseTrainGroup{}
	for _, p := range pipelines {
		if !focuspipeline.Finished(p) {
			continue
		}
		sess, err := a.sessions.Get(p.SessionID)
		if err != nil {
			continue // its session is gone; the sweep removes the file
		}
		repo := a.sessionRepo(ctx, sess)
		if repo == "" {
			continue
		}
		g, ok := byRepo[repo]
		if !ok {
			g = &releaseTrainGroup{Repo: repo, Items: []releaseTrainItem{}}
			if tag, found := a.latestTag(ctx, repo); found {
				since := tag.at.UTC()
				g.LastTag, g.Since = tag.name, &since
			}
			byRepo[repo] = g
		}
		if g.Since != nil && !p.UpdatedAt.After(*g.Since) {
			continue
		}
		g.Items = append(g.Items, releaseTrainItem{
			SessionID: p.SessionID,
			Title:     sess.Title,
			Goal:      p.Goal,
			PR:        p.PR,
			UpdatedAt: p.UpdatedAt,
		})
	}
	out := make([]releaseTrainGroup, 0, len(byRepo))
	for _, g := range byRepo {
		if len(g.Items) == 0 {
			continue
		}
		// Oldest first: the order a changelog reads in.
		sort.SliceStable(g.Items, func(i, j int) bool { return g.Items[i].UpdatedAt.Before(g.Items[j].UpdatedAt) })
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Repo < out[j].Repo })
	return out
}

// sessionRepo is the repository a session's work lands in: the checkout an
// isolated session's worktree came from, else the top level of its working
// folder; "" outside any repository.
func (a *focusReleaseAPI) sessionRepo(ctx context.Context, sess session.Session) string {
	if sess.Worktree != nil && strings.TrimSpace(sess.Worktree.RepoRoot) != "" {
		return sess.Worktree.RepoRoot
	}
	dir := strings.TrimSpace(sess.CurrentDir)
	if dir == "" {
		return ""
	}
	return a.repoRoot(ctx, dir)
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, releaseGitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) // NOSONAR: git is the person's own toolchain, resolved from their PATH like every other TARS git call.
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// gitTopLevel is the repository top level of dir, or "" outside one.
func gitTopLevel(ctx context.Context, dir string) string {
	root, err := runGit(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return root
}

// gitLatestReleaseTag is the most recently created v* tag of repo.
func gitLatestReleaseTag(ctx context.Context, repo string) (releaseTag, bool) {
	out, err := runGit(ctx, repo, "for-each-ref", "--sort=-creatordate", "--count=1",
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
