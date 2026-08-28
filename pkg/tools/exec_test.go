package tools

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
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

func TestExecTool_MaxTimeoutOptionClampsRequest(t *testing.T) {
	root := t.TempDir()
	tl := NewExecToolWithOptions(SingleDirPolicy(root), nil, ExecToolOptions{MaxTimeoutMS: 200})

	// Request 5s but factory caps at 200ms; sleep 1s should hit the cap.
	result, err := tl.Execute(context.Background(), json.RawMessage(`{"command":"sleep 1","timeout_ms":5000}`))
	if err != nil {
		t.Fatalf("execute exec tool: %v", err)
	}
	var body execResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !body.TimedOut {
		t.Fatalf("expected timeout because cap shrinks to 200ms, got %+v", body)
	}
	if body.DurationMS > 1500 {
		t.Fatalf("expected duration capped near 200ms, got %dms", body.DurationMS)
	}
}

func TestExecTool_StreamsStdoutLinesViaContext(t *testing.T) {
	root := t.TempDir()
	tl := NewExecTool(root)

	rec := &recordingEmitter{}
	streamer := BindLineEmitter(rec, "call-1")
	ctx := WithToolOutputStreamer(context.Background(), streamer)

	result, err := tl.Execute(ctx, json.RawMessage(`{"command":"printf line-a\\nline-b\\nline-c\\n","timeout_ms":2000}`))
	if err != nil {
		t.Fatalf("execute exec tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %s", result.Text())
	}
	if len(rec.events) != 3 {
		t.Fatalf("expected 3 streamed lines, got %d: %+v", len(rec.events), rec.events)
	}
	for i, want := range []string{"line-a", "line-b", "line-c"} {
		if rec.events[i].toolCallID != "call-1" || rec.events[i].stream != StreamStdout || rec.events[i].text != want {
			t.Fatalf("event %d mismatch: %+v (want %s)", i, rec.events[i], want)
		}
	}
}

func TestExecTool_StreamsStderrSeparately(t *testing.T) {
	root := t.TempDir()
	tl := NewExecTool(root)

	rec := &recordingEmitter{}
	streamer := BindLineEmitter(rec, "call-2")
	ctx := WithToolOutputStreamer(context.Background(), streamer)

	// `ls` against a non-existent path reliably writes to stderr without
	// needing shell quoting (which strings.Fields would mangle).
	result, err := tl.Execute(ctx, json.RawMessage(`{"command":"ls /nonexistent-path-for-test","timeout_ms":2000}`))
	if err != nil {
		t.Fatalf("execute exec tool: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected non-zero exit, got %s", result.Text())
	}
	var sawStderr bool
	for _, ev := range rec.events {
		if ev.stream == StreamStderr && ev.text != "" {
			sawStderr = true
		}
	}
	if !sawStderr {
		t.Fatalf("expected at least one stderr event, got %+v", rec.events)
	}
}

// TestExecTool_StreamsCaptureFastExitUnderContention reproduces the CI
// failure mode where cmd.Wait raced ahead of the scanner goroutines on a
// CPU-saturated host (the scheduler hadn't yet picked up the goroutines,
// Wait closed the pipe read fds, and the scanners hit EOF immediately).
//
// Forcing GOMAXPROCS=1 plus a tiny scheduling delay on the scanner side
// reliably triggered the original bug; with the drain-then-reap order it
// stays green even under that pressure.
func TestExecTool_StreamsCaptureFastExitUnderContention(t *testing.T) {
	prev := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(prev) })

	root := t.TempDir()
	tl := NewExecTool(root)

	const iterations = 25
	for i := 0; i < iterations; i++ {
		rec := &recordingEmitter{}
		streamer := BindLineEmitter(rec, "stress")
		ctx := WithToolOutputStreamer(context.Background(), streamer)
		result, err := tl.Execute(ctx, json.RawMessage(`{"command":"printf line-a\\nline-b\\nline-c\\n","timeout_ms":2000}`))
		if err != nil {
			t.Fatalf("iteration %d execute: %v", i, err)
		}
		if result.IsError {
			t.Fatalf("iteration %d unexpected error: %s", i, result.Text())
		}
		if len(rec.events) != 3 {
			t.Fatalf("iteration %d: expected 3 streamed lines, got %d (events=%+v)", i, len(rec.events), rec.events)
		}
	}
}

func TestExecTool_EmptyCommandIncludesHint(t *testing.T) {
	root := t.TempDir()
	tl := NewExecTool(root)

	result, err := tl.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute exec tool: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected missing command error, got %s", result.Text())
	}

	var body execResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !strings.Contains(body.Message, "command is required") {
		t.Fatalf("expected required command message, got %q", body.Message)
	}
	if !strings.Contains(body.Message, "\"command\":\"pwd\"") {
		t.Fatalf("expected command hint in error message, got %q", body.Message)
	}
}
