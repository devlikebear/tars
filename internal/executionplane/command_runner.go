package executionplane

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

type CommandSpec struct {
	Command    string
	Args       []string
	Dir        string
	Env        []string
	Stdin      []byte
	InheritEnv bool
}

type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type CommandRunner interface {
	Run(context.Context, CommandSpec) (CommandResult, error)
}

type OSCommandRunner struct{}

func (OSCommandRunner) Run(ctx context.Context, spec CommandSpec) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, spec.Command, spec.Args...)
	cmd.Dir = spec.Dir
	if spec.InheritEnv {
		cmd.Env = append(os.Environ(), spec.Env...)
	} else {
		cmd.Env = append([]string(nil), spec.Env...)
	}
	if len(spec.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(spec.Stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return result, fmt.Errorf("executionplane: command %q failed: %w", spec.Command, err)
	}
	return result, nil
}

var _ CommandRunner = OSCommandRunner{}
