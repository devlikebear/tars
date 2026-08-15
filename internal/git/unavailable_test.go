package git

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// stubExecutable makes git resolution fail for the duration of the test.
// Executable is a func var precisely so this seam exists; these tests must not
// run in parallel with anything that shells out to git.
func stubExecutable(t *testing.T, err error) {
	t.Helper()
	previous := Executable
	Executable = func() (string, error) { return "", err }
	t.Cleanup(func() { Executable = previous })
}

// TestRepositoryRootReportsMissingGitDistinctly is the regression this
// distinction exists for: with git unresolvable, every call used to come back
// as "not a git repository", which sent anyone debugging it at the directory
// instead of at their missing git.
func TestRepositoryRootReportsMissingGitDistinctly(t *testing.T) {
	stubExecutable(t, fmt.Errorf("%w: no git here", ErrGitUnavailable))

	_, err := NewClient().RepositoryRoot(context.Background(), t.TempDir())
	if !errors.Is(err, ErrGitUnavailable) {
		t.Fatalf("expected ErrGitUnavailable, got %v", err)
	}
	if errors.Is(err, ErrNotRepository) {
		t.Fatal("a missing git binary must not be reported as a missing repository")
	}
}

func TestStatusPropagatesMissingGit(t *testing.T) {
	stubExecutable(t, fmt.Errorf("%w: no git here", ErrGitUnavailable))

	if _, err := NewClient().Status(context.Background(), t.TempDir()); !errors.Is(err, ErrGitUnavailable) {
		t.Fatalf("expected ErrGitUnavailable, got %v", err)
	}
}
