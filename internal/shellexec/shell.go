// Package shellexec resolves the POSIX shell used to run command strings that
// users, skills, and agents supply.
//
// Those strings are written as shell script — pipes, &&, $VAR, quoting — so
// they need a POSIX shell, not merely "some shell". cmd.exe would mis-execute
// them rather than fail cleanly, which is why Windows resolution looks for sh
// or bash instead of falling back to the native interpreter.
package shellexec

import (
	"errors"
	"fmt"
	"sync"

	"github.com/devlikebear/tars/internal/exepath"
)

// ErrShellUnavailable reports that no POSIX shell could be located. Callers
// should surface it rather than substituting another interpreter: silently
// running a POSIX command string through something else is worse than not
// running it.
var ErrShellUnavailable = errors.New("posix shell not found")

// Executable returns the absolute path of the POSIX shell to run, cached for
// the process lifetime.
var Executable = sync.OnceValues(resolveExecutable)

func resolveExecutable() (string, error) {
	path, err := exepath.Resolve(exepath.Candidates{
		DefaultPath: defaultShellPath,
		LookupNames: shellLookupNames,
		Fallbacks:   fallbackShellPaths,
	})
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrShellUnavailable, err)
	}
	return path, nil
}
