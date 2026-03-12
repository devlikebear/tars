package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const (
	defaultServiceLabel      = "io.tars.server"
	defaultServiceLaunchPath = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
)

type serviceOptions struct {
	action          string
	workspaceDir    string
	configPath      string
	label           string
	plistPath       string
	stdoutLog       string
	stderrLog       string
	launchctlDomain string
	launchPath      string
	keepAlive       bool
	runAtLoad       bool
}

type launchctlStatus struct {
	loaded bool
	state  string
	pid    string
	detail string
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
		workspaceDir:    defaultWorkspaceDir(),
		label:           defaultServiceLabel,
		launchctlDomain: "gui/" + strconv.Itoa(serviceGetuid()),
		launchPath:      defaultServiceLaunchPath,
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
	cmd.Flags().StringVar(&opts.workspaceDir, "workspace-dir", opts.workspaceDir, "workspace directory")
	cmd.Flags().StringVar(&opts.configPath, "config", "", "config file path")
	cmd.Flags().StringVar(&opts.label, "label", opts.label, "launch agent label")
	cmd.Flags().StringVar(&opts.plistPath, "plist-path", opts.plistPath, "override launch agent plist path")
	cmd.Flags().StringVar(&opts.stdoutLog, "stdout-log", opts.stdoutLog, "stdout log file path")
	cmd.Flags().StringVar(&opts.stderrLog, "stderr-log", opts.stderrLog, "stderr log file path")
	cmd.Flags().StringVar(&opts.launchctlDomain, "domain", opts.launchctlDomain, "launchctl domain (for example gui/501)")
}

func runServiceCommand(ctx context.Context, opts serviceOptions, stdout, _ io.Writer) error {
	if serviceRuntimeGOOS != "darwin" {
		return fmt.Errorf("service commands are only supported on macOS")
	}

	workspaceAbs, err := resolveWorkspaceDir(opts.workspaceDir)
	if err != nil {
		return fmt.Errorf("resolve workspace dir: %w", err)
	}
	configPath, err := resolveConfigPath(opts.configPath, workspaceAbs)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	label := strings.TrimSpace(firstNonEmpty(opts.label, defaultServiceLabel))
	plistPath, err := defaultedServicePlistPath(opts.plistPath, label)
	if err != nil {
		return err
	}
	stdoutLog := defaultedServiceLogPath(opts.stdoutLog, "Library/Logs/tars-server.out.log")
	stderrLog := defaultedServiceLogPath(opts.stderrLog, "Library/Logs/tars-server.err.log")
	domain := strings.TrimSpace(firstNonEmpty(opts.launchctlDomain, "gui/"+strconv.Itoa(serviceGetuid())))

	switch strings.TrimSpace(opts.action) {
	case "install":
		report, reportErr := buildDoctorReport(doctorOptions{
			workspaceDir: workspaceAbs,
			configPath:   configPath,
		})
		if reportErr != nil {
			renderDoctorReport(stdout, report)
			return fmt.Errorf("service install requires a healthy local setup")
		}

		exe, err := serviceExecutablePath()
		if err != nil {
			return fmt.Errorf("resolve executable: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(stdoutLog), 0o755); err != nil {
			return fmt.Errorf("create stdout log dir: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(stderrLog), 0o755); err != nil {
			return fmt.Errorf("create stderr log dir: %w", err)
		}
		content := buildServiceLaunchAgentPlist(serviceLaunchAgentConfig{
			Label:            label,
			ProgramArguments: []string{exe, "serve", "--config", configPath},
			WorkingDirectory: workspaceAbs,
			StdoutPath:       stdoutLog,
			StderrPath:       stderrLog,
			KeepAlive:        opts.keepAlive,
			RunAtLoad:        opts.runAtLoad,
			LaunchPath:       strings.TrimSpace(firstNonEmpty(opts.launchPath, defaultServiceLaunchPath)),
		})
		if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
			return fmt.Errorf("create launchagent dir: %w", err)
		}
		if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write launchagent plist: %w", err)
		}
		_, _ = fmt.Fprintf(stdout, "service installed\nlabel: %s\nplist: %s\nconfig: %s\nstdout log: %s\nstderr log: %s\nnext: tars service start --config %s --workspace-dir %s\n", label, plistPath, configPath, stdoutLog, stderrLog, configPath, workspaceAbs)
		return nil
	case "start":
		if exists, err := pathExists(plistPath); err != nil {
			return fmt.Errorf("stat plist path: %w", err)
		} else if !exists {
			return fmt.Errorf("service plist not found: %s", plistPath)
		}
		_, _ = serviceLaunchctlRun(ctx, "bootout", domain, plistPath)
		if out, err := serviceLaunchctlRun(ctx, "bootstrap", domain, plistPath); err != nil {
			return fmt.Errorf("launchctl bootstrap failed: %w: %s", err, strings.TrimSpace(out))
		}
		if out, err := serviceLaunchctlRun(ctx, "kickstart", "-k", domain+"/"+label); err != nil {
			return fmt.Errorf("launchctl kickstart failed: %w: %s", err, strings.TrimSpace(out))
		}
		_, _ = fmt.Fprintf(stdout, "service started\nlabel: %s\ndomain: %s\nplist: %s\n", label, domain, plistPath)
		return nil
	case "stop":
		out, err := serviceLaunchctlRun(ctx, "bootout", domain, plistPath)
		if err != nil && !looksLikeMissingLaunchctlService(out, err) {
			return fmt.Errorf("launchctl bootout failed: %w: %s", err, strings.TrimSpace(out))
		}
		_, _ = fmt.Fprintf(stdout, "service stopped\nlabel: %s\ndomain: %s\nplist: %s\n", label, domain, plistPath)
		return nil
	case "status":
		status, err := serviceStatus(ctx, label, plistPath, domain)
		if err != nil {
			return err
		}
		renderServiceStatus(stdout, label, plistPath, stdoutLog, stderrLog, status)
		return nil
	default:
		return fmt.Errorf("unsupported service action: %s", strings.TrimSpace(opts.action))
	}
}

