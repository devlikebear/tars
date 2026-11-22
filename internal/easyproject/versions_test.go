package easyproject

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		return "<missing>"
	}
	return string(raw)
}

// versionedProject is a project with three versions: the start, a sign-up
// page, then a green button with a new file.
func versionedProject(t *testing.T) (dir string, start, signUp, green Version) {
	t.Helper()
	ctx := context.Background()
	dir = t.TempDir()
	writeFiles(t, dir, map[string]string{".gitignore": "node_modules\n", "index.html": "empty\n"})
	if err := InitGit(ctx, dir); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dir, map[string]string{"index.html": "sign-up\n"})
	if _, err := SaveVersion(ctx, dir, "A sign-up page"); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dir, map[string]string{"index.html": "sign-up, green\n", "style.css": "green\n"})
	if _, err := SaveVersion(ctx, dir, "Make the button green"); err != nil {
		t.Fatal(err)
	}
	versions, err := Versions(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 {
		t.Fatalf("versions = %+v", versions)
	}
	return dir, versions[2], versions[1], versions[0]
}

func TestVersionsListNewestFirst(t *testing.T) {
	_, start, signUp, green := versionedProject(t)
	if start.Kind != VersionStart || start.Title != startMessage || start.Current {
		t.Fatalf("start = %+v", start)
	}
	if signUp.Kind != VersionSaved || signUp.Title != "A sign-up page" || signUp.Current || signUp.At.IsZero() {
		t.Fatalf("sign-up = %+v", signUp)
	}
	if green.Kind != VersionSaved || !green.Current || len(green.Commit) != 40 {
		t.Fatalf("green = %+v", green)
	}
}

func TestRestoreVersionSavesTheOldFilesAsANewVersion(t *testing.T) {
	ctx := context.Background()
	dir, _, signUp, green := versionedProject(t)
	// Installed packages and work not yet saved.
	writeFiles(t, dir, map[string]string{"node_modules/x/index.js": "pkg\n", "draft.txt": "unsaved\n"})

	back, err := RestoreVersion(ctx, dir, signUp.Commit)
	if err != nil {
		t.Fatal(err)
	}
	if back.Kind != VersionRestored || back.Title != "A sign-up page" || !back.Current {
		t.Fatalf("restored = %+v", back)
	}
	if got := readFile(t, filepath.Join(dir, "index.html")); got != "sign-up\n" {
		t.Fatalf("index.html = %q", got)
	}
	for _, gone := range []string{"style.css", "draft.txt"} {
		if got := readFile(t, filepath.Join(dir, gone)); got != "<missing>" {
			t.Fatalf("%s is still there: %q", gone, got)
		}
	}
	if got := readFile(t, filepath.Join(dir, "node_modules/x/index.js")); got != "pkg\n" {
		t.Fatalf("an ignored file was touched: %q", got)
	}
	if status, _ := git(ctx, dir, "status", "--porcelain"); status != "" {
		t.Fatalf("left unsaved:\n%s", status)
	}

	versions, err := Versions(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	titles := make([]string, 0, len(versions))
	for _, v := range versions {
		titles = append(titles, string(v.Kind)+":"+v.Title)
	}
	want := "restored:A sign-up page|saved:" + unsavedMessage + "|saved:Make the button green|saved:A sign-up page|start:" + startMessage
	if strings.Join(titles, "|") != want {
		t.Fatalf("history = %s", strings.Join(titles, "|"))
	}

	// The unsaved draft is in the version left behind, and going back to
	// it brings the green button back.
	again, err := RestoreVersion(ctx, dir, green.Commit)
	if err != nil {
		t.Fatal(err)
	}
	if again.Title != "Make the button green" || readFile(t, filepath.Join(dir, "style.css")) != "green\n" {
		t.Fatalf("back to green = %+v", again)
	}
	// Going back to a restore names the version it restored.
	twice, err := RestoreVersion(ctx, dir, back.Commit)
	if err != nil || twice.Title != "A sign-up page" || twice.Kind != VersionRestored {
		t.Fatalf("back to a restore = %+v, %v", twice, err)
	}
}

func TestRestoreVersionOfTheCurrentOneSavesNothing(t *testing.T) {
	ctx := context.Background()
	dir, _, _, green := versionedProject(t)
	got, err := RestoreVersion(ctx, dir, green.Commit)
	if err != nil || got.Commit != green.Commit {
		t.Fatalf("got %+v, %v", got, err)
	}
	if versions, _ := Versions(ctx, dir); len(versions) != 3 {
		t.Fatalf("a version was added: %+v", versions)
	}
}

func TestRestoreVersionRefusesWhatIsNotAVersion(t *testing.T) {
	ctx := context.Background()
	dir, _, _, _ := versionedProject(t)
	other := t.TempDir()
	writeFiles(t, other, map[string]string{"index.html": "another project\n"})
	if err := InitGit(ctx, other); err != nil {
		t.Fatal(err)
	}
	foreign, err := git(ctx, other, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	for _, commit := range []string{"", "HEAD", "--help", strings.Repeat("0", 40), foreign} {
		if _, err := RestoreVersion(ctx, dir, commit); !errors.Is(err, ErrVersionNotFound) {
			t.Fatalf("%q: %v", commit, err)
		}
	}
}
