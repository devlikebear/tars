package project

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type GitHubAuthChecker func(context.Context) error

func defaultGitHubAuthChecker(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "gh", "auth", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = strings.TrimSpace(err.Error())
		}
		return fmt.Errorf("github auth precondition failed: gh auth status: %s", message)
	}
	return nil
}

func verificationStatus(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case value == "":
		return "blocked"
	case strings.Contains(value, "fail"), strings.Contains(value, "error"), strings.Contains(value, "blocked"):
		return "failed"
	case strings.Contains(value, "pass"), strings.Contains(value, "ok"), strings.Contains(value, "success"):
		return "passed"
	default:
		return "blocked"
	}
}
