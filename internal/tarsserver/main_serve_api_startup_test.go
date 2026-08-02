package tarsserver

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/embodiment"
	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func TestStartBackgrounds_LogsStartupStepDurations(t *testing.T) {
	workspace := t.TempDir()
	manager, err := extensions.NewManager(extensions.Options{
		WorkspaceDir: workspace,
	})
	if err != nil {
		t.Fatalf("new extensions manager: %v", err)
	}

	var logs bytes.Buffer
	logger := zerolog.New(&logs).Level(zerolog.DebugLevel)
	runtime := &serveAPIRuntime{
		cfg: config.Config{
			RuntimeConfig: config.RuntimeConfig{
				WorkspaceDir: workspace,
			},
		},
		extensionsManager: manager,
	}

	if err := startBackgrounds(context.Background(), runtime, logger); err != nil {
		t.Fatalf("start backgrounds: %v", err)
	}

	content := logs.String()
	for _, want := range []string{
		`"step":"start_backgrounds"`,
		`"step":"remote_access_reconcile"`,
		`"step":"extensions_manager"`,
		`"duration_ms":`,
		`"message":"background startup step started"`,
		`"message":"background startup step completed"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected log %s in:\n%s", want, content)
		}
	}
}

func TestStartBackgrounds_EmbodimentSubsystemLifecycle(t *testing.T) {
	cfg := config.Config{
		Embodiment: config.EmbodimentConfig{
			Enabled: true,
			Providers: []config.EmbodimentProviderConfig{{
				Name:         "host",
				Enabled:      true,
				Transport:    "webhook",
				Capabilities: []string{"hearing"},
			}},
		},
	}
	subsystem := embodiment.New(cfg.Embodiment, zerolog.New(io.Discard))
	runtime := &serveAPIRuntime{
		cfg:                 cfg,
		embodimentSubsystem: subsystem,
	}

	if err := startBackgrounds(context.Background(), runtime, zerolog.New(io.Discard)); err != nil {
		t.Fatalf("start backgrounds: %v", err)
	}
	if status := subsystem.Status(); !status.Enabled || len(status.Providers) != 1 {
		t.Fatalf("expected started embodiment status, got %+v", status)
	}

	shutdownRuntime(context.Background(), runtime)
}

func TestShutdownRuntimeClosesWorkLedger(t *testing.T) {
	t.Parallel()

	store, err := workstore.Open(context.Background(), filepath.Join(t.TempDir(), "work-ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatalf("open work ledger: %v", err)
	}
	runtime := &serveAPIRuntime{workLedger: store}

	shutdownRuntime(context.Background(), runtime)

	if _, err := store.ListWorks(context.Background(), workstore.ListWorksFilter{WorkspaceID: defaultWorkspaceID}); err == nil {
		t.Fatal("expected closed work ledger to reject reads")
	}
}
