//go:build linux

package protocol

import (
	"os"
	"os/exec"
)

// Register makes exe the current user's tars:// handler.
func Register(exe string) (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	r := LinuxRegistrar{DataHome: DataHome(os.Getenv, home)}
	if _, err := exec.LookPath("xdg-mime"); err == nil {
		r.Run = func(name string, args ...string) error { return exec.Command(name, args...).Run() }
	}
	return r.Register(exe)
}
