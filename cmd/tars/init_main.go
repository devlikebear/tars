package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devlikebear/tars/internal/assetpath"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/plugin"
	"github.com/spf13/cobra"
)

type initOptions struct {
	workspaceDir string
}

var initRunner = runInitCommand

func defaultInitOptions() initOptions {
	return initOptions{
		workspaceDir: defaultWorkspaceDir(),
	}
}

func newInitCommand(stdout, stderr io.Writer) *cobra.Command {
	opts := defaultInitOptions()
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a starter workspace and config",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return initRunner(cmd.Context(), opts, stdout, stderr)
		},
	}
	cmd.Flags().StringVar(&opts.workspaceDir, "workspace-dir", opts.workspaceDir, "workspace directory")
	return cmd
}

func runInitCommand(_ context.Context, opts initOptions, stdout, _ io.Writer) error {
	workspaceAbs, err := resolveWorkspaceDir(opts.workspaceDir)
	if err != nil {
		return fmt.Errorf("resolve workspace dir: %w", err)
	}
	configPath := starterConfigPath(workspaceAbs)

	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config already exists: %s", configPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat config path %s: %w", configPath, err)
	}

	if err := ensureStarterWorkspaceLayout(workspaceAbs, defaultStarterBundledPluginsDir()); err != nil {
		return err
	}
	if err := writeStarterConfigFile(workspaceAbs, configPath); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stdout, "initialized TARS workspace\nworkspace: %s\nconfig: %s\n\n", workspaceAbs, configPath)
	_, _ = fmt.Fprintf(stdout, "BYOK:\n")
	_, _ = fmt.Fprintf(stdout, "  default starter provider: openai\n")
	_, _ = fmt.Fprintf(stdout, "  export OPENAI_API_KEY='your-api-key'\n")
	_, _ = fmt.Fprintf(stdout, "  or set llm_provider: claude-code-cli in %s to use the local Claude Code CLI\n", configPath)
	_, _ = fmt.Fprintf(stdout, "  or edit llm_provider / llm_api_key in %s for anthropic or gemini\n\n", configPath)
	_, _ = fmt.Fprintf(stdout, "Next:\n")
	_, _ = fmt.Fprintf(stdout, "  tars serve --config %s\n", configPath)
	_, _ = fmt.Fprintf(stdout, "  tars\n")
	return nil
}

func defaultWorkspaceDir() string {
	return strings.TrimSpace(firstNonEmpty(os.Getenv("TARS_WORKSPACE_DIR"), "./workspace"))
}

func resolveWorkspaceDir(raw string) (string, error) {
	workspaceDir := strings.TrimSpace(raw)
	if workspaceDir == "" {
		workspaceDir = "./workspace"
	}
	return filepath.Abs(workspaceDir)
}

func starterConfigPath(workspaceAbs string) string {
	return filepath.Join(workspaceAbs, "config", "tars.config.yaml")
}

func resolveConfigPath(raw, workspaceAbs string) (string, error) {
	configPath := strings.TrimSpace(raw)
	if configPath == "" {
		configPath = starterConfigPath(workspaceAbs)
	}
	configPath = os.ExpandEnv(configPath)
	if filepath.IsAbs(configPath) {
		return configPath, nil
	}
	return filepath.Abs(configPath)
}

func ensureStarterWorkspaceLayout(workspaceAbs string, bundledPluginsDir string) error {
	if err := memory.EnsureWorkspace(workspaceAbs); err != nil {
		return fmt.Errorf("ensure workspace: %w", err)
	}
	if _, err := installStarterWorkspacePlugins(workspaceAbs, bundledPluginsDir); err != nil {
		return fmt.Errorf("install bundled workspace plugins: %w", err)
	}
	return nil
}

func defaultStarterBundledPluginsDir() string {
	return strings.TrimSpace(firstNonEmpty(os.Getenv("TARS_PLUGINS_BUNDLED_DIR"), config.Default().PluginsBundledDir))
}

