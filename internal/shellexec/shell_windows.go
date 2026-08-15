//go:build windows

package shellexec

import (
	"path/filepath"

	gitrepo "github.com/devlikebear/tars/internal/git"
)

// Windows ships no POSIX shell, so there is no default path to try.
const defaultShellPath = ""

var shellLookupNames = []string{"sh", "bash"}

// shellsUnderGitRoot are the shells Git for Windows installs, relative to the
// installation root.
var shellsUnderGitRoot = [][]string{
	{"usr", "bin", "sh.exe"},
	{"bin", "bash.exe"},
	{"usr", "bin", "bash.exe"},
}

// fallbackShellPaths looks for the shell Git for Windows bundles, which is
// normally installed but absent from PATH. See git.InstallRoots.
func fallbackShellPaths() []string {
	var paths []string
	for _, root := range gitrepo.InstallRoots() {
		for _, parts := range shellsUnderGitRoot {
			paths = append(paths, filepath.Join(append([]string{root}, parts...)...))
		}
	}
	return paths
}
