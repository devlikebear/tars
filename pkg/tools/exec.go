package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	defaultExecTimeoutMS    = 5000
	minExecTimeoutMS        = 100
	defaultExecMaxTimeoutMS = 300000 // 5 minutes
	maxExecOutputBytes      = 8192
	missingCommandHint      = `command is required; provide JSON like {"command":"pwd"}`
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

// ExecToolOptions tunes the exec tool factory. The zero value picks safe
// defaults so callers (especially tests) can omit fields they don't care
// about.
type ExecToolOptions struct {
	// MaxTimeoutMS caps the per-call timeout for synchronous (foreground)
	// exec calls. 0 → defaultExecMaxTimeoutMS (5 minutes).
	MaxTimeoutMS int
	// MaxBackgroundTimeoutMS caps the per-call timeout for async
	// (background:true) calls dispatched through the ProcessManager. 0 →
	// defaultProcessMaxTimeoutMS (30 minutes). Watchers like
	// `gh pr checks --watch` and long builds run here, so this cap is
	// independently larger than MaxTimeoutMS.
	MaxBackgroundTimeoutMS int
}

type execResponse struct {
	Command    string `json:"command"`
	Status     string `json:"status,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	Message    string `json:"message,omitempty"`
}

func NewExecTool(workspaceDir string) Tool {
	return NewExecToolWithManager(workspaceDir, nil)
}

func NewExecToolWithManager(workspaceDir string, manager *ProcessManager) Tool {
	return NewExecToolWithPolicy(SingleDirPolicy(workspaceDir), manager)
}

func NewExecToolWithPolicy(policy PathPolicy, manager *ProcessManager) Tool {
	return NewExecToolWithOptions(policy, manager, ExecToolOptions{})
}

// NewExecToolWithOptions exposes the timeout knobs so chat handlers can
// raise per-call caps from runtime config. Foreground and background
// caps are independent: foreground stays modest (default 5min) while
// background tolerates long watchers (default 30min).
func NewExecToolWithOptions(policy PathPolicy, manager *ProcessManager, opts ExecToolOptions) Tool {
	maxTimeoutMS := opts.MaxTimeoutMS
	if maxTimeoutMS <= 0 {
		maxTimeoutMS = defaultExecMaxTimeoutMS
	}
	if maxTimeoutMS < minExecTimeoutMS {
		maxTimeoutMS = minExecTimeoutMS
	}
	maxBackgroundTimeoutMS := opts.MaxBackgroundTimeoutMS
	if maxBackgroundTimeoutMS <= 0 {
		maxBackgroundTimeoutMS = defaultProcessMaxTimeoutMS
	}
	if maxBackgroundTimeoutMS < minExecTimeoutMS {
		maxBackgroundTimeoutMS = minExecTimeoutMS
	}
	parameters := json.RawMessage(fmt.Sprintf(`{
  "type":"object",
  "properties":{
    "command":{"type":"string","description":"Command and arguments, e.g. ls -la"},
    "timeout_ms":{"type":"integer","minimum":%d,"maximum":%d,"default":%d,"description":"Per-call timeout in ms. Capped to %d for foreground calls and %d when background=true."},
    "background":{"type":"boolean","default":false,"description":"When true, returns a session_id immediately and runs the command in the background. Pair with the process tool's wait action for long-running commands like builds, installs, and gh pr checks --watch."}
  },
  "required":["command"],
  "additionalProperties":false
}`, minExecTimeoutMS, maxBackgroundTimeoutMS, defaultExecTimeoutMS, maxTimeoutMS, maxBackgroundTimeoutMS))
	return Tool{
		Name:        "exec",
		Description: "Run a shell command in workspace with timeout and safety restrictions. For commands expected to run longer than ~30s (builds, installs, CI watchers), set background:true and use the `process` tool's `wait` action instead of blocking the chat.",
		Parameters:  parameters,
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			commandLine, timeoutMS, background, err := parseExecInput(params)
			if err != nil {
				return execErrorResult("", fmt.Sprintf("invalid arguments: %v", err), -1, "", "", 0, false), nil
			}
			commandLine = strings.TrimSpace(commandLine)
			if commandLine == "" {
				return execErrorResult("", missingCommandHint, -1, "", "", 0, false), nil
			}
			if strings.ContainsAny(commandLine, "\n\r") {
				return execErrorResult(commandLine, "multi-line command is not allowed", -1, "", "", 0, false), nil
			}

			fields := strings.Fields(commandLine)
			if len(fields) == 0 {
				return execErrorResult(commandLine, missingCommandHint, -1, "", "", 0, false), nil
			}
			command := fields[0]
			if _, blocked := blockedExecCommands[strings.ToLower(command)]; blocked {
				return execErrorResult(commandLine, fmt.Sprintf("blocked command: %s", command), -1, "", "", 0, false), nil
			}

			if timeoutMS < minExecTimeoutMS {
				timeoutMS = minExecTimeoutMS
			}
			cap := maxTimeoutMS
			if background {
				cap = maxBackgroundTimeoutMS
			}
			if timeoutMS > cap {
				timeoutMS = cap
			}
			if background {
				if manager == nil {
					return execErrorResult(commandLine, "background execution requires process manager", -1, "", "", 0, false), nil
				}
				snap, err := manager.Start(ctx, policy.PrimaryDir, commandLine, timeoutMS)
				if err != nil {
					return execErrorResult(commandLine, err.Error(), -1, "", "", 0, false), nil
				}
				return JSONTextResult(execResponse{
					Command:   commandLine,
					Status:    "running",
					SessionID: snap.SessionID,
					ExitCode:  0,
					Message:   "process started in background",
				}, false), nil
			}

			runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
			defer cancel()

			cmd := exec.CommandContext(runCtx, command, fields[1:]...)
			cmd.Dir = policy.PrimaryDir

			stdoutPipe, err := cmd.StdoutPipe()
			if err != nil {
				return execErrorResult(commandLine, fmt.Sprintf("stdout pipe: %v", err), -1, "", "", 0, false), nil
			}
			stderrPipe, err := cmd.StderrPipe()
			if err != nil {
				return execErrorResult(commandLine, fmt.Sprintf("stderr pipe: %v", err), -1, "", "", 0, false), nil
			}

			start := time.Now()
			if err := cmd.Start(); err != nil {
				return execErrorResult(commandLine, err.Error(), -1, "", "", 0, false), nil
			}

			streamer := ToolOutputStreamerFromContext(ctx)
			var stdout, stderr bytes.Buffer
			var wg sync.WaitGroup
			wg.Add(2)
			go scanAndCapture(stdoutPipe, &stdout, streamer, StreamStdout, &wg)
			go scanAndCapture(stderrPipe, &stderr, streamer, StreamStderr, &wg)

			// Drain pipes before reaping the process. Scanner goroutines reach
			// EOF naturally when the child closes its stdout/stderr on exit,
			// so wg.Wait first guarantees every line is captured. Calling
			// cmd.Wait first closes our read fds before the goroutines have
			// scheduled on a CPU-saturated host, which dropped streamed
			// output on busy CI runners.
			wg.Wait()
			runErr := cmd.Wait()
			durationMS := time.Since(start).Milliseconds()
			timedOut := runCtx.Err() == context.DeadlineExceeded

			stdoutText := trimOutput(stdout.String(), maxExecOutputBytes)
			stderrText := trimOutput(stderr.String(), maxExecOutputBytes)

			if runErr == nil {
				return JSONTextResult(execResponse{
					Command:    commandLine,
					ExitCode:   0,
					Stdout:     stdoutText,
					Stderr:     stderrText,
					DurationMS: durationMS,
				}, false), nil
			}

			exitCode := -1
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			}

			message := runErr.Error()
			if timedOut {
				message = fmt.Sprintf("command timed out after %dms", timeoutMS)
			}
			return execErrorResult(commandLine, message, exitCode, stdoutText, stderrText, durationMS, timedOut), nil
		},
	}
}

// scanAndCapture reads lines from a tool's stdout/stderr pipe,
// simultaneously buffering them into `dst` (for the final result) and
// fanning each line out to `streamer` (for live SSE delivery).
func scanAndCapture(reader io.Reader, dst *bytes.Buffer, streamer ToolOutputStreamer, stream string, wg *sync.WaitGroup) {
	defer wg.Done()
	if reader == nil {
		return
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		dst.WriteString(line)
		dst.WriteByte('\n')
		if streamer != nil {
			streamer.EmitLine(stream, line)
		}
	}
}

func execErrorResult(commandLine, message string, exitCode int, stdout, stderr string, durationMS int64, timedOut bool) Result {
	return JSONTextResult(execResponse{
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

func parseExecInput(params json.RawMessage) (string, int, bool, error) {
	raw := strings.TrimSpace(string(params))
	if raw == "" || raw == "null" {
		return "", defaultExecTimeoutMS, false, nil
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(params, &payload); err != nil {
		return "", 0, false, err
	}
	if payload == nil {
		payload = map[string]json.RawMessage{}
	}

	timeoutMS := defaultExecTimeoutMS
	if v, ok := payload["timeout_ms"]; ok {
		if err := json.Unmarshal(v, &timeoutMS); err != nil {
			return "", 0, false, fmt.Errorf("timeout_ms must be integer")
		}
	}
	background := false
	if v, ok := payload["background"]; ok {
		if err := json.Unmarshal(v, &background); err != nil {
			return "", 0, false, fmt.Errorf("background must be boolean")
		}
	}

	var commandLine string
	if v, ok := payload["command"]; ok {
		if err := json.Unmarshal(v, &commandLine); err != nil {
			return "", 0, false, fmt.Errorf("command must be string")
		}
	}
	return commandLine, timeoutMS, background, nil
}
