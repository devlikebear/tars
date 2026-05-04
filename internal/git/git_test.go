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

func TestClientCommitDetailReturnsFilesAndStats(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init", "-b", "main")
	runGitCmd(t, repo, "config", "user.email", "tars@example.test")
	runGitCmd(t, repo, "config", "user.name", "TARS Test")

	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "old.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("write old.txt: %v", err)
	}
	runGitCmd(t, repo, "add", "README.md", "old.txt")
	runGitCmd(t, repo, "commit", "-m", "initial")

	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatalf("modify readme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "added.txt"), []byte("brand new\n"), 0o644); err != nil {
		t.Fatalf("write added.txt: %v", err)
	}
	if err := os.Remove(filepath.Join(repo, "old.txt")); err != nil {
		t.Fatalf("remove old.txt: %v", err)
	}
	runGitCmd(t, repo, "add", "-A")
	runGitCmd(t, repo, "commit", "-m", "second commit\n\nbody line one\nbody line two\n")

	client := NewClient()
	headOut, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v\n%s", err, headOut)
	}
	hash := strings.TrimSpace(string(headOut))

	detail, err := client.CommitDetail(context.Background(), repo, hash)
	if err != nil {
		t.Fatalf("commit detail: %v", err)
	}
	if !detail.IsGit || detail.Hash != hash || detail.Subject != "second commit" {
		t.Fatalf("unexpected commit metadata: %+v", detail)
	}
	if !strings.Contains(detail.Body, "body line one") || !strings.Contains(detail.Body, "body line two") {
		t.Fatalf("expected body with two lines, got %q", detail.Body)
	}
	if len(detail.Parents) != 1 {
		t.Fatalf("expected one parent, got %v", detail.Parents)
	}

	wantStatus := map[string]string{"README.md": "modified", "added.txt": "added", "old.txt": "deleted"}
	for path, status := range wantStatus {
		got := commitFileByPath(detail.Files, path)
		if got == nil {
			t.Fatalf("expected file %s in commit, got %+v", path, detail.Files)
		}
		if got.Status != status {
			t.Fatalf("expected %s status=%s, got %s", path, status, got.Status)
		}
	}
	readme := commitFileByPath(detail.Files, "README.md")
	if readme.Additions != 1 || readme.Deletions != 0 {
		t.Fatalf("expected README.md +1 -0, got +%d -%d", readme.Additions, readme.Deletions)
	}

	// Diff for a specific commit + path
	diff, err := client.Diff(context.Background(), DiffOptions{StartDir: repo, Hash: hash, Path: "README.md"})
	if err != nil {
		t.Fatalf("commit diff: %v", err)
	}
	if diff.Hash != hash || !strings.Contains(diff.Patch, "+world") {
		t.Fatalf("expected commit diff with +world, got %+v", diff)
	}

	// Initial commit (no parent) — must still report files via --root
	logOut, err := exec.Command("git", "-C", repo, "rev-list", "--max-parents=0", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-list root: %v\n%s", err, logOut)
	}
	rootHash := strings.TrimSpace(string(logOut))
	rootDetail, err := client.CommitDetail(context.Background(), repo, rootHash)
	if err != nil {
		t.Fatalf("root commit detail: %v", err)
	}
	if len(rootDetail.Files) != 2 {
		t.Fatalf("expected 2 files in initial commit, got %+v", rootDetail.Files)
	}
}

