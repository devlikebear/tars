package git

import (
	"errors"
	"fmt"
	"sync"

	"github.com/devlikebear/tars/internal/exepath"
)

// ErrGitUnavailable reports that no git executable could be located. It is
// deliberately distinct from ErrNotRepository: a missing git binary and a
// directory that simply is not a repository call for very different fixes, and
// collapsing them made "not a git repository" the symptom of a missing git on
// platforms without defaultGitPath.
var ErrGitUnavailable = errors.New("git executable not found")

// Executable returns the absolute path of the git binary to run. The result is
// cached: git does not move while the process runs, and every git subcommand
// would otherwise re-walk PATH.
var Executable = sync.OnceValues(resolveExecutable)

func resolveExecutable() (string, error) {
	path, err := exepath.Resolve(exepath.Candidates{
		DefaultPath: defaultGitPath,
		LookupNames: []string{"git"},
	})
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrGitUnavailable, err)
	}
	return path, nil
}