func installStarterWorkspacePlugins(workspaceAbs string, bundledPluginsDir string) ([]string, error) {
	resolvedDir, ok := assetpath.ResolveExistingDir(bundledPluginsDir)
	if !ok {
		return nil, fmt.Errorf("bundled plugins dir not found: %s", strings.TrimSpace(bundledPluginsDir))
	}

	snapshot, err := plugin.Load(plugin.LoadOptions{
		Sources: []plugin.SourceDir{{Source: plugin.SourceBundled, Dir: resolvedDir}},
	})
	if err != nil {
		return nil, fmt.Errorf("load bundled plugins: %w", err)
	}

	workspacePluginsDir := filepath.Join(workspaceAbs, "plugins")
	if err := os.MkdirAll(workspacePluginsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace plugins dir: %w", err)
	}

	installed := make([]string, 0, len(snapshot.Plugins))
	for _, def := range snapshot.Plugins {
		dstRoot := filepath.Join(workspacePluginsDir, filepath.Base(def.RootDir))
		manifestPath := filepath.Join(dstRoot, filepath.Base(def.ManifestPath))
		manifestExists, err := pathExists(manifestPath)
		if err != nil {
			return installed, fmt.Errorf("stat workspace plugin manifest %s: %w", manifestPath, err)
		}
		if err := copyDirMissing(def.RootDir, dstRoot); err != nil {
			return installed, fmt.Errorf("copy bundled plugin %s: %w", def.ID, err)
		}
		if !manifestExists {
			installed = append(installed, strings.TrimSpace(def.ID))
		}
	}
	sort.Strings(installed)
	return installed, nil
}

func bundledWorkspacePluginManifestPaths(workspaceAbs string, bundledPluginsDir string) []string {
	resolvedDir, ok := assetpath.ResolveExistingDir(bundledPluginsDir)
	if !ok {
		return nil
	}
	snapshot, err := plugin.Load(plugin.LoadOptions{
		Sources: []plugin.SourceDir{{Source: plugin.SourceBundled, Dir: resolvedDir}},
	})
	if err != nil {
		return nil
	}

	paths := make([]string, 0, len(snapshot.Plugins))
	for _, def := range snapshot.Plugins {
		paths = append(paths, filepath.Join(workspaceAbs, "plugins", filepath.Base(def.RootDir), filepath.Base(def.ManifestPath)))
	}
	sort.Strings(paths)
	return paths
}

func copyDirMissing(srcRoot, dstRoot string) error {
	return filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		target := dstRoot
		if rel != "." {
			target = filepath.Join(dstRoot, rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if exists, err := pathExists(target); err != nil {
			return err
		} else if exists {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func writeStarterConfigFile(workspaceAbs, configPath string) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(renderStarterConfig(workspaceAbs)), 0o644); err != nil {
		return fmt.Errorf("write starter config: %w", err)
	}
	return nil
}

func renderStarterConfig(workspaceDir string) string {
	return strings.TrimSpace(fmt.Sprintf(`
# TARS starter config generated by "tars init"
# This file is intentionally minimal for a first local setup.

mode: standalone
workspace_dir: %s

# Local-only starter auth. Change to "required" before exposing beyond localhost.
api_auth_mode: off
api_allow_insecure_local_auth: true

# BYOK starter provider.
# Other common choices:
# - anthropic -> ${ANTHROPIC_API_KEY}
# - gemini -> ${GEMINI_API_KEY}
# - claude-code-cli -> local Claude Code install, no API key required
llm_provider: openai
llm_auth_mode: api-key
llm_base_url: https://api.openai.com/v1
llm_model: gpt-4o-mini
llm_api_key: ${OPENAI_API_KEY}

# Gateway is enabled so bundled project workflows can dispatch local agents.
gateway_enabled: true
`+"\n", workspaceDir))
}
