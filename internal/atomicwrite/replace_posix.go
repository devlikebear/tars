//go:build !windows

package atomicwrite

import "fmt"

func replaceAfterRenameError(tmpPath string, path string, renameErr error) error {
	return fmt.Errorf("atomicwrite: rename %q -> %q: %w", tmpPath, path, renameErr)
}
