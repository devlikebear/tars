package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/launchagent"
	"github.com/devlikebear/tars/internal/onboarding"
	"github.com/spf13/cobra"
)

const (
	baseServiceLaunchPath   = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	defaultServiceStdoutLog = "Library/Logs/tars-server.out.log"
	defaultServiceStderrLog = "Library/Logs/tars-server.err.log"
)

// defaultServiceLaunchPath is the PATH injected into launchd. It leads with
// ~/.local/bin, where the native installers of the claude and agy CLIs put
// their binaries, so the service finds what an interactive shell finds.
func defaultServiceLaunchPath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return baseServiceLaunchPath
	}
	return filepath.Join(home, ".local", "bin") + ":" + baseServiceLaunchPath
}

type serviceOptions struct {
	action          string
	label           string
	plistPath       string
	stdoutLog       string
	stderrLog       string
	launchctlDomain string
	launchPath      string
	apiAddr         string // optional: bake --api-addr into ProgramArguments
	keepAlive       bool
	runAtLoad       bool
	skipLLMChecks   bool // onboarding: allow install when config is in setup-only mode
	// installIfMissing makes `start` set up a fresh machine the way `tars
	// init` does (starter config + workspace, then the plist) instead of
	// failing on a missing plist, and reinstall a plist whose binary is gone.
	// The desktop app's Start server uses it.
	installIfMissing bool
}

type launchctlStatus struct {
	loaded bool
	state  string
	pid    string
	detail string
}

type serviceTarget struct {
	label     string
	plistPath string
	stdoutLog string
	stderrLog string
	domain    string
}

var (
	serviceRunner         = runServiceCommand
	serviceRuntimeGOOS    = runtime.GOOS
	serviceExecutablePath = os.Executable
	serviceUserHomeDir    = os.UserHomeDir
	serviceGetuid         = os.Getuid
	serviceLaunchctlRun   = runLaunchctl
)

func defaultServiceOptions() serviceOptions {
	return serviceOptions{
		label:           launchagent.DefaultServerLabel,
		launchctlDomain: defaultServiceDomain(),
		launchPath:      defaultServiceLaunchPath(),
		keepAlive:       true,
		runAtLoad:       true,
	}
}

func newServiceCommand(stdout, stderr io.Writer) *cobra.Command {
	opts := defaultServiceOptions()
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage the macOS launchd service for tars serve",
	}

	installCmd := &cobra.Command{
		Use:          "install",
		Short:        "Install the LaunchAgent plist for tars serve",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			runOpts := opts
			runOpts.action = "install"
			return serviceRunner(cmd.Context(), runOpts, stdout, stderr)
		},
	}
	bindServiceFlags(installCmd, &opts)
	installCmd.Flags().BoolVar(&opts.keepAlive, "keep-alive", opts.keepAlive, "set KeepAlive in the LaunchAgent plist")
	installCmd.Flags().BoolVar(&opts.runAtLoad, "run-at-load", opts.runAtLoad, "set RunAtLoad in the LaunchAgent plist")
	installCmd.Flags().StringVar(&opts.launchPath, "launch-path", opts.launchPath, "PATH value injected into launchd")
	installCmd.Flags().StringVar(&opts.apiAddr, "api-addr", opts.apiAddr, "bake --api-addr 127.0.0.1:<port> into ProgramArguments")
	installCmd.Flags().BoolVar(&opts.skipLLMChecks, "allow-needs-setup", opts.skipLLMChecks, "skip LLM doctor checks; required when installing before completing the wizard")

	startCmd := &cobra.Command{
		Use:          "start",
		Short:        "Load and start the LaunchAgent service",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			runOpts := opts
			runOpts.action = "start"
			return serviceRunner(cmd.Context(), runOpts, stdout, stderr)
		},
	}
	bindServiceFlags(startCmd, &opts)
	startCmd.Flags().BoolVar(&opts.installIfMissing, "install-if-missing", opts.installIfMissing, "install the LaunchAgent (and a starter config) first when it is missing or its binary is gone")
	startCmd.Flags().StringVar(&opts.apiAddr, "api-addr", opts.apiAddr, "with --install-if-missing: bake --api-addr 127.0.0.1:<port> into a new plist")

	stopCmd := &cobra.Command{
		Use:          "stop",
		Short:        "Stop and unload the LaunchAgent service",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			runOpts := opts
			runOpts.action = "stop"
			return serviceRunner(cmd.Context(), runOpts, stdout, stderr)
		},
	}
	bindServiceFlags(stopCmd, &opts)

	statusCmd := &cobra.Command{
		Use:          "status",
		Short:        "Show LaunchAgent installation and load status",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			runOpts := opts
			runOpts.action = "status"
			return serviceRunner(cmd.Context(), runOpts, stdout, stderr)
		},
	}
	bindServiceFlags(statusCmd, &opts)

	cmd.AddCommand(installCmd, startCmd, stopCmd, statusCmd)
	return cmd
}

