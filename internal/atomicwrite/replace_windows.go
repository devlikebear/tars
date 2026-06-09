//go:build windows

package atomicwrite

import (
	"fmt"
	"os"
)

func replaceAfterRenameError(tmpPath string, path string, renameErr error) error {
	if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
		return fmt.Errorf("atomicwrite: replace existing %q: %w", path, removeErr)
	}
	if retryErr := os.Rename(tmpPath, path); retryErr != nil {
		return fmt.Errorf("atomicwrite: rename %q -> %q: %w", tmpPath, path, retryErr)
	}
	return nil
}
