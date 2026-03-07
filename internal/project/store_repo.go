package project

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func cloneProjectRepo(projectDir string, repo string) error {
	target := filepath.Join(projectDir, "repo")
	if _, err := os.Stat(target); err == nil {
		return nil
	}
	cmd := exec.Command("git", "clone", "--depth", "1", repo, target)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("clone project repo: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
