package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var ErrNotRepository = errors.New("not a git repository")

const (
	gitSubcommandAdd            = "add"
	gitSubcommandBranch         = "branch"
	gitSubcommandCheckRefFormat = "check-ref-format"
	gitSubcommandCheckout       = "checkout"
	gitSubcommandClean          = "clean"
	gitSubcommandCommit         = "commit"
	gitSubcommandDiff           = "diff"
	gitSubcommandDiffTree       = "diff-tree"
	gitSubcommandFetch          = "fetch"
	gitSubcommandForEachRef     = "for-each-ref"
	gitSubcommandLog            = "log"
	gitSubcommandRemote         = "remote"
	gitSubcommandRestore        = "restore"
	gitSubcommandRevParse       = "rev-parse"
	gitSubcommandShow           = "show"
	gitSubcommandStatus         = "status"
	gitSubcommandSwitch         = "switch"
	gitSubcommandWorktree       = "worktree"

	gitOptionBranch = "--branch"
)

var allowedGitSubcommands = map[string]struct{}{
	gitSubcommandAdd:            {},
	gitSubcommandBranch:         {},
	gitSubcommandCheckRefFormat: {},
	gitSubcommandCheckout:       {},
	gitSubcommandClean:          {},
	gitSubcommandCommit:         {},
	gitSubcommandDiff:           {},
	gitSubcommandDiffTree:       {},
	gitSubcommandFetch:          {},
	gitSubcommandForEachRef:     {},
	gitSubcommandLog:            {},
	gitSubcommandRemote:         {},
	gitSubcommandRestore:        {},
	gitSubcommandRevParse:       {},
	gitSubcommandShow:           {},
	gitSubcommandStatus:         {},
	gitSubcommandSwitch:         {},
	gitSubcommandWorktree:       {},
}

type Client struct{}

type Remote struct {
	Name     string `json:"name"`
	FetchURL string `json:"fetch_url,omitempty"`
	PushURL  string `json:"push_url,omitempty"`
}

type StatusFile struct {
	Path      string `json:"path"`
	OldPath   string `json:"old_path,omitempty"`
	Index     string `json:"index,omitempty"`
	Worktree  string `json:"worktree,omitempty"`
	Status    string `json:"status"`
	Staged    bool   `json:"staged"`
	Unstaged  bool   `json:"unstaged"`
	Untracked bool   `json:"untracked,omitempty"`
}

type Status struct {
	IsGit    bool         `json:"is_git"`
	Root     string       `json:"root"`
	Branch   string       `json:"branch,omitempty"`
	Head     string       `json:"head,omitempty"`
	Upstream string       `json:"upstream,omitempty"`
	Remotes  []Remote     `json:"remotes,omitempty"`
	Files    []StatusFile `json:"files,omitempty"`
}

type DiffOptions struct {
	StartDir string
	Path     string
	Staged   bool
	Hash     string
}

type Diff struct {
	IsGit  bool   `json:"is_git"`
	Root   string `json:"root"`
	Path   string `json:"path,omitempty"`
	Staged bool   `json:"staged"`
	Hash   string `json:"hash,omitempty"`
	Patch  string `json:"patch"`
}

type Commit struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"short_hash"`
	Author    string `json:"author,omitempty"`
	Date      string `json:"date,omitempty"`
	Subject   string `json:"subject"`
}

type Log struct {
	IsGit   bool     `json:"is_git"`
	Root    string   `json:"root"`
	Commits []Commit `json:"commits"`
}

type Branch struct {
	Name     string `json:"name"`
	Current  bool   `json:"current,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	Remote   bool   `json:"remote,omitempty"`
	Head     string `json:"head,omitempty"`
}

type Branches struct {
	IsGit    bool     `json:"is_git"`
	Root     string   `json:"root"`
	Branches []Branch `json:"branches"`
}

type CommitFile struct {
	Path      string `json:"path"`
	OldPath   string `json:"old_path,omitempty"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary,omitempty"`
}

