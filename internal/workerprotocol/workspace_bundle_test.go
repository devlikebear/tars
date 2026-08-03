package workerprotocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDirectoryWorkspaceBundleIsDeterministicAndExcludesSecrets(t *testing.T) {
	t.Parallel()

	source := t.TempDir()
	writeWorkspaceTestFile(t, source, "README.md", []byte("hello\n"), 0o644)
	writeWorkspaceTestFile(t, source, "bin/run.sh", []byte("#!/bin/sh\necho ready\n"), 0o755)
	writeWorkspaceTestFile(t, source, ".env", []byte("API_TOKEN=must-not-leave-gateway\n"), 0o600)
	writeWorkspaceTestFile(t, source, ".git/config", []byte("private remote\n"), 0o600)
	writeWorkspaceTestFile(t, source, "node_modules/dependency.js", []byte("ignored\n"), 0o644)

	first, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: source, Mode: SyncModeDirectory})
	if err != nil {
		t.Fatalf("build directory bundle: %v", err)
	}
	second, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: source, Mode: SyncModeDirectory})
	if err != nil {
		t.Fatalf("rebuild directory bundle: %v", err)
	}
	if first.Manifest.Digest == "" || first.Manifest.Digest != second.Manifest.Digest || len(first.Files) != 2 {
		t.Fatalf("non-deterministic bundle first=%+v second=%+v", first.Manifest, second.Manifest)
	}
	if !containsString(first.Manifest.ExcludedPaths, ".env") {
		t.Fatalf("secret exclusion not audited: %+v", first.Manifest.ExcludedPaths)
	}
	if bytes.Contains(mustMarshalJSON(t, first), []byte("must-not-leave-gateway")) {
		t.Fatal("workspace bundle contained excluded secret")
	}
	if err := VerifyWorkspaceBundle(first, DefaultWorkspaceBundleLimits()); err != nil {
		t.Fatalf("verify workspace bundle: %v", err)
	}

	destination := t.TempDir()
	if err := ApplyWorkspaceBundle(context.Background(), destination, first, DefaultWorkspaceBundleLimits()); err != nil {
		t.Fatalf("apply workspace bundle: %v", err)
	}
	readme, err := os.ReadFile(filepath.Join(destination, "README.md"))
	if err != nil || string(readme) != "hello\n" {
		t.Fatalf("applied README=%q error=%v", readme, err)
	}
	if _, err := os.Stat(filepath.Join(destination, ".env")); !os.IsNotExist(err) {
		t.Fatalf("excluded .env exists at worker destination: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(destination, "bin/run.sh"))
		if err != nil || info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("executable mode was not preserved: info=%v error=%v", info, err)
		}
	}
}

func TestWorkspaceBundleRejectsSymlinkTraversalAndDigestTampering(t *testing.T) {
	t.Parallel()

	source := t.TempDir()
	writeWorkspaceTestFile(t, source, "safe.txt", []byte("safe\n"), 0o644)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(source, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: source, Mode: SyncModeDirectory}); !errors.Is(err, ErrUnsafeWorkspace) {
		t.Fatalf("symlink bundle error=%v want ErrUnsafeWorkspace", err)
	}
	if err := os.Remove(filepath.Join(source, "escape")); err != nil {
		t.Fatalf("remove test symlink: %v", err)
	}
	bundle, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: source, Mode: SyncModeDirectory})
	if err != nil {
		t.Fatalf("build safe bundle: %v", err)
	}
	bundle.Files[0].Data[0] ^= 0xff
	if err := VerifyWorkspaceBundle(bundle, DefaultWorkspaceBundleLimits()); !errors.Is(err, ErrManifestMismatch) {
		t.Fatalf("tampered bundle error=%v want ErrManifestMismatch", err)
	}
	bundle.Files[0].Path = "../escape"
	if err := VerifyWorkspaceBundle(bundle, DefaultWorkspaceBundleLimits()); !errors.Is(err, ErrUnsafeWorkspace) {
		t.Fatalf("traversal bundle error=%v want ErrUnsafeWorkspace", err)
	}
}

func TestGitWorkspaceBundleUsesTrackedFilesAndRevision(t *testing.T) {
	t.Parallel()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	repo := t.TempDir()
	runGitTestCommand(t, gitPath, repo, "init")
	runGitTestCommand(t, gitPath, repo, "config", "user.email", "test@example.com")
	runGitTestCommand(t, gitPath, repo, "config", "user.name", "TARS Test")
	writeWorkspaceTestFile(t, repo, "tracked.txt", []byte("tracked\n"), 0o644)
	runGitTestCommand(t, gitPath, repo, "add", "tracked.txt")
	runGitTestCommand(t, gitPath, repo, "commit", "-m", "test fixture")
	writeWorkspaceTestFile(t, repo, "untracked.txt", []byte("untracked\n"), 0o644)

	bundle, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: repo, Mode: SyncModeGit, GitPath: gitPath})
	if err != nil {
		t.Fatalf("build git bundle: %v", err)
	}
	if bundle.Manifest.Revision == "" || len(bundle.Files) != 1 || bundle.Files[0].Path != "tracked.txt" {
		t.Fatalf("git manifest did not bind tracked revision: %+v files=%+v", bundle.Manifest, bundle.Files)
	}
}

func writeWorkspaceTestFile(t *testing.T, root, relative string, raw []byte, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, raw, mode); err != nil {
		t.Fatalf("write fixture %s: %v", relative, err)
	}
}

func runGitTestCommand(t *testing.T, gitPath, dir string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", dir}, args...)
	command := exec.Command(gitPath, commandArgs...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func mustMarshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return raw
}
