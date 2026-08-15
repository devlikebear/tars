package agentruntime

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gitrepo "github.com/devlikebear/tars/internal/git"
)

// stubGitUnavailable makes git resolution fail for the duration of the test.
// gitrepo.Executable is a func var so this seam exists; these tests must not
// run in parallel with anything that shells out to git.
func stubGitUnavailable(t *testing.T) {
	t.Helper()
	previous := gitrepo.Executable
	gitrepo.Executable = func() (string, error) {
		return "", fmt.Errorf("%w: nothing installed", gitrepo.ErrGitUnavailable)
	}
	t.Cleanup(func() { gitrepo.Executable = previous })
}

// The diff timeline is a best-effort enrichment, so both helpers degrade to
// "no diff" rather than failing the run when git cannot be found.
func TestGitOutputDegradesWhenGitIsUnavailable(t *testing.T) {
	stubGitUnavailable(t)

	if out, ok := gitOutput(t.TempDir(), "status", "--porcelain"); ok {
		t.Fatalf("expected no output without git, got %q", out)
	}
}

func TestGitNoIndexPatchDegradesWhenGitIsUnavailable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	stubGitUnavailable(t)

	if out, ok := gitNoIndexPatch(dir, "untracked.txt"); ok {
		t.Fatalf("expected no patch without git, got %q", out)
	}
}
