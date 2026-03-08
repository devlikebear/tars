package tarsserver

import (
	"testing"

	"github.com/devlikebear/tarsncase/internal/config"
)

func TestBuildBrowserService_UsesRuntimeConfig(t *testing.T) {
	cfg := config.Default()
	cfg.BrowserRuntimeEnabled = true
	cfg.WorkspaceDir = t.TempDir()

	service := buildBrowserService(cfg, nil, nil)
	if service == nil {
		t.Fatal("expected browser service")
	}
}
