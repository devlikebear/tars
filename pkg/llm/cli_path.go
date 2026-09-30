package llm

import (
	"os"
	"os/exec"
	"path/filepath"
)

// lookPathOrUserBin resolves name on PATH, then in ~/.local/bin. The native
// installers of both claude and agy put their binary there, and a launchd
// service runs with a minimal PATH that leaves it out, so a PATH lookup alone
// sends a working install into setup-only mode.
func lookPathOrUserBin(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err == nil {
		return path, nil
	}
	home, homeErr := os.UserHomeDir()
	if homeErr != nil || home == "" {
		return "", err
	}
	if userPath, userErr := exec.LookPath(filepath.Join(home, ".local", "bin", name)); userErr == nil {
		return userPath, nil
	}
	return "", err
}
