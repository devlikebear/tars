package logwatcher

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

type fakeDockerRunner struct {
	args   [][]string
	output []byte
	err    error
}

func (f *fakeDockerRunner) run(_ context.Context, args []string) ([]byte, error) {
	f.args = append(f.args, append([]string(nil), args...))
	return f.output, f.err
}

func TestDockerLogs_ParsesJSONStructuredLines(t *testing.T) {
	runner := &fakeDockerRunner{
		output: []byte(`{"time":"2026-04-19T00:00:00Z","level":"ERROR","msg":"boom"}
{"time":"2026-04-19T00:00:01Z","level":"INFO","message":"ok"}
plain text line
`),
	}
	tl := newDockerLogsTool(runner.run)
	res, err := tl.Execute(context.Background(), json.RawMessage(`{"container_name":"foo"}`))
	if err != nil {
		t.Fatalf("execute returned err: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Text())
	}
	var out struct {
		Container string `json:"container"`
		Lines     []struct {
			Timestamp string `json:"ts"`
			Level     string `json:"level"`
			Message   string `json:"message"`
			Raw       string `json:"raw"`
		} `json:"lines"`
		Truncated bool `json:"truncated"`
		Since     string
		Tail      int
	}
	if err := json.Unmarshal([]byte(res.Text()), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if out.Container != "foo" {
		t.Fatalf("container mismatch: %q", out.Container)
	}
	if len(out.Lines) != 3 {
		t.Fatalf("expected 3 parsed lines, got %d", len(out.Lines))
	}
	if out.Lines[0].Level != "ERROR" || out.Lines[0].Message != "boom" {
		t.Fatalf("first line not parsed: %+v", out.Lines[0])
	}
	if out.Lines[1].Level != "INFO" || out.Lines[1].Message != "ok" {
		t.Fatalf("second line not parsed: %+v", out.Lines[1])
	}
	if out.Lines[2].Level != "" || out.Lines[2].Message != "" {
		t.Fatalf("third line should not be parsed as JSON: %+v", out.Lines[2])
	}
	if out.Lines[2].Raw != "plain text line" {
		t.Fatalf("third raw mismatch: %q", out.Lines[2].Raw)
	}
	if out.Since != "1h" || out.Tail != 200 {
		t.Fatalf("defaults not applied: since=%s tail=%d", out.Since, out.Tail)
	}
	if len(runner.args) != 1 {
		t.Fatalf("expected 1 docker call, got %d", len(runner.args))
	}
	wanted := []string{"logs", "--since", "1h", "--tail", "200", "foo"}
	for i, w := range wanted {
		if runner.args[0][i] != w {
			t.Fatalf("arg %d mismatch: got %q, want %q", i, runner.args[0][i], w)
		}
	}
}

func TestDockerLogs_RejectsMissingContainer(t *testing.T) {
	tl := newDockerLogsTool((&fakeDockerRunner{}).run)
	res, err := tl.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute err: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error result")
	}
	if !strings.Contains(res.Text(), "container_name is required") {
		t.Fatalf("unexpected message: %s", res.Text())
	}
}

func TestDockerLogs_RejectsInvalidContainerName(t *testing.T) {
	tl := newDockerLogsTool((&fakeDockerRunner{}).run)
	res, _ := tl.Execute(context.Background(), json.RawMessage(`{"container_name":"foo; rm -rf /"}`))
	if !res.IsError || !strings.Contains(res.Text(), "invalid characters") {
		t.Fatalf("expected invalid-char error, got: %s", res.Text())
	}
}

func TestDockerLogs_HandlesMissingCLI(t *testing.T) {
	runner := &fakeDockerRunner{err: exec.ErrNotFound}
	tl := newDockerLogsTool(runner.run)
	res, _ := tl.Execute(context.Background(), json.RawMessage(`{"container_name":"foo"}`))
	if !res.IsError {
		t.Fatalf("expected error")
	}
	if !strings.Contains(res.Text(), "docker CLI not found") {
		t.Fatalf("message mismatch: %s", res.Text())
	}
}

func TestDockerLogs_HandlesDaemonFailure(t *testing.T) {
	runner := &fakeDockerRunner{
		err:    errors.New("Cannot connect to the Docker daemon"),
		output: []byte("Cannot connect to the Docker daemon at unix:///var/run/docker.sock."),
	}
	tl := newDockerLogsTool(runner.run)
	res, _ := tl.Execute(context.Background(), json.RawMessage(`{"container_name":"foo"}`))
	if !res.IsError {
		t.Fatalf("expected error")
	}
	if !strings.Contains(res.Text(), "docker logs failed") {
		t.Fatalf("message mismatch: %s", res.Text())
	}
}

func TestDockerLogs_ClampsTailBounds(t *testing.T) {
	runner := &fakeDockerRunner{}
	tl := newDockerLogsTool(runner.run)
	_, _ = tl.Execute(context.Background(), json.RawMessage(`{"container_name":"foo","tail":99999}`))
	if len(runner.args) != 1 {
		t.Fatalf("expected 1 call")
	}
	// args: logs --since 1h --tail 2000 foo
	if runner.args[0][4] != "2000" {
		t.Fatalf("tail not clamped to 2000, got %q", runner.args[0][4])
	}
}
