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

// gitRootSearchDepth is how far above git.exe the installation root can sit.
// git resolves to <root>\cmd\git.exe on a stock install but to
// <root>\mingw64\bin\git.exe from inside a Git Bash environment, so the root is
// two or three levels up depending on which one PATH happened to yield.
const gitRootSearchDepth = 3

// fallbackShellPaths derives a shell from git's own install.
//
// Git for Windows bundles a full MSYS2 environment, but its installer only
// puts <root>\cmd on PATH — that directory holds git.exe and little else, so
// sh.exe and bash.exe are usually absent from PATH even though they are
// installed. tars already requires git, so walking up from git to the
// installation root finds the shell on a stock install.
func fallbackShellPaths() []string {
	gitPath, err := gitrepo.Executable()
	if err != nil {
		return nil
	}
	var paths []string
	dir := filepath.Dir(gitPath)
	for depth := 0; depth < gitRootSearchDepth; depth++ {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
		for _, parts := range shellsUnderGitRoot {
			paths = append(paths, filepath.Join(append([]string{dir}, parts...)...))
		}
	}
	return paths
}
