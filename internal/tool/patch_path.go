package tool

import (
	"errors"
	"fmt"
	"sync"

	"github.com/devlikebear/tars/internal/exepath"
)

// ErrPatchUnavailable reports that the patch utility could not be located.
// apply_patch surfaces it as its own message so the caller sees a missing
// dependency rather than a patch that appeared to run and fail.
var ErrPatchUnavailable = errors.New("patch utility not found")

// patchExecutable returns the absolute path of the patch utility, resolved
// once per process.
var patchExecutable = sync.OnceValues(resolvePatchExecutable)

func resolvePatchExecutable() (string, error) {
	path, err := exepath.Resolve(exepath.Candidates{
		DefaultPath: defaultPatchPath,
		LookupNames: []string{"patch"},
		Fallbacks:   bundledPatchPaths,
	})
	if err != nil {
		return "", fmt.Errorf("%w: %v: install patch or a git distribution that bundles it", ErrPatchUnavailable, err)
	}
	return path, nil
}
