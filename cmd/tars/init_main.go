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
	"gopkg.in/yaml.v3"
)

type initOptions struct {
	workspaceDir string
}

type initMoveOptions struct {
	to string
}

var initRunner = runInitCommand
var initMoveRunner = runInitMoveCommand

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

	moveCmd := newInitMoveCommand(stdout, stderr)
	cmd.AddCommand(moveCmd)
	return cmd
}

func newInitMoveCommand(stdout, stderr io.Writer) *cobra.Command {
	moveOpts := initMoveOptions{}
	cmd := &cobra.Command{
		Use:   "move",
		Short: "Move the workspace directory to a new location",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return initMoveRunner(cmd.Context(), moveOpts, stdout, stderr)
		},
	}
	cmd.Flags().StringVar(&moveOpts.to, "to", "", "target directory for the workspace (required)")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

func runInitCommand(_ context.Context, opts initOptions, stdout, _ io.Writer) error {
	workspaceAbs, err := resolveWorkspaceDir(opts.workspaceDir)
	if err != nil {
		return fmt.Errorf("resolve workspace dir: %w", err)
	}
	configPath := config.FixedConfigPath()

	// Check if config already exists at the fixed path.
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config already exists: %s", configPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat config path %s: %w", configPath, err)
	}

	// Try to migrate legacy config if found.
	if migrated, legacyPath := tryMigrateLegacyConfig(configPath, stdout); migrated {
		// Update workspace_dir in migrated config if it was relative.
		updateMigratedWorkspaceDir(configPath, workspaceAbs)
		_, _ = fmt.Fprintf(stdout, "migrated legacy config\n  from: %s\n  to:   %s\n\n", legacyPath, configPath)
		_, _ = fmt.Fprintf(stdout, "the original file has been kept. you can remove it manually:\n  rm %s\n\n", legacyPath)
		return nil
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
	_, _ = fmt.Fprintf(stdout, "  tars serve\n")
	_, _ = fmt.Fprintf(stdout, "  tars service install && tars service start\n")
	return nil
}

// tryMigrateLegacyConfig checks for legacy config locations and copies to the
// fixed config path. Returns true and the source path if migration occurred.
func tryMigrateLegacyConfig(fixedPath string, stdout io.Writer) (bool, string) {
	legacyCandidates := []string{
		"workspace/config/tars.config.yaml",
		"config/standalone.yaml",
	}
	for _, candidate := range legacyCandidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err != nil {
			continue
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fixedPath), 0o755); err != nil {
			_, _ = fmt.Fprintf(stdout, "warning: failed to create config dir: %v\n", err)
			return false, ""
		}
		if err := os.WriteFile(fixedPath, data, 0o644); err != nil {
			_, _ = fmt.Fprintf(stdout, "warning: failed to write migrated config: %v\n", err)
			return false, ""
		}
		return true, abs
	}
	return false, ""
}

// updateMigratedWorkspaceDir reads the migrated config and converts a relative
// workspace_dir to an absolute path.
func updateMigratedWorkspaceDir(configPath, defaultWorkspace string) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	parsed := map[string]any{}
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return
	}
	wsRaw, ok := parsed["workspace_dir"]
	if !ok {
		parsed["workspace_dir"] = defaultWorkspace
	} else if ws, ok := wsRaw.(string); ok && !filepath.IsAbs(ws) {
		abs, err := filepath.Abs(ws)
		if err != nil {
			return
		}
		parsed["workspace_dir"] = abs
	}
	out, err := yaml.Marshal(parsed)
	if err != nil {
		return
	}
	_ = os.WriteFile(configPath, out, 0o644)
}

func runInitMoveCommand(_ context.Context, opts initMoveOptions, stdout, _ io.Writer) error {
	configPath := config.FixedConfigPath()

	// Load current workspace_dir from config.
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config %s: %w", configPath, err)
	}
	currentWorkspace := cfg.WorkspaceDir
	if currentWorkspace == "" {
		return fmt.Errorf("workspace_dir not set in config %s", configPath)
	}
	currentAbs, err := filepath.Abs(currentWorkspace)
	if err != nil {
		return fmt.Errorf("resolve current workspace: %w", err)
	}

	targetAbs, err := filepath.Abs(strings.TrimSpace(opts.to))
	if err != nil {
		return fmt.Errorf("resolve target path: %w", err)
	}

	if currentAbs == targetAbs {
		return fmt.Errorf("target is the same as current workspace: %s", currentAbs)
	}

	// Verify source exists.
	if _, err := os.Stat(currentAbs); os.IsNotExist(err) {
		return fmt.Errorf("current workspace does not exist: %s", currentAbs)
	}
	// Verify target does not exist.
	if _, err := os.Stat(targetAbs); err == nil {
		return fmt.Errorf("target already exists: %s", targetAbs)
	}

	// Ensure parent dir of target exists.
	if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
		return fmt.Errorf("create target parent dir: %w", err)
	}

	// Move workspace.
	if err := os.Rename(currentAbs, targetAbs); err != nil {
		// Cross-device: copy + delete.
		if err := copyDirAll(currentAbs, targetAbs); err != nil {
			return fmt.Errorf("copy workspace: %w", err)
		}
		if err := os.RemoveAll(currentAbs); err != nil {
			_, _ = fmt.Fprintf(stdout, "warning: workspace copied but failed to remove original: %v\n", err)
		}
	}

	// Update workspace_dir in config.
	if err := config.PatchYAML(configPath, map[string]any{"workspace_dir": targetAbs}); err != nil {
		return fmt.Errorf("update config workspace_dir: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "workspace moved\n  from: %s\n  to:   %s\n  config updated: %s\n", currentAbs, targetAbs, configPath)

	// Check if LaunchAgent plist exists and advise restart.
	home, err := os.UserHomeDir()
	if err == nil {
		plistPath := filepath.Join(home, "Library", "LaunchAgents", "io.tars.server.plist")
		if _, err := os.Stat(plistPath); err == nil {
			_, _ = fmt.Fprintf(stdout, "\nLaunchAgent detected. restart the service:\n  tars service stop && tars service install && tars service start\n")
		}
	}
	return nil
}

// copyDirAll recursively copies a directory tree (used for cross-device moves).
func copyDirAll(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
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

func defaultWorkspaceDir() string {
	return strings.TrimSpace(firstNonEmpty(os.Getenv("TARS_WORKSPACE_DIR"), config.DefaultWorkspaceDir()))
}

func resolveWorkspaceDir(raw string) (string, error) {
	workspaceDir := strings.TrimSpace(raw)
	if workspaceDir == "" {
		workspaceDir = config.DefaultWorkspaceDir()
	}
	return filepath.Abs(workspaceDir)
}

func resolveConfigPath(raw, _ string) (string, error) {
	configPath := strings.TrimSpace(raw)
	if configPath == "" {
		return config.FixedConfigPath(), nil
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

# Gateway is enabled so agents can dispatch local subagents.
gateway_enabled: true

# Optional subagent limits for parallel read-only research in chat.
# gateway_subagents_max_threads: 4
# gateway_subagents_max_depth: 1
`+"\n", workspaceDir))
}
