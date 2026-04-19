package logwatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/tool"
)

// dockerRunner executes a docker CLI invocation and returns combined stdout
// + stderr. Extracted as a variable for test injection.
type dockerRunner func(ctx context.Context, args []string) ([]byte, error)

var defaultDockerRunner dockerRunner = func(ctx context.Context, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	return cmd.CombinedOutput()
}

var validContainerName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.\-/]*$`)

type dockerLogsInput struct {
	ContainerName string `json:"container_name"`
	Since         string `json:"since,omitempty"`
	Tail          int    `json:"tail,omitempty"`
}

type logLine struct {
	Timestamp string `json:"ts,omitempty"`
	Level     string `json:"level,omitempty"`
	Message   string `json:"message,omitempty"`
	Raw       string `json:"raw"`
}

type dockerLogsOutput struct {
	Container string    `json:"container"`
	Since     string    `json:"since"`
	Tail      int       `json:"tail"`
	Lines     []logLine `json:"lines"`
	Truncated bool      `json:"truncated"`
}

const dockerLogsMaxTail = 2000

func newDockerLogsTool(runner dockerRunner) tool.Tool {
	if runner == nil {
		runner = defaultDockerRunner
	}
	return tool.Tool{
		Name: "docker_logs",
		Description: "Fetch recent logs from a named Docker container. " +
			"Returns parsed log lines (JSON structured logs are decoded when possible) " +
			"plus a 'truncated' flag when output was capped at --tail. Requires the " +
			"docker CLI and a running Docker daemon.",
		Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "container_name": {"type":"string","description":"Name or ID of the Docker container."},
    "since":          {"type":"string","description":"Relative duration (e.g. '15m', '1h') or RFC3339 timestamp.","default":"1h"},
    "tail":           {"type":"integer","description":"Max lines to return (1..2000).","default":200}
  },
  "required": ["container_name"],
  "additionalProperties": false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (tool.Result, error) {
			var input dockerLogsInput
			if err := json.Unmarshal(params, &input); err != nil {
				return tool.JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			input.ContainerName = strings.TrimSpace(input.ContainerName)
			input.Since = strings.TrimSpace(input.Since)
			if input.ContainerName == "" {
				return tool.JSONTextResult(map[string]any{"message": "container_name is required"}, true), nil
			}
			if !validContainerName.MatchString(input.ContainerName) {
				return tool.JSONTextResult(map[string]any{"message": "container_name contains invalid characters"}, true), nil
			}
			since := input.Since
			if since == "" {
				since = "1h"
			}
			tail := input.Tail
			if tail <= 0 {
				tail = 200
			}
			if tail > dockerLogsMaxTail {
				tail = dockerLogsMaxTail
			}

			args := []string{"logs", "--since", since, "--tail", strconv.Itoa(tail), input.ContainerName}
			output, err := runner(ctx, args)
			if err != nil {
				msg := strings.TrimSpace(string(output))
				var exitErr *exec.ExitError
				switch {
				case errors.Is(err, exec.ErrNotFound):
					return tool.JSONTextResult(map[string]any{"message": "docker CLI not found in PATH"}, true), nil
				case errors.As(err, &exitErr):
					if msg == "" {
						msg = exitErr.Error()
					}
					return tool.JSONTextResult(map[string]any{"message": "docker logs failed", "detail": msg}, true), nil
				default:
					return tool.JSONTextResult(map[string]any{"message": "docker logs failed", "detail": err.Error()}, true), nil
				}
			}

			lines := parseLogLines(string(output))
			out := dockerLogsOutput{
				Container: input.ContainerName,
				Since:     since,
				Tail:      tail,
				Lines:     lines,
				Truncated: len(lines) >= tail,
			}
			return tool.JSONTextResult(out, false), nil
		},
	}
}

func parseLogLines(raw string) []logLine {
	raw = strings.TrimRight(raw, "\n")
	if raw == "" {
		return []logLine{}
	}
	rawLines := strings.Split(raw, "\n")
	out := make([]logLine, 0, len(rawLines))
	for _, text := range rawLines {
		trimmed := strings.TrimRight(text, "\r")
		if trimmed == "" {
			continue
		}
		line := logLine{Raw: trimmed}
		if decoded, ok := tryDecodeJSONLog(trimmed); ok {
			line.Timestamp = decoded.Timestamp
			line.Level = decoded.Level
			line.Message = decoded.Message
		}
		out = append(out, line)
	}
	return out
}

type jsonLog struct {
	Timestamp string
	Level     string
	Message   string
}

func tryDecodeJSONLog(raw string) (jsonLog, bool) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") {
		return jsonLog{}, false
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
		return jsonLog{}, false
	}
	out := jsonLog{}
	for _, key := range []string{"time", "timestamp", "ts", "@timestamp"} {
		if v, ok := fields[key]; ok {
			if s := formatLogValue(v); s != "" {
				out.Timestamp = s
				break
			}
		}
	}
	for _, key := range []string{"level", "severity", "lvl"} {
		if v, ok := fields[key]; ok {
			if s := formatLogValue(v); s != "" {
				out.Level = s
				break
			}
		}
	}
	for _, key := range []string{"message", "msg", "body"} {
		if v, ok := fields[key]; ok {
			if s := formatLogValue(v); s != "" {
				out.Message = s
				break
			}
		}
	}
	if out.Timestamp == "" && out.Level == "" && out.Message == "" {
		return jsonLog{}, false
	}
	return out, true
}

func formatLogValue(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case time.Time:
		return t.Format(time.RFC3339)
	default:
		return ""
	}
}
