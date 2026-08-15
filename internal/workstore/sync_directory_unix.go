//go:build !windows

package workstore

import "os"

// syncDirectory flushes a directory entry to disk.
//
// Renaming a temporary file over its destination is only atomic once the
// directory entry itself is durable, so publishTemporaryFile fsyncs the parent
// directory after the rename. Without it a crash can leave the ledger's backup
// or export directory referencing a file that never made it to disk.
func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	return directory.Sync()
}