func bindServiceFlags(cmd *cobra.Command, opts *serviceOptions) {
	cmd.Flags().StringVar(&opts.label, "label", opts.label, "launch agent label")
	cmd.Flags().StringVar(&opts.plistPath, "plist-path", opts.plistPath, "override launch agent plist path")
	cmd.Flags().StringVar(&opts.stdoutLog, "stdout-log", opts.stdoutLog, "stdout log file path")
	cmd.Flags().StringVar(&opts.stderrLog, "stderr-log", opts.stderrLog, "stderr log file path")
	cmd.Flags().StringVar(&opts.launchctlDomain, "domain", opts.launchctlDomain, "launchctl domain (for example gui/501)")
}

func resolveServiceTarget(opts serviceOptions) (serviceTarget, error) {
	label := strings.TrimSpace(firstNonEmpty(opts.label, launchagent.DefaultServerLabel))
	plistPath, err := defaultedServicePlistPath(opts.plistPath, label)
	if err != nil {
		return serviceTarget{}, err
	}
	stdoutLog := defaultedServiceLogPath(opts.stdoutLog, defaultServiceStdoutLog)
	stderrLog := defaultedServiceLogPath(opts.stderrLog, defaultServiceStderrLog)
	domain := strings.TrimSpace(firstNonEmpty(opts.launchctlDomain, defaultServiceDomain()))
	return serviceTarget{
		label:     label,
		plistPath: plistPath,
		stdoutLog: stdoutLog,
		stderrLog: stderrLog,
		domain:    domain,
	}, nil
}

func defaultServerServiceTarget() (serviceTarget, error) {
	return resolveServiceTarget(serviceOptions{label: launchagent.DefaultServerLabel})
}

func defaultServiceDomain() string {
	return launchagent.DefaultDomainForUID(serviceGetuid())
}

func runServiceCommand(ctx context.Context, opts serviceOptions, stdout, _ io.Writer) error {
	if serviceRuntimeGOOS != "darwin" {
		return fmt.Errorf("service commands are only supported on macOS")
	}

	target, err := resolveServiceTarget(opts)
	if err != nil {
		return err
	}

	switch strings.TrimSpace(opts.action) {
	case "install":
		params := serviceInstallParams{
			label:         target.label,
			plistPath:     target.plistPath,
			stdoutLog:     target.stdoutLog,
			stderrLog:     target.stderrLog,
			domain:        target.domain,
			launchPath:    opts.launchPath,
			apiAddr:       opts.apiAddr,
			keepAlive:     opts.keepAlive,
			runAtLoad:     opts.runAtLoad,
			skipLLMChecks: opts.skipLLMChecks,
		}
		summary, err := installLaunchAgent(params, stdout)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(stdout, summary)
		return nil
	case "start":
		if err := prepareServiceStart(target, opts, stdout); err != nil {
			return err
		}
		for _, logPath := range []string{target.stdoutLog, target.stderrLog} {
			if rotated := rotateServiceLog(logPath, serviceLogRotateBytes); rotated != "" {
				_, _ = fmt.Fprintf(stdout, "rotated %s to %s\n", logPath, rotated)
			}
		}
		summary, err := startLaunchAgent(ctx, target.label, target.plistPath, target.domain)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(stdout, summary)
		return nil
	case "stop":
		out, err := serviceLaunchctlRun(ctx, "bootout", target.domain, target.plistPath)
		if err != nil && !looksLikeMissingLaunchctlService(out, err) {
			return fmt.Errorf("launchctl bootout failed: %w: %s", err, strings.TrimSpace(out))
		}
		_, _ = fmt.Fprintf(stdout, "service stopped\nlabel: %s\ndomain: %s\nplist: %s\n", target.label, target.domain, target.plistPath)
		return nil
	case "status":
		status, err := serviceStatus(ctx, target.label, target.plistPath, target.domain)
		if err != nil {
			return err
		}
		renderServiceStatus(stdout, target.label, target.plistPath, target.stdoutLog, target.stderrLog, status)
		return nil
	default:
		return fmt.Errorf("unsupported service action: %s", strings.TrimSpace(opts.action))
	}
}

