package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/assetpath"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/launchagent"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/onboarding"
	"github.com/devlikebear/tars/internal/plugin"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type initOptions struct {
	workspaceDir string
	apiAddr      string
	port         int
	noServer     bool
	noBrowser    bool
	force        bool
}

type initMoveOptions struct {
	to string
}

// initStartParams describes what the orchestrator wants the server
// starter to do. Tests swap initServerStarter for a recorder.
type initStartParams struct {
	apiAddr      string
	configPath   string
	workspaceDir string
	useService   bool
}

// initStartResult is what the starter reports back to the orchestrator
// so it can print where logs went and what mode the server is running
// in.
type initStartResult struct {
	mode    string // "service" | "detached"
	pid     int    // detached mode
	label   string // service mode
	logPath string
}

var (
	initRunner        = runInitCommand
	initMoveRunner    = runInitMoveCommand
	initServerStarter = realInitServerStarter
	initHealthProber  = realInitHealthProber
	initBrowserOpener = realInitBrowserOpener
	initRuntimeGOOS   = runtime.GOOS
	initHealthTimeout = 15 * time.Second
)

func defaultInitOptions() initOptions {
	return initOptions{
		workspaceDir: defaultWorkspaceDir(),
	}
}

func newInitCommand(stdout, stderr io.Writer) *cobra.Command {
	opts := defaultInitOptions()
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize workspace, start the server, and open the setup wizard",
		Long: "Run the onboarding orchestrator. Picks a free port, writes a " +
			"skeleton config to ~/.tars/config/config.yaml, ensures the " +
			"workspace, starts the server (LaunchAgent on macOS by " +
			"default; detached `tars serve` otherwise), waits for it to " +
			"become healthy, and opens the setup wizard in your browser.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return initRunner(cmd.Context(), opts, stdout, stderr)
		},
	}
	cmd.Flags().StringVar(&opts.workspaceDir, "workspace-dir", opts.workspaceDir, "workspace directory")
	cmd.Flags().StringVar(&opts.apiAddr, "api-addr", opts.apiAddr, "explicit listen address (host:port); overrides --port")
	cmd.Flags().IntVar(&opts.port, "port", opts.port, "preferred port (default: auto-pick from 43180-43199)")
	cmd.Flags().BoolVar(&opts.noServer, "no-server", opts.noServer, "skip starting the server (write config only)")
	cmd.Flags().BoolVar(&opts.noBrowser, "no-browser", opts.noBrowser, "skip opening the browser to the setup wizard")
	cmd.Flags().BoolVar(&opts.force, "force", opts.force, "overwrite an existing config and re-run onboarding")

	moveCmd := newInitMoveCommand(stdout, stderr)
	cmd.AddCommand(moveCmd)
	return cmd
}

func newInitMoveCommand(stdout, stderr io.Writer) *cobra.Command {
	moveOpts := initMoveOptions{}
	cmd := &cobra.Command{
		Use:          "move",
		Short:        "Move the workspace directory to a new location",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return initMoveRunner(cmd.Context(), moveOpts, stdout, stderr)
		},
	}
	cmd.Flags().StringVar(&moveOpts.to, "to", "", "target directory for the workspace (required)")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

