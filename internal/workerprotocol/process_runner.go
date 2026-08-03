package workerprotocol

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

type ProcessSpec struct {
	Path           string
	Args           []string
	Env            []string
	Stdin          []byte
	InheritEnv     bool
	MaxOutputBytes int64
}

type ProcessResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

type ProcessRunner interface {
	Run(context.Context, ProcessSpec) (ProcessResult, error)
}

type OSProcessRunner struct{}

func (OSProcessRunner) Run(ctx context.Context, spec ProcessSpec) (ProcessResult, error) {
	if spec.Path == "" || spec.MaxOutputBytes <= 0 {
		return ProcessResult{}, ErrTransportConfig
	}
	command := exec.CommandContext(ctx, spec.Path, spec.Args...)
	if spec.InheritEnv {
		command.Env = append(os.Environ(), spec.Env...)
	} else {
		command.Env = make([]string, 0, len(spec.Env))
		command.Env = append(command.Env, spec.Env...)
	}
	command.Stdin = bytes.NewReader(spec.Stdin)
	stdout := newBoundedProcessBuffer(spec.MaxOutputBytes)
	stderr := newBoundedProcessBuffer(spec.MaxOutputBytes)
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	result := ProcessResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if command.ProcessState != nil {
		result.ExitCode = command.ProcessState.ExitCode()
	}
	if stdout.overflow || stderr.overflow {
		return result, ErrTransportLimit
	}
	if err != nil {
		return result, fmt.Errorf("workerprotocol: process %q failed: %w", spec.Path, err)
	}
	return result, nil
}

type boundedProcessBuffer struct {
	buffer   bytes.Buffer
	limit    int64
	overflow bool
}

func newBoundedProcessBuffer(limit int64) *boundedProcessBuffer {
	return &boundedProcessBuffer{limit: limit}
}

func (buffer *boundedProcessBuffer) Write(raw []byte) (int, error) {
	originalLength := len(raw)
	remaining := buffer.limit - int64(buffer.buffer.Len())
	if remaining <= 0 {
		buffer.overflow = buffer.overflow || originalLength > 0
		return originalLength, nil
	}
	if int64(len(raw)) > remaining {
		raw = raw[:remaining]
		buffer.overflow = true
	}
	_, _ = buffer.buffer.Write(raw)
	return originalLength, nil
}

func (buffer *boundedProcessBuffer) Bytes() []byte {
	return append([]byte(nil), buffer.buffer.Bytes()...)
}

var _ ProcessRunner = OSProcessRunner{}