// serviceLogRotateBytes is the size past which `tars service start` sets a
// service log aside. launchd appends to these files and nothing else trims
// them.
const serviceLogRotateBytes = 32 << 20

// rotateServiceLog renames a log larger than maxBytes to "<path>.1",
// replacing the previous one, and returns the new name. launchd creates a
// fresh file when it starts the job. Best effort: any failure leaves the log
// where it is.
func rotateServiceLog(path string, maxBytes int64) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= maxBytes {
		return ""
	}
	rotated := path + ".1"
	if err := os.Rename(path, rotated); err != nil {
		return ""
	}
	return rotated
}

// serviceInstallParams captures everything installLaunchAgent needs to
// write a LaunchAgent plist. The fields mirror serviceOptions but use
// pre-resolved (absolute) paths so the helper has no defaulting logic.
type serviceInstallParams struct {
	label         string
	plistPath     string
	stdoutLog     string
	stderrLog     string
	domain        string
	launchPath    string
	apiAddr       string
	keepAlive     bool
	runAtLoad     bool
	skipLLMChecks bool
	// allowLLMFailures installs even when a complete LLM config fails its
	// checks (a CLI provider missing from PATH, a bad key): the server then
	// boots in setup-only mode and the wizard can repair it, which beats a
	// Start server that cannot start anything.
	allowLLMFailures bool
}

// installLaunchAgent loads the fixed config, optionally runs the
// doctor gate (unless skipLLMChecks is set and the config is in
// setup-only mode), and writes the LaunchAgent plist. Returns a
// human-readable summary the caller can print. Used by both the
// `tars service install` cobra command and the `tars init` orchestrator.
func installLaunchAgent(params serviceInstallParams, doctorOut io.Writer) (string, error) {
	configPath := config.FixedConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil {
		return "", fmt.Errorf("load config %s: %w", configPath, err)
	}
	workspaceAbs, err := resolveWorkspaceDir(cfg.WorkspaceDir)
	if err != nil {
		return "", fmt.Errorf("resolve workspace dir: %w", err)
	}

	report, reportErr := buildDoctorReport(doctorOptions{
		workspaceDir: workspaceAbs,
		configPath:   configPath,
	})
	if reportErr != nil {
		// Setup-only mode (no LLM yet) is a legitimate state during
		// onboarding. When the caller opts in, ignore failures whose
		// only cause is the missing LLM configuration.
		if (params.allowLLMFailures || params.skipLLMChecks && config.NeedsSetup(cfg)) && doctorReportOnlyLLMFailures(report) {
			// Print the report so the user still sees what was skipped.
			renderDoctorReport(doctorOut, report)
		} else {
			renderDoctorReport(doctorOut, report)
			return "", fmt.Errorf("service install requires a healthy local setup")
		}
	}

	exe, err := serviceExecutablePath()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(params.stdoutLog), 0o755); err != nil {
		return "", fmt.Errorf("create stdout log dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(params.stderrLog), 0o755); err != nil {
		return "", fmt.Errorf("create stderr log dir: %w", err)
	}

	args := []string{exe, "serve", "--config", configPath}
	if addr := strings.TrimSpace(params.apiAddr); addr != "" {
		args = append(args, "--api-addr", addr)
	}

	content := launchagent.BuildPlist(launchagent.Config{
		Label:            params.label,
		DefaultLabel:     launchagent.DefaultServerLabel,
		ProgramArguments: args,
		WorkingDirectory: workspaceAbs,
		StdoutPath:       params.stdoutLog,
		StderrPath:       params.stderrLog,
		KeepAlive:        params.keepAlive,
		RunAtLoad:        params.runAtLoad,
		Environment: map[string]string{
			"PATH":                       strings.TrimSpace(firstNonEmpty(params.launchPath, defaultServiceLaunchPath())),
			launchagent.ServiceLabelEnv:  params.label,
			launchagent.ServiceDomainEnv: params.domain,
		},
	})
	if err := launchagent.Install(params.plistPath, content); err != nil {
		return "", fmt.Errorf("write launchagent plist: %w", err)
	}

	addrLine := ""
	if addr := strings.TrimSpace(params.apiAddr); addr != "" {
		addrLine = fmt.Sprintf("api addr: %s\n", addr)
	}
	return fmt.Sprintf("service installed\nlabel: %s\nplist: %s\nconfig: %s\nworkspace: %s\n%sstdout log: %s\nstderr log: %s\nnext: tars service start\n",
		params.label, params.plistPath, configPath, workspaceAbs, addrLine, params.stdoutLog, params.stderrLog), nil
}

