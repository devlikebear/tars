package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/launchagent"
	"github.com/devlikebear/tars/internal/onboarding"
	"github.com/spf13/cobra"
)

type initResetOptions struct {
	yes           bool
	wipeWorkspace bool
	apiAddr       string
	port          int
	noBrowser     bool
}

var (
	initResetRunner    = runInitResetCommand
	initResetConfirmer = readInitResetConfirmation
)

// initResetTimestamp is overridden in tests to make the .bak suffix
// deterministic. Production code uses time.Now().
var initResetTimestamp = func() string { return time.Now().Format("20060102-150405") }

func newInitResetCommand(stdout, stderr io.Writer) *cobra.Command {
	opts := initResetOptions{}
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Re-run onboarding from scratch (regenerate config, restart service, reopen wizard)",
		Long: "Reset TARS to a fresh wizard skeleton without losing recoverable " +
			"data. The current config is backed up to config.yaml.bak (single " +
			"slot, overwritten on each reset). The workspace is preserved by " +
			"default — pass --wipe-workspace to also rename it to " +
			"<workspace>.bak.<timestamp> (the directory is renamed, not " +
			"deleted, so sessions/memory/plugins remain recoverable until you " +
			"rm the .bak yourself). The server is stopped and restarted as a " +
			"managed service so post-reset management uses the standard " +
			"`tars service` commands.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return initResetRunner(cmd.Context(), opts, cmd.InOrStdin(), stdout, stderr)
		},
	}
	cmd.Flags().BoolVar(&opts.yes, "yes", opts.yes, "skip the confirmation prompt (for scripts/CI)")
	cmd.Flags().BoolVar(&opts.wipeWorkspace, "wipe-workspace", opts.wipeWorkspace, "also rename the workspace to <workspace>.bak.<timestamp> (sessions/memory/plugins recoverable from the .bak)")
	cmd.Flags().StringVar(&opts.apiAddr, "api-addr", opts.apiAddr, "explicit listen address (host:port); overrides --port and the inherited plist value")
	cmd.Flags().IntVar(&opts.port, "port", opts.port, "preferred port (overrides the inherited plist value; default: keep whatever the previous service used, else auto-pick)")
	cmd.Flags().BoolVar(&opts.noBrowser, "no-browser", opts.noBrowser, "skip opening the browser to the setup wizard")
	return cmd
}

func runInitResetCommand(ctx context.Context, opts initResetOptions, stdin io.Reader, stdout, stderr io.Writer) error {
	configPath := config.FixedConfigPath()
	configExists := false
	if info, statErr := os.Stat(configPath); statErr == nil && !info.IsDir() {
		configExists = true
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("stat config %s: %w", configPath, statErr)
	}

	// Resolve the workspace path the existing config points at — that's
	// what we'll rename if --wipe-workspace is set, and what we'll
	// preserve otherwise. Falls back to the default if the config is
	// missing or unreadable.
	workspaceAbs := defaultWorkspaceDir()
	if configExists {
		if cfg, err := config.Load(configPath); err == nil && strings.TrimSpace(cfg.WorkspaceDir) != "" {
			workspaceAbs = cfg.WorkspaceDir
		}
	}

	// Prefer the api-addr the previous service was using so reset is
	// non-destructive port-wise. Explicit --port / --api-addr override.
	prevAddr := ""
	if existing, ok := readExistingAPIAddrFromPlist(); ok {
		prevAddr = existing
	}
	apiAddr, err := pickInitResetAPIAddr(opts, prevAddr)
	if err != nil {
		return fmt.Errorf("pick api addr: %w", err)
	}

	if err := confirmInitReset(opts, configPath, configExists, workspaceAbs, apiAddr, stdin, stdout); err != nil {
		return err
	}

	// 1. Stop the running service so we can rewrite the plist and
	//    backup the config without races.
	if initRuntimeGOOS == "darwin" {
		if err := stopExistingService(ctx, stdout); err != nil {
			// Stop failures are non-fatal — the service may already be
			// gone. We warn and proceed.
			_, _ = fmt.Fprintf(stderr, "warning: stop service: %v\n", err)
		}
	}

	// 2. Backup config (single .bak slot, overwritten each reset).
	if configExists {
		bakPath := configPath + ".bak"
		if err := copyFile(configPath, bakPath); err != nil {
			return fmt.Errorf("backup config to %s: %w", bakPath, err)
		}
		_, _ = fmt.Fprintf(stdout, "backed up config\n  %s → %s\n\n", configPath, bakPath)
	}

	// 3. Wipe workspace (rename to .bak.<timestamp>) if asked.
	if opts.wipeWorkspace {
		if _, err := os.Stat(workspaceAbs); err == nil {
			bakWorkspace := workspaceAbs + ".bak." + initResetTimestamp()
			if err := os.Rename(workspaceAbs, bakWorkspace); err != nil {
				return fmt.Errorf("rename workspace to %s: %w", bakWorkspace, err)
			}
			_, _ = fmt.Fprintf(stdout, "renamed workspace\n  %s → %s\n  (sessions/memory/plugins recoverable; rm the .bak when you're sure)\n\n", workspaceAbs, bakWorkspace)
		}
	}

	// 4. Re-scaffold workspace (creates dirs if missing; idempotent
	//    when preserved). Soft on missing bundled plugins.
	if err := ensureStarterWorkspaceLayout(workspaceAbs, defaultStarterBundledPluginsDir()); err != nil {
		return err
	}

	// 5. Write the fresh wizard skeleton.
	if err := writeOnboardingConfigFile(workspaceAbs, apiAddr, configPath); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stdout, "regenerated config\nworkspace: %s\nconfig: %s\napi addr: %s\n\n", workspaceAbs, configPath, apiAddr)

	// 6. Start server (service mode on macOS, detached spawn elsewhere).
	useService := initRuntimeGOOS == "darwin"
	startRes, err := initServerStarter(ctx, initStartParams{
		apiAddr:      apiAddr,
		configPath:   configPath,
		workspaceDir: workspaceAbs,
		useService:   useService,
	}, stdout, stderr)
	if err != nil {
		return fmt.Errorf("start server: %w", err)
	}

	// 7. Wait for healthz, then open browser.
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

