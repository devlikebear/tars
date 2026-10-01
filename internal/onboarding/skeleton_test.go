package onboarding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

func TestSkeletonConfigLoadsAsLocalOnlySetup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config", "config.yaml")
	workspace := filepath.Join(dir, "workspace")
	if err := WriteSkeletonConfig(path, workspace, "127.0.0.1:43180"); err != nil {
		t.Fatalf("write skeleton: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load skeleton: %v", err)
	}
	if !config.NeedsSetup(cfg) {
		t.Fatal("skeleton must leave the wizard to fill in llm providers and tiers")
	}
	// The wizard's first PATCH goes to /v1/admin/*: without these, a fresh
	// install has no token to send and every save is a 401.
	if cfg.APIAuthMode != "off" || !cfg.APIAllowInsecureLocalAuth {
		t.Fatalf("auth mode = %q, allow insecure local = %v", cfg.APIAuthMode, cfg.APIAllowInsecureLocalAuth)
	}
	if cfg.WorkspaceDir != workspace {
		t.Fatalf("workspace = %q, want %q", cfg.WorkspaceDir, workspace)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "127.0.0.1:43180") {
		t.Fatalf("skeleton should record the listen address for operators:\n%s", data)
	}
}

func TestWriteSkeletonConfigReportsUnwritablePaths(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteSkeletonConfig(filepath.Join(blocker, "config", "config.yaml"), "/ws", "127.0.0.1:43180"); err == nil {
		t.Fatal("a config dir under a file must fail")
	}
	dir := t.TempDir()
	if err := WriteSkeletonConfig(dir, "/ws", "127.0.0.1:43180"); err == nil {
		t.Fatal("writing over a directory must fail")
	}
}
