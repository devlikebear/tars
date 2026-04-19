package githubops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeGitRunner struct {
	calls  []fakeGitCall
	output []byte
	err    error
}

type fakeGitCall struct {
	dir  string
	args []string
}

func (f *fakeGitRunner) run(_ context.Context, dir string, args []string) ([]byte, error) {
	f.calls = append(f.calls, fakeGitCall{dir: dir, args: append([]string(nil), args...)})
	return f.output, f.err
}

func TestWorktreeSetup_CreatesManagedPath(t *testing.T) {
	workspace := t.TempDir()
	repoDir := filepath.Join(workspace, "src-repo")
	if err := os.Mkdir(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	runner := &fakeGitRunner{}
	managedRoot := func() string { return filepath.Join(workspace, "managed-repos") }
	tl := newWorktreeSetupTool(runner.run, managedRoot)

	payload, _ := json.Marshal(map[string]any{
		"repo_path":   repoDir,
		"branch_name": "fix/bug-1",
		"slug":        "tars-examples-foo",
	})
	res, _ := tl.Execute(context.Background(), payload)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Text())
	}
	var out struct {
		WorktreePath string `json:"worktree_path"`
		Branch       string `json:"branch"`
		Base         string `json:"base"`
	}
	if err := json.Unmarshal([]byte(res.Text()), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	wantPath := filepath.Join(managedRoot(), "tars-examples-foo", "fix", "bug-1")
	if out.WorktreePath != wantPath {
		t.Fatalf("worktree_path mismatch: got %q want %q", out.WorktreePath, wantPath)
	}
	if out.Base != "main" {
		t.Fatalf("default base not main: %q", out.Base)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 git call")
	}
	args := strings.Join(runner.calls[0].args, " ")
	for _, expected := range []string{"worktree add", "-b fix/bug-1", wantPath, "main"} {
		if !strings.Contains(args, expected) {
			t.Fatalf("args missing %q: %s", expected, args)
		}
	}
}

func TestWorktreeSetup_RejectsRelativeRepoPath(t *testing.T) {
	tl := newWorktreeSetupTool((&fakeGitRunner{}).run, func() string { return "/tmp/managed" })
	res, _ := tl.Execute(context.Background(), json.RawMessage(`{"repo_path":"relative/repo","branch_name":"b"}`))
	if !res.IsError || !strings.Contains(res.Text(), "absolute") {
		t.Fatalf("expected absolute error: %s", res.Text())
	}
}

func TestWorktreeSetup_RejectsDuplicate(t *testing.T) {
	workspace := t.TempDir()
	repoDir := filepath.Join(workspace, "src-repo")
	_ = os.Mkdir(repoDir, 0o755)
	managed := filepath.Join(workspace, "managed-repos", "src-repo", "fix", "dup")
	if err := os.MkdirAll(managed, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tl := newWorktreeSetupTool((&fakeGitRunner{}).run, func() string { return filepath.Join(workspace, "managed-repos") })
	payload, _ := json.Marshal(map[string]any{"repo_path": repoDir, "branch_name": "fix/dup"})
	res, _ := tl.Execute(context.Background(), payload)
	if !res.IsError || !strings.Contains(res.Text(), "already exists") {
		t.Fatalf("expected duplicate error: %s", res.Text())
	}
}

func TestWorktreeSetup_RejectsMissingWorkspace(t *testing.T) {
	workspace := t.TempDir()
	repoDir := filepath.Join(workspace, "src-repo")
	_ = os.Mkdir(repoDir, 0o755)
	tl := newWorktreeSetupTool((&fakeGitRunner{}).run, func() string { return "" })
	payload, _ := json.Marshal(map[string]any{"repo_path": repoDir, "branch_name": "fix/x"})
	res, _ := tl.Execute(context.Background(), payload)
	if !res.IsError || !strings.Contains(res.Text(), "workspace not configured") {
		t.Fatalf("expected workspace error: %s", res.Text())
	}
}

func TestWorktreeCleanup_RejectsPathOutsideRoot(t *testing.T) {
	managed := t.TempDir()
	tl := newWorktreeCleanupTool((&fakeGitRunner{}).run, func() string { return managed })
	payload, _ := json.Marshal(map[string]any{
		"repo_path":     "/tmp/src-repo",
		"worktree_path": "/tmp/unrelated/path",
	})
	res, _ := tl.Execute(context.Background(), payload)
	if !res.IsError || !strings.Contains(res.Text(), "managed-repos root") {
		t.Fatalf("expected root-containment error: %s", res.Text())
	}
}

func TestWorktreeCleanup_Success(t *testing.T) {
	managed := t.TempDir()
	worktreePath := filepath.Join(managed, "repo", "fix", "x")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	runner := &fakeGitRunner{output: []byte("")}
	tl := newWorktreeCleanupTool(runner.run, func() string { return managed })
	payload, _ := json.Marshal(map[string]any{
		"repo_path":     "/tmp/src-repo",
		"worktree_path": worktreePath,
		"force":         true,
	})
	res, _ := tl.Execute(context.Background(), payload)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Text())
	}
	args := strings.Join(runner.calls[0].args, " ")
	if !strings.Contains(args, "--force") || !strings.Contains(args, "worktree remove") {
		t.Fatalf("args mismatch: %s", args)
	}
}

func TestIsWithin(t *testing.T) {
	cases := []struct {
		path, root string
		want       bool
	}{
		{"/tmp/root/a", "/tmp/root", true},
		{"/tmp/root", "/tmp/root", false},
		{"/tmp/other", "/tmp/root", false},
		{"/tmp/rootsuffix", "/tmp/root", false},
	}
	for _, c := range cases {
		if got := isWithin(c.path, c.root); got != c.want {
			t.Fatalf("isWithin(%q,%q)=%v want %v", c.path, c.root, got, c.want)
		}
	}
}
