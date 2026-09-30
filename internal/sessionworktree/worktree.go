// Package sessionworktree gives a chat session its own git worktree (#971).
//
// A foreground session works in its checkout; a second session in the same
// repository, or an unattended run, works in a worktree of its own on a
// branch named tars/session-<id>, so their edits do not mix. When the work is
// done, the session's changes are applied to the checkout it came from as a
// patch on the working tree, kept on the branch for a manual merge, or
// discarded.
//
// Everything goes through the git CLI in the repository the person already
// uses. TARS writes nothing under their .git beyond what `git worktree` and
// `git branch` themselves record, and the worktrees live in TARS's data
// directory, not next to the checkout.
package sessionworktree

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/pkg/session"
)

// BranchPrefix names the branches session worktrees check out.
const BranchPrefix = "tars/session-"

// ErrNotRepository means the folder is not inside a git work tree.
var ErrNotRepository = errors.New("not a git repository")

// ErrNoCommits means the repository has no commit to branch from.
var ErrNoCommits = errors.New("the repository has no commits yet")

// ConflictError reports that the session's changes do not apply cleanly to
// the checkout they came from. The worktree is left as it was.
type ConflictError struct {
	Detail string
}

func (e *ConflictError) Error() string {
	return "the changes do not apply cleanly to the checkout: " + e.Detail
}

// Manager creates and retires session worktrees under root.
type Manager struct {
	root string
	now  func() time.Time

	// includeWait is how long Create waits for includes to be copied
	// before it returns and leaves the rest to the background.
	includeWait time.Duration
	// copyLimit caps the bytes one worktree's includes may copy when the
	// file system cannot clone them.
	copyLimit int64
	clone     func(src, dst string) error
	// copyHook lets tests hold a copy mid-way.
	copyHook func(ctx context.Context)

	mu        sync.Mutex
	jobs      map[string]*includeJob
	onInclude func(IncludeDone)
}

// New returns a manager that keeps worktrees and their records under root.
func New(root string) *Manager {
	return &Manager{
		root:        filepath.Clean(root),
		now:         time.Now,
		includeWait: defaultIncludeWait,
		copyLimit:   defaultCopyLimit,
		clone:       cloneFile,
		jobs:        map[string]*includeJob{},
	}
}

// Root is the folder the manager keeps worktrees in.
func (m *Manager) Root() string { return m.root }

// RepoRoot returns the top level of the git work tree dir is in.
func RepoRoot(ctx context.Context, dir string) (string, error) {
	out, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", ErrNotRepository
	}
	root := filepath.Clean(filepath.FromSlash(strings.TrimSpace(out)))
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return root, nil
}

// CreateRequest describes the worktree a session needs.
type CreateRequest struct {
	SessionID string
	// SourceDir is the folder the session works in; the worktree mirrors
	// its place in the repository.
	SourceDir string
	// Include lists gitignored files and folders, relative to the
	// repository root, to copy into the worktree (.env, local settings,
	// node_modules). See include.go for the rules.
	Include []string
	// Reason records why the session was isolated: manual, lease or
	// unattended.
	Reason string
}

// CreateResult is the new worktree and what was copied into it.
type CreateResult struct {
	Worktree session.SessionWorktree
	Copied   []string
	// Skipped lists include entries that were not copied, with why.
	Skipped []string
	// Pending lists include entries still being copied in the background;
	// OnIncludeDone reports them when they finish.
	Pending []string
}

