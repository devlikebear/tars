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

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/launchagent"
	"github.com/spf13/cobra"
)

const (
	defaultServiceLaunchPath = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
)

type serviceOptions struct {
	action          string
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
		label:           launchagent.DefaultServerLabel,
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

	label := strings.TrimSpace(firstNonEmpty(opts.label, launchagent.DefaultServerLabel))
	plistPath, err := defaultedServicePlistPath(opts.plistPath, label)
	if err != nil {
		return err
	}
	stdoutLog := defaultedServiceLogPath(opts.stdoutLog, "Library/Logs/tars-server.out.log")
	stderrLog := defaultedServiceLogPath(opts.stderrLog, "Library/Logs/tars-server.err.log")
	domain := strings.TrimSpace(firstNonEmpty(opts.launchctlDomain, "gui/"+strconv.Itoa(serviceGetuid())))

	switch strings.TrimSpace(opts.action) {
	case "install":
		configPath := config.FixedConfigPath()
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load config %s: %w", configPath, err)
		}
		workspaceAbs, err := resolveWorkspaceDir(cfg.WorkspaceDir)
		if err != nil {
			return fmt.Errorf("resolve workspace dir: %w", err)
		}
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
		content := launchagent.BuildPlist(launchagent.Config{
			Label:            label,
			DefaultLabel:     launchagent.DefaultServerLabel,
			ProgramArguments: []string{exe, "serve", "--config", configPath},
			WorkingDirectory: workspaceAbs,
			StdoutPath:       stdoutLog,
			StderrPath:       stderrLog,
			KeepAlive:        opts.keepAlive,
			RunAtLoad:        opts.runAtLoad,
			Environment: map[string]string{
				"PATH":                       strings.TrimSpace(firstNonEmpty(opts.launchPath, defaultServiceLaunchPath)),
				launchagent.ServiceLabelEnv:  label,
				launchagent.ServiceDomainEnv: domain,
			},
		})
		if err := launchagent.Install(plistPath, content); err != nil {
			return fmt.Errorf("write launchagent plist: %w", err)
		}
		_, _ = fmt.Fprintf(stdout, "service installed\nlabel: %s\nplist: %s\nconfig: %s\nworkspace: %s\nstdout log: %s\nstderr log: %s\nnext: tars service start\n", label, plistPath, configPath, workspaceAbs, stdoutLog, stderrLog)
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
	cmd := exec.CommandContext(ctx, "launchctl", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
