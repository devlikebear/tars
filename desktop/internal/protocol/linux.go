package protocol

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// LinuxRegistrar writes the desktop entry and makes it the tars:// handler.
type LinuxRegistrar struct {
	// DataHome is $XDG_DATA_HOME, usually ~/.local/share.
	DataHome string
	// Run runs a helper command (xdg-mime); nil skips it.
	Run func(name string, args ...string) error
}

// DataHome resolves $XDG_DATA_HOME with its ~/.local/share default.
func DataHome(getenv func(string) string, home string) string {
	if dir := getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(home, ".local", "share")
}

// Register points tars:// at exe. It rewrites the entry only when it
// changed, and reports changed so the caller can log a move.
func (r LinuxRegistrar) Register(exe string) (changed bool, err error) {
	if r.DataHome == "" {
		return false, fmt.Errorf("protocol: no data home")
	}
	dir := filepath.Join(r.DataHome, "applications")
	path := filepath.Join(dir, DesktopFileName)
	want := []byte(DesktopEntry(exe))
	if have, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(have, want) {
		return false, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, want, 0o644); err != nil {
		return false, err
	}
	if r.Run != nil {
		if err := r.Run("xdg-mime", "default", DesktopFileName, "x-scheme-handler/"+Scheme); err != nil {
			return true, fmt.Errorf("protocol: xdg-mime: %w", err)
		}
	}
	return true, nil
}
