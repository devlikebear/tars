package tarsclient

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestExecuteCommand_ProjectCommandsRedirectToCLI(t *testing.T) {
	runtime := runtimeClient{serverURL: "http://127.0.0.1:43180"}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	commands := []string{
		"/project board proj_1",
		"/project activity proj_1 2",
		"/project dispatch proj_1 todo",
		"/project autopilot start proj_1",
		"/project autopilot advance proj_1",
		"/project autopilot status proj_1",
	}
	for _, line := range commands {
		stdout.Reset()
		if _, _, err := executeCommand(context.Background(), runtime, line, "", stdout, stderr); err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		out := stdout.String()
		if !strings.Contains(out, "legacy TUI no longer handles /project") || !strings.Contains(out, "tars project") {
			t.Fatalf("unexpected redirect output for %s: %q", line, out)
		}
	}
}
