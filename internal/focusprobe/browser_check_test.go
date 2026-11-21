package focusprobe

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// noAgentBrowser hides every copy of the CLI this host may have: no
// override, an empty PATH, no known install folders.
func noAgentBrowser(t *testing.T) {
	t.Helper()
	t.Setenv(AgentBrowserPathEnv, "")
	t.Setenv("PATH", t.TempDir())
	old := agentBrowserKnownDirs
	agentBrowserKnownDirs = func() []string { return nil }
	t.Cleanup(func() { agentBrowserKnownDirs = old })
}

func fakeCLI(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "agent-browser")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFindAgentBrowserMissing(t *testing.T) {
	noAgentBrowser(t)
	if inv, path := FindAgentBrowser(t.TempDir()); inv != "" || path != "" {
		t.Fatalf("found %q %q on a desk without the CLI", inv, path)
	}
	if got := BrowserCheckCLI(""); got != "" {
		t.Fatalf("BrowserCheckCLI = %q", got)
	}
}

func TestFindAgentBrowserOverride(t *testing.T) {
	noAgentBrowser(t)
	cli := fakeCLI(t, t.TempDir())
	t.Setenv(AgentBrowserPathEnv, cli)
	if inv, path := FindAgentBrowser(""); inv != cli || path != cli {
		t.Fatalf("override: %q %q", inv, path)
	}

	// A wrong override is not replaced by a guess, even with a copy in
	// the working folder.
	repo := t.TempDir()
	fakeCLI(t, filepath.Join(repo, "node_modules", ".bin"))
	for _, bad := range []string{filepath.Join(t.TempDir(), "gone"), "relative/agent-browser", filepath.Dir(cli)} {
		t.Setenv(AgentBrowserPathEnv, bad)
		if inv, _ := FindAgentBrowser(repo); inv != "" {
			t.Errorf("override %q gave %q", bad, inv)
		}
	}

	// A path that cannot sit between single quotes is refused: the plan
	// guidance writes it that way.
	quoted := fakeCLI(t, filepath.Join(t.TempDir(), "it's"))
	t.Setenv(AgentBrowserPathEnv, quoted)
	if inv, _ := FindAgentBrowser(""); inv != "" {
		t.Errorf("quoted path accepted: %q", inv)
	}
}

func TestFindAgentBrowserOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a #!/bin/sh stand-in is not an executable on Windows")
	}
	noAgentBrowser(t)
	bin := t.TempDir()
	cli := fakeCLI(t, bin)
	t.Setenv("PATH", bin)
	inv, path := FindAgentBrowser(t.TempDir())
	if inv != "agent-browser" || path != cli {
		t.Fatalf("on PATH: %q %q", inv, path)
	}
}

func TestFindAgentBrowserRepoLocalThenKnownDirs(t *testing.T) {
	noAgentBrowser(t)
	known := t.TempDir()
	knownCLI := fakeCLI(t, known)
	agentBrowserKnownDirs = func() []string { return []string{t.TempDir(), known} }

	repo := t.TempDir()
	if inv, path := FindAgentBrowser(repo); inv != knownCLI || path != knownCLI {
		t.Fatalf("known dir: %q %q", inv, path)
	}
	local := fakeCLI(t, filepath.Join(repo, "node_modules", ".bin"))
	inv, path := FindAgentBrowser(" " + repo + " ")
	if inv != "./node_modules/.bin/agent-browser" || path != local {
		t.Fatalf("repo-local: %q %q", inv, path)
	}
	// A folder named like the CLI is not the CLI.
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(other, "node_modules", ".bin", "agent-browser"), 0o755); err != nil {
		t.Fatal(err)
	}
	if inv, _ := FindAgentBrowser(other); inv != knownCLI {
		t.Fatalf("a folder was taken for the CLI: %q", inv)
	}
}

func TestAgentBrowserKnownDirs(t *testing.T) {
	dirs := agentBrowserKnownDirs()
	if len(dirs) < 2 || dirs[len(dirs)-1] != "/usr/local/bin" {
		t.Fatalf("known dirs = %v", dirs)
	}
}

// A check's own account of its failure survives the excerpt of a long
// output: the block is found wherever it sits, with the lines after it.
func TestFailureExcerptKeepsCheckReport(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 400; i++ {
		b.WriteString("vite v5 building for production... chunk rendered\n")
	}
	b.WriteString("CHECK FAILED\nscreen: Cart page (http://127.0.0.1:4173/)\nexpected: the total reads 30\nseen: the total reads 25\n")
	for i := 0; i < 400; i++ {
		b.WriteString("stopping the preview server, closing the browser\n")
	}
	got := FailureExcerpt(b.String(), 4096)
	for _, want := range []string{"CHECK FAILED", "screen: Cart page", "expected: the total reads 30", "seen: the total reads 25"} {
		if !strings.Contains(got, want) {
			t.Fatalf("excerpt lost %q:\n%s", want, got)
		}
	}
}
