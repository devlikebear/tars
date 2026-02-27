package assistant

import (
	"context"
	"strings"
	"testing"
)

func TestResolveRuntimeMode_FallsBackWhenHotkeyUnavailable(t *testing.T) {
	mode, warning := resolveRuntimeMode(nil, "Ctrl+Option+Space", "hotkey unavailable")
	if mode != runtimeModeFallback {
		t.Fatalf("expected fallback mode, got %q", mode)
	}
	if !strings.Contains(strings.ToLower(warning), "fallback") {
		t.Fatalf("expected fallback warning, got %q", warning)
	}
}

func TestResolveRuntimeMode_UsesHotkeyWhenAvailable(t *testing.T) {
	listener := &noopHotkeyListener{}
	mode, warning := resolveRuntimeMode(listener, "Ctrl+Option+Space", "")
	if mode != runtimeModeHotkey {
		t.Fatalf("expected hotkey mode, got %q", mode)
	}
	if strings.TrimSpace(warning) != "" {
		t.Fatalf("expected empty warning, got %q", warning)
	}
}

type noopHotkeyListener struct{}

func (n *noopHotkeyListener) WaitPress(context.Context) error   { return nil }
func (n *noopHotkeyListener) WaitRelease(context.Context) error { return nil }
func (n *noopHotkeyListener) Close() error                      { return nil }
