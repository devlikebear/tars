package githubops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/tool"
)

type worktreeSetupInput struct {
	RepoPath   string `json:"repo_path"`
	BranchName string `json:"branch_name"`
	Base       string `json:"base,omitempty"`
	Slug       string `json:"slug,omitempty"`
}

type worktreeSetupOutput struct {
	WorktreePath string `json:"worktree_path"`
	Branch       string `json:"branch"`
	Base         string `json:"base"`
}

// managedRootFn is injected so tests can redirect the managed-repos root.
type managedRootFn func() string

func newWorktreeSetupTool(run gitRunner, managedRoot managedRootFn) tool.Tool {
	if run == nil {
		run = defaultGitRunner
	}
	return tool.Tool{
		Name: "gh_worktree_setup",
		Description: "Create an isolated git worktree for an external repository under " +
			"workspace/managed-repos/<slug>/<branch>/. Uses `git worktree add -b <branch> <base>`.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "repo_path":  {"type":"string","description":"Absolute path of the local clone of the target repo."},
    "branch_name":{"type":"string","description":"New branch to create."},
    "base":       {"type":"string","description":"Base branch or commit (default 'main')."},
    "slug":       {"type":"string","description":"Optional subdirectory name under managed-repos (defaults to the repo folder name)."}
  },
  "required":["repo_path","branch_name"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (tool.Result, error) {
			var input worktreeSetupInput
			if err := json.Unmarshal(params, &input); err != nil {
				return tool.JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			input.RepoPath = strings.TrimSpace(input.RepoPath)
			input.BranchName = strings.TrimSpace(input.BranchName)
			input.Base = strings.TrimSpace(input.Base)
			input.Slug = strings.TrimSpace(input.Slug)
			if input.RepoPath == "" {
				return tool.JSONTextResult(map[string]any{"message": "repo_path is required"}, true), nil
			}
			if input.BranchName == "" || !validBranch.MatchString(input.BranchName) {
				return tool.JSONTextResult(map[string]any{"message": "branch_name is invalid"}, true), nil
			}
			if input.Base == "" {
				input.Base = "main"
			}
			if !validBranch.MatchString(input.Base) {
				return tool.JSONTextResult(map[string]any{"message": "base is invalid"}, true), nil
			}
			if !filepath.IsAbs(input.RepoPath) {
				return tool.JSONTextResult(map[string]any{"message": "repo_path must be absolute"}, true), nil
			}
			if info, err := os.Stat(input.RepoPath); err != nil || !info.IsDir() {
				return tool.JSONTextResult(map[string]any{"message": "repo_path does not exist or is not a directory"}, true), nil
			}

			root := managedRoot()
			if root == "" {
				return tool.JSONTextResult(map[string]any{"message": "managed-repos root unavailable (workspace not configured)"}, true), nil
			}
			slug := input.Slug
			if slug == "" {
				slug = filepath.Base(input.RepoPath)
			}
			if !validBranch.MatchString(slug) {
				return tool.JSONTextResult(map[string]any{"message": "slug contains invalid characters"}, true), nil
			}
			worktreePath := filepath.Join(root, slug, filepath.FromSlash(input.BranchName))
			if _, err := os.Stat(worktreePath); err == nil {
				return tool.JSONTextResult(map[string]any{"message": "worktree path already exists", "worktree_path": worktreePath}, true), nil
			} else if !errors.Is(err, fs.ErrNotExist) {
				return tool.JSONTextResult(map[string]any{"message": "stat worktree path failed", "detail": err.Error()}, true), nil
			}
			if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
				return tool.JSONTextResult(map[string]any{"message": "create parent dir failed", "detail": err.Error()}, true), nil
			}

			args := []string{"-C", input.RepoPath, "worktree", "add", "-b", input.BranchName, worktreePath, input.Base}
			output, err := run(ctx, "", args)
			if err != nil {
				msg, detail := wrapRunError(err, output)
				resp := map[string]any{"message": msg}
				for k, v := range detail {
					resp[k] = v
				}
				return tool.JSONTextResult(resp, true), nil
			}
			return tool.JSONTextResult(worktreeSetupOutput{
				WorktreePath: worktreePath,
				Branch:       input.BranchName,
				Base:         input.Base,
			}, false), nil
		},
	}
}

type worktreeCleanupInput struct {
	RepoPath     string `json:"repo_path"`
	WorktreePath string `json:"worktree_path"`
	Force        bool   `json:"force,omitempty"`
}

func newWorktreeCleanupTool(run gitRunner, managedRoot managedRootFn) tool.Tool {
	if run == nil {
		run = defaultGitRunner
	}
	return tool.Tool{
		Name: "gh_worktree_cleanup",
		Description: "Remove an isolated worktree previously created by gh_worktree_setup. " +
			"Only paths under workspace/managed-repos/ are accepted.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "repo_path":     {"type":"string"},
    "worktree_path": {"type":"string"},
    "force":         {"type":"boolean","default":false}
  },
  "required":["repo_path","worktree_path"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (tool.Result, error) {
			var input worktreeCleanupInput
			if err := json.Unmarshal(params, &input); err != nil {
				return tool.JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			input.RepoPath = strings.TrimSpace(input.RepoPath)
			input.WorktreePath = strings.TrimSpace(input.WorktreePath)
			if input.RepoPath == "" || !filepath.IsAbs(input.RepoPath) {
				return tool.JSONTextResult(map[string]any{"message": "repo_path must be absolute"}, true), nil
			}
			if input.WorktreePath == "" || !filepath.IsAbs(input.WorktreePath) {
				return tool.JSONTextResult(map[string]any{"message": "worktree_path must be absolute"}, true), nil
			}
			root := managedRoot()
			if root == "" {
				return tool.JSONTextResult(map[string]any{"message": "managed-repos root unavailable (workspace not configured)"}, true), nil
			}
			if !isWithin(input.WorktreePath, root) {
				return tool.JSONTextResult(map[string]any{"message": "worktree_path must be under managed-repos root", "root": root}, true), nil
			}

			args := []string{"-C", input.RepoPath, "worktree", "remove", input.WorktreePath}
			if input.Force {
				args = append(args, "--force")
			}
			output, err := run(ctx, "", args)
			if err != nil {
				msg, detail := wrapRunError(err, output)
				resp := map[string]any{"message": msg, "removed": false}
				for k, v := range detail {
					resp[k] = v
				}
				return tool.JSONTextResult(resp, true), nil
			}
			return tool.JSONTextResult(map[string]any{"removed": true, "worktree_path": input.WorktreePath}, false), nil
		},
	}
}

func isWithin(path, root string) bool {
	clean := filepath.Clean(path)
	rootClean := filepath.Clean(root)
	if clean == rootClean {
		return false
	}
	prefix := rootClean + string(os.PathSeparator)
	return strings.HasPrefix(clean, prefix)
}
