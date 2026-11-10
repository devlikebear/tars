package execsandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// liveBackend returns this host's sandbox, or skips: Windows has none, and a
// Linux host may lack bwrap or forbid the user namespaces it needs.
func liveBackend(t *testing.T) Backend {
	t.Helper()
	backend, err := Detect()
	if err != nil {
		t.Skipf("no sandbox on this host: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := backend.Probe(ctx); err != nil {
		t.Skipf("%s cannot start a sandbox here: %v", backend.Name, err)
	}
	return backend
}

// outsideDir is a folder the sandbox must not write to. It cannot live under
// the temp dir, which the sandbox allows, so it goes in the package dir.
func outsideDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", ".sandbox-outside-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func runSandboxed(t *testing.T, backend Backend, policy Policy, dir, script string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	name, args := backend.Command(policy, dir, []string{"/bin/sh", "-c", script})
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestLiveSandboxConfinesWritesAndHidesCredentials(t *testing.T) {
	backend := liveBackend(t)

	base := t.TempDir()
	work := filepath.Join(base, "work")
	home := filepath.Join(base, "home")
	for _, dir := range []string{work, filepath.Join(home, ".ssh")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	key := filepath.Join(home, ".ssh", "id_test")
	if err := os.WriteFile(key, []byte("not a real key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := outsideDir(t)

	t.Setenv("HOME", home)
	t.Setenv("SSH_AUTH_SOCK", "")
	policy := Resolve(Options{Dirs: []string{work}})

	if out, err := runSandboxed(t, backend, policy, work, "echo ok > inside.txt"); err != nil {
		t.Fatalf("write inside the session folder failed: %v\n%s", err, out)
	}
	if data, err := os.ReadFile(filepath.Join(work, "inside.txt")); err != nil || strings.TrimSpace(string(data)) != "ok" {
		t.Fatalf("inside.txt = %q, %v", data, err)
	}

	out, err := runSandboxed(t, backend, policy, work, "echo leaked > '"+filepath.Join(outside, "f.txt")+"'")
	if err == nil {
		t.Fatalf("write outside the session folder succeeded:\n%s", out)
	}
	t.Logf("outside write refused: %s", strings.TrimSpace(out))
	if _, statErr := os.Stat(filepath.Join(outside, "f.txt")); statErr == nil {
		t.Fatal("the file outside the session folder was created")
	}

	out, err = runSandboxed(t, backend, policy, work, "cat '"+key+"'")
	if err == nil || strings.Contains(out, "not a real key") {
		t.Fatalf("reading ~/.ssh succeeded: %v\n%s", err, out)
	}
	t.Logf("credential read refused: %s", strings.TrimSpace(out))
}

func TestLiveSandboxLetsAWorktreeCommitButNotPlantHooks(t *testing.T) {
	backend := liveBackend(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	wt := filepath.Join(base, "wt")
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git(repo, "init", "-q")
	git(repo, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "--allow-empty", "-m", "root")
	git(repo, "worktree", "add", "-q", wt)

	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	policy := Resolve(Options{Dirs: []string{wt}})

	commit := "echo change > f.txt && git add f.txt && git -c user.name=t -c user.email=t@example.invalid commit -q -m change"
	if out, err := runSandboxed(t, backend, policy, wt, commit); err != nil {
		t.Fatalf("commit in the worktree failed: %v\n%s", err, out)
	}

	hook := filepath.Join(repo, ".git", "hooks", "post-checkout")
	out, err := runSandboxed(t, backend, policy, wt, "echo 'echo pwned' > '"+hook+"'")
	if err == nil {
		t.Fatalf("planting a git hook succeeded:\n%s", out)
	}
	t.Logf("hook write refused: %s", strings.TrimSpace(out))
	if _, statErr := os.Stat(hook); statErr == nil {
		t.Fatal("the hook was written")
	}
}
