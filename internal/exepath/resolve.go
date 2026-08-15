// Package exepath resolves absolute paths to the external executables tars
// shells out to.
//
// tars used to name these by absolute path (/usr/bin/git, /bin/sh,
// /usr/bin/patch) so that exec never received a bare name for the OS to
// resolve against PATH at run time. Those paths do not exist on every
// platform, so resolution has to be dynamic — but the guarantee is kept: a
// caller either gets an absolute path or an error.
package exepath

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ErrNotFound reports that none of the candidates existed. Callers wrap it
// with a sentinel naming the tool they were looking for.
var ErrNotFound = errors.New("executable not found")

// Candidates describes where to look for an executable, in priority order.
type Candidates struct {
	// DefaultPath is the platform's canonical absolute location, tried first
	// so platforms that have one keep byte-identical behaviour. Empty when the
	// platform has no such location.
	DefaultPath string

	// LookupNames are searched on PATH, in order. A result that is not
	// absolute is rejected rather than returned.
	LookupNames []string

	// Fallbacks supplies absolute paths to try last, for tools that are
	// installed but unreachable by name. It is only called if the earlier
	// steps miss, since building the list can itself be expensive.
	Fallbacks func() []string
}

// Resolve returns the first candidate that exists, or ErrNotFound.
func Resolve(candidates Candidates) (string, error) {
	if candidates.DefaultPath != "" && isFile(candidates.DefaultPath) {
		return candidates.DefaultPath, nil
	}
	for _, name := range candidates.LookupNames {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if filepath.IsAbs(path) {
			return path, nil
		}
	}
	if candidates.Fallbacks != nil {
		for _, path := range candidates.Fallbacks() {
			if isFile(path) {
				return path, nil
			}
		}
	}
	return "", fmt.Errorf("%w: tried %v", ErrNotFound, candidates.LookupNames)
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
