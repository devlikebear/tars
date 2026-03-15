package gateway

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSanitizeMetadataEnvValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{name: "accepts normal identifier", input: "run_123", want: "run_123", wantOK: true},
		{name: "rejects newline", input: "run_123\nINJECT=1", want: "", wantOK: false},
		{name: "rejects null byte", input: "session\x00abc", want: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := sanitizeMetadataEnvValue(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("expected ok=%v, got %v", tt.wantOK, ok)
			}
			if got != tt.want {
				t.Fatalf("expected value %q, got %q", tt.want, got)
			}
		})
	}
}

func TestNewCommandExecutor_ValidateRequiredFields(t *testing.T) {
	if _, err := NewCommandExecutor(CommandExecutorOptions{
		Command: "sh",
		Args:    []string{"-c", "echo ok"},
	}); err == nil {
		t.Fatalf("expected name validation error")
	}
	if _, err := NewCommandExecutor(CommandExecutorOptions{
		Name: "worker",
	}); err == nil {
		t.Fatalf("expected command validation error")
	}
}

func TestCommandExecutor_Execute_StdinAndMetadata(t *testing.T) {
	executor, err := NewCommandExecutor(CommandExecutorOptions{
		Name:    "worker",
		Command: "sh",
		Args:    []string{"-c", `input=$(cat); printf "%s|%s|%s" "$TARS_RUN_ID" "$TARS_SESSION_ID" "$input"`},
	})
	if err != nil {
		t.Fatalf("new command executor: %v", err)
	}

	out, err := executor.Execute(context.Background(), ExecuteRequest{
		RunID:     "run_123",
		SessionID: "session_abc",
		Prompt:    "hello from prompt",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "run_123|session_abc|hello from prompt" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCommandExecutor_Execute_SkipsUnsafeMetadataEnvValues(t *testing.T) {
	executor, err := NewCommandExecutor(CommandExecutorOptions{
		Name:    "worker",
		Command: "sh",
		Args:    []string{"-c", `printf "%s|%s|%s" "${TARS_RUN_ID:-missing}" "${TARS_SESSION_ID:-missing}" "${TARS_WORKSPACE_ID:-missing}"`},
	})
	if err != nil {
		t.Fatalf("new command executor: %v", err)
	}

	out, err := executor.Execute(context.Background(), ExecuteRequest{
		RunID:       "run_123\nINJECT=1",
		SessionID:   "session_abc\r\nINJECT=1",
		WorkspaceID: "workspace\x00abc",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "missing|missing|missing" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCommandExecutor_Execute_Timeout(t *testing.T) {
	executor, err := NewCommandExecutor(CommandExecutorOptions{
		Name:    "slow-worker",
		Command: "sh",
		Args:    []string{"-c", "sleep 1; echo done"},
		Timeout: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new command executor: %v", err)
	}

	_, err = executor.Execute(context.Background(), ExecuteRequest{Prompt: "hello"})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	lower := strings.ToLower(err.Error())
	if !strings.Contains(lower, "deadline") && !strings.Contains(lower, "timeout") && !strings.Contains(lower, "killed") {
		t.Fatalf("expected timeout/deadline error, got %v", err)
	}
}

func TestCommandExecutor_InfoIncludesEntry(t *testing.T) {
	executor, err := NewCommandExecutor(CommandExecutorOptions{
		Name:    "worker",
		Command: "sh",
		Args:    []string{"-c", "echo ok"},
	})
	if err != nil {
		t.Fatalf("new command executor: %v", err)
	}
	info := executor.Info()
	if strings.TrimSpace(info.Entry) == "" {
		t.Fatalf("expected non-empty entry in command executor info")
	}
}

func TestCommandExecutor_Execute_StderrOnFailure(t *testing.T) {
	executor, err := NewCommandExecutor(CommandExecutorOptions{
		Name:    "failing-worker",
		Command: "sh",
		Args:    []string{"-c", "echo boom 1>&2; exit 3"},
	})
	if err != nil {
		t.Fatalf("new command executor: %v", err)
	}

	_, err = executor.Execute(context.Background(), ExecuteRequest{Prompt: "hello"})
	if err == nil {
		t.Fatalf("expected execution error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected stderr in error, got %v", err)
	}
}
