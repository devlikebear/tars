//go:build windows

package git

import "path/filepath"

// installRootSearchDepth is how far above git.exe the installation root can
// sit. git resolves to <root>\cmd\git.exe on a stock install but to
// <root>\mingw64\bin\git.exe from inside a Git Bash environment, so the root is
// two or three levels up depending on which one PATH happened to yield.
const installRootSearchDepth = 3

// InstallRoots returns candidate Git for Windows installation roots, nearest
// first, derived from the resolved git binary.
//
// Git for Windows bundles a full MSYS2 environment — sh.exe, bash.exe,
// patch.exe and more under <root>\usr\bin — but its installer only puts
// <root>\cmd on PATH. Those tools are therefore installed yet unreachable by
// name. Since tars already requires git, walking up from it locates them.
//
// The roots are candidates, not guarantees: callers must check that the file
// they want actually exists under one.
func InstallRoots() []string {
	gitPath, err := Executable()
	if err != nil {
		return nil
	}
	var roots []string
	dir := filepath.Dir(gitPath)
	for depth := 0; depth < installRootSearchDepth; depth++ {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
		roots = append(roots, dir)
	}
	return roots
}
