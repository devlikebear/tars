package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientStatusAndDiffAreReadOnly(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init", "-b", "main")
	runGitCmd(t, repo, "config", "user.email", "tars@example.test")
	runGitCmd(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "initial")
	runGitCmd(t, repo, "remote", "add", "origin", "https://example.test/tars.git")

	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\nchanged\n"), 0o644); err != nil {
		t.Fatalf("modify readme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "notes.md"), []byte("new note\n"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	runGitCmd(t, repo, "add", "notes.md")

	client := NewClient()
	status, err := client.Status(context.Background(), repo)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !status.IsGit || status.Root != canonicalTestPath(t, repo) || status.Branch != "main" {
		t.Fatalf("unexpected repo metadata: %+v", status)
	}
	if got := fileByPath(status.Files, "README.md"); got == nil || !got.Unstaged || got.Staged || got.Status != "modified" {
		t.Fatalf("expected unstaged README.md modification, got %+v", got)
	}
	if got := fileByPath(status.Files, "notes.md"); got == nil || !got.Staged || got.Unstaged || got.Status != "added" {
		t.Fatalf("expected staged notes.md addition, got %+v", got)
	}
	if len(status.Remotes) != 1 || status.Remotes[0].Name != "origin" || status.Remotes[0].FetchURL != "https://example.test/tars.git" {
		t.Fatalf("expected origin remote, got %+v", status.Remotes)
	}

	unstaged, err := client.Diff(context.Background(), DiffOptions{StartDir: repo, Path: "README.md"})
	if err != nil {
		t.Fatalf("unstaged diff: %v", err)
	}
	if unstaged.Staged || !strings.Contains(unstaged.Patch, "+changed") {
		t.Fatalf("expected unstaged README diff, got %+v", unstaged)
	}

	staged, err := client.Diff(context.Background(), DiffOptions{StartDir: repo, Path: "notes.md", Staged: true})
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if !staged.Staged || !strings.Contains(staged.Patch, "+new note") {
		t.Fatalf("expected staged notes diff, got %+v", staged)
	}

	log, err := client.Log(context.Background(), repo, 5)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if len(log.Commits) != 1 || log.Commits[0].Subject != "initial" {
		t.Fatalf("expected initial commit in log, got %+v", log.Commits)
	}

	branches, err := client.Branches(context.Background(), repo)
	if err != nil {
		t.Fatalf("branches: %v", err)
	}
	if len(branches.Branches) == 0 || branches.Branches[0].Name != "main" || !branches.Branches[0].Current {
		t.Fatalf("expected current main branch, got %+v", branches.Branches)
	}
}

func TestClientMutationsStageCommitAndSwitch(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init", "-b", "main")
	runGitCmd(t, repo, "config", "user.email", "tars@example.test")
	runGitCmd(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\nchanged\n"), 0o644); err != nil {
		t.Fatalf("modify readme: %v", err)
	}

	client := NewClient()
	stage, err := client.Mutate(context.Background(), MutationOptions{
		StartDir: repo,
		Action:   MutationStage,
		Path:     "README.md",
	})
	if err != nil {
		t.Fatalf("stage mutation: %v", err)
	}
	if stage.Action != MutationStage || !strings.Contains(stage.Output, "staged README.md") {
		t.Fatalf("unexpected stage result: %+v", stage)
	}
	status, err := client.Status(context.Background(), repo)
	if err != nil {
		t.Fatalf("status after stage: %v", err)
	}
	if got := fileByPath(status.Files, "README.md"); got == nil || !got.Staged || got.Unstaged {
		t.Fatalf("expected staged README.md after mutation, got %+v", got)
	}

	commit, err := client.Mutate(context.Background(), MutationOptions{
		StartDir: repo,
		Action:   MutationCommit,
		Message:  "update readme",
	})
	if err != nil {
		t.Fatalf("commit mutation: %v", err)
	}
	if commit.Action != MutationCommit || !strings.Contains(commit.Output, "update readme") {
		t.Fatalf("unexpected commit result: %+v", commit)
	}

	runGitCmd(t, repo, "branch", "feature/demo")
	switched, err := client.Mutate(context.Background(), MutationOptions{
		StartDir: repo,
		Action:   MutationSwitchBranch,
		Branch:   "feature/demo",
	})
	if err != nil {
		t.Fatalf("switch mutation: %v", err)
	}
	if switched.Action != MutationSwitchBranch || !strings.Contains(switched.Output, "feature/demo") {
		t.Fatalf("unexpected switch result: %+v", switched)
	}
	status, err = client.Status(context.Background(), repo)
	if err != nil {
		t.Fatalf("status after switch: %v", err)
	}
	if status.Branch != "feature/demo" {
		t.Fatalf("expected feature/demo branch, got %+v", status)
	}
}

func TestClientDiscardMutationRestoresWorktree(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init", "-b", "main")
	runGitCmd(t, repo, "config", "user.email", "tars@example.test")
	runGitCmd(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("oops\n"), 0o644); err != nil {
		t.Fatalf("modify readme: %v", err)
	}

	client := NewClient()
	result, err := client.Mutate(context.Background(), MutationOptions{
		StartDir: repo,
		Action:   MutationDiscard,
		Path:     "README.md",
	})
	if err != nil {
		t.Fatalf("discard mutation: %v", err)
	}
	if result.Action != MutationDiscard || !result.Destructive {
		t.Fatalf("expected destructive discard result, got %+v", result)
	}
	raw, err := os.ReadFile(filepath.Join(repo, "README.md"))
	if err != nil {
		t.Fatalf("read readme: %v", err)
	}
	if string(raw) != "hello\n" {
		t.Fatalf("expected discard to restore file, got %q", raw)
	}
}

func fileByPath(files []StatusFile, path string) *StatusFile {
	for i := range files {
		if files[i].Path == path {
			return &files[i]
		}
	}
	return nil
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func canonicalTestPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(resolved)
}
