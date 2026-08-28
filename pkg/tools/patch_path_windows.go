//go:build windows

package tools

import (
	"path/filepath"

	gitrepo "github.com/devlikebear/tars/internal/git"
)

// Windows has no system patch utility, so there is no default path to try.
const defaultPatchPath = ""

// bundledPatchPaths looks for the patch.exe that Git for Windows installs
// under <root>\usr\bin, which is normally present but absent from PATH. See
// git.InstallRoots.
func bundledPatchPaths() []string {
	var paths []string
	for _, root := range gitrepo.InstallRoots() {
		paths = append(paths, filepath.Join(root, "usr", "bin", "patch.exe"))
	}
	return paths
}
