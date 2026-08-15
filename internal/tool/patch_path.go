package tool

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// ErrPatchUnavailable reports that the patch utility could not be located.
// apply_patch surfaces it as its own message so the caller sees a missing
// dependency rather than a patch that appeared to run and fail.
var ErrPatchUnavailable = errors.New("patch utility not found")

// patchExecutable returns the absolute path of the patch utility, resolved
// once per process. Like git.Executable and shellexec.Executable, it never
// returns a bare name for the OS to resolve at exec time.
var patchExecutable = sync.OnceValues(resolvePatchExecutable)

func resolvePatchExecutable() (string, error) {
	if defaultPatchPath != "" {
		if info, err := os.Stat(defaultPatchPath); err == nil && !info.IsDir() {
			return defaultPatchPath, nil
		}
	}
	if path, err := exec.LookPath("patch"); err == nil && filepath.IsAbs(path) {
		return path, nil
	}
	for _, path := range bundledPatchPaths() {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("%w: install patch or a git distribution that bundles it", ErrPatchUnavailable)
}
