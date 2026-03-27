package opsservicedemo

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBootstrapDemoRepoCreatesStandaloneGoModule(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("bootstrap script uses POSIX shell")
	}

	root := repoRoot(t)
	script := filepath.Join(root, "examples", "ops-service-demo", "bootstrap-demo-repo.sh")
	dest := filepath.Join(t.TempDir(), "repo")

	cmd := exec.Command("sh", script, dest)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bootstrap demo repo: %v\n%s", err, output)
	}

	data, err := os.ReadFile(filepath.Join(dest, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if got := string(data); !strings.Contains(got, "module example.com/ops-service-demo") {
		t.Fatalf("expected default module path, got:\n%s", got)
	}

	testCmd := exec.Command("go", "test", "./...")
	testCmd.Dir = dest
	output, err = testCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test in bootstrapped repo: %v\n%s", err, output)
	}
}

func TestBootstrapDemoRepoUsesGitHubRepoAsModulePath(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("bootstrap script uses POSIX shell")
	}

	root := repoRoot(t)
	script := filepath.Join(root, "examples", "ops-service-demo", "bootstrap-demo-repo.sh")
	dest := filepath.Join(t.TempDir(), "repo")

	cmd := exec.Command("sh", script, dest, "--github", "acme/ops-demo")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+fakeCommandPath(t, map[string]string{
		"gh": "#!/bin/sh\nexit 0\n",
	})+":"+os.Getenv("PATH"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bootstrap demo repo with github repo: %v\n%s", err, output)
	}

	data, err := os.ReadFile(filepath.Join(dest, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if got := string(data); !strings.Contains(got, "module github.com/acme/ops-demo") {
		t.Fatalf("expected github-derived module path, got:\n%s", got)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func fakeCommandPath(t *testing.T, scripts map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range scripts {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("write fake command %s: %v", name, err)
		}
	}
	return dir
}
