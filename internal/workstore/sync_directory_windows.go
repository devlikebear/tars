//go:build windows

package workstore

import "os"

// syncDirectory verifies the directory but does not flush it, because Windows
// has no equivalent of the POSIX directory fsync.
//
// os.Open on a directory yields a read-only handle, and FlushFileBuffers needs
// write access, so the unix implementation fails outright here with "Access is
// denied" — which took every backup, export, and quarantine operation with it.
// Skipping the flush is not a durability regression: NTFS journals the
// metadata change that publishes the rename, so there is no directory entry
// left for user space to force out. Go databases such as bbolt and etcd make
// the same call.
//
// The Stat is kept so a caller naming a directory that does not exist still
// gets an error rather than a silent success.
func syncDirectory(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return nil
}
