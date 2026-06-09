//go:build windows

package atomicwrite

import (
	"fmt"
	"os"
)

func replaceFile(tmpPath string, path string) error {
	if err := os.Rename(tmpPath, path); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("atomicwrite: replace existing %q: %w", path, err)
		}
		if retryErr := os.Rename(tmpPath, path); retryErr != nil {
			return fmt.Errorf("atomicwrite: rename %q -> %q: %w", tmpPath, path, retryErr)
		}
	}
	return nil
}
