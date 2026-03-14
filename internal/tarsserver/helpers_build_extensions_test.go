package tarsserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/assetpath"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/plugin"
	"github.com/devlikebear/tars/internal/skill"
)

func TestBuildSkillSources_UsesPrimaryAndLegacyUserDirs(t *testing.T) {
	home := t.TempDir()
	prevHome, hadHome := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	defer func() {
		if hadHome {
			_ = os.Setenv("HOME", prevHome)
			return
		}
		_ = os.Unsetenv("HOME")
	}()

	cfg := config.Config{WorkspaceDir: filepath.Join(home, "workspace")}
	got := buildSkillSources(cfg)

	want := []skill.SourceDir{
		{Source: skill.SourceUser, Dir: filepath.Join(home, ".tarsncase", "skills")},
		{Source: skill.SourceUser, Dir: filepath.Join(home, ".tars", "skills")},
		{Source: skill.SourceWorkspace, Dir: filepath.Join(cfg.WorkspaceDir, "skills")},
	}
	assertSkillSourcesContainInOrder(t, got, want)
}

func TestBuildPluginSources_UsesPrimaryAndLegacyUserDirs(t *testing.T) {
	home := t.TempDir()
	prevHome, hadHome := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	defer func() {
		if hadHome {
			_ = os.Setenv("HOME", prevHome)
			return
		}
		_ = os.Unsetenv("HOME")
	}()

	cfg := config.Config{WorkspaceDir: filepath.Join(home, "workspace")}
	got := buildPluginSources(cfg)

	want := []extensions.PluginSourceDir{
		{Source: plugin.SourceUser, Dir: filepath.Join(home, ".tarsncase", "plugins")},
		{Source: plugin.SourceUser, Dir: filepath.Join(home, ".tars", "plugins")},
		{Source: plugin.SourceWorkspace, Dir: filepath.Join(cfg.WorkspaceDir, "plugins")},
	}
	assertPluginSourcesContainInOrder(t, got, want)
}

func TestBuildPluginSources_ResolvesBundledDirRelativeToExecutable(t *testing.T) {
	root := t.TempDir()
	bundledDir := filepath.Join(root, "share", "tars", "plugins")
	if err := os.MkdirAll(bundledDir, 0o755); err != nil {
		t.Fatalf("mkdir bundled dir: %v", err)
	}

	previous := assetpath.ExecutablePathFunc
	assetpath.ExecutablePathFunc = func() (string, error) {
		return filepath.Join(root, "tars"), nil
	}
	defer func() { assetpath.ExecutablePathFunc = previous }()

	cfg := config.Config{
		WorkspaceDir:      filepath.Join(root, "workspace"),
		PluginsBundledDir: "./plugins",
		PluginsExtraDirs:  []string{},
	}
	got := buildPluginSources(cfg)
	if len(got) == 0 || got[0].Dir != bundledDir {
		t.Fatalf("expected bundled plugin source %q, got %+v", bundledDir, got)
	}
}

func assertSkillSourcesContainInOrder(t *testing.T, got []skill.SourceDir, want []skill.SourceDir) {
	t.Helper()
	idx := 0
	for _, source := range got {
		if idx >= len(want) {
			break
		}
		if source == want[idx] {
			idx++
		}
	}
	if idx != len(want) {
		t.Fatalf("missing ordered skill sources: want=%+v got=%+v", want, got)
	}
}

func assertPluginSourcesContainInOrder(t *testing.T, got []extensions.PluginSourceDir, want []extensions.PluginSourceDir) {
	t.Helper()
	idx := 0
	for _, source := range got {
		if idx >= len(want) {
			break
		}
		if source == want[idx] {
			idx++
		}
	}
	if idx != len(want) {
		t.Fatalf("missing ordered plugin sources: want=%+v got=%+v", want, got)
	}
}
