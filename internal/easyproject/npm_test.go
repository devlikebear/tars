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
	_, log := fakeNode(t, scaffoldStandIn, "exit 0")
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

// scaffoldStandIn writes what the scaffolder writes: a folder of the name
// it is given ($3), in the folder it runs in, holding package.json and the
// installed packages.
const scaffoldStandIn = `/bin/mkdir -p "$3/node_modules/.bin" && echo '{}' > "$3/package.json" && pwd > "$3/ran-in"`

func TestNPMScaffolderScaffoldsAKoreanFolderUnderAnASCIIName(t *testing.T) {
	_, log := fakeNode(t, scaffoldStandIn, "exit 0")
	base := t.TempDir()
	dir, err := NewFolder(base, "동네 독서모임 신청 페이지")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != "동네-독서모임-신청-페이지" {
		t.Fatalf("folder = %q", dir)
	}
	if err := (NPMScaffolder{}).Scaffold(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(log)
	name := ScaffoldName(filepath.Base(dir))
	if !validScaffoldName(name) || !strings.HasPrefix(string(raw), "npx --yes create-cloudflare@"+CreateCloudflareVersion+" "+name+" ") {
		t.Fatalf("name %q, calls:\n%s", name, raw)
	}
	// What the scaffolder made is in the project's folder, and nothing
	// of the staging is left.
	for _, want := range []string{"package.json", "node_modules", "ran-in"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Fatalf("%s: %v", want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, scaffoldStaging)); !os.IsNotExist(err) {
		t.Fatalf("staging left: %v", err)
	}
	entries, _ := os.ReadDir(base)
	if len(entries) != 1 {
		t.Fatalf("base holds %d entries", len(entries))
	}
}

func TestNPMScaffolderStagingFailures(t *testing.T) {
	korean := func(t *testing.T) string {
		dir, err := NewFolder(t.TempDir(), "독서모임")
		if err != nil {
			t.Fatal(err)
		}
		return dir
	}
	t.Run("staging folder already there", func(t *testing.T) {
		fakeNode(t, scaffoldStandIn, "exit 0")
		dir := korean(t)
		if err := os.Mkdir(filepath.Join(dir, scaffoldStaging), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := (NPMScaffolder{}).Scaffold(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "scaffold") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("scaffolder made nothing", func(t *testing.T) {
		fakeNode(t, "exit 0", "exit 0")
		if err := (NPMScaffolder{}).Scaffold(context.Background(), korean(t)); err == nil || !strings.Contains(err.Error(), "scaffold") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("the folder already holds what it made", func(t *testing.T) {
		fakeNode(t, scaffoldStandIn, "exit 0")
		dir := korean(t)
		if err := os.MkdirAll(filepath.Join(dir, "package.json", "x"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := (NPMScaffolder{}).Scaffold(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "scaffold") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestScaffoldName(t *testing.T) {
	if got := ScaffoldName("가-" + strings.Repeat("abcdefghij-", 6)); got != "abcdefghij-abcdefghij-abcdefghij-abcdefg" {
		t.Errorf("long name = %q", got)
	}
	for folder, want := range map[string]string{
		"독서모임-sign-up-page": "sign-up-page",
		"café-menu":         "caf-menu",
		"book-club":         "book-club",
	} {
		if got := ScaffoldName(folder); got != want {
			t.Errorf("ScaffoldName(%q) = %q, want %q", folder, got, want)
		}
	}
	a, b := ScaffoldName("동네-독서모임"), ScaffoldName("카페-소개")
	if a == b || !validScaffoldName(a) || !validScaffoldName(b) || !strings.HasPrefix(a, "app-") {
		t.Fatalf("names %q, %q", a, b)
	}
	if ScaffoldName("동네-독서모임") != a {
		t.Fatal("not stable")
	}
	for name, want := range map[string]bool{"book-club": true, "a": true, "-a": false, "a-": false, "A": false, "독서": false, "": false, strings.Repeat("a", 59): false} {
		if validScaffoldName(name) != want {
			t.Errorf("validScaffoldName(%q) != %v", name, want)
		}
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
		fakeNode(t, scaffoldStandIn, "exit 3")
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
