package tarsserver

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/extensions"
	"github.com/devlikebear/tarsncase/internal/plugin"
	"github.com/devlikebear/tarsncase/internal/skill"
)

func buildExtensionsManager(cfg config.Config, runtime extensions.MPRuntime) (*extensions.Manager, error) {
	manager, err := extensions.NewManager(extensions.Options{
		WorkspaceDir:   cfg.WorkspaceDir,
		SkillsEnabled:  cfg.SkillsEnabled,
		PluginsEnabled: cfg.PluginsEnabled,
		SkillSources:   buildSkillSources(cfg),
		PluginSources:  buildPluginSources(cfg),
		MCPBaseServers: append([]config.MCPServer(nil), cfg.MCPServers...),
		MCPRuntime:     runtime,
		WatchSkills:    cfg.SkillsWatch,
		WatchPlugins:   cfg.PluginsWatch,
		WatchDebounce:  resolveExtensionsWatchDebounce(cfg),
	})
	if err != nil {
		return nil, err
	}
	return manager, nil
}

func buildSkillSources(cfg config.Config) []skill.SourceDir {
	out := make([]skill.SourceDir, 0)
	appendSource := func(source skill.Source, path string) {
		trimmed := strings.TrimSpace(os.ExpandEnv(path))
		if trimmed == "" {
			return
		}
		out = append(out, skill.SourceDir{Source: source, Dir: trimmed})
	}

	appendSource(skill.SourceBundled, cfg.SkillsBundledDir)
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		appendSource(skill.SourceUser, filepath.Join(home, ".tarsncase", "skills"))
	}
	for _, extra := range cfg.SkillsExtraDirs {
		appendSource(skill.SourceUser, extra)
	}
	appendSource(skill.SourceWorkspace, filepath.Join(cfg.WorkspaceDir, "skills"))
	return out
}

func buildPluginSources(cfg config.Config) []extensions.PluginSourceDir {
	out := make([]extensions.PluginSourceDir, 0)
	appendSource := func(source plugin.Source, path string) {
		trimmed := strings.TrimSpace(os.ExpandEnv(path))
		if trimmed == "" {
			return
		}
		out = append(out, extensions.PluginSourceDir{Source: source, Dir: trimmed})
	}

	appendSource(plugin.SourceBundled, cfg.PluginsBundledDir)
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		appendSource(plugin.SourceUser, filepath.Join(home, ".tarsncase", "plugins"))
	}
	for _, extra := range cfg.PluginsExtraDirs {
		appendSource(plugin.SourceUser, extra)
	}
	appendSource(plugin.SourceWorkspace, filepath.Join(cfg.WorkspaceDir, "plugins"))
	return out
}

func resolveExtensionsWatchDebounce(cfg config.Config) time.Duration {
	values := []int{cfg.SkillsWatchDebounceMS, cfg.PluginsWatchDebounceMS}
	min := 0
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if min == 0 || value < min {
			min = value
		}
	}
	if min <= 0 {
		min = 200
	}
	return time.Duration(min) * time.Millisecond
}
