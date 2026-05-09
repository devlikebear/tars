package tarsserver

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/extensions"
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
