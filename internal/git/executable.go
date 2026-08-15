package git

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// ErrGitUnavailable reports that no git executable could be located. It is
// deliberately distinct from ErrNotRepository: a missing git binary and a
// directory that simply is not a repository call for very different fixes, and
// collapsing them made "not a git repository" the symptom of a missing git on
// platforms without defaultGitPath.
var ErrGitUnavailable = errors.New("git executable not found")

// Executable returns the absolute path of the git binary to run.
//
// Callers get an absolute path or an error — never a bare "git" that the OS
// would resolve against PATH at exec time, which is what the previous
// hardcoded /usr/bin/git was guarding against. defaultGitPath keeps that
// guarantee for free on platforms where git has a canonical location; where it
// does not (Windows, and unix installs that put git elsewhere), PATH is
// consulted once and the result is required to be absolute.
//
// The result is cached: git does not move while the process runs, and every
// git subcommand would otherwise re-walk PATH.
var Executable = sync.OnceValues(resolveExecutable)

func resolveExecutable() (string, error) {
	if defaultGitPath != "" {
		if info, err := os.Stat(defaultGitPath); err == nil && !info.IsDir() {
			return defaultGitPath, nil
		}
	}
	path, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrGitUnavailable, err)
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: %q is not an absolute path", ErrGitUnavailable, path)
	}
	return path, nil
}
