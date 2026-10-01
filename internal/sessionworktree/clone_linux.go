//go:build linux

package sessionworktree

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// cloneFile makes dst a copy-on-write clone of the regular file src with
// FICLONE (Btrfs, XFS and other reflink file systems). It fails when dst
// exists and leaves nothing behind when the file system cannot clone.
func cloneFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if err := unix.IoctlFileClone(int(out.Fd()), int(in.Fd())); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return fmt.Errorf("%w: %v", errCloneUnsupported, err)
	}
	return out.Close()
}
