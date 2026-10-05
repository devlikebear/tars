package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExecTool_ShellQuotedPrintf(t *testing.T) {
	requirePOSIXShell(t)

	body := executeShellCommand(t, `printf '%s' 'hello world'`, false)
	if body.ExitCode != 0 {
		t.Fatalf("expected exit_code 0, got %d: %+v", body.ExitCode, body)
	}
	if strings.TrimSpace(body.Stdout) != "hello world" {
		t.Fatalf("expected quoted argument to remain intact, got %q", body.Stdout)
	}
}

func TestExecTool_ShellSemicolon(t *testing.T) {
	requirePOSIXShell(t)

	body := executeShellCommand(t, `echo first; echo second`, false)
	if body.ExitCode != 0 {
		t.Fatalf("expected exit_code 0, got %d: %+v", body.ExitCode, body)
	}
	if got := strings.Fields(body.Stdout); len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("expected both semicolon-separated commands to run, got %q", body.Stdout)
	}
}

func TestExecTool_ShellPipeline(t *testing.T) {
	requirePOSIXShell(t)

	body := executeShellCommand(t, `printf 'alpha\nbeta\n' | grep beta`, false)
	if body.ExitCode != 0 {
		t.Fatalf("expected exit_code 0, got %d: %+v", body.ExitCode, body)
	}
	if strings.TrimSpace(body.Stdout) != "beta" {
		t.Fatalf("expected pipeline output %q, got %q", "beta", body.Stdout)
	}
}

func TestExecTool_ShellBackgroundQuotedCommand(t *testing.T) {
	requirePOSIXShell(t)

	root := t.TempDir()
	outputPath := filepath.Join(root, "quoted-output.txt")
	manager := NewProcessManager()
	tl := NewExecToolWithManager(root, manager)
	args, err := json.Marshal(map[string]any{
		"command":    `printf '%s' 'hello background' > quoted-output.txt`,
		"background": true,
	})
	if err != nil {
		t.Fatalf("encode arguments: %v", err)
	}

	result, err := tl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute exec tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected background command to start, got %s", result.Text())
	}
	var body execResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	snap, timedOut, err := manager.Wait(context.Background(), body.SessionID, 2000)
	if err != nil || timedOut || snap.ExitCode == nil || *snap.ExitCode != 0 {
		t.Fatalf("completion: %+v %v", snap, err)
	}
	contents, err := os.ReadFile(outputPath)
	if err != nil || string(contents) != "hello background" {
		t.Fatalf("output: %q %v", contents, err)
	}

}

func executeShellCommand(t *testing.T, command string, background bool) execResponse {
	t.Helper()

	root := t.TempDir()
	tl := NewExecTool(root)
	args, err := json.Marshal(map[string]any{
		"command":    command,
		"background": background,
	})
	if err != nil {
		t.Fatalf("encode arguments: %v", err)
	}

	result, err := tl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute exec tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %s", result.Text())
	}

	var body execResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return body
}

func requirePOSIXShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell semantics are not available on Windows")
	}
}
