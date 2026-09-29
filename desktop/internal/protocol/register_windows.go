//go:build windows

package protocol

import (
	"golang.org/x/sys/windows/registry"
)

// Register makes exe the current user's tars:// handler.
func Register(exe string) (bool, error) {
	base := `Software\Classes\` + Scheme
	command := WindowsCommand(exe)
	if key, err := registry.OpenKey(registry.CURRENT_USER, base+`\shell\open\command`, registry.QUERY_VALUE); err == nil {
		have, _, readErr := key.GetStringValue("")
		_ = key.Close()
		if readErr == nil && have == command {
			return false, nil
		}
	}
	root, _, err := registry.CreateKey(registry.CURRENT_USER, base, registry.SET_VALUE)
	if err != nil {
		return false, err
	}
	defer func() { _ = root.Close() }()
	if err := root.SetStringValue("", "URL:TARS"); err != nil {
		return false, err
	}
	if err := root.SetStringValue("URL Protocol", ""); err != nil {
		return false, err
	}
	cmd, _, err := registry.CreateKey(registry.CURRENT_USER, base+`\shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return false, err
	}
	defer func() { _ = cmd.Close() }()
	if err := cmd.SetStringValue("", command); err != nil {
		return false, err
	}
	return true, nil
}