// prepareServiceStart runs before `service start`. It warns when the plist
// runs another tars than this one, and with --install-if-missing installs
// the plist (writing the starter config and workspace when there is no
// config yet) if it is missing or names a binary that no longer exists.
func prepareServiceStart(target serviceTarget, opts serviceOptions, stdout io.Writer) error {
	state, err := inspectServiceBinary(target.plistPath)
	if err != nil {
		// Best effort: an unreadable plist is launchctl's to report.
		return nil
	}
	needsInstall := !state.Installed || state.Missing
	if !opts.installIfMissing || !needsInstall {
		if warning := state.Warning(); warning != "" {
			_, _ = fmt.Fprintf(stdout, "warning: %s\n", warning)
		}
		return nil
	}
	if state.Missing {
		_, _ = fmt.Fprintf(stdout, "the service ran %s, which no longer exists; reinstalling it for %s\n", state.ServiceBinary, state.CurrentBinary)
	}
	if err := ensureStarterConfig(opts.apiAddr, stdout); err != nil {
		return err
	}
	summary, err := installLaunchAgent(serviceInstallParams{
		label:            target.label,
		plistPath:        target.plistPath,
		stdoutLog:        target.stdoutLog,
		stderrLog:        target.stderrLog,
		domain:           target.domain,
		launchPath:       opts.launchPath,
		apiAddr:          opts.apiAddr,
		keepAlive:        true,
		runAtLoad:        true,
		skipLLMChecks:    true,
		allowLLMFailures: true,
	}, stdout)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(stdout, summary)
	return nil
}

// ensureStarterConfig writes what `tars init` writes on a fresh machine (the
// wizard skeleton config and the starter workspace) when the fixed config
// does not exist yet. An existing config is left alone.
func ensureStarterConfig(apiAddr string, stdout io.Writer) error {
	configPath := config.FixedConfigPath()
	if exists, err := pathExists(configPath); err != nil {
		return fmt.Errorf("stat config path %s: %w", configPath, err)
	} else if exists {
		return nil
	}
	workspaceAbs, err := resolveWorkspaceDir("")
	if err != nil {
		return fmt.Errorf("resolve workspace dir: %w", err)
	}
	if err := ensureStarterWorkspaceLayout(workspaceAbs, defaultStarterBundledPluginsDir()); err != nil {
		return err
	}
	addr := strings.TrimSpace(apiAddr)
	if addr == "" {
		addr = onboarding.FormatLoopbackAddr(onboarding.DefaultPortRangeStart)
	}
	if err := writeOnboardingConfigFile(workspaceAbs, addr, configPath); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "initialized TARS workspace\nworkspace: %s\nconfig: %s\n", workspaceAbs, configPath)
	return nil
}

// startLaunchAgent loads then kickstarts the named service. Mirrors
// the inline logic of `case "start"` so `tars init` can call it
// without going through the cobra command. Returns a summary string
// the caller can print.
func startLaunchAgent(ctx context.Context, label, plistPath, domain string) (string, error) {
	if exists, err := pathExists(plistPath); err != nil {
		return "", fmt.Errorf("stat plist path: %w", err)
	} else if !exists {
		return "", fmt.Errorf("service plist not found: %s", plistPath)
	}
	_, _ = serviceLaunchctlRun(ctx, "bootout", domain, plistPath)
	if out, err := serviceLaunchctlRun(ctx, "bootstrap", domain, plistPath); err != nil {
		return "", fmt.Errorf("launchctl bootstrap failed: %w: %s", err, strings.TrimSpace(out))
	}
	if out, err := serviceLaunchctlRun(ctx, "kickstart", "-k", domain+"/"+label); err != nil {
		return "", fmt.Errorf("launchctl kickstart failed: %w: %s", err, strings.TrimSpace(out))
	}
	return fmt.Sprintf("service started\nlabel: %s\ndomain: %s\nplist: %s\n", label, domain, plistPath), nil
}

