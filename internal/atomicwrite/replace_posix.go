//go:build !windows

package atomicwrite

import (
	"fmt"
	"os"
)

func replaceFile(tmpPath string, path string) error {
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("atomicwrite: rename %q -> %q: %w", tmpPath, path, err)
	}
	return nil
}