func runInitCommand(ctx context.Context, opts initOptions, stdout, stderr io.Writer) error {
	workspaceAbs, err := resolveWorkspaceDir(opts.workspaceDir)
	if err != nil {
		return fmt.Errorf("resolve workspace dir: %w", err)
	}
	configPath := config.FixedConfigPath()

	// Check if config already exists at the fixed path.
	configExists := false
	if info, statErr := os.Stat(configPath); statErr == nil && !info.IsDir() {
		configExists = true
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("stat config path %s: %w", configPath, statErr)
	}

	if configExists && !opts.force {
		return fmt.Errorf("config already exists: %s (pass --force to overwrite, or run `tars init reset`)", configPath)
	}

	// Try to migrate legacy config if found and we don't already have one.
	if !configExists {
		if migrated, legacyPath := tryMigrateLegacyConfig(configPath, stdout); migrated {
			if err := updateMigratedWorkspaceDir(configPath, workspaceAbs); err != nil {
				return fmt.Errorf("update migrated workspace_dir: %w", err)
			}
			_, _ = fmt.Fprintf(stdout, "migrated legacy config\n  from: %s\n  to:   %s\n\n", legacyPath, configPath)
			_, _ = fmt.Fprintf(stdout, "the original file has been kept. you can remove it manually:\n  rm %s\n\n", legacyPath)
			return nil
		}
	}

	apiAddr, err := pickInitAPIAddr(opts)
	if err != nil {
		return fmt.Errorf("pick api addr: %w", err)
	}

	if err := ensureStarterWorkspaceLayout(workspaceAbs, defaultStarterBundledPluginsDir()); err != nil {
		return err
	}
	if err := writeOnboardingConfigFile(workspaceAbs, apiAddr, configPath); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stdout, "initialized TARS workspace\nworkspace: %s\nconfig: %s\napi addr: %s\n\n", workspaceAbs, configPath, apiAddr)

	if opts.noServer {
		_, _ = fmt.Fprintf(stdout, "skipped server start (--no-server)\nNext:\n  tars serve --api-addr %s\n  tars service install --api-addr %s --allow-needs-setup && tars service start\n", apiAddr, apiAddr)
		return nil
	}

	useService := !opts.noServer && initRuntimeGOOS == "darwin"
	startRes, err := initServerStarter(ctx, initStartParams{
		apiAddr:      apiAddr,
		configPath:   configPath,
		workspaceDir: workspaceAbs,
		useService:   useService,
	}, stdout, stderr)
	if err != nil {
		return fmt.Errorf("start server: %w", err)
	}

	healthCtx, cancel := context.WithTimeout(ctx, initHealthTimeout)
	defer cancel()
	baseURL := "http://" + apiAddr
	if err := initHealthProber(healthCtx, baseURL); err != nil {
		hint := ""
		if startRes.logPath != "" {
			hint = fmt.Sprintf(" (check log: %s)", startRes.logPath)
		}
		return fmt.Errorf("server did not become healthy: %w%s", err, hint)
	}

	_, _ = fmt.Fprintf(stdout, "server is up\nmode: %s\n", startRes.mode)
	if startRes.logPath != "" {
		_, _ = fmt.Fprintf(stdout, "log: %s\n", startRes.logPath)
	}
	_, _ = fmt.Fprintln(stdout)

	consoleURL := baseURL + "/console/"
	if !opts.noBrowser {
		if err := initBrowserOpener(ctx, consoleURL); err != nil {
			_, _ = fmt.Fprintf(stderr, "browser open failed: %v\n", err)
		}
	}

	_, _ = fmt.Fprintf(stdout, "complete onboarding at %s\n", consoleURL)
	if apiAddr != onboarding.FormatLoopbackAddr(onboarding.DefaultPortRangeStart) {
		_, _ = fmt.Fprintf(stdout, "\nUsing non-default port. For other tars commands in this shell:\n  export TARS_SERVER_URL=http://%s\n", apiAddr)
	}
	return nil
}

// pickInitAPIAddr resolves the listen address from explicit flags or
// auto-picks the first free port in DefaultPortRangeStart..End.
func pickInitAPIAddr(opts initOptions) (string, error) {
	if addr := strings.TrimSpace(opts.apiAddr); addr != "" {
		// Round-trip the port so we surface a clear error early.
		if _, err := onboarding.ParseAPIAddrPort(addr); err != nil {
			return "", err
		}
		return addr, nil
	}
	if opts.port > 0 {
		return onboarding.FormatLoopbackAddr(opts.port), nil
	}
	port, err := onboarding.PickFreePort(onboarding.PortRange(onboarding.DefaultPortRangeStart, onboarding.DefaultPortRangeEnd))
	if err != nil {
		return "", err
	}
	return onboarding.FormatLoopbackAddr(port), nil
}

// realInitServerStarter starts the server in either macOS LaunchAgent
// mode or as a detached `tars serve` process. The choice is made by
// the caller (init orchestrator) based on the OS and --no-service.
func realInitServerStarter(ctx context.Context, params initStartParams, stdout, _ io.Writer) (initStartResult, error) {
	if params.useService {
		return startInitService(ctx, params, stdout)
	}
	return startInitDetached(params)
}

func startInitService(ctx context.Context, params initStartParams, stdout io.Writer) (initStartResult, error) {
	label := launchagent.DefaultServerLabel
	plistPath, err := defaultedServicePlistPath("", label)
	if err != nil {
		return initStartResult{}, err
	}
	stdoutLog := defaultedServiceLogPath("", "Library/Logs/tars-server.out.log")
	stderrLog := defaultedServiceLogPath("", "Library/Logs/tars-server.err.log")
	domain := "gui/" + strconv.Itoa(serviceGetuid())

	summary, err := installLaunchAgent(serviceInstallParams{
		label:         label,
		plistPath:     plistPath,
		stdoutLog:     stdoutLog,
		stderrLog:     stderrLog,
		domain:        domain,
		launchPath:    defaultServiceLaunchPath,
		apiAddr:       params.apiAddr,
		keepAlive:     true,
		runAtLoad:     true,
		skipLLMChecks: true,
	}, stdout)
	if err != nil {
		return initStartResult{}, err
	}
	_, _ = fmt.Fprint(stdout, summary)

	startSummary, err := startLaunchAgent(ctx, label, plistPath, domain)
	if err != nil {
		return initStartResult{}, err
	}
	_, _ = fmt.Fprint(stdout, startSummary)
	return initStartResult{
		mode:    "service",
		label:   label,
		logPath: stderrLog,
	}, nil
}

