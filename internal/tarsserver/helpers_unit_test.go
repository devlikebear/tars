package tarsserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/tool"
)

func TestResolveExtensionsWatchDebounce(t *testing.T) {
	cfg := config.Config{SkillsWatchDebounceMS: 350, PluginsWatchDebounceMS: 120}
	if got, want := resolveExtensionsWatchDebounce(cfg), 120*time.Millisecond; got != want {
		t.Fatalf("unexpected debounce: got=%s want=%s", got, want)
	}

	cfg = config.Config{}
	if got, want := resolveExtensionsWatchDebounce(cfg), 200*time.Millisecond; got != want {
		t.Fatalf("unexpected default debounce: got=%s want=%s", got, want)
	}
}

func TestEffectiveCronDeliveryMode(t *testing.T) {
	if got := effectiveCronDeliveryMode("", "main"); got != "session" {
		t.Fatalf("expected default session mode for main target, got %q", got)
	}
	if got := effectiveCronDeliveryMode("", "other"); got != "daily_log" {
		t.Fatalf("expected default daily_log mode, got %q", got)
	}
	if got := effectiveCronDeliveryMode("invalid", "main"); got != "daily_log" {
		t.Fatalf("expected invalid mode fallback to daily_log, got %q", got)
	}
}

func TestShouldPromoteToMemory(t *testing.T) {
	if !shouldPromoteToMemory("remember I like black coffee") {
		t.Fatal("expected remember prefix to trigger promotion")
	}
	if !shouldPromoteToMemory("기억해 나는 산책을 좋아해") {
		t.Fatal("expected korean remember prefix to trigger promotion")
	}
	if shouldPromoteToMemory("what time is it") {
		t.Fatal("did not expect non-memory message to trigger promotion")
	}
}

func TestNormalizeAllowedToolsForRegistry(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(tool.Tool{
		Name:       "exec",
		Parameters: json.RawMessage(`{"type":"object","properties":{}}`),
		Execute: func(_ context.Context, _ json.RawMessage) (tool.Result, error) {
			return tool.Result{}, nil
		},
	})
	registry.Register(tool.Tool{
		Name:       "read_file",
		Parameters: json.RawMessage(`{"type":"object","properties":{}}`),
		Execute: func(_ context.Context, _ json.RawMessage) (tool.Result, error) {
			return tool.Result{}, nil
		},
	})

	got := normalizeAllowedToolsForRegistry([]string{"shell_execute", "read_file", "unknown", "read_file"}, registry)
	if len(got) != 2 || got[0] != "exec" || got[1] != "read_file" {
		t.Fatalf("unexpected normalized tools: %+v", got)
	}
}
