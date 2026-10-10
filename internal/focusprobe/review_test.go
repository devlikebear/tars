package focusprobe

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/focuspipeline"
)

// gitRun runs git in dir and returns its output, failing the test on error.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestFocusDiffExcerpt(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/b.go b/b.go",
		"--- a/b.go",
		"+++ b/b.go",
		"@@ -1,4 +1,4 @@",
		" package b",
		"-var x = 1",
		"+var x = 2",
		" ",
		" func f() {}",
		"@@ -20,3 +20,4 @@ func g() {",
		" a",
		"+b",
		" c",
		" d",
	}, "\n")
	tests := []struct {
		name string
		line int
		want []string
		not  []string
	}{
		{name: "line in first hunk", line: 2, want: []string{"@@ -1,4 +1,4 @@", "-var x = 1", "+var x = 2"}, not: []string{"+b"}},
		{name: "line in second hunk", line: 21, want: []string{"@@ -20,3 +20,4 @@", "+b"}, not: []string{"var x"}},
		{name: "line outside the diff", line: 200},
		{name: "no line takes the first hunk", line: 0, want: []string{"@@ -1,4 +1,4 @@", "+var x = 2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := focusDiffExcerpt(diff, tt.line, 3)
			if len(tt.want) == 0 && got != "" {
				t.Fatalf("excerpt = %q, want none", got)
			}
			for _, s := range tt.want {
				if !strings.Contains(got, s) {
					t.Errorf("excerpt lacks %q:\n%s", s, got)
				}
			}
			for _, s := range tt.not {
				if strings.Contains(got, s) {
					t.Errorf("excerpt has %q:\n%s", s, got)
				}
			}
		})
	}
}

// focusReviewRepo is a git repository with a committed b.go; it returns
// the repo and its HEAD.
func focusReviewRepo(t *testing.T) (string, string) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	repo := t.TempDir()
	gitRun(t, repo, "init", "-q", "-b", "main")
	gitRun(t, repo, "config", "commit.gpgSign", "false")
	gitRun(t, repo, "config", "core.autocrlf", "false")
	lines := make([]string, 0, 30)
	for i := 1; i <= 30; i++ {
		lines = append(lines, "line "+strconv.Itoa(i))
	}
	writeRepoFile(t, repo, "b.go", strings.Join(lines, "\n")+"\n")
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-q", "-m", "init")
	return repo, strings.TrimSpace(gitRun(t, repo, "rev-parse", "HEAD"))
}

