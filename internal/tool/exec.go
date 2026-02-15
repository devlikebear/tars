package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultExecTimeoutMS = 5000
	minExecTimeoutMS     = 100
	maxExecTimeoutMS     = 30000
	maxExecOutputBytes   = 8192
)

var blockedExecCommands = map[string]struct{}{
	"sudo":     {},
	"rm":       {},
	"shutdown": {},
	"reboot":   {},
	"halt":     {},
	"poweroff": {},
	"mkfs":     {},
	"dd":       {},
	"fdisk":    {},
	"kill":     {},
	"killall":  {},
}

type execResponse struct {
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	Message    string `json:"message,omitempty"`
}

func NewExecTool(workspaceDir string) Tool {
	return Tool{
		Name:        "exec",
		Description: "Run a shell command in workspace with timeout and safety restrictions.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "command":{"type":"string","description":"Command and arguments, e.g. ls -la"},
    "timeout_ms":{"type":"integer","minimum":100,"maximum":30000,"default":5000}
  },
  "required":["command"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Command   string `json:"command"`
				TimeoutMS *int   `json:"timeout_ms,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return execErrorResult("", fmt.Sprintf("invalid arguments: %v", err), -1, "", "", 0, false), nil
			}

			commandLine := strings.TrimSpace(input.Command)
			if commandLine == "" {
				return execErrorResult("", "command is required", -1, "", "", 0, false), nil
			}
			if strings.ContainsAny(commandLine, "\n\r") {
				return execErrorResult(commandLine, "multi-line command is not allowed", -1, "", "", 0, false), nil
			}

			fields := strings.Fields(commandLine)
			if len(fields) == 0 {
				return execErrorResult(commandLine, "command is required", -1, "", "", 0, false), nil
			}
			command := fields[0]
			if _, blocked := blockedExecCommands[strings.ToLower(command)]; blocked {
				return execErrorResult(commandLine, fmt.Sprintf("blocked command: %s", command), -1, "", "", 0, false), nil
			}

			timeoutMS := defaultExecTimeoutMS
			if input.TimeoutMS != nil {
				timeoutMS = *input.TimeoutMS
			}
			if timeoutMS < minExecTimeoutMS {
				timeoutMS = minExecTimeoutMS
			}
			if timeoutMS > maxExecTimeoutMS {
				timeoutMS = maxExecTimeoutMS
			}

			runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
			defer cancel()

			cmd := exec.CommandContext(runCtx, command, fields[1:]...)
			cmd.Dir = workspaceDir

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			start := time.Now()
			err := cmd.Run()
			durationMS := time.Since(start).Milliseconds()
			timedOut := runCtx.Err() == context.DeadlineExceeded

			stdoutText := trimOutput(stdout.String(), maxExecOutputBytes)
			stderrText := trimOutput(stderr.String(), maxExecOutputBytes)

			if err == nil {
				return jsonTextResult(execResponse{
					Command:    commandLine,
					ExitCode:   0,
					Stdout:     stdoutText,
					Stderr:     stderrText,
					DurationMS: durationMS,
				}, false), nil
			}

			exitCode := -1
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			}

			message := err.Error()
			if timedOut {
				message = fmt.Sprintf("command timed out after %dms", timeoutMS)
			}
			return execErrorResult(commandLine, message, exitCode, stdoutText, stderrText, durationMS, timedOut), nil
		},
	}
}

func execErrorResult(commandLine, message string, exitCode int, stdout, stderr string, durationMS int64, timedOut bool) Result {
	return jsonTextResult(execResponse{
		Command:    commandLine,
		ExitCode:   exitCode,
		Stdout:     stdout,
		Stderr:     stderr,
		DurationMS: durationMS,
		TimedOut:   timedOut,
		Message:    message,
	}, true)
}

func trimOutput(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	if maxBytes <= 3 {
		return value[:maxBytes]
	}
	return value[:maxBytes-3] + "..."
}