// doctorReportOnlyLLMFailures returns true when every failing check
// in the report is LLM-related (credentials or runtime). The
// installLaunchAgent skipLLMChecks gate uses this to ensure the
// onboarding bypass does not mask other genuine failures (workspace
// missing, config invalid, etc.).
func doctorReportOnlyLLMFailures(report doctorReport) bool {
	hasFailure := false
	for _, check := range report.checks {
		if check.status != "fail" {
			continue
		}
		hasFailure = true
		switch check.name {
		case "llm credentials", "llm runtime":
			continue
		default:
			return false
		}
	}
	return hasFailure
}

func defaultedServicePlistPath(raw, label string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		return filepath.Abs(os.ExpandEnv(trimmed))
	}
	home, err := serviceUserHomeDir()
	if err != nil {
		return "", err
	}
	return launchagent.PathForHome(home, label, launchagent.DefaultServerLabel), nil
}

func defaultedServiceLogPath(raw, fallback string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		if abs, err := filepath.Abs(os.ExpandEnv(trimmed)); err == nil {
			return abs
		}
		return os.ExpandEnv(trimmed)
	}
	home, err := serviceUserHomeDir()
	if err != nil {
		return fallback
	}
	return filepath.Join(home, filepath.FromSlash(fallback))
}

func renderServiceStatus(stdout io.Writer, label, plistPath, stdoutLog, stderrLog string, status launchctlStatus) {
	installed := "no"
	if exists, _ := pathExists(plistPath); exists {
		installed = "yes"
	}
	loaded := "no"
	if status.loaded {
		loaded = "yes"
	}
	state := strings.TrimSpace(firstNonEmpty(status.state, "stopped"))
	_, _ = fmt.Fprintf(stdout, "service status\nlabel: %s\ninstalled: %s\nloaded: %s\nstate: %s\nplist: %s\nstdout log: %s\nstderr log: %s\n", label, installed, loaded, state, plistPath, stdoutLog, stderrLog)
	if strings.TrimSpace(status.pid) != "" {
		_, _ = fmt.Fprintf(stdout, "pid: %s\n", strings.TrimSpace(status.pid))
	}
	if strings.TrimSpace(status.detail) != "" {
		_, _ = fmt.Fprintf(stdout, "detail: %s\n", strings.TrimSpace(status.detail))
	}
}

func serviceStatus(ctx context.Context, label, plistPath, domain string) (launchctlStatus, error) {
	status := launchctlStatus{}
	if exists, err := pathExists(plistPath); err != nil {
		return status, fmt.Errorf("stat plist path: %w", err)
	} else if !exists {
		status.detail = "service plist not installed"
		return status, nil
	}
	out, err := serviceLaunchctlRun(ctx, "print", domain+"/"+label)
	if err != nil {
		if looksLikeMissingLaunchctlService(out, err) {
			status.detail = strings.TrimSpace(firstNonEmpty(out, err.Error()))
			return status, nil
		}
		return status, fmt.Errorf("launchctl print failed: %w: %s", err, strings.TrimSpace(out))
	}
	status.loaded = true
	status.state = extractLaunchctlField(out, "state")
	status.pid = extractLaunchctlField(out, "pid")
	return status, nil
}

func extractLaunchctlField(output, key string) string {
	want := strings.TrimSpace(key) + " = "
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, want) {
			return strings.TrimSpace(strings.TrimPrefix(line, want))
		}
	}
	return ""
}

func looksLikeMissingLaunchctlService(output string, err error) bool {
	raw := strings.ToLower(strings.TrimSpace(firstNonEmpty(output, errorString(err))))
	return strings.Contains(raw, "could not find service") ||
		strings.Contains(raw, "no such process") ||
		strings.Contains(raw, "service not found") ||
		strings.Contains(raw, "not loaded") ||
		strings.Contains(raw, "input/output error")
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func runLaunchctl(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "/bin/launchctl", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