// Create adds a worktree for the session on a new branch at the checkout's
// HEAD. Uncommitted changes in the checkout stay there; the worktree starts
// from the last commit.
func (m *Manager) Create(ctx context.Context, req CreateRequest) (CreateResult, error) {
	sessionID := strings.TrimSpace(req.SessionID)
	if !validSessionID(sessionID) {
		return CreateResult{}, fmt.Errorf("invalid session id %q", req.SessionID)
	}
	source, err := filepath.Abs(strings.TrimSpace(req.SourceDir))
	if err != nil {
		return CreateResult{}, err
	}
	if resolved, err := filepath.EvalSymlinks(source); err == nil {
		source = resolved
	}
	repo, err := RepoRoot(ctx, source)
	if err != nil {
		return CreateResult{}, err
	}
	base, err := git(ctx, repo, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return CreateResult{}, ErrNoCommits
	}
	base = strings.TrimSpace(base)
	rel, err := filepath.Rel(repo, source)
	// A folder named with ".." anywhere is refused too, so the worktree's
	// working folder can never point outside the worktree.
	if err != nil || strings.Contains(rel, "..") {
		return CreateResult{}, fmt.Errorf("%s is not inside %s", source, repo)
	}

	path := filepath.Join(m.root, repoKey(repo), sessionID)
	if _, err := os.Stat(path); err == nil {
		return CreateResult{}, fmt.Errorf("session %s already has a worktree at %s", sessionID, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return CreateResult{}, err
	}
	branch, err := freeBranch(ctx, repo, BranchPrefix+sessionID)
	if err != nil {
		return CreateResult{}, err
	}
	if _, err := git(ctx, repo, "worktree", "add", "-b", branch, path, base); err != nil {
		return CreateResult{}, fmt.Errorf("git worktree add: %w", err)
	}
	wt := session.SessionWorktree{
		Path:       path,
		Dir:        filepath.Join(path, rel),
		Branch:     branch,
		BaseCommit: base,
		RepoRoot:   repo,
		SourceDir:  source,
		Reason:     strings.TrimSpace(req.Reason),
		CreatedAt:  m.now().UTC(),
	}
	if err := os.MkdirAll(wt.Dir, 0o755); err != nil {
		return CreateResult{}, err
	}
	result := CreateResult{Worktree: wt}
	if err := m.writeRecord(sessionID, wt); err != nil {
		_ = m.remove(ctx, wt, true)
		return CreateResult{}, err
	}
	items, refused := planIncludes(repo, req.Include)
	copied, skipped, pending := m.startIncludes(sessionID, wt, items)
	result.Copied, result.Pending = copied, pending
	result.Skipped = append(refused, skipped...)
	return result, nil
}

// Status is what a session changed in its worktree since it branched.
type Status struct {
	// Files are the paths, relative to the repository root, that differ
	// from the base commit: committed on the branch or not.
	Files []string `json:"files"`
	// Commits counts commits on the branch since the base.
	Commits int `json:"commits"`
}

// Status lists the files the session changed.
func (m *Manager) Status(ctx context.Context, wt session.SessionWorktree) (Status, error) {
	files, err := changedFiles(ctx, wt)
	if err != nil {
		return Status{}, err
	}
	out, err := git(ctx, wt.Path, "rev-list", "--count", wt.BaseCommit+"..HEAD")
	if err != nil {
		return Status{}, err
	}
	commits, _ := strconv.Atoi(strings.TrimSpace(out))
	return Status{Files: files, Commits: commits}, nil
}

// Apply writes the session's changes into the checkout it came from, as
// uncommitted edits, and removes the worktree and its branch. Nothing is
// committed, staged or stashed in the checkout. When the changes conflict
// with the checkout, it returns a ConflictError and changes nothing.
func (m *Manager) Apply(ctx context.Context, wt session.SessionWorktree) ([]string, error) {
	files, err := changedFiles(ctx, wt)
	if err != nil {
		return nil, err
	}
	if len(files) > 0 {
		patch, err := worktreePatch(ctx, wt)
		if err != nil {
			return nil, err
		}
		if _, err := gitInput(ctx, wt.RepoRoot, patch, "apply", "--check", "--binary", "-"); err != nil {
			return nil, &ConflictError{Detail: strings.TrimSpace(err.Error())}
		}
		if _, err := gitInput(ctx, wt.RepoRoot, patch, "apply", "--binary", "-"); err != nil {
			return nil, &ConflictError{Detail: strings.TrimSpace(err.Error())}
		}
	}
	return files, m.remove(ctx, wt, true)
}

// Keep commits whatever the session left uncommitted to its branch and
// removes the worktree folder, leaving the branch for a manual merge. It
// returns the branch.
func (m *Manager) Keep(ctx context.Context, wt session.SessionWorktree, message string) (string, error) {
	if _, err := git(ctx, wt.Path, "add", "-A"); err != nil {
		return "", err
	}
	if _, err := git(ctx, wt.Path, "diff", "--cached", "--quiet"); err != nil {
		if strings.TrimSpace(message) == "" {
			message = "TARS session " + filepath.Base(wt.Path)
		}
		args := commitIdentity(ctx, wt.Path)
		args = append(args, "-c", "commit.gpgSign=false", "commit", "--no-verify", "-q", "-m", message)
		if _, err := git(ctx, wt.Path, args...); err != nil {
			return "", fmt.Errorf("commit to %s: %w", wt.Branch, err)
		}
	}
	return wt.Branch, m.remove(ctx, wt, false)
}

// Discard removes the worktree and its branch, dropping the session's
// changes.
func (m *Manager) Discard(ctx context.Context, wt session.SessionWorktree) error {
	return m.remove(ctx, wt, true)
}

// Record is a worktree the manager knows about, read back from its folder.
type Record struct {
	SessionID string
	Worktree  session.SessionWorktree
}

// Records lists the worktrees the manager created and has not retired, so
// a server can find the ones whose session is gone.
func (m *Manager) Records() ([]Record, error) {
	matches, err := filepath.Glob(filepath.Join(m.root, "*", "*.json"))
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, file := range matches {
		raw, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var wt session.SessionWorktree
		if json.Unmarshal(raw, &wt) != nil || wt.Path == "" {
			continue
		}
		records = append(records, Record{SessionID: strings.TrimSuffix(filepath.Base(file), ".json"), Worktree: wt})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].SessionID < records[j].SessionID })
	return records, nil
}

func (m *Manager) recordPath(sessionID, repo string) string {
	return filepath.Join(m.root, repoKey(repo), sessionID+".json")
}

