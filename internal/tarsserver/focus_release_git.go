package tarsserver

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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

// releaseGitTimeout bounds each local git probe; releaseFetchTimeout the tag
// fetch; releaseKillGrace how long a cancelled git gets between SIGTERM and
// SIGKILL and between being killed and its pipes being closed;
// releaseFetchWindow how long one fetch result is reused for a repository.
var (
	releaseGitTimeout   = 2 * time.Second
	releaseFetchTimeout = 5 * time.Second
	releaseKillGrace    = time.Second
	releaseFetchWindow  = 5 * time.Minute
)

// releaseSSHOptions keep ssh from prompting or waiting long to connect.
const releaseSSHOptions = "-o BatchMode=yes -o ConnectTimeout=5"

func runGit(ctx context.Context, timeout time.Duration, dir string, env []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) // NOSONAR: git is the person's own toolchain, resolved from their PATH like every other TARS git call.
	// Never wait on a credential prompt nobody can answer.
	cmd.Env = append(append(os.Environ(), "GIT_TERMINAL_PROMPT=0"), env...)
	cmd.WaitDelay = releaseKillGrace
	killProcessGroupOnCancel(cmd, releaseKillGrace)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// gitMainCheckout is the main working tree of dir's repository — the same
// for the checkout and every linked worktree (they share one common git
// dir) — or "" outside a repository.
func gitMainCheckout(ctx context.Context, dir string) string {
	common, err := runGit(ctx, releaseGitTimeout, dir, nil, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err == nil && filepath.Base(common) == ".git" {
		return filepath.Dir(common)
	}
	// A bare repository's worktree, a submodule, or a git without
	// --path-format: its own top level.
	root, err := runGit(ctx, releaseGitTimeout, dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return root
}

// gitFetchTags fetches repo's tags from its default remote. A repository
// without a remote has nothing to fetch and its tags are current.
func gitFetchTags(ctx context.Context, repo string) bool {
	remotes, err := runGit(ctx, releaseGitTimeout, repo, nil, "remote")
	if err != nil {
		return false
	}
	if remotes == "" {
		return true
	}
	_, err = runGit(ctx, releaseFetchTimeout, repo, []string{"GIT_SSH_COMMAND=" + batchSSHCommand(ctx, repo)},
		"fetch", "--tags", "--quiet", "--no-recurse-submodules")
	return err == nil
}

// batchSSHCommand is the ssh command the fetch runs: the person's own
// (GIT_SSH_COMMAND, else core.sshCommand, else ssh) with batch-mode options
// appended, so a custom key or wrapper keeps working.
func batchSSHCommand(ctx context.Context, repo string) string {
	base := strings.TrimSpace(os.Getenv("GIT_SSH_COMMAND"))
	if base == "" {
		base, _ = runGit(ctx, releaseGitTimeout, repo, nil, "config", "--get", "core.sshCommand")
	}
	if base == "" {
		base = "ssh"
	}
	return base + " " + releaseSSHOptions
}

// gitLatestReleaseTag is the most recently created v* tag of repo.
func gitLatestReleaseTag(ctx context.Context, repo string) (releaseTag, bool) {
	out, err := runGit(ctx, releaseGitTimeout, repo, nil, "for-each-ref", "--sort=-creatordate", "--count=1",
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

// tagFetchCache runs at most one tag fetch per repository per window;
// requests arriving while one runs wait for its result instead of starting
// another.
type tagFetchCache struct {
	fetch   func(ctx context.Context, repo string) bool
	now     func() time.Time
	window  time.Duration
	mu      sync.Mutex
	entries map[string]*tagFetch
}

type tagFetch struct {
	done chan struct{}
	ok   bool
	at   time.Time
}

func newTagFetchCache(fetch func(ctx context.Context, repo string) bool) *tagFetchCache {
	return &tagFetchCache{fetch: fetch, now: time.Now, window: releaseFetchWindow, entries: map[string]*tagFetch{}}
}

// fetched reports whether repo's tags are current: the result of the fetch
// made within the window, of the one running now, or of a new one.
func (c *tagFetchCache) fetched(ctx context.Context, repo string) bool {
	c.mu.Lock()
	if e, ok := c.entries[repo]; ok {
		select {
		case <-e.done:
			if c.now().Sub(e.at) < c.window {
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
	e.ok, e.at = ok, c.now()
	close(e.done)
	c.mu.Unlock()
	return ok
}
