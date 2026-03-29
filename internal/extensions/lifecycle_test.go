package extensions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/plugin"
)

func TestRunLifecycleHooks_OnStart(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "started.txt")

	plugins := []plugin.Definition{
		{
			ID:      "test-plugin",
			RootDir: dir,
			Lifecycle: &plugin.Lifecycle{
				OnStart: "echo hello > " + marker,
			},
		},
	}

	diags := runLifecycleHooks(context.Background(), plugins, "on_start", 5*time.Second)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("marker file not created: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestRunLifecycleHooks_Timeout(t *testing.T) {
	dir := t.TempDir()
	plugins := []plugin.Definition{
		{
			ID:      "slow-plugin",
			RootDir: dir,
			Lifecycle: &plugin.Lifecycle{
				OnStart: "sleep 60",
			},
		},
	}

	diags := runLifecycleHooks(context.Background(), plugins, "on_start", 100*time.Millisecond)
	if len(diags) == 0 {
		t.Fatal("expected timeout diagnostic")
	}
	if !strings.Contains(diags[0], "slow-plugin") {
		t.Fatalf("expected diagnostic to mention plugin id, got %q", diags[0])
	}
}

func TestRunLifecycleHooks_NoHook(t *testing.T) {
	plugins := []plugin.Definition{
		{ID: "no-hooks", RootDir: t.TempDir()},
		{ID: "nil-lifecycle", RootDir: t.TempDir(), Lifecycle: &plugin.Lifecycle{}},
	}

	diags := runLifecycleHooks(context.Background(), plugins, "on_start", 5*time.Second)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics for plugins without hooks: %v", diags)
	}
}
