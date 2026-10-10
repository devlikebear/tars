package focusprobe

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The release train's git probes. Fetching tags reaches the network, so it
// is bounded three ways: a timeout whose cancel ends the whole process group
// (an ssh child holding the output pipe would otherwise keep Output waiting
// long after git is killed — measured at 1m15s against an unreachable ssh
// remote), ssh in batch mode with a connect timeout, and a per-repository
// cache so one page load does not fetch on every request.

// GitTimeout bounds each local git probe; releaseFetchTimeout bounds the tag
// fetch; KillGrace is how long a cancelled git gets between SIGTERM and
// SIGKILL and between being killed and its pipes being closed; FetchWindow
// is how long one fetch result is reused for a repository.
var (
	GitTimeout          = 2 * time.Second
	releaseFetchTimeout = 5 * time.Second
	KillGrace           = time.Second
	FetchWindow         = 5 * time.Minute
)

// releaseSSHOptions keep ssh from prompting or waiting long to connect.
const releaseSSHOptions = "-o BatchMode=yes -o ConnectTimeout=5"

// gitObjectID matches a full or abbreviated commit id. Commit ids that reach
// git as bare arguments come from outside this process — gh's JSON, the
// pipeline file — and a value starting with "-" would be read as an option.
var gitObjectID = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)

func IsGitObjectID(id string) bool {
	return gitObjectID.MatchString(id)
}

func RunGit(ctx context.Context, timeout time.Duration, dir string, env []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) // NOSONAR: git is the person's own toolchain, resolved from their PATH like every other TARS git call.
	// Never wait on a credential prompt nobody can answer.
	cmd.Env = append(append(os.Environ(), "GIT_TERMINAL_PROMPT=0"), env...)
	cmd.WaitDelay = KillGrace
	KillProcessGroupOnCancel(cmd, KillGrace)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// MainCheckout is the main working tree of dir's repository — the same
// for the checkout and every linked worktree (they share one common git
// dir) — or "" outside a repository.
func MainCheckout(ctx context.Context, dir string) string {
	common, err := RunGit(ctx, GitTimeout, dir, nil, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err == nil && filepath.Base(common) == ".git" {
		return filepath.Dir(common)
	}
	// A bare repository's worktree, a submodule, or a git without
	// --path-format: its own top level.
	root, err := RunGit(ctx, GitTimeout, dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return root
}

// FetchTags fetches repo's tags from its default remote. A repository
// without a remote has nothing to fetch and its tags are current.
func FetchTags(ctx context.Context, repo string) bool {
	remotes, err := RunGit(ctx, GitTimeout, repo, nil, "remote")
	if err != nil {
		return false
	}
	if remotes == "" {
		return true
	}
	_, err = RunGit(ctx, releaseFetchTimeout, repo, []string{"GIT_SSH_COMMAND=" + batchSSHCommand(ctx, repo)},
		"fetch", "--tags", "--quiet", "--no-recurse-submodules")
	return err == nil
}

// batchSSHCommand is the ssh command the fetch runs: the person's own
// (GIT_SSH_COMMAND, else core.sshCommand, else ssh) with batch-mode options
// appended, so a custom key or wrapper keeps working.
func batchSSHCommand(ctx context.Context, repo string) string {
	base := strings.TrimSpace(os.Getenv("GIT_SSH_COMMAND"))
	if base == "" {
		base, _ = RunGit(ctx, GitTimeout, repo, nil, "config", "--get", "core.sshCommand")
	}
	if base == "" {
		base = "ssh"
	}
	return base + " " + releaseSSHOptions
}

// ReleaseTag is a repository's latest v* tag and its date.
type ReleaseTag struct {
	Name string
	At   time.Time
}

// LatestReleaseTag is the most recently created v* tag of repo.
func LatestReleaseTag(ctx context.Context, repo string) (ReleaseTag, bool) {
	out, err := RunGit(ctx, GitTimeout, repo, nil, "for-each-ref", "--sort=-creatordate", "--count=1",
		"--format=%(refname:short)%09%(creatordate:iso-strict)", "refs/tags/v*")
	if err != nil || out == "" {
		return ReleaseTag{}, false
	}
	name, date, ok := strings.Cut(out, "\t")
	if !ok {
		return ReleaseTag{}, false
	}
	at, err := time.Parse(time.RFC3339, date)
	if err != nil {
		return ReleaseTag{}, false
	}
	return ReleaseTag{Name: name, At: at}, true
}

// TagHasCommit reports whether commit is in tag's history. A commit the
// repository does not have is not: the tag's own history is all local.
func TagHasCommit(ctx context.Context, repo, tag, commit string) bool {
	if !IsGitObjectID(commit) {
		return false
	}
	_, err := RunGit(ctx, GitTimeout, repo, nil, "merge-base", "--is-ancestor", commit+"^{commit}", "refs/tags/"+tag+"^{commit}")
	return err == nil
}

// mergeSubject matches the subject GitHub gives a pull request's commit on
// the base branch: "title (#12)" for a squash or rebase, "Merge pull request
// #12 from …" for a merge commit.
var mergeSubject = regexp.MustCompile(`^(?:Merge pull request #(\d+) |.*\(#(\d+)\)$)`)

// TagMergedPRs is the numbers of the pull requests whose merge is in
// tag's history, read from commit subjects only — a body that mentions a
// pull request does not ship it. Never nil.
func TagMergedPRs(ctx context.Context, repo, tag string) map[int]bool {
	prs := map[int]bool{}
	out, err := RunGit(ctx, GitTimeout, repo, nil, "log", "--format=%s", "refs/tags/"+tag, "--")
	if err != nil {
		return prs
	}
	for _, subject := range strings.Split(out, "\n") {
		m := mergeSubject.FindStringSubmatch(strings.TrimSpace(subject))
		if m == nil {
			continue
		}
		if n, err := strconv.Atoi(m[1] + m[2]); err == nil {
			prs[n] = true
		}
	}
	return prs
}

// TagFetchCache runs at most one tag fetch per repository per window;
// requests arriving while one runs wait for its result instead of starting
// another.
type TagFetchCache struct {
	fetch   func(ctx context.Context, repo string) bool
	Now     func() time.Time
	window  time.Duration
	mu      sync.Mutex
	entries map[string]*tagFetch
}

type tagFetch struct {
	done chan struct{}
	ok   bool
	at   time.Time
}

func NewTagFetchCache(fetch func(ctx context.Context, repo string) bool) *TagFetchCache {
	return &TagFetchCache{fetch: fetch, Now: time.Now, window: FetchWindow, entries: map[string]*tagFetch{}}
}

// fetched reports whether repo's tags are current: the result of the fetch
// made within the window, of the one running now, or of a new one.
func (c *TagFetchCache) Fetched(ctx context.Context, repo string) bool {
	c.mu.Lock()
	if e, ok := c.entries[repo]; ok {
		select {
		case <-e.done:
			if c.Now().Sub(e.at) < c.window {
				c.mu.Unlock()
				return e.ok
			}
		default:
			c.mu.Unlock()
			select {
			case <-e.done:
				return e.ok
			case <-ctx.Done():
				return false
			}
		}
	}
	e := &tagFetch{done: make(chan struct{})}
	c.entries[repo] = e
	c.mu.Unlock()

	// Shared by every waiting request: one caller leaving must not cancel it.
	ok := c.fetch(context.WithoutCancel(ctx), repo)
	c.mu.Lock()
	e.ok, e.at = ok, c.Now()
	close(e.done)
	c.mu.Unlock()
	return ok
}
