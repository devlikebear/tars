package executionplane

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedWorktreeProviderPreservesDirtySourceAndCleansOwnedEnvironment(t *testing.T) {
	t.Parallel()

	source := initGitRepository(t)
	tracked := filepath.Join(source, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("user dirty change\n"), 0o600); err != nil {
		t.Fatalf("dirty tracked file: %v", err)
	}
	untracked := filepath.Join(source, "user-note.txt")
	if err := os.WriteFile(untracked, []byte("keep me\n"), 0o600); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}
	managedRoot := filepath.Join(t.TempDir(), "managed")
	provider, err := NewManagedWorktreeProvider(source, managedRoot)
	if err != nil {
		t.Fatalf("new managed worktree provider: %v", err)
	}
	execution := testExecution()
	environment, err := provider.Provision(context.Background(), ProvisionRequest{Execution: execution, SourceDir: source})
	if err != nil {
		t.Fatalf("provision managed worktree: %v", err)
	}
	if environment.Kind != "managed-worktree" || !pathWithin(managedRoot, environment.RootDir) {
		t.Fatalf("managed environment = %#v", environment)
	}
	worktreeTracked, err := os.ReadFile(filepath.Join(environment.RootDir, "tracked.txt"))
	if err != nil || string(worktreeTracked) != "committed\n" {
		t.Fatalf("worktree inherited dirty source: %q, %v", worktreeTracked, err)
	}
	if _, err := os.Stat(filepath.Join(environment.RootDir, "user-note.txt")); !os.IsNotExist(err) {
		t.Fatalf("worktree copied source untracked file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(environment.RootDir, "worker.txt"), []byte("worker output\n"), 0o600); err != nil {
		t.Fatalf("write worker output: %v", err)
	}
	snapshot, err := provider.Sync(context.Background(), environment)
	if err != nil || snapshot.Digest == "" {
		t.Fatalf("sync managed worktree = %#v, %v", snapshot, err)
	}
	recovered, err := provider.Recover(context.Background(), environment)
	if err != nil || recovered.ID != environment.ID {
		t.Fatalf("recover managed worktree = %#v, %v", recovered, err)
	}
	if err := provider.Destroy(context.Background(), environment); err != nil {
		t.Fatalf("destroy managed worktree: %v", err)
	}
	if _, err := os.Stat(environment.RootDir); !os.IsNotExist(err) {
		t.Fatalf("managed worktree remains after destroy: %v", err)
	}
	if got, err := os.ReadFile(tracked); err != nil || string(got) != "user dirty change\n" {
		t.Fatalf("source tracked change was altered: %q, %v", got, err)
	}
	if got, err := os.ReadFile(untracked); err != nil || string(got) != "keep me\n" {
		t.Fatalf("source untracked file was altered: %q, %v", got, err)
	}
	worktrees := runGit(t, source, "worktree", "list", "--porcelain")
	if strings.Contains(worktrees, environment.RootDir) {
		t.Fatalf("git worktree registration remains: %s", worktrees)
	}
	capabilities := provider.Capabilities()
	if !capabilities.Recoverable || !capabilities.Snapshot || !capabilities.Cleanup || !capabilities.FilesystemIsolation || capabilities.EgressPolicy {
		t.Fatalf("managed worktree capabilities = %#v", capabilities)
	}
}

func TestManagedWorktreeProviderRefusesUnownedTarget(t *testing.T) {
	t.Parallel()

	source := initGitRepository(t)
	managedRoot := filepath.Join(t.TempDir(), "managed")
	provider, err := NewManagedWorktreeProvider(source, managedRoot)
	if err != nil {
		t.Fatalf("new managed worktree provider: %v", err)
	}
	target := filepath.Join(managedRoot, "attempt-1")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("create foreign target: %v", err)
	}
	foreign := filepath.Join(target, "foreign.txt")
	if err := os.WriteFile(foreign, []byte("do not delete\n"), 0o600); err != nil {
		t.Fatalf("write foreign target: %v", err)
	}
	if _, err := provider.Provision(context.Background(), ProvisionRequest{Execution: testExecution(), SourceDir: source}); err == nil {
		t.Fatal("provider accepted an unowned existing target")
	}
	if got, err := os.ReadFile(foreign); err != nil || string(got) != "do not delete\n" {
		t.Fatalf("provider altered foreign target: %q, %v", got, err)
	}
}

func initGitRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.name", "TARS Test")
	runGit(t, root, "config", "user.email", "tars@example.invalid")
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("committed\n"), 0o600); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	runGit(t, root, "add", "tracked.txt")
	runGit(t, root, "commit", "-m", "initial")
	return root
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	commandArgs := append([]string{"-C", root}, args...)
	cmd := exec.Command("git", commandArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}

func pathWithin(root, path string) bool {
	absoluteRoot, _ := filepath.EvalSymlinks(root)
	absolutePath, _ := filepath.EvalSymlinks(path)
	relative, err := filepath.Rel(absoluteRoot, absolutePath)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
