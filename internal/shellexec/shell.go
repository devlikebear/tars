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
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// ErrShellUnavailable reports that no POSIX shell could be located. Callers
// should surface it rather than substituting another interpreter: silently
// running a POSIX command string through something else is worse than not
// running it.
var ErrShellUnavailable = errors.New("posix shell not found")

// Executable returns the absolute path of the POSIX shell to run.
//
// Like git.Executable, the result is always absolute — never a bare "sh" left
// for the OS to resolve at exec time — and is cached for the process lifetime.
var Executable = sync.OnceValues(resolveExecutable)

func resolveExecutable() (string, error) {
	if defaultShellPath != "" {
		if info, err := os.Stat(defaultShellPath); err == nil && !info.IsDir() {
			return defaultShellPath, nil
		}
	}
	for _, name := range shellLookupNames {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if filepath.IsAbs(path) {
			return path, nil
		}
	}
	for _, path := range fallbackShellPaths() {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("%w: tried %v", ErrShellUnavailable, shellLookupNames)
}