func writeRepoFile(t *testing.T, repo, name, content string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFocusFindingExcerpts(t *testing.T) {
	repo, base := focusReviewRepo(t)
	// One committed change and one uncommitted, both after base.
	content, _ := os.ReadFile(filepath.Join(repo, "b.go"))
	changed := strings.Replace(string(content), "line 5\n", "line five\n", 1)
	writeRepoFile(t, repo, "b.go", changed)
	gitRun(t, repo, "commit", "-qam", "five")
	writeRepoFile(t, repo, "b.go", strings.Replace(changed, "line 25\n", "line twenty-five\n", 1))
	writeRepoFile(t, repo, "new.go", "package x\nfunc New() {}\n")

	findings := []focuspipeline.Finding{
		{ID: "f1", File: "b.go", Line: 5},
		{ID: "f2", File: "b.go", Line: 25},
		{ID: "f3", File: "new.go", Line: 2},
		{ID: "f4", File: "../outside.go", Line: 1},
		{ID: "f5", File: "b.go", Line: 15},
		{ID: "f6", File: "-p", Line: 1},
	}
	got := FindingExcerpts(context.Background(), repo, base, findings)
	checks := map[string][]string{
		"f1": {"-line 5", "+line five"},
		"f2": {"+line twenty-five"},
		"f3": {"+func New() {}"},
	}
	for i, f := range got {
		want, ok := checks[f.ID]
		if !ok {
			if f.Excerpt != "" {
				t.Errorf("%s: excerpt = %q, want none", f.ID, f.Excerpt)
			}
			continue
		}
		for _, s := range want {
			if !strings.Contains(f.Excerpt, s) {
				t.Errorf("%s lacks %q:\n%s", f.ID, s, f.Excerpt)
			}
		}
		if findings[i].Excerpt != "" {
			t.Fatal("the input findings were modified")
		}
	}
	if none := FindingExcerpts(context.Background(), "", base, findings); none[0].Excerpt != "" {
		t.Fatal("no folder must give no excerpt")
	}
}

// f3: findings name files from the repository top level even when the
// session folder is a subfolder.
func TestFocusFindingExcerptsFromASubfolder(t *testing.T) {
	repo, _ := focusReviewRepo(t)
	writeRepoFile(t, repo, "sub/x.go", "a\nb\nc\n")
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-qm", "sub")
	base := strings.TrimSpace(gitRun(t, repo, "rev-parse", "HEAD"))
	writeRepoFile(t, repo, "sub/x.go", "a\nbee\nc\n")
	writeRepoFile(t, repo, "sub/new.go", "package sub\nfunc New() {}\n")
	got := FindingExcerpts(context.Background(), filepath.Join(repo, "sub"), base, []focuspipeline.Finding{
		{ID: "f1", File: "sub/x.go", Line: 2},
		{ID: "f2", File: "sub/new.go", Line: 2},
	})
	if !strings.Contains(got[0].Excerpt, "+bee") {
		t.Errorf("tracked: %q", got[0].Excerpt)
	}
	if !strings.Contains(got[1].Excerpt, "+func New() {}") {
		t.Errorf("untracked: %q", got[1].Excerpt)
	}
}

// I2: an untracked path through a symlink never reads outside the
// repository, and an untracked file is read only up to the cap.
func TestFocusFindingExcerptsStayInTheRepository(t *testing.T) {
	repo, base := focusReviewRepo(t)
	outside := t.TempDir()
	writeRepoFile(t, outside, "secret.txt", "outside secret\n")
	if err := os.Symlink(outside, filepath.Join(repo, "linkdir")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(repo, "leak.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	var big strings.Builder
	for i := 1; big.Len() <= 2*focusUntrackedMaxBytes; i++ {
		big.WriteString("row " + strconv.Itoa(i) + " " + strings.Repeat("x", 60) + "\n")
	}
	writeRepoFile(t, repo, "big.txt", big.String())

	got := FindingExcerpts(context.Background(), repo, base, []focuspipeline.Finding{
		{ID: "dir", File: "linkdir/secret.txt", Line: 1},
		{ID: "file", File: "leak.txt", Line: 1},
		{ID: "big", File: "big.txt", Line: 1500},
	})
	for _, f := range got[:2] {
		if strings.Contains(f.Excerpt, "outside secret") {
			t.Errorf("%s read outside the repository: %q", f.ID, f.Excerpt)
		}
	}
	if strings.Contains(got[2].Excerpt, "row 1500 ") {
		t.Errorf("read past the cap: %q", got[2].Excerpt)
	}
}

// A base that is not a commit id must not reach git: `git diff --output=F`
// writes the diff to F, so a tampered pipeline file could otherwise make the
// server write wherever it liked.
func TestFocusFindingExcerptsIgnoresOptionShapedBase(t *testing.T) {
	repo, _ := focusReviewRepo(t)
	content, _ := os.ReadFile(filepath.Join(repo, "b.go"))
	writeRepoFile(t, repo, "b.go", strings.Replace(string(content), "line 5\n", "line five\n", 1))

	leak := filepath.Join(t.TempDir(), "leak")
	got := FindingExcerpts(context.Background(), repo, "--output="+leak, []focuspipeline.Finding{
		{ID: "f1", File: "b.go", Line: 5},
	})
	if _, err := os.Stat(leak); err == nil {
		t.Fatal("the base was passed to git as an option and wrote a file")
	}
	// It falls back to HEAD, like a pipeline with no recorded base.
	if !strings.Contains(got[0].Excerpt, "line five") {
		t.Fatalf("excerpt = %q, want the change against HEAD", got[0].Excerpt)
	}
}