func TestClientCommitDetailRejectsInvalidHash(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init", "-b", "main")
	runGitCmd(t, repo, "config", "user.email", "tars@example.test")
	runGitCmd(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitCmd(t, repo, "add", "f")
	runGitCmd(t, repo, "commit", "-m", "init")
	client := NewClient()
	if _, err := client.CommitDetail(context.Background(), repo, ""); err == nil {
		t.Fatalf("expected error for empty hash")
	}
	if _, err := client.CommitDetail(context.Background(), repo, "deadbeefdeadbeef"); err == nil {
		t.Fatalf("expected error for unknown hash")
	}
}

func TestClientWorktreesEnumerates(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init", "-b", "main")
	runGitCmd(t, repo, "config", "user.email", "tars@example.test")
	runGitCmd(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitCmd(t, repo, "add", "f")
	runGitCmd(t, repo, "commit", "-m", "init")

	wtDir := filepath.Join(t.TempDir(), "wt")
	runGitCmd(t, repo, "worktree", "add", "-b", "feature", wtDir)

	client := NewClient()
	res, err := client.Worktrees(context.Background(), repo)
	if err != nil {
		t.Fatalf("worktrees: %v", err)
	}
	if !res.IsGit || len(res.Worktrees) != 2 {
		t.Fatalf("expected 2 worktrees, got %+v", res)
	}
	var foundMain, foundFeature bool
	for _, wt := range res.Worktrees {
		switch wt.Branch {
		case "main":
			foundMain = true
			if !wt.Current {
				t.Fatalf("expected main worktree to be current, got %+v", wt)
			}
		case "feature":
			foundFeature = true
			if wt.Current {
				t.Fatalf("did not expect feature worktree to be current, got %+v", wt)
			}
			if filepath.Clean(wt.Path) != canonicalTestPath(t, wtDir) {
				t.Fatalf("expected feature worktree path %s, got %s", wtDir, wt.Path)
			}
		}
	}
	if !foundMain || !foundFeature {
		t.Fatalf("expected both main and feature worktrees, got %+v", res.Worktrees)
	}
}

func TestClientCheckoutCommitDetachesAndCanCreateBranch(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init", "-b", "main")
	runGitCmd(t, repo, "config", "user.email", "tars@example.test")
	runGitCmd(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitCmd(t, repo, "add", "f")
	runGitCmd(t, repo, "commit", "-m", "first")
	first := strings.TrimSpace(string(runGitOutput(t, repo, "rev-parse", "HEAD")))
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("b\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitCmd(t, repo, "add", "f")
	runGitCmd(t, repo, "commit", "-m", "second")

	client := NewClient()
	res, err := client.Mutate(context.Background(), MutationOptions{StartDir: repo, Action: MutationCheckoutCommit, Hash: first})
	if err != nil {
		t.Fatalf("checkout commit: %v", err)
	}
	if res.Hash != first || !res.Destructive {
		t.Fatalf("expected destructive checkout result, got %+v", res)
	}
	branch := strings.TrimSpace(string(runGitOutput(t, repo, "branch", "--show-current")))
	if branch != "" {
		t.Fatalf("expected detached HEAD (empty branch), got %q", branch)
	}
	runGitCmd(t, repo, "switch", "main")
	res2, err := client.Mutate(context.Background(), MutationOptions{StartDir: repo, Action: MutationCheckoutCommit, Hash: first, NewBranch: "from-checkout"})
	if err != nil {
		t.Fatalf("checkout commit with new branch: %v", err)
	}
	if res2.NewBranch != "from-checkout" || res2.Destructive {
		t.Fatalf("unexpected new-branch checkout result: %+v", res2)
	}
	cur := strings.TrimSpace(string(runGitOutput(t, repo, "branch", "--show-current")))
	if cur != "from-checkout" {
		t.Fatalf("expected to be on from-checkout, got %q", cur)
	}
}

func TestClientCheckoutCommitRejectsInvalidHash(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init", "-b", "main")
	runGitCmd(t, repo, "config", "user.email", "tars@example.test")
	runGitCmd(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitCmd(t, repo, "add", "f")
	runGitCmd(t, repo, "commit", "-m", "first")
	client := NewClient()
	if _, err := client.Mutate(context.Background(), MutationOptions{StartDir: repo, Action: MutationCheckoutCommit}); err == nil {
		t.Fatalf("expected error for missing hash")
	}
	if _, err := client.Mutate(context.Background(), MutationOptions{StartDir: repo, Action: MutationCheckoutCommit, Hash: "deadbeefdead"}); err == nil {
		t.Fatalf("expected error for unknown hash")
	}
}

func TestClientWorktreeAddAndRemove(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init", "-b", "main")
	runGitCmd(t, repo, "config", "user.email", "tars@example.test")
	runGitCmd(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitCmd(t, repo, "add", "f")
	runGitCmd(t, repo, "commit", "-m", "first")

	wtPath := filepath.Join(t.TempDir(), "wt-feature")
	client := NewClient()
	add, err := client.Mutate(context.Background(), MutationOptions{
		StartDir:     repo,
		Action:       MutationWorktreeAdd,
		WorktreePath: wtPath,
		NewBranch:    "feature",
	})
	if err != nil {
		t.Fatalf("worktree add: %v", err)
	}
	if filepath.Clean(add.WorktreePath) != filepath.Clean(wtPath) || add.NewBranch != "feature" {
		t.Fatalf("unexpected worktree add result: %+v", add)
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("expected worktree dir to exist: %v", err)
	}

	trees, err := client.Worktrees(context.Background(), repo)
	if err != nil {
		t.Fatalf("worktrees: %v", err)
	}
	if len(trees.Worktrees) != 2 {
		t.Fatalf("expected 2 worktrees, got %+v", trees)
	}

	remove, err := client.Mutate(context.Background(), MutationOptions{
		StartDir:     repo,
		Action:       MutationWorktreeRemove,
		WorktreePath: wtPath,
	})
	if err != nil {
		t.Fatalf("worktree remove: %v", err)
	}
	if !remove.Destructive {
		t.Fatalf("expected destructive remove result, got %+v", remove)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("expected worktree dir removed, got err=%v", err)
	}
}

func TestClientWorktreeAddRejectsMissingPath(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init", "-b", "main")
	runGitCmd(t, repo, "config", "user.email", "tars@example.test")
	runGitCmd(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitCmd(t, repo, "add", "f")
	runGitCmd(t, repo, "commit", "-m", "first")
	client := NewClient()
	if _, err := client.Mutate(context.Background(), MutationOptions{StartDir: repo, Action: MutationWorktreeAdd}); err == nil {
		t.Fatalf("expected error for missing worktree_path")
	}
	if _, err := client.Mutate(context.Background(), MutationOptions{StartDir: repo, Action: MutationWorktreeRemove}); err == nil {
		t.Fatalf("expected error for missing worktree_path")
	}
}

func TestClientFetchUpdatesRemoteRefs(t *testing.T) {
	upstream := t.TempDir()
	runGitCmd(t, upstream, "init", "-b", "main", "--bare")

	clone := t.TempDir()
	runGitCmd(t, clone, "init", "-b", "main")
	runGitCmd(t, clone, "config", "user.email", "tars@example.test")
	runGitCmd(t, clone, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(clone, "f"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitCmd(t, clone, "add", "f")
	runGitCmd(t, clone, "commit", "-m", "first")
	runGitCmd(t, clone, "remote", "add", "origin", upstream)
	runGitCmd(t, clone, "push", "origin", "main")

	worker := t.TempDir()
	runGitCmd(t, worker, "clone", upstream, ".")
	runGitCmd(t, worker, "config", "user.email", "tars@example.test")
	runGitCmd(t, worker, "config", "user.name", "TARS Test")
	runGitCmd(t, worker, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(worker, "g"), []byte("b\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitCmd(t, worker, "add", "g")
	runGitCmd(t, worker, "commit", "-m", "feature commit")
	runGitCmd(t, worker, "push", "origin", "feature")

	client := NewClient()
	res, err := client.Mutate(context.Background(), MutationOptions{StartDir: clone, Action: MutationFetch})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if res.Action != MutationFetch || res.Destructive {
		t.Fatalf("unexpected fetch result: %+v", res)
	}

	branches, err := client.Branches(context.Background(), clone)
	if err != nil {
		t.Fatalf("branches: %v", err)
	}
	var foundFeature bool
	for _, b := range branches.Branches {
		if b.Remote && b.Name == "origin/feature" {
			foundFeature = true
			break
		}
	}
	if !foundFeature {
		t.Fatalf("expected origin/feature after fetch, got %+v", branches.Branches)
	}
}

func TestClientSwitchBranchDWIMCreatesTrackingFromRemote(t *testing.T) {
	upstream := t.TempDir()
	runGitCmd(t, upstream, "init", "-b", "main", "--bare")

	worker := t.TempDir()
	runGitCmd(t, worker, "clone", upstream, ".")
	runGitCmd(t, worker, "config", "user.email", "tars@example.test")
	runGitCmd(t, worker, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(worker, "f"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitCmd(t, worker, "add", "f")
	runGitCmd(t, worker, "commit", "-m", "main commit")
	runGitCmd(t, worker, "push", "origin", "main")
	runGitCmd(t, worker, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(worker, "g"), []byte("b\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitCmd(t, worker, "add", "g")
	runGitCmd(t, worker, "commit", "-m", "feature commit")
	runGitCmd(t, worker, "push", "origin", "feature")

	clone := t.TempDir()
	runGitCmd(t, clone, "clone", upstream, ".")

	client := NewClient()
	res, err := client.Mutate(context.Background(), MutationOptions{StartDir: clone, Action: MutationSwitchBranch, Branch: "feature"})
	if err != nil {
		t.Fatalf("switch to remote-only branch: %v", err)
	}
	if res.Branch != "feature" {
		t.Fatalf("unexpected switch result: %+v", res)
	}
	cur := strings.TrimSpace(string(runGitOutput(t, clone, "branch", "--show-current")))
	if cur != "feature" {
		t.Fatalf("expected to be on feature, got %q", cur)
	}
	upstreamRef := strings.TrimSpace(string(runGitOutput(t, clone, "rev-parse", "--abbrev-ref", "feature@{upstream}")))
	if upstreamRef != "origin/feature" {
		t.Fatalf("expected upstream origin/feature, got %q", upstreamRef)
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func commitFileByPath(files []CommitFile, path string) *CommitFile {
	for i := range files {
		if files[i].Path == path {
			return &files[i]
		}
	}
	return nil
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
