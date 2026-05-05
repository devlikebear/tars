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
	migrate      bool
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
		// Reject unknown positional args so e.g. typos error loudly
		// instead of silently re-running the orchestrator. Subcommands
		// (move, reset) are still dispatched normally by cobra.
		Args:         cobra.NoArgs,
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
	cmd.Flags().BoolVar(&opts.migrate, "migrate", opts.migrate, "import a legacy config (workspace/config/tars.config.yaml or config/{default,standalone}.yaml) instead of writing a fresh wizard skeleton")

	cmd.AddCommand(newInitMoveCommand(stdout, stderr))
	cmd.AddCommand(newInitResetCommand(stdout, stderr))
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

	if opts.force && opts.migrate {
		return fmt.Errorf("--force and --migrate are mutually exclusive: --force writes a fresh wizard skeleton, --migrate imports a legacy config")
	}

	// Migration is now an explicit opt-in (--migrate). The probe used
	// to be automatic on every fresh init, but that surprised users
	// who happened to run `tars init` from a directory with one of
	// the scanned-for legacy paths (e.g. the TARS source repo, where
	// config/default.yaml is checked in). When migration is opt-in we
	// can still help users discover it: detect a legacy config and
	// print a hint pointing at --migrate, then proceed with the
	// fresh wizard skeleton.
	migrated := false
	switch {
	case opts.migrate:
		legacyPath, ok := findLegacyConfig()
		if !ok {
			return fmt.Errorf("--migrate: no legacy config found relative to %s (looked for %s)", mustGetwd(), strings.Join(legacyConfigCandidates(), ", "))
		}
		if err := importLegacyConfig(configPath, legacyPath, stdout); err != nil {
			return err
		}
		if err := updateMigratedWorkspaceDir(configPath, workspaceAbs); err != nil {
			return fmt.Errorf("update migrated workspace_dir: %w", err)
		}
		migrated = true
	case !configExists:
		if legacyPath, ok := findLegacyConfig(); ok {
			_, _ = fmt.Fprintf(stdout, "note: detected possible legacy config at %s\n      run `tars init --migrate` to import it instead of starting fresh\n\n", legacyPath)
		}
	}

	apiAddr, err := pickInitAPIAddr(opts)
	if err != nil {
		return fmt.Errorf("pick api addr: %w", err)
	}

	// For fresh installs we scaffold the workspace and write the
	// wizard skeleton config. Migrated installs already have a
	// populated workspace (referenced by the migrated workspace_dir)
	// and an authoritative config — touching either would either
	// duplicate the workspace at the default path (~/.tars/workspace)
	// or destroy the user's existing settings.
	if !migrated {
		if err := ensureStarterWorkspaceLayout(workspaceAbs, defaultStarterBundledPluginsDir()); err != nil {
			return err
		}
		if err := writeOnboardingConfigFile(workspaceAbs, apiAddr, configPath); err != nil {
			return err
		}
	}

	// Resolve the workspace path the started server will actually see.
	// For migrated installs this is the value baked into the migrated
	// config; for fresh installs it is the value we just wrote.
	runtimeWorkspace := workspaceAbs
	if migrated {
		if cfg, loadErr := config.Load(configPath); loadErr == nil && strings.TrimSpace(cfg.WorkspaceDir) != "" {
			runtimeWorkspace = cfg.WorkspaceDir
		}
	}

	if migrated {
		_, _ = fmt.Fprintf(stdout, "starting server with migrated config\nworkspace: %s\nconfig: %s\napi addr: %s\n\n", runtimeWorkspace, configPath, apiAddr)
	} else {
		_, _ = fmt.Fprintf(stdout, "initialized TARS workspace\nworkspace: %s\nconfig: %s\napi addr: %s\n\n", runtimeWorkspace, configPath, apiAddr)
	}

	if opts.noServer {
		_, _ = fmt.Fprintf(stdout, "skipped server start (--no-server)\nNext:\n  tars serve --api-addr %s\n  tars service install --api-addr %s --allow-needs-setup && tars service start\n", apiAddr, apiAddr)
		return nil
	}

	useService := !opts.noServer && initRuntimeGOOS == "darwin"
	startRes, err := initServerStarter(ctx, initStartParams{
		apiAddr:      apiAddr,
		configPath:   configPath,
		workspaceDir: runtimeWorkspace,
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

// legacyConfigCandidates returns the cwd-relative paths the legacy
// migration probe scans, in priority order.
func legacyConfigCandidates() []string {
	return []string{
		"workspace/config/tars.config.yaml",
		"config/default.yaml",
		"config/standalone.yaml", // pre-rename layout, kept for upgrade migrations
	}
}

// findLegacyConfig probes the candidate paths and returns the first
// existing legacy config (resolved to absolute) without copying it.
// Used both by the explicit --migrate flow (then importLegacyConfig
// runs) and by the discovery hint that prints when the user runs
// `tars init` without --migrate.
func findLegacyConfig() (string, bool) {
	for _, candidate := range legacyConfigCandidates() {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if info, err := os.Stat(abs); err != nil || info.IsDir() {
			continue
		}
		return abs, true
	}
	return "", false
}

// importLegacyConfig copies the legacy file at legacyPath to the
// fixed config path and prints the migration banner. Caller is
// responsible for any post-import patching (workspace_dir, etc.).
func importLegacyConfig(fixedPath, legacyPath string, stdout io.Writer) error {
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return fmt.Errorf("read legacy config %s: %w", legacyPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(fixedPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(fixedPath, data, 0o644); err != nil {
		return fmt.Errorf("write migrated config: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "migrated legacy config\n  from: %s\n  to:   %s\n\n", legacyPath, fixedPath)
	_, _ = fmt.Fprintf(stdout, "the original file has been kept. you can remove it manually:\n  rm %s\n\n", legacyPath)
	return nil
}

// mustGetwd returns the current working directory or "." on error,
// for inclusion in user-facing diagnostics. Never returns an error.
func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// updateMigratedWorkspaceDir absolutizes the workspace_dir in the
// migrated config so the relative paths used by the legacy launcher
// (`./workspace`) keep pointing at the same on-disk location after
// the binary moves into ~/.tars. It accepts both layouts the loader
// understands: top-level `workspace_dir:` and the nested
// `runtime.workspace_dir:` form (the canonical post-flatten key is
// the same; see internal/config/yaml.go's `runtime` root alias).
//
// Behavior:
//   - nested runtime.workspace_dir present and relative → absolutize in place
//   - top-level workspace_dir present and relative → absolutize in place
//   - neither present → add a top-level entry pointing at defaultWorkspace
//   - already absolute → no-op (avoids creating a duplicate entry that
//     could lose to / clobber the user's actual setting after flatten)
func updateMigratedWorkspaceDir(configPath, defaultWorkspace string) error {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	parsed := map[string]any{}
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	changed := false

	// Try nested runtime.workspace_dir first — that's the form the
	// post-rename starter config uses, and the form most legacy
	// configs ended up with after the runtime/automation regroup.
	if runtimeRaw, ok := parsed["runtime"]; ok {
		if runtimeMap, ok := runtimeRaw.(map[string]any); ok {
			if wsRaw, ok := runtimeMap["workspace_dir"]; ok {
				if ws, ok := wsRaw.(string); ok && !filepath.IsAbs(ws) {
					abs, err := filepath.Abs(ws)
					if err != nil {
						return fmt.Errorf("resolve runtime.workspace_dir: %w", err)
					}
					runtimeMap["workspace_dir"] = abs
					parsed["runtime"] = runtimeMap
					changed = true
				}
				if changed {
					goto write
				}
				// Already absolute — nothing to patch.
				return nil
			}
		}
	}

	// Fall back to top-level workspace_dir.
	if wsRaw, ok := parsed["workspace_dir"]; ok {
		if ws, ok := wsRaw.(string); ok && !filepath.IsAbs(ws) {
			abs, err := filepath.Abs(ws)
			if err != nil {
				return fmt.Errorf("resolve workspace_dir: %w", err)
			}
			parsed["workspace_dir"] = abs
			changed = true
			goto write
		}
		// Already absolute — nothing to patch.
		return nil
	}

	// Nothing on disk — bootstrap with the default. Use top-level so
	// it's discoverable in either tree shape.
	parsed["workspace_dir"] = defaultWorkspace
	changed = true

write:
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
	// Bundled plugins seed the workspace's plugins/ dir but are not
	// strictly required — the system boots without them and the user
	// can install plugins later. When the bundled dir cannot be
	// resolved (the assetpath probe didn't find it next to the exe,
	// in cwd, or in source-tree fallbacks) treat that as a soft
	// condition rather than a hard failure. This keeps `tars init`
	// usable for dev binaries built outside a release tree.
	if _, ok := assetpath.ResolveExistingDir(bundledPluginsDir); !ok {
		return nil
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