// pickInitResetAPIAddr resolves the listen address with this priority:
// explicit flag → previous plist (port inheritance) → auto-pick.
func pickInitResetAPIAddr(opts initResetOptions, prevAddr string) (string, error) {
	if addr := strings.TrimSpace(opts.apiAddr); addr != "" {
		if _, err := onboarding.ParseAPIAddrPort(addr); err != nil {
			return "", err
		}
		return addr, nil
	}
	if opts.port > 0 {
		return onboarding.FormatLoopbackAddr(opts.port), nil
	}
	if prev := strings.TrimSpace(prevAddr); prev != "" {
		if _, err := onboarding.ParseAPIAddrPort(prev); err == nil {
			return prev, nil
		}
	}
	port, err := onboarding.PickFreePort(onboarding.PortRange(onboarding.DefaultPortRangeStart, onboarding.DefaultPortRangeEnd))
	if err != nil {
		return "", err
	}
	return onboarding.FormatLoopbackAddr(port), nil
}

// confirmInitReset prints a summary of what reset will do and asks the
// user to confirm. --yes skips the prompt. Returns an error if the
// user declines or stdin reads fail.
func confirmInitReset(opts initResetOptions, configPath string, configExists bool, workspaceAbs, apiAddr string, stdin io.Reader, stdout io.Writer) error {
	_, _ = fmt.Fprintln(stdout, "tars init reset will:")
	if configExists {
		_, _ = fmt.Fprintf(stdout, "  - back up %s → %s.bak (overwriting any previous .bak)\n", configPath, configPath)
		_, _ = fmt.Fprintf(stdout, "  - regenerate %s as a wizard skeleton\n", configPath)
	} else {
		_, _ = fmt.Fprintf(stdout, "  - write a fresh wizard skeleton at %s (no existing config to back up)\n", configPath)
	}
	if initRuntimeGOOS == "darwin" {
		_, _ = fmt.Fprintln(stdout, "  - stop the LaunchAgent service (if running), reinstall, and start it")
	} else {
		_, _ = fmt.Fprintln(stdout, "  - spawn `tars serve` as a detached background process")
	}
	_, _ = fmt.Fprintf(stdout, "  - reopen the setup wizard at http://%s/console/\n", apiAddr)
	if opts.wipeWorkspace {
		_, _ = fmt.Fprintln(stdout)
		_, _ = fmt.Fprintf(stdout, "Workspace WILL be renamed: %s → %s.bak.<timestamp>\n", workspaceAbs, workspaceAbs)
		_, _ = fmt.Fprintln(stdout, "Sessions, memory, and installed plugins are kept in the .bak directory")
		_, _ = fmt.Fprintln(stdout, "for recovery — `rm -rf` the .bak when you're sure you don't need it.")
	} else {
		_, _ = fmt.Fprintf(stdout, "  - workspace at %s is preserved\n", workspaceAbs)
	}
	_, _ = fmt.Fprintln(stdout)

	if opts.yes {
		return nil
	}
	return initResetConfirmer(stdin, stdout)
}

func readInitResetConfirmation(stdin io.Reader, stdout io.Writer) error {
	_, _ = fmt.Fprint(stdout, "Continue? [y/N]: ")
	reader := bufio.NewReader(stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return fmt.Errorf("read confirmation: %w", err)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" {
		return fmt.Errorf("aborted by user")
	}
	return nil
}

// stopExistingService bootouts the LaunchAgent service if its plist is
// present. Missing-service errors are swallowed (caller treats stop
// failures as non-fatal).
func stopExistingService(ctx context.Context, stdout io.Writer) error {
	target, err := defaultServerServiceTarget()
	if err != nil {
		return err
	}
	if exists, _ := pathExists(target.plistPath); !exists {
		return nil
	}
	out, err := serviceLaunchctlRun(ctx, "bootout", target.domain, target.plistPath)
	if err != nil && !looksLikeMissingLaunchctlService(out, err) {
		return fmt.Errorf("launchctl bootout: %w: %s", err, strings.TrimSpace(out))
	}
	_, _ = fmt.Fprintf(stdout, "stopped existing service\n  label: %s\n  plist: %s\n\n", target.label, target.plistPath)
	return nil
}

// readExistingAPIAddrFromPlist scans the LaunchAgent plist for the
// `--api-addr <addr>` token in ProgramArguments and returns the addr.
// Used by reset to keep the chosen port stable across re-installs.
func readExistingAPIAddrFromPlist() (string, bool) {
	target, err := defaultServerServiceTarget()
	if err != nil {
		return "", false
	}
	data, err := os.ReadFile(target.plistPath)
	if err != nil {
		return "", false
	}
	args, err := launchagent.ProgramArgumentsFromPlist(data)
	if err != nil {
		return "", false
	}
	addr, ok := launchagent.ArgumentValue(args, "--api-addr")
	if !ok {
		return "", false
	}
	return addr, true
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
