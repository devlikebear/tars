package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/execsandbox"
)

func stubDetectSandbox(t *testing.T, detect func() (execsandbox.Backend, error)) {
	t.Helper()
	previous := detectSandbox
	detectSandbox = detect
	t.Cleanup(func() { detectSandbox = previous })
}

func decodeExec(t *testing.T, result Result) execResponse {
	t.Helper()
	var body execResponse
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode %s: %v", result.Text(), err)
	}
	return body
}

func TestExecSandboxOffLeavesTheToolUnchanged(t *testing.T) {
	root := t.TempDir()
	plain := NewExecToolWithOptions(SingleDirPolicy(root), nil, ExecToolOptions{})
	if strings.Contains(plain.Description, "sandbox") {
		t.Fatalf("description mentions a sandbox that is off: %s", plain.Description)
	}
	stubDetectSandbox(t, func() (execsandbox.Backend, error) {
		t.Fatal("an exec tool without a sandbox must not look for one")
		return execsandbox.Backend{}, nil
	})
	result, err := plain.Execute(context.Background(), json.RawMessage(`{"command":"echo hello"}`))
	if err != nil || result.IsError {
		t.Fatalf("plain exec failed: %v %s", err, result.Text())
	}
}

func TestExecSandboxDescription(t *testing.T) {
	root := t.TempDir()
	blocked := NewExecToolWithOptions(SingleDirPolicy(root), nil, ExecToolOptions{Sandbox: &ExecSandbox{}})
	if !strings.Contains(blocked.Description, "OS sandbox") || !strings.Contains(blocked.Description, "network access other than localhost is blocked") {
		t.Fatalf("description = %s", blocked.Description)
	}
	open := NewExecToolWithOptions(SingleDirPolicy(root), nil, ExecToolOptions{Sandbox: &ExecSandbox{Network: true}})
	if !strings.Contains(open.Description, "OS sandbox") || strings.Contains(open.Description, "network") {
		t.Fatalf("description = %s", open.Description)
	}
}

// With the sandbox on and no backend on the host, the command must be
// refused — foreground and background — and nothing may run.
func TestExecSandboxFailsClosed(t *testing.T) {
	stubDetectSandbox(t, func() (execsandbox.Backend, error) {
		return execsandbox.Backend{}, fmt.Errorf("%w: bwrap not found", execsandbox.ErrUnavailable)
	})
	root := t.TempDir()
	tool := NewExecToolWithOptions(SingleDirPolicy(root), NewProcessManager(), ExecToolOptions{Sandbox: &ExecSandbox{}})

	for _, background := range []bool{false, true} {
		t.Run(fmt.Sprintf("background=%v", background), func(t *testing.T) {
			marker := filepath.Join(root, fmt.Sprintf("ran-%v", background))
			params, _ := json.Marshal(map[string]any{"command": "echo x > " + filepath.Base(marker), "background": background})
			result, err := tool.Execute(context.Background(), params)
			if err != nil {
				t.Fatal(err)
			}
			body := decodeExec(t, result)
			if !result.IsError || !strings.Contains(body.Message, "bwrap not found") || !strings.Contains(body.Message, "the command was not run") {
				t.Fatalf("result = %s", result.Text())
			}
			time.Sleep(50 * time.Millisecond)
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("the command ran: %v", err)
			}
		})
	}
}

func liveSandboxOrSkip(t *testing.T) {
	t.Helper()
	backend, err := execsandbox.Detect()
	if err != nil {
		t.Skipf("no sandbox on this host: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := backend.Probe(ctx); err != nil {
		t.Skipf("%s cannot start a sandbox here: %v", backend.Name, err)
	}
}

func TestExecSandboxLive(t *testing.T) {
	liveSandboxOrSkip(t)

	root := t.TempDir()
	outside, err := os.MkdirTemp(".", ".sandbox-outside-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(outside) })
	outside, _ = filepath.Abs(outside)

	manager := NewProcessManager()
	tool := NewExecToolWithOptions(SingleDirPolicy(root), manager, ExecToolOptions{Sandbox: &ExecSandbox{}})
	run := func(command string, background bool) execResponse {
		t.Helper()
		params, _ := json.Marshal(map[string]any{"command": command, "background": background, "timeout_ms": 20000})
		result, err := tool.Execute(context.Background(), params)
		if err != nil {
			t.Fatal(err)
		}
		body := decodeExec(t, result)
		if !background {
			return body
		}
		snap, _, err := manager.Wait(context.Background(), body.SessionID, 20000)
		if err != nil {
			t.Fatal(err)
		}
		return execResponse{ExitCode: snap.ExitCode, Stderr: snap.Stderr}
	}

	for _, background := range []bool{false, true} {
		inside := run(fmt.Sprintf("echo ok > inside-%v.txt", background), background)
		if exitCodeOf(inside) != 0 {
			t.Fatalf("background=%v: write in the folder failed: %+v", background, inside)
		}
		target := filepath.Join(outside, fmt.Sprintf("f-%v.txt", background))
		escaped := run("echo leaked > '"+target+"'", background)
		if exitCodeOf(escaped) == 0 {
			t.Fatalf("background=%v: write outside the folder succeeded", background)
		}
		if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("background=%v: %s was written", background, target)
		}
	}
}