type CommitDetail struct {
	IsGit     bool         `json:"is_git"`
	Root      string       `json:"root"`
	Hash      string       `json:"hash"`
	ShortHash string       `json:"short_hash"`
	Author    string       `json:"author,omitempty"`
	Email     string       `json:"email,omitempty"`
	Date      string       `json:"date,omitempty"`
	Parents   []string     `json:"parents,omitempty"`
	Subject   string       `json:"subject"`
	Body      string       `json:"body,omitempty"`
	Files     []CommitFile `json:"files"`
}

type Worktree struct {
	Path        string `json:"path"`
	Head        string `json:"head,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Detached    bool   `json:"detached,omitempty"`
	Locked      bool   `json:"locked,omitempty"`
	LockReason  string `json:"lock_reason,omitempty"`
	Prunable    bool   `json:"prunable,omitempty"`
	PruneReason string `json:"prune_reason,omitempty"`
	Bare        bool   `json:"bare,omitempty"`
	Current     bool   `json:"current,omitempty"`
}

type Worktrees struct {
	IsGit     bool       `json:"is_git"`
	Root      string     `json:"root"`
	Worktrees []Worktree `json:"worktrees"`
}

type MutationAction string

const (
	MutationStage          MutationAction = "stage"
	MutationUnstage        MutationAction = "unstage"
	MutationDiscard        MutationAction = "discard"
	MutationCommit         MutationAction = "commit"
	MutationSwitchBranch   MutationAction = "switch_branch"
	MutationCheckoutCommit MutationAction = "checkout_commit"
	MutationWorktreeAdd    MutationAction = "worktree_add"
	MutationWorktreeRemove MutationAction = "worktree_remove"
	MutationFetch          MutationAction = "fetch"
)

type MutationOptions struct {
	StartDir     string
	Action       MutationAction
	Path         string
	Branch       string
	Message      string
	Hash         string
	WorktreePath string
	NewBranch    string
}

type MutationResult struct {
	IsGit        bool           `json:"is_git"`
	Root         string         `json:"root"`
	Action       MutationAction `json:"action"`
	Path         string         `json:"path,omitempty"`
	Branch       string         `json:"branch,omitempty"`
	Message      string         `json:"message,omitempty"`
	Hash         string         `json:"hash,omitempty"`
	WorktreePath string         `json:"worktree_path,omitempty"`
	NewBranch    string         `json:"new_branch,omitempty"`
	Destructive  bool           `json:"destructive,omitempty"`
	Output       string         `json:"output,omitempty"`
}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) RepositoryRoot(ctx context.Context, startDir string) (string, error) {
	startDir, err := normalizeStartDir(startDir)
	if err != nil {
		return "", err
	}
	out, err := runGit(ctx, startDir, gitSubcommandRevParse, "--show-toplevel")
	if err != nil {
		return "", ErrNotRepository
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", ErrNotRepository
	}
	return filepath.Clean(root), nil
}

func (c *Client) Status(ctx context.Context, startDir string) (Status, error) {
	root, err := c.RepositoryRoot(ctx, startDir)
	if err != nil {
		return Status{}, err
	}
	status := Status{IsGit: true, Root: root}
	status.Branch = optionalGitString(ctx, root, gitSubcommandBranch, "--show-current")
	status.Head = optionalGitString(ctx, root, gitSubcommandRevParse, "--short", "HEAD")
	status.Upstream = optionalGitString(ctx, root, gitSubcommandRevParse, "--abbrev-ref", "--symbolic-full-name", "@{u}")
	status.Remotes = parseRemotes(optionalGitString(ctx, root, gitSubcommandRemote, "-v"))

	out, err := runGit(ctx, root, gitSubcommandStatus, "--porcelain=v1", "-z", "-uall")
	if err != nil {
		return Status{}, err
	}
	status.Files = parsePorcelainStatus(string(out))
	return status, nil
}

func (c *Client) Diff(ctx context.Context, opts DiffOptions) (Diff, error) {
	root, err := c.RepositoryRoot(ctx, opts.StartDir)
	if err != nil {
		return Diff{}, err
	}
	path := strings.TrimSpace(opts.Path)
	hash := strings.TrimSpace(opts.Hash)
	var args []string
	if hash != "" {
		args = []string{gitSubcommandShow, "--no-ext-diff", "--format=", hash}
		if path != "" {
			args = append(args, "--", path)
		}
	} else {
		args = []string{gitSubcommandDiff, "--no-ext-diff"}
		if opts.Staged {
			args = append(args, "--cached")
		}
		if path != "" {
			args = append(args, "--", path)
		}
	}
	out, err := runGit(ctx, root, args...)
	if err != nil {
		return Diff{}, err
	}
	return Diff{IsGit: true, Root: root, Path: path, Staged: opts.Staged, Hash: hash, Patch: string(out)}, nil
}

func (c *Client) Log(ctx context.Context, startDir string, limit int) (Log, error) {
	root, err := c.RepositoryRoot(ctx, startDir)
	if err != nil {
		return Log{}, err
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	out, err := runGit(ctx, root, gitSubcommandLog, "--format=%H%x00%h%x00%an%x00%aI%x00%s", "-n", strconv.Itoa(limit))
	if err != nil {
		if strings.Contains(err.Error(), "does not have any commits") || strings.Contains(err.Error(), "current branch") {
			return Log{IsGit: true, Root: root, Commits: []Commit{}}, nil
		}
		return Log{}, err
	}
	return Log{IsGit: true, Root: root, Commits: parseLog(string(out))}, nil
}

func (c *Client) Branches(ctx context.Context, startDir string) (Branches, error) {
	root, err := c.RepositoryRoot(ctx, startDir)
	if err != nil {
		return Branches{}, err
	}
	out, err := runGit(ctx, root, gitSubcommandForEachRef, "--format=%(refname:short)%00%(HEAD)%00%(upstream:short)%00%(refname)%00%(objectname:short)", "refs/heads", "refs/remotes")
	if err != nil {
		return Branches{}, err
	}
	return Branches{IsGit: true, Root: root, Branches: parseBranches(string(out))}, nil
}

func (c *Client) CommitDetail(ctx context.Context, startDir, hash string) (CommitDetail, error) {
	root, err := c.RepositoryRoot(ctx, startDir)
	if err != nil {
		return CommitDetail{}, err
	}
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return CommitDetail{}, fmt.Errorf("hash is required")
	}
	resolved, err := runGit(ctx, root, gitSubcommandRevParse, "--verify", hash+"^{commit}")
	if err != nil {
		return CommitDetail{}, fmt.Errorf("invalid commit hash: %s", hash)
	}
	fullHash := strings.TrimSpace(string(resolved))
	metaOut, err := runGit(ctx, root, gitSubcommandShow, "--no-patch", "--format=%H%x00%h%x00%an%x00%ae%x00%aI%x00%P%x00%s%x1e%b", fullHash)
	if err != nil {
		return CommitDetail{}, err
	}
	detail, err := parseCommitMeta(strings.TrimRight(string(metaOut), "\n"))
	if err != nil {
		return CommitDetail{}, err
	}
	detail.IsGit = true
	detail.Root = root
	files, err := commitFiles(ctx, root, fullHash)
	if err != nil {
		return CommitDetail{}, err
	}
	detail.Files = files
	return detail, nil
}

func (c *Client) Worktrees(ctx context.Context, startDir string) (Worktrees, error) {
	root, err := c.RepositoryRoot(ctx, startDir)
	if err != nil {
		return Worktrees{}, err
	}
	out, err := runGit(ctx, root, gitSubcommandWorktree, "list", "--porcelain", "-z")
	if err != nil {
		return Worktrees{}, err
	}
	list := parseWorktreeList(string(out))
	for i := range list {
		if filepath.Clean(list[i].Path) == root {
			list[i].Current = true
		}
	}
	return Worktrees{IsGit: true, Root: root, Worktrees: list}, nil
}

func (c *Client) Mutate(ctx context.Context, opts MutationOptions) (MutationResult, error) {
	root, err := c.RepositoryRoot(ctx, opts.StartDir)
	if err != nil {
		return MutationResult{}, err
	}
	action := opts.Action
	result := MutationResult{IsGit: true, Root: root, Action: action}
	switch action {
	case MutationStage:
		path, err := normalizeMutationPath(opts.Path)
		if err != nil {
			return MutationResult{}, err
		}
		if _, err := runGit(ctx, root, gitSubcommandAdd, "--", path); err != nil {
			return MutationResult{}, err
		}
		result.Path = path
		result.Output = "staged " + path
	case MutationUnstage:
		path, err := normalizeMutationPath(opts.Path)
		if err != nil {
			return MutationResult{}, err
		}
		if _, err := runGit(ctx, root, gitSubcommandRestore, "--staged", "--", path); err != nil {
			return MutationResult{}, err
		}
		result.Path = path
		result.Output = "unstaged " + path
	case MutationDiscard:
		path, err := normalizeMutationPath(opts.Path)
		if err != nil {
			return MutationResult{}, err
		}
		if isUntrackedPath(ctx, root, path) {
			if _, err := runGit(ctx, root, gitSubcommandClean, "-f", "--", path); err != nil {
				return MutationResult{}, err
			}
		} else if _, err := runGit(ctx, root, gitSubcommandRestore, "--worktree", "--", path); err != nil {
			return MutationResult{}, err
		}
		result.Path = path
		result.Destructive = true
		result.Output = "discarded " + path
	case MutationCommit:
		message := strings.TrimSpace(opts.Message)
		if message == "" {
			return MutationResult{}, fmt.Errorf("commit message is required")
		}
		out, err := runGit(ctx, root, gitSubcommandCommit, "-m", message)
		if err != nil {
			return MutationResult{}, err
		}
		result.Message = message
		result.Output = strings.TrimSpace(string(out))
	case MutationSwitchBranch:
		branch := strings.TrimSpace(opts.Branch)
		if branch == "" {
			return MutationResult{}, fmt.Errorf("branch is required")
		}
		if _, err := runGit(ctx, root, gitSubcommandCheckRefFormat, gitOptionBranch, branch); err != nil {
			return MutationResult{}, err
		}
		out, err := runGit(ctx, root, gitSubcommandSwitch, "--", branch)
		if err != nil {
			return MutationResult{}, err
		}
		result.Branch = branch
		output := strings.TrimSpace(string(out))
		if output == "" {
			output = "switched to " + branch
		}
		result.Output = output
	case MutationCheckoutCommit:
		hash := strings.TrimSpace(opts.Hash)
		if hash == "" {
			return MutationResult{}, fmt.Errorf("hash is required")
		}
		if _, err := runGit(ctx, root, gitSubcommandRevParse, "--verify", hash+"^{commit}"); err != nil {
			return MutationResult{}, fmt.Errorf("invalid commit hash: %s", hash)
		}
		newBranch := strings.TrimSpace(opts.NewBranch)
		args := []string{gitSubcommandCheckout}
		if newBranch != "" {
			if _, err := runGit(ctx, root, gitSubcommandCheckRefFormat, gitOptionBranch, newBranch); err != nil {
				return MutationResult{}, err
			}
			args = append(args, "-b", newBranch, hash)
		} else {
			args = append(args, "--detach", hash)
		}
		out, err := runGit(ctx, root, args...)
		if err != nil {
			return MutationResult{}, err
		}
		result.Hash = hash
		result.NewBranch = newBranch
		result.Destructive = newBranch == ""
		output := strings.TrimSpace(string(out))
		if output == "" {
			if newBranch != "" {
				output = "switched to new branch " + newBranch + " at " + hash
			} else {
				output = "checked out " + hash + " (detached HEAD)"
			}
		}
		result.Output = output
	case MutationWorktreeAdd:
		wtPath, err := normalizeWorktreePath(opts.WorktreePath)
		if err != nil {
			return MutationResult{}, err
		}
		branch := strings.TrimSpace(opts.Branch)
		newBranch := strings.TrimSpace(opts.NewBranch)
		args := []string{gitSubcommandWorktree, "add"}
		if newBranch != "" {
			if _, err := runGit(ctx, root, gitSubcommandCheckRefFormat, gitOptionBranch, newBranch); err != nil {
				return MutationResult{}, err
			}
			args = append(args, "-b", newBranch, wtPath)
			if branch != "" {
				args = append(args, branch)
			}
		} else {
			args = append(args, wtPath)
			if branch != "" {
				args = append(args, branch)
			}
		}
		out, err := runGit(ctx, root, args...)
		if err != nil {
			return MutationResult{}, err
		}
		result.WorktreePath = wtPath
		result.Branch = branch
		result.NewBranch = newBranch
		output := strings.TrimSpace(string(out))
		if output == "" {
			output = "added worktree at " + wtPath
		}
		result.Output = output
	case MutationWorktreeRemove:
		wtPath, err := normalizeWorktreePath(opts.WorktreePath)
		if err != nil {
			return MutationResult{}, err
		}
		out, err := runGit(ctx, root, gitSubcommandWorktree, "remove", "--", wtPath)
		if err != nil {
			return MutationResult{}, err
		}
		result.WorktreePath = wtPath
		result.Destructive = true
		output := strings.TrimSpace(string(out))
		if output == "" {
			output = "removed worktree " + wtPath
		}
		result.Output = output
	case MutationFetch:
		out, err := runGit(ctx, root, gitSubcommandFetch, "--all", "--prune")
		if err != nil {
			return MutationResult{}, err
		}
		output := strings.TrimSpace(string(out))
		if output == "" {
			output = "fetched all remotes"
		}
		result.Output = output
	default:
		return MutationResult{}, fmt.Errorf("unsupported git mutation action: %s", action)
	}
	return result, nil
}

func normalizeStartDir(startDir string) (string, error) {
	startDir = strings.TrimSpace(startDir)
	if startDir == "" {
		startDir = "."
	}
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func normalizeMutationPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be repository-relative")
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path must stay inside the repository")
	}
	return clean, nil
}

func normalizeWorktreePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("worktree_path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("worktree_path: %w", err)
	}
	return filepath.Clean(abs), nil
}

func isUntrackedPath(ctx context.Context, root string, path string) bool {
	out, err := runGit(ctx, root, gitSubcommandStatus, "--porcelain=v1", "-z", "--", path)
	if err != nil {
		return false
	}
	records := strings.Split(string(out), "\x00")
	for _, rec := range records {
		if len(rec) >= 4 && rec[0:2] == "??" {
			return true
		}
	}
	return false
}

func runGit(ctx context.Context, startDir string, args ...string) ([]byte, error) {
	if err := validateGitInvocation(startDir, args); err != nil {
		return nil, err
	}
	cmdArgs := append([]string{"-C", startDir}, args...)
	cmd := exec.CommandContext(ctx, "/usr/bin/git")
	cmd.Args = append([]string{"/usr/bin/git"}, cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func validateGitInvocation(startDir string, args []string) error {
	if strings.TrimSpace(startDir) == "" {
		return fmt.Errorf("git start directory is required")
	}
	if len(args) == 0 {
		return fmt.Errorf("git subcommand is required")
	}
	subcommand := strings.TrimSpace(args[0])
	if subcommand == "" || strings.HasPrefix(subcommand, "-") {
		return fmt.Errorf("unsupported git subcommand: %q", args[0])
	}
	if _, ok := allowedGitSubcommands[subcommand]; !ok {
		return fmt.Errorf("unsupported git subcommand: %s", subcommand)
	}
	for _, arg := range args {
		if strings.ContainsRune(arg, '\x00') {
			return fmt.Errorf("git argument contains NUL byte")
		}
	}
	return nil
}

func optionalGitString(ctx context.Context, root string, args ...string) string {
	out, err := runGit(ctx, root, args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func parseRemotes(raw string) []Remote {
	byName := map[string]*Remote{}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}
		name, url, mode := fields[0], fields[1], strings.Trim(fields[2], "()")
		remote := byName[name]
		if remote == nil {
			remote = &Remote{Name: name}
			byName[name] = remote
		}
		switch mode {
		case "fetch":
			remote.FetchURL = url
		case "push":
			remote.PushURL = url
		}
	}
	out := make([]Remote, 0, len(byName))
	for _, remote := range byName {
		out = append(out, *remote)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func parsePorcelainStatus(raw string) []StatusFile {
	records := strings.Split(raw, "\x00")
	files := make([]StatusFile, 0, len(records))
	for i := 0; i < len(records); i++ {
		rec := records[i]
		if len(rec) < 4 {
			continue
		}
		indexStatus := rec[0:1]
		worktreeStatus := rec[1:2]
		path := rec[3:]
		oldPath := ""
		if indexStatus == "R" || indexStatus == "C" {
			if i+1 < len(records) {
				i++
				oldPath = records[i]
			}
		}
		file := StatusFile{
			Path:     path,
			OldPath:  oldPath,
			Index:    strings.TrimSpace(indexStatus),
			Worktree: strings.TrimSpace(worktreeStatus),
		}
		file.Untracked = indexStatus == "?" && worktreeStatus == "?"
		file.Staged = !file.Untracked && strings.TrimSpace(indexStatus) != ""
		file.Unstaged = file.Untracked || strings.TrimSpace(worktreeStatus) != ""
		file.Status = statusLabel(indexStatus, worktreeStatus)
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

func statusLabel(indexStatus, worktreeStatus string) string {
	if indexStatus == "?" && worktreeStatus == "?" {
		return "untracked"
	}
	for _, code := range []string{indexStatus, worktreeStatus} {
		switch code {
		case "A":
			return "added"
		case "M":
			return "modified"
		case "D":
			return "deleted"
		case "R":
			return "renamed"
		case "C":
			return "copied"
		case "U":
			return "unmerged"
		}
	}
	return "changed"
}

func parseLog(raw string) []Commit {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	commits := make([]Commit, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 5)
		if len(parts) < 5 {
			continue
		}
		commits = append(commits, Commit{
			Hash:      parts[0],
			ShortHash: parts[1],
			Author:    parts[2],
			Date:      parts[3],
			Subject:   parts[4],
		})
	}
	return commits
}

func parseBranches(raw string) []Branch {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	branches := make([]Branch, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 5)
		if len(parts) < 5 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		refName := strings.TrimSpace(parts[3])
		if name == "" || strings.HasSuffix(name, "/HEAD") {
			continue
		}
		branches = append(branches, Branch{
			Name:     name,
			Current:  strings.TrimSpace(parts[1]) == "*",
			Upstream: strings.TrimSpace(parts[2]),
			Remote:   strings.HasPrefix(refName, "refs/remotes/"),
			Head:     strings.TrimSpace(parts[4]),
		})
	}
	sort.SliceStable(branches, func(i, j int) bool {
		if branches[i].Current != branches[j].Current {
			return branches[i].Current
		}
		if branches[i].Remote != branches[j].Remote {
			return !branches[i].Remote
		}
		return strings.ToLower(branches[i].Name) < strings.ToLower(branches[j].Name)
	})
	return branches
}

func parseCommitMeta(raw string) (CommitDetail, error) {
	parts := strings.SplitN(raw, "\x00", 7)
	if len(parts) < 7 {
		return CommitDetail{}, fmt.Errorf("malformed commit metadata")
	}
	detail := CommitDetail{
		Hash:      parts[0],
		ShortHash: parts[1],
		Author:    parts[2],
		Email:     parts[3],
		Date:      parts[4],
	}
	if parents := strings.TrimSpace(parts[5]); parents != "" {
		detail.Parents = strings.Fields(parents)
	}
	subjBody := parts[6]
	if idx := strings.IndexRune(subjBody, '\x1e'); idx >= 0 {
		detail.Subject = subjBody[:idx]
		detail.Body = strings.TrimRight(subjBody[idx+1:], "\n")
	} else {
		detail.Subject = strings.TrimRight(subjBody, "\n")
	}
	return detail, nil
}

func commitFiles(ctx context.Context, root, hash string) ([]CommitFile, error) {
	nameStatusOut, err := runGit(ctx, root, gitSubcommandDiffTree, "--no-commit-id", "--name-status", "--root", "-r", "-z", hash)
	if err != nil {
		return nil, err
	}
	numstatOut, err := runGit(ctx, root, gitSubcommandDiffTree, "--no-commit-id", "--numstat", "--root", "-r", "-z", hash)
	if err != nil {
		return nil, err
	}
	files := parseCommitNameStatus(string(nameStatusOut))
	applyCommitNumstat(files, string(numstatOut))
	return files, nil
}

func parseCommitNameStatus(raw string) []CommitFile {
	files := []CommitFile{}
	fields := strings.Split(raw, "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f == "" {
			continue
		}
		code := f[:1]
		if code == "R" || code == "C" {
			if i+2 >= len(fields) {
				break
			}
			old := fields[i+1]
			next := fields[i+2]
			i += 2
			files = append(files, CommitFile{Path: next, OldPath: old, Status: statusLabelForDiffCode(code)})
		} else {
			if i+1 >= len(fields) {
				break
			}
			path := fields[i+1]
			i++
			files = append(files, CommitFile{Path: path, Status: statusLabelForDiffCode(code)})
		}
	}
	return files
}

func applyCommitNumstat(files []CommitFile, raw string) {
	byPath := make(map[string]int, len(files))
	for i, f := range files {
		byPath[f.Path] = i
	}
	idx := 0
	for idx < len(raw) {
		end := strings.IndexByte(raw[idx:], '\x00')
		if end < 0 {
			break
		}
		rec := raw[idx : idx+end]
		idx += end + 1
		if rec == "" {
			continue
		}
		tab1 := strings.IndexByte(rec, '\t')
		if tab1 < 0 {
			continue
		}
		rest := rec[tab1+1:]
		tab2 := strings.IndexByte(rest, '\t')
		if tab2 < 0 {
			continue
		}
		addStr := rec[:tab1]
		delStr := rest[:tab2]
		pathField := rest[tab2+1:]
		var path string
		if pathField == "" {
			if idx >= len(raw) {
				break
			}
			end1 := strings.IndexByte(raw[idx:], '\x00')
			if end1 < 0 {
				break
			}
			idx += end1 + 1
			end2 := strings.IndexByte(raw[idx:], '\x00')
			if end2 < 0 {
				break
			}
			path = raw[idx : idx+end2]
			idx += end2 + 1
		} else {
			path = pathField
		}
		fileIdx, ok := byPath[path]
		if !ok {
			continue
		}
		if addStr == "-" && delStr == "-" {
			files[fileIdx].Binary = true
			continue
		}
		add, _ := strconv.Atoi(addStr)
		del, _ := strconv.Atoi(delStr)
		files[fileIdx].Additions = add
		files[fileIdx].Deletions = del
	}
}

func statusLabelForDiffCode(code string) string {
	switch code {
	case "A":
		return "added"
	case "M":
		return "modified"
	case "D":
		return "deleted"
	case "R":
		return "renamed"
	case "C":
		return "copied"
	case "T":
		return "type_changed"
	case "U":
		return "unmerged"
	default:
		return "changed"
	}
}

func parseWorktreeList(raw string) []Worktree {
	result := []Worktree{}
	blocks := strings.Split(raw, "\x00\x00")
	for _, block := range blocks {
		block = strings.Trim(block, "\x00")
		if block == "" {
			continue
		}
		var wt Worktree
		for _, line := range strings.Split(block, "\x00") {
			if line == "" {
				continue
			}
			key, val, _ := strings.Cut(line, " ")
			switch key {
			case "worktree":
				wt.Path = filepath.Clean(val)
			case "HEAD":
				wt.Head = val
			case "branch":
				wt.Branch = strings.TrimPrefix(val, "refs/heads/")
			case "detached":
				wt.Detached = true
			case "locked":
				wt.Locked = true
				wt.LockReason = val
			case "prunable":
				wt.Prunable = true
				wt.PruneReason = val
			case "bare":
				wt.Bare = true
			}
		}
		if wt.Path != "" {
			result = append(result, wt)
		}
	}
	return result
}
