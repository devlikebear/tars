package githubops

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ghRunner runs the `gh` CLI with the given arguments and returns combined output.
type ghRunner func(ctx context.Context, args []string) ([]byte, error)

// gitRunner runs a `git` command in the given working directory.
type gitRunner func(ctx context.Context, dir string, args []string) ([]byte, error)

var defaultGHRunner ghRunner = func(ctx context.Context, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	return cmd.CombinedOutput()
}

var defaultGitRunner gitRunner = func(ctx context.Context, dir string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.CombinedOutput()
}

// validRepo matches owner/repo slugs that gh accepts, rejecting shell tricks.
var validRepo = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-]*\/[A-Za-z0-9][A-Za-z0-9_.\-]*$`)

// validBranch rejects branch names with spaces, shell metacharacters, or leading dashes.
var validBranch = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-\/]*$`)

// wrapRunError formats an execution error with combined output for LLM consumption.
func wrapRunError(err error, output []byte) (string, map[string]any) {
	msg := strings.TrimSpace(string(output))
	var exitErr *exec.ExitError
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return "required CLI not found in PATH", nil
	case errors.As(err, &exitErr):
		if msg == "" {
			msg = exitErr.Error()
		}
		return "command failed", map[string]any{"detail": msg, "exit_code": exitErr.ExitCode()}
	default:
		return "command failed", map[string]any{"detail": fmt.Sprintf("%v", err)}
	}
}
