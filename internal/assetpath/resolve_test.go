package assetpath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveExistingDir_UsesExecutableParentForRepoStyleLayouts(t *testing.T) {
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("mkdir plugins dir: %v", err)
	}

	previous := ExecutablePathFunc
	ExecutablePathFunc = func() (string, error) {
		return filepath.Join(root, "bin", "tars"), nil
	}
	defer func() { ExecutablePathFunc = previous }()

	got, ok := ResolveExistingDir("./plugins")
	if !ok {
		t.Fatal("expected repo-style plugins dir to resolve")
	}
	if got != pluginsDir {
		t.Fatalf("expected %q, got %q", pluginsDir, got)
	}
}

func TestResolveExistingDir_UsesSharePathForInstalledLayouts(t *testing.T) {
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "share", "tars", "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("mkdir plugins dir: %v", err)
	}

	previous := ExecutablePathFunc
	ExecutablePathFunc = func() (string, error) {
		return filepath.Join(root, "tars"), nil
	}
	defer func() { ExecutablePathFunc = previous }()

	got, ok := ResolveExistingDir("./plugins")
	if !ok {
		t.Fatal("expected installed share plugins dir to resolve")
	}
	if got != pluginsDir {
		t.Fatalf("expected %q, got %q", pluginsDir, got)
	}
}