type serviceLaunchAgentConfig struct {
	Label            string
	ProgramArguments []string
	WorkingDirectory string
	StdoutPath       string
	StderrPath       string
	KeepAlive        bool
	RunAtLoad        bool
	LaunchPath       string
}

func buildServiceLaunchAgentPlist(cfg serviceLaunchAgentConfig) string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n")
	b.WriteString("<plist version=\"1.0\">\n")
	b.WriteString("<dict>\n")
	b.WriteString("  <key>Label</key>\n")
	_, _ = fmt.Fprintf(&b, "  <string>%s</string>\n", xmlEscape(strings.TrimSpace(cfg.Label)))
	b.WriteString("  <key>ProgramArguments</key>\n")
	b.WriteString("  <array>\n")
	for _, arg := range cfg.ProgramArguments {
		_, _ = fmt.Fprintf(&b, "    <string>%s</string>\n", xmlEscape(strings.TrimSpace(arg)))
	}
	b.WriteString("  </array>\n")
	if v := strings.TrimSpace(cfg.WorkingDirectory); v != "" {
		b.WriteString("  <key>WorkingDirectory</key>\n")
		_, _ = fmt.Fprintf(&b, "  <string>%s</string>\n", xmlEscape(v))
	}
	if v := strings.TrimSpace(cfg.StdoutPath); v != "" {
		b.WriteString("  <key>StandardOutPath</key>\n")
		_, _ = fmt.Fprintf(&b, "  <string>%s</string>\n", xmlEscape(v))
	}
	if v := strings.TrimSpace(cfg.StderrPath); v != "" {
		b.WriteString("  <key>StandardErrorPath</key>\n")
		_, _ = fmt.Fprintf(&b, "  <string>%s</string>\n", xmlEscape(v))
	}
	if v := strings.TrimSpace(cfg.LaunchPath); v != "" {
		b.WriteString("  <key>EnvironmentVariables</key>\n")
		b.WriteString("  <dict>\n")
		b.WriteString("    <key>PATH</key>\n")
		_, _ = fmt.Fprintf(&b, "    <string>%s</string>\n", xmlEscape(v))
		b.WriteString("  </dict>\n")
	}
	b.WriteString("  <key>RunAtLoad</key>\n")
	if cfg.RunAtLoad {
		b.WriteString("  <true/>\n")
	} else {
		b.WriteString("  <false/>\n")
	}
	b.WriteString("  <key>KeepAlive</key>\n")
	if cfg.KeepAlive {
		b.WriteString("  <true/>\n")
	} else {
		b.WriteString("  <false/>\n")
	}
	b.WriteString("</dict>\n")
	b.WriteString("</plist>\n")
	return b.String()
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
	return filepath.Join(home, "Library", "LaunchAgents", strings.TrimSpace(label)+".plist"), nil
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
	cmd := exec.CommandContext(ctx, "launchctl", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func xmlEscape(v string) string {
	out := strings.TrimSpace(v)
	out = strings.ReplaceAll(out, "&", "&amp;")
	out = strings.ReplaceAll(out, "<", "&lt;")
	out = strings.ReplaceAll(out, ">", "&gt;")
	out = strings.ReplaceAll(out, `"`, "&quot;")
	out = strings.ReplaceAll(out, "'", "&apos;")
	return out
}
