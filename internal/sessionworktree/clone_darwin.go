//go:build darwin

package sessionworktree

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// cloneFile makes dst a copy-on-write clone of the regular file src (APFS).
// It never follows a symlink at src and fails when dst exists.
func cloneFile(src, dst string) error {
	if err := unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW); err != nil {
		return fmt.Errorf("%w: %v", errCloneUnsupported, err)
	}
	return nil
}
