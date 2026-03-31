package heartbeat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/memory"
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

func TestRunOnceWithLLMResultWithPolicy_SkipsOutsideActiveHours(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "HEARTBEAT.md"), []byte("active-hours"), 0o644); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	called := false
	result, err := RunOnceWithLLMResultWithPolicy(
		context.Background(),
		root,
		time.Date(2026, 2, 16, 20, 0, 0, 0, time.UTC),
		func(_ context.Context, _ string) (string, error) {
			called = true
			return "should not run", nil
		},
		Policy{
			ActiveHours: "09:00-18:00",
			Timezone:    "UTC",
		},
	)
	if err != nil {
		t.Fatalf("run once with policy: %v", err)
	}
	if !result.Skipped {
		t.Fatalf("expected skipped result outside active hours")
	}
	if called {
		t.Fatalf("expected ask not called when outside active hours")
	}
}

func TestRunOnceWithLLMResultWithPolicy_QueueGateSkips(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "HEARTBEAT.md"), []byte("queue-gate"), 0o644); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	called := false
	result, err := RunOnceWithLLMResultWithPolicy(
		context.Background(),
		root,
		time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC),
		func(_ context.Context, _ string) (string, error) {
			called = true
			return "should not run", nil
		},
		Policy{
			ShouldRun: func(context.Context, time.Time) (bool, string) {
				return false, "busy"
			},
		},
	)
	if err != nil {
		t.Fatalf("run once with policy: %v", err)
	}
	if !result.Skipped {
		t.Fatalf("expected skipped result when queue gate blocks run")
	}
	if result.SkipReason != "busy" {
		t.Fatalf("expected skip reason busy, got %q", result.SkipReason)
	}
	if called {
		t.Fatalf("expected ask not called when queue gate blocks run")
	}
}

func TestRunOnceWithLLMResultWithPolicy_HeartbeatOKSuppressesDailyLog(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "HEARTBEAT.md"), []byte("ack-test"), 0o644); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	now := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
	result, err := RunOnceWithLLMResultWithPolicy(
		context.Background(),
		root,
		now,
		func(_ context.Context, _ string) (string, error) {
			return "HEARTBEAT_OK", nil
		},
		Policy{},
	)
	if err != nil {
		t.Fatalf("run once with policy: %v", err)
	}
	if !result.Acknowledged {
		t.Fatalf("expected heartbeat response to be treated as acknowledgement")
	}
	if result.Logged {
		t.Fatalf("expected ack response to skip daily log append")
	}

	data, err := os.ReadFile(filepath.Join(root, "memory", "2026-02-16.md"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read daily log: %v", err)
	}
	if os.IsNotExist(err) {
		return
	}
	content := string(data)
	if strings.Contains(content, "heartbeat tick") || strings.Contains(content, "heartbeat llm response") {
		t.Fatalf("expected heartbeat logs to be suppressed on ack, got %q", content)
	}
}

func TestRunOnceWithLLMResultWithPolicy_IncludesSessionContextAndAppendsTurn(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "HEARTBEAT.md"), []byte("bridge"), 0o644); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	var appended TurnRecord
	result, err := RunOnceWithLLMResultWithPolicy(
		context.Background(),
		root,
		time.Date(2026, 2, 16, 11, 0, 0, 0, time.UTC),
		func(_ context.Context, prompt string) (string, error) {
			if !strings.Contains(prompt, "MAIN_SESSION_CONTEXT:") {
				t.Fatalf("expected main session context in prompt, got %q", prompt)
			}
			if !strings.Contains(prompt, "last task: ship cron reliability") {
				t.Fatalf("expected session context payload in prompt, got %q", prompt)
			}
			return "next action", nil
		},
		Policy{
			LoadSessionContext: func(context.Context, time.Time) (string, error) {
				return "last task: ship cron reliability", nil
			},
			AppendSessionTurn: func(_ context.Context, turn TurnRecord) error {
				appended = turn
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("run once with policy: %v", err)
	}
	if result.Skipped {
		t.Fatalf("did not expect skipped result")
	}
	if appended.Response != "next action" {
		t.Fatalf("expected session turn append with response, got %+v", appended)
	}
}

func TestParseClockMinutes_AutoCorrects(t *testing.T) {
	cases := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"09:00", 9 * 60, false},
		{"23:59", 23*60 + 59, false},
		{"00:00", 0, false},
		{"24:00", 23*60 + 59, false},  // auto-correct
		{"0:00", 0, false},            // auto-correct
		{"25:30", 23*60 + 30, false},  // clamp hour
		{"12:99", 12*60 + 59, false},  // clamp minute
		{"abc", 0, true},
	}
	for _, tc := range cases {
		got, err := parseClockMinutes(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseClockMinutes(%q) expected error", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseClockMinutes(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseClockMinutes(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}
