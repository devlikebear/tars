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
}

type Diff struct {
	IsGit  bool   `json:"is_git"`
	Root   string `json:"root"`
	Path   string `json:"path,omitempty"`
	Staged bool   `json:"staged"`
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

func NewClient() *Client {
	return &Client{}
}

func (c *Client) RepositoryRoot(ctx context.Context, startDir string) (string, error) {
	startDir, err := normalizeStartDir(startDir)
	if err != nil {
		return "", err
	}
	out, err := runGit(ctx, startDir, "rev-parse", "--show-toplevel")
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
	status.Branch = optionalGitString(ctx, root, "branch", "--show-current")
	status.Head = optionalGitString(ctx, root, "rev-parse", "--short", "HEAD")
	status.Upstream = optionalGitString(ctx, root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	status.Remotes = parseRemotes(optionalGitString(ctx, root, "remote", "-v"))

	out, err := runGit(ctx, root, "status", "--porcelain=v1", "-z", "-uall")
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
	args := []string{"diff", "--no-ext-diff"}
	if opts.Staged {
		args = append(args, "--cached")
	}
	if path != "" {
		args = append(args, "--", path)
	}
	out, err := runGit(ctx, root, args...)
	if err != nil {
		return Diff{}, err
	}
	return Diff{IsGit: true, Root: root, Path: path, Staged: opts.Staged, Patch: string(out)}, nil
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
	out, err := runGit(ctx, root, "log", "--format=%H%x00%h%x00%an%x00%aI%x00%s", "-n", strconv.Itoa(limit))
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
	out, err := runGit(ctx, root, "for-each-ref", "--format=%(refname:short)%00%(HEAD)%00%(upstream:short)%00%(refname)%00%(objectname:short)", "refs/heads", "refs/remotes")
	if err != nil {
		return Branches{}, err
	}
	return Branches{IsGit: true, Root: root, Branches: parseBranches(string(out))}, nil
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

func runGit(ctx context.Context, startDir string, args ...string) ([]byte, error) {
	cmdArgs := append([]string{"-C", startDir}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
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
