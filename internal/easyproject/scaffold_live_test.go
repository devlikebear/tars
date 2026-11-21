//go:build integration

package easyproject

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLiveNPMScaffold runs the real scaffolder (network, npm) into a fresh
// folder and checks the stack is there. Skipped without npm.
//
//	go test -tags integration ./internal/easyproject -run TestLiveNPMScaffold -v
func TestLiveNPMScaffold(t *testing.T) {
	s := NPMScaffolder{}
	if err := s.Ready(); err != nil {
		t.Skip("npm is not installed")
	}
	base := t.TempDir()
	dir, err := NewFolder(base, "A sign-up page for a book club")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := s.Scaffold(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	t.Logf("scaffold and install took %s", time.Since(start).Round(time.Second))
	for _, f := range []string{"package.json", "vite.config.ts", "wrangler.jsonc", "worker/index.ts",
		"node_modules/hono/package.json", "node_modules/@tailwindcss/vite/package.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	if err := InitGit(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", dir, "ls-files").Output()
	if err != nil {
		t.Fatal(err)
	}
	files := strings.Split(strings.TrimSpace(string(out)), "\n")
	t.Logf("first version holds %d files", len(files))
	for _, f := range files {
		if strings.HasPrefix(f, "node_modules/") || strings.HasPrefix(f, ".wrangler/") {
			t.Fatalf("the first version holds %s", f)
		}
	}
}
