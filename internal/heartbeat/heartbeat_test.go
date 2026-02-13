package heartbeat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tarsncase/internal/memory"
)

func TestRunOnce_AppendsDailyLog(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "HEARTBEAT.md"), []byte("check tasks"), 0o644); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	now := time.Date(2026, 2, 13, 11, 0, 0, 0, time.UTC)
	if err := RunOnce(root, now); err != nil {
		t.Fatalf("run once: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "memory", "2026-02-13.md"))
	if err != nil {
		t.Fatalf("read daily log: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "heartbeat tick") {
		t.Fatalf("expected heartbeat tick in daily log: %q", content)
	}
}

func TestRunOnce_MissingHeartbeatReturnsError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	err := RunOnce(root, time.Now())
	if err == nil {
		t.Fatal("expected error for missing HEARTBEAT.md, got nil")
	}
}

func TestRunLoop_WithMaxHeartbeats(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "HEARTBEAT.md"), []byte("loop"), 0o644); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	count, err := RunLoop(t.Context(), root, 5*time.Millisecond, 2, time.Now)
	if err != nil {
		t.Fatalf("run loop: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 heartbeats, got %d", count)
	}
}

func TestRunLoop_InvalidIntervalReturnsError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "HEARTBEAT.md"), []byte("loop"), 0o644); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	_, err := RunLoop(t.Context(), root, 0, 1, time.Now)
	if err == nil {
		t.Fatal("expected error for invalid interval, got nil")
	}
}

func TestRunOnceWithLLM_AppendsResponse(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "HEARTBEAT.md"), []byte("llm-test"), 0o644); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	now := time.Date(2026, 2, 13, 11, 0, 0, 0, time.UTC)
	err := RunOnceWithLLM(context.Background(), root, now, func(_ context.Context, prompt string) (string, error) {
		if !strings.Contains(prompt, "HEARTBEAT:") {
			t.Fatalf("unexpected prompt: %q", prompt)
		}
		return "next action", nil
	})
	if err != nil {
		t.Fatalf("run once with llm: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "memory", "2026-02-13.md"))
	if err != nil {
		t.Fatalf("read daily log: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "heartbeat llm response") {
		t.Fatalf("expected llm response log in daily log: %q", content)
	}
}
