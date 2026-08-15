package onboarding

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SpawnOptions describes a detached child process. The child inherits
// the current environment unless Env is set; both stdout and stderr
// are written to LogPath (or os.DevNull when LogPath is empty).
type SpawnOptions struct {
	Executable string
	Args       []string
	WorkingDir string
	Env        []string
	LogPath    string
}

// SpawnResult reports the launched child's PID and the resolved log
// path so the caller can surface where output went.
type SpawnResult struct {
	PID     int
	LogPath string
}

// SpawnDetached starts a background process that survives the parent
// exiting. Stdout/stderr go to LogPath (created if needed). Used by
// `tars init` when running in --no-service mode (or on non-darwin
// systems) to start `tars serve` without blocking. Detaching itself is
// OS-specific and lives in configureDetachedProcess.
func SpawnDetached(opts SpawnOptions) (SpawnResult, error) {
	exe := strings.TrimSpace(opts.Executable)
	if exe == "" {
		return SpawnResult{}, fmt.Errorf("executable is required")
	}

	logPath := strings.TrimSpace(opts.LogPath)
	var output *os.File
	var err error
	if logPath == "" {
		output, err = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			return SpawnResult{}, fmt.Errorf("open %s: %w", os.DevNull, err)
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
			return SpawnResult{}, fmt.Errorf("create log dir: %w", err)
		}
		output, err = os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return SpawnResult{}, fmt.Errorf("open log file: %w", err)
		}
	}
	defer func() { _ = output.Close() }()

	cmd := exec.Command(exe, opts.Args...)
	if strings.TrimSpace(opts.WorkingDir) != "" {
		cmd.Dir = opts.WorkingDir
	}
	if len(opts.Env) > 0 {
		cmd.Env = opts.Env
	}
	cmd.Stdout = output
	cmd.Stderr = output
	configureDetachedProcess(cmd)

	if err := cmd.Start(); err != nil {
		return SpawnResult{}, fmt.Errorf("start %s: %w", exe, err)
	}
	pid := cmd.Process.Pid

	// Release the process so it isn't reaped when the parent exits.
	// Process.Release zeroes the Pid field, so capture it first.
	if err := cmd.Process.Release(); err != nil {
		return SpawnResult{}, fmt.Errorf("release process: %w", err)
	}

	return SpawnResult{
		PID:     pid,
		LogPath: logPath,
	}, nil
}
