package easyproject

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeNode makes a folder with stand-in npm and npx shell scripts that log
// their arguments, and puts only that folder on PATH.
func fakeNode(t *testing.T, npx, npm string) (bin, log string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-ins are #!/bin/sh scripts")
	}
	bin = t.TempDir()
	log = filepath.Join(t.TempDir(), "calls.log")
	write := func(name, body string) {
		script := "#!/bin/sh\necho \"" + name + " $*\" >> '" + log + "'\n" + body + "\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("npx", npx)
	write("npm", npm)
	t.Setenv("PATH", bin)
	old := knownNodeDirs
	knownNodeDirs = nil
	t.Cleanup(func() { knownNodeDirs = old })
	return bin, log
}

func TestNPMScaffolderRunsThePinnedScaffolderThenInstalls(t *testing.T) {
	_, log := fakeNode(t, `echo '{}' > "$3/package.json"`, "exit 0")
	if err := (NPMScaffolder{}).Ready(); err != nil {
		t.Fatal(err)
	}
	dir, err := NewFolder(t.TempDir(), "Book club")
	if err != nil {
		t.Fatal(err)
	}
	if err := (NPMScaffolder{}).Scaffold(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(log)
	calls := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(calls) != 2 ||
		!strings.HasPrefix(calls[0], "npx --yes create-cloudflare@"+CreateCloudflareVersion+" book-club --framework=react ") ||
		!strings.Contains(calls[0], "--no-deploy") || !strings.Contains(calls[0], "--no-git") ||
		calls[1] != "npm install --no-audit --no-fund hono@^4 tailwindcss@^4 @tailwindcss/vite@^4" {
		t.Fatalf("calls:\n%s", raw)
	}
}

func TestNPMScaffolderFailures(t *testing.T) {
	t.Run("scaffolder fails", func(t *testing.T) {
		fakeNode(t, "echo 'registry unreachable' >&2; exit 1", "exit 0")
		err := (NPMScaffolder{}).Scaffold(context.Background(), t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "registry unreachable") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("nothing written", func(t *testing.T) {
		fakeNode(t, "exit 0", "exit 0")
		if err := (NPMScaffolder{}).Scaffold(context.Background(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "no package.json") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("install fails", func(t *testing.T) {
		fakeNode(t, `echo '{}' > "$3/package.json"`, "exit 3")
		dir, _ := NewFolder(t.TempDir(), "x")
		if err := (NPMScaffolder{}).Scaffold(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "install") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("no npm", func(t *testing.T) {
		fakeNode(t, "exit 0", "exit 0")
		t.Setenv("PATH", t.TempDir())
		if err := (NPMScaffolder{}).Ready(); !errors.Is(err, ErrNodeMissing) {
			t.Fatalf("ready = %v", err)
		}
		if err := (NPMScaffolder{}).Scaffold(context.Background(), t.TempDir()); !errors.Is(err, ErrNodeMissing) {
			t.Fatalf("scaffold = %v", err)
		}
	})
	t.Run("npm in a known folder", func(t *testing.T) {
		bin, _ := fakeNode(t, "exit 0", "exit 0")
		t.Setenv("PATH", t.TempDir())
		knownNodeDirs = []string{bin}
		if err := (NPMScaffolder{}).Ready(); err != nil {
			t.Fatalf("ready = %v", err)
		}
	})
}
