//go:build !darwin

package assistant

import (
	"fmt"
	"runtime"
)

func newGlobalHotkeyListener(raw string) (hotkeyListener, error) {
	if _, err := parseHotkeySpec(raw); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("global hotkey is unsupported on %s", runtime.GOOS)
}