func startInitDetached(params initStartParams) (initStartResult, error) {
	exe, err := serviceExecutablePath()
	if err != nil {
		return initStartResult{}, fmt.Errorf("resolve executable: %w", err)
	}
	logPath := filepath.Join(config.TarsHomeDir(), "logs", "tars-serve.log")
	res, err := onboarding.SpawnDetached(onboarding.SpawnOptions{
		Executable: exe,
		Args:       []string{"serve", "--config", params.configPath, "--api-addr", params.apiAddr},
		WorkingDir: params.workspaceDir,
		LogPath:    logPath,
	})
	if err != nil {
		return initStartResult{}, err
	}
	return initStartResult{
		mode:    "detached",
		pid:     res.PID,
		logPath: res.LogPath,
	}, nil
}

func realInitHealthProber(ctx context.Context, baseURL string) error {
	return onboarding.WaitForHealthz(ctx, baseURL, 250*time.Millisecond)
}

func realInitBrowserOpener(ctx context.Context, target string) error {
	return openConsoleURL(ctx, target)
}

// writeOnboardingConfigFile writes a minimal config that triggers the
// setup wizard (NeedsSetup=true: no LLM providers/tiers). The user
// completes provider + tier configuration through the web wizard.
func writeOnboardingConfigFile(workspaceAbs, apiAddr, configPath string) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(renderOnboardingConfig(workspaceAbs, apiAddr)), 0o644); err != nil {
		return fmt.Errorf("write onboarding config: %w", err)
	}
	return nil
}

func renderOnboardingConfig(workspaceDir, apiAddr string) string {
	return strings.TrimSpace(fmt.Sprintf(`
# TARS skeleton config generated by "tars init".
# The setup wizard at /console fills in llm_providers and llm_tiers.
# Re-run "tars init reset" to start the wizard from scratch.

runtime:
  workspace_dir: %s

# Local-only starter auth. Change to "required" before exposing beyond localhost.
api:
  auth_mode: off
  allow_insecure_local_auth: true
# Note: api.addr is not a config field; the listen address is passed via
# --api-addr to "tars serve" (baked into the LaunchAgent plist by init,
# or into the detached spawn). This file records it as a comment for
# operators: %s

agentruntime:
  enabled: true
`+"\n", workspaceDir, apiAddr))
}

// tryMigrateLegacyConfig checks for legacy config locations and copies to the
// fixed config path. Returns true and the source path if migration occurred.
func tryMigrateLegacyConfig(fixedPath string, stdout io.Writer) (bool, string) {
	legacyCandidates := []string{
		"workspace/config/tars.config.yaml",
		"config/default.yaml",
		"config/standalone.yaml", // pre-rename layout, kept for upgrade migrations
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
func updateMigratedWorkspaceDir(configPath, defaultWorkspace string) error {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	parsed := map[string]any{}
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	wsRaw, ok := parsed["workspace_dir"]
	changed := false
	if !ok {
		parsed["workspace_dir"] = defaultWorkspace
		changed = true
	} else if ws, ok := wsRaw.(string); ok && !filepath.IsAbs(ws) {
		abs, err := filepath.Abs(ws)
		if err != nil {
			return fmt.Errorf("resolve workspace_dir: %w", err)
		}
		parsed["workspace_dir"] = abs
		changed = true
	}
	if !changed {
		return nil
	}
	out, err := yaml.Marshal(parsed)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(configPath, out, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
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

runtime:
  workspace_dir: %s

# Local-only starter auth. Change to "required" before exposing beyond localhost.
api:
  auth_mode: off
  allow_insecure_local_auth: true

llm:
  # BYOK starter provider pool. Each alias is one credential + endpoint.
  # Other common kinds: anthropic, gemini, gemini-native, kimi,
  # openai-codex, claude-code-cli.
  providers:
    default:
      kind: openai
      auth_mode: api-key
      base_url: https://api.openai.com/v1
      api_key: ${OPENAI_API_KEY}

  # Three tiers must be defined; they may all bind to the same provider.
  tiers:
    heavy:
      provider: default
      model: gpt-4o-mini
      reasoning_effort: high
    standard:
      provider: default
      model: gpt-4o-mini
      reasoning_effort: medium
    light:
      provider: default
      model: gpt-4o-mini
      reasoning_effort: minimal

  default_tier: standard

# Agent Runtime is enabled so agents can dispatch local subagents.
agentruntime:
  enabled: true

# Optional subagent limits for parallel read-only research in chat.
#   subagents:
#     max_threads: 4
#     max_depth: 1
`+"\n", workspaceDir))
}
