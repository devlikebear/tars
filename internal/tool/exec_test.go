package tool

import (
	"context"
	"encoding/json"
	"testing"
)

func TestExecTool_RunsCommand(t *testing.T) {
	root := t.TempDir()
	tl := NewExecTool(root)

	result, err := tl.Execute(context.Background(), json.RawMessage(`{"command":"echo hello"}`))
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
	if body.ExitCode != 0 {
		t.Fatalf("expected exit_code 0, got %d", body.ExitCode)
	}
	if body.Stdout == "" {
		t.Fatalf("expected stdout, got empty")
	}
}

func TestExecTool_BlocksDangerousCommand(t *testing.T) {
	root := t.TempDir()
	tl := NewExecTool(root)

	result, err := tl.Execute(context.Background(), json.RawMessage(`{"command":"rm -rf ./"}`))
	if err != nil {
		t.Fatalf("execute exec tool: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected blocked command error, got %s", result.Text())
	}

	var body execResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if body.Message == "" {
		t.Fatalf("expected error message")
	}
}

func TestExecTool_Timeout(t *testing.T) {
	root := t.TempDir()
	tl := NewExecTool(root)

	result, err := tl.Execute(context.Background(), json.RawMessage(`{"command":"sleep 1","timeout_ms":100}`))
	if err != nil {
		t.Fatalf("execute exec tool: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected timeout error, got %s", result.Text())
	}

	var body execResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !body.TimedOut {
		t.Fatalf("expected timed_out=true")
	}
}

func TestExecTool_AllowsCmdAlias(t *testing.T) {
	root := t.TempDir()
	tl := NewExecTool(root)

	result, err := tl.Execute(context.Background(), json.RawMessage(`{"cmd":"echo alias"}`))
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
	if body.ExitCode != 0 {
		t.Fatalf("expected exit_code 0, got %d", body.ExitCode)
	}
	if body.Command != "echo alias" {
		t.Fatalf("expected canonical command to be preserved, got %q", body.Command)
	}
}

func TestExecTool_RejectsNonObjectArguments(t *testing.T) {
	root := t.TempDir()
	tl := NewExecTool(root)

	result, err := tl.Execute(context.Background(), json.RawMessage(`["echo hi"]`))
	if err != nil {
		t.Fatalf("execute exec tool: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected argument error, got %s", result.Text())
	}

	var body execResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if body.Message == "" || body.Message == "command is required" {
		t.Fatalf("expected structured invalid argument message, got %q", body.Message)
	}
}

func TestExecTool_RejectsNonStringCommand(t *testing.T) {
	root := t.TempDir()
	tl := NewExecTool(root)

	result, err := tl.Execute(context.Background(), json.RawMessage(`{"command":123}`))
	if err != nil {
		t.Fatalf("execute exec tool: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected argument error, got %s", result.Text())
	}

	var body execResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if body.Message == "" || body.Message == "command is required" {
		t.Fatalf("expected invalid arguments for non-string command, got %q", body.Message)
	}
}