func (m *Manager) writeRecord(sessionID string, wt session.SessionWorktree) error {
	raw, err := json.MarshalIndent(wt, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.recordPath(sessionID, wt.RepoRoot), raw, 0o600)
}

func (m *Manager) remove(ctx context.Context, wt session.SessionWorktree, deleteBranch bool) error {
	if !m.owns(wt.Path) {
		return fmt.Errorf("%s is not a TARS session worktree", wt.Path)
	}
	m.stopIncludes(wt)
	if _, err := os.Stat(wt.Path); err == nil {
		if _, err := git(ctx, wt.RepoRoot, "worktree", "remove", "--force", wt.Path); err != nil {
			// The checkout may be gone; drop the folder and let prune forget it.
			if rmErr := os.RemoveAll(wt.Path); rmErr != nil {
				return fmt.Errorf("remove worktree: %w", err)
			}
		}
	}
	_, _ = git(ctx, wt.RepoRoot, "worktree", "prune")
	if deleteBranch && strings.HasPrefix(wt.Branch, BranchPrefix) {
		_, _ = git(ctx, wt.RepoRoot, "branch", "-D", wt.Branch)
	}
	sessionID := filepath.Base(wt.Path)
	if err := os.Remove(m.recordPath(sessionID, wt.RepoRoot)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// owns reports whether path is a worktree folder under the manager's root,
// so a tampered session record cannot aim a removal elsewhere.
func (m *Manager) owns(path string) bool {
	rel, err := filepath.Rel(m.root, filepath.Clean(path))
	if err != nil {
		return false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	return len(parts) == 2 && parts[0] != ".." && parts[0] != "." && validSessionID(parts[1])
}

func changedFiles(ctx context.Context, wt session.SessionWorktree) ([]string, error) {
	// Tracked changes against the base, committed or not, plus new files.
	out, err := git(ctx, wt.Path, "diff", "--name-only", "-z", wt.BaseCommit)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var files []string
	add := func(list string) {
		for _, name := range strings.Split(list, "\x00") {
			if name != "" && !seen[name] {
				seen[name] = true
				files = append(files, name)
			}
		}
	}
	add(out)
	untracked, err := git(ctx, wt.Path, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	add(untracked)
	sort.Strings(files)
	return files, nil
}

// worktreePatch is the binary diff from the base commit to the worktree's
// files, new files included. It stages into a throwaway index so the
// worktree's own index is left alone.
func worktreePatch(ctx context.Context, wt session.SessionWorktree) ([]byte, error) {
	index, err := os.CreateTemp("", "tars-worktree-index-*")
	if err != nil {
		return nil, err
	}
	indexPath := index.Name()
	_ = index.Close()
	_ = os.Remove(indexPath)
	defer func() { _ = os.Remove(indexPath) }()
	env := []string{"GIT_INDEX_FILE=" + indexPath}
	if _, err := gitEnv(ctx, wt.Path, env, nil, "read-tree", wt.BaseCommit); err != nil {
		return nil, err
	}
	if _, err := gitEnv(ctx, wt.Path, env, nil, "add", "-A"); err != nil {
		return nil, err
	}
	out, err := gitEnv(ctx, wt.Path, env, nil, "diff", "--cached", "--binary", "--no-color", "--no-ext-diff", wt.BaseCommit)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

// commitIdentity supplies a committer only when the repository has none,
// so the person's own identity is used whenever it is set.
func commitIdentity(ctx context.Context, dir string) []string {
	var args []string
	if out, err := git(ctx, dir, "config", "user.name"); err != nil || strings.TrimSpace(out) == "" {
		args = append(args, "-c", "user.name=TARS")
	}
	if out, err := git(ctx, dir, "config", "user.email"); err != nil || strings.TrimSpace(out) == "" {
		args = append(args, "-c", "user.email=tars@localhost")
	}
	return args
}

func freeBranch(ctx context.Context, repo, want string) (string, error) {
	for i := 0; i < 100; i++ {
		name := want
		if i > 0 {
			name = fmt.Sprintf("%s-%d", want, i+1)
		}
		if _, err := git(ctx, repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+name); err != nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("no free branch name for %s", want)
}

func repoKey(repo string) string {
	sum := sha256.Sum256([]byte(filepath.ToSlash(repo)))
	return hex.EncodeToString(sum[:6])
}

// sessionIDPattern is the only shape of session ID a worktree path is built
// from, so an ID can never add a path separator or "..".
var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func validSessionID(id string) bool {
	return sessionIDPattern.MatchString(id)
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	return gitEnv(ctx, dir, nil, nil, args...)
}

func gitInput(ctx context.Context, dir string, input []byte, args ...string) (string, error) {
	return gitEnv(ctx, dir, nil, input, args...)
}

func gitEnv(ctx context.Context, dir string, env []string, input []byte, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.quotepath=false"}, args...)...) // NOSONAR: git is the person's own toolchain, resolved from their PATH like every other TARS git call.
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	cmd.Env = append(cmd.Env, env...)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("git %s: %s", args[0], msg)
	}
	return stdout.String(), nil
}
