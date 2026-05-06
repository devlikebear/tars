package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/devlikebear/tars/internal/remoteaccess"
	"github.com/spf13/cobra"
)

type remoteStatusOptions struct {
	json      bool
	httpsPort int
}

type remoteAccessOptions struct {
	httpsPort    int
	configPath   string
	workspaceDir string
}

var remoteStatusRunner = runRemoteStatus
var remoteEnableRunner = runRemoteEnable
var remoteDisableRunner = runRemoteDisable
var remoteURLRunner = runRemoteURL

func defaultRemoteStatusOptions() remoteStatusOptions {
	return remoteStatusOptions{httpsPort: remoteaccess.DefaultHTTPSPort}
}

func defaultRemoteAccessOptions() remoteAccessOptions {
	return remoteAccessOptions{httpsPort: remoteaccess.DefaultHTTPSPort}
}

func newRemoteCommand(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Inspect remote access status",
	}
	statusOpts := defaultRemoteStatusOptions()
	statusCmd := &cobra.Command{
		Use:          "status",
		Short:        "Show Tailscale Serve remote access status",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return remoteStatusRunner(cmd.Context(), statusOpts, stdout, stderr)
		},
	}
	statusCmd.Flags().BoolVar(&statusOpts.json, "json", false, "print JSON")
	statusCmd.Flags().IntVar(&statusOpts.httpsPort, "port", statusOpts.httpsPort, "Tailscale Serve HTTPS port")

	enableOpts := defaultRemoteAccessOptions()
	enableCmd := &cobra.Command{
		Use:          "enable",
		Short:        "Enable Tailscale Serve remote access",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return remoteEnableRunner(cmd.Context(), enableOpts, stdout, stderr)
		},
	}
	enableCmd.Flags().IntVar(&enableOpts.httpsPort, "port", enableOpts.httpsPort, "Tailscale Serve HTTPS port")
	enableCmd.Flags().StringVar(&enableOpts.configPath, "config", "", "config file path")
	enableCmd.Flags().StringVar(&enableOpts.workspaceDir, "workspace-dir", "", "workspace directory override")

	disableOpts := defaultRemoteAccessOptions()
	disableCmd := &cobra.Command{
		Use:          "disable",
		Short:        "Disable the TARS-owned Tailscale Serve target",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return remoteDisableRunner(cmd.Context(), disableOpts, stdout, stderr)
		},
	}
	disableCmd.Flags().IntVar(&disableOpts.httpsPort, "port", disableOpts.httpsPort, "Tailscale Serve HTTPS port")
	disableCmd.Flags().StringVar(&disableOpts.configPath, "config", "", "config file path")

	urlOpts := defaultRemoteAccessOptions()
	urlCmd := &cobra.Command{
		Use:          "url",
		Short:        "Print the Tailscale Serve HTTPS URL",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return remoteURLRunner(cmd.Context(), urlOpts, stdout, stderr)
		},
	}
	urlCmd.Flags().IntVar(&urlOpts.httpsPort, "port", urlOpts.httpsPort, "Tailscale Serve HTTPS port")

	cmd.AddCommand(statusCmd, enableCmd, disableCmd, urlCmd)
	return cmd
}

func runRemoteStatus(ctx context.Context, opts remoteStatusOptions, stdout, _ io.Writer) error {
	status, err := remoteaccess.Detect(ctx, remoteaccess.Options{HTTPSPort: opts.httpsPort})
	if err != nil {
		return err
	}
	if opts.json {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(status)
	}
	availability := "not installed"
	if status.Installed {
		availability = "installed"
	}
	login := "logged out"
	if status.LoggedIn {
		login = "logged in"
	}
	serve := "idle"
	if status.ServeActive && status.OwnedByTARS {
		serve = fmt.Sprintf("serving on https:%d", status.ServePort)
	} else if status.ServeActive {
		serve = fmt.Sprintf("port %d is used by another Serve target", status.ServePort)
	}
	_, err = fmt.Fprintf(stdout, "tailscale: %s, %s\nhost: %s\nurl: %s\nserve: %s\n", availability, login, status.HostName, status.TailnetURL, serve)
	return err
}

func runRemoteEnable(ctx context.Context, opts remoteAccessOptions, stdout, _ io.Writer) error {
	if err := validateRemoteAccessCLIAuth(opts); err != nil {
		return err
	}
	if err := remoteaccess.Enable(ctx, remoteaccess.Options{HTTPSPort: opts.httpsPort}); err != nil {
		return err
	}
	if err := patchRemoteAccessCLIState(opts, true); err != nil {
		return err
	}
	_, err := fmt.Fprintf(stdout, "remote access enabled on https:%d\n", opts.httpsPort)
	return err
}

func runRemoteDisable(ctx context.Context, opts remoteAccessOptions, stdout, _ io.Writer) error {
	if err := remoteaccess.Disable(ctx, remoteaccess.Options{HTTPSPort: opts.httpsPort}); err != nil {
		return err
	}
	if err := patchRemoteAccessCLIState(opts, false); err != nil {
		return err
	}
	_, err := fmt.Fprintf(stdout, "remote access disabled for https:%d\n", opts.httpsPort)
	return err
}

func runRemoteURL(ctx context.Context, opts remoteAccessOptions, stdout, _ io.Writer) error {
	status, err := remoteaccess.Detect(ctx, remoteaccess.Options{HTTPSPort: opts.httpsPort})
	if err != nil {
		return err
	}
	if !status.Installed {
		return fmt.Errorf("tailscale is not installed")
	}
	if !status.LoggedIn {
		return fmt.Errorf("tailscale is not logged in")
	}
	if status.TailnetURL == "" {
		return fmt.Errorf("tailscale DNS name is unavailable")
	}
	url := "https://" + status.TailnetURL
	if opts.httpsPort > 0 && opts.httpsPort != 443 {
		url = fmt.Sprintf("%s:%d", url, opts.httpsPort)
	}
	_, err = fmt.Fprintln(stdout, url)
	return err
}

func validateRemoteAccessCLIAuth(opts remoteAccessOptions) error {
	cfg, err := loadRemoteAccessCLIConfig(opts)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(cfg.APIAuthMode), "required") {
		return fmt.Errorf("remote access requires api_auth_mode: required")
	}
	store := consoleauth.NewStore(cfg.WorkspaceDir)
	if ok, err := store.HasPassword(consoleauth.RoleAdmin); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("remote access requires an admin browser password; run `tars auth init` or set it in Settings")
	}
	if ok, err := store.HasPassword(consoleauth.RoleUser); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("remote access requires a user browser password; run `tars auth passwd user` or set it in Settings")
	}
	return nil
}

func patchRemoteAccessCLIState(opts remoteAccessOptions, enabled bool) error {
	path := config.ResolveConfigPath(opts.configPath)
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("config path is empty")
	}
	return config.PatchYAML(path, map[string]any{
		"remote_access_tailscale_serve_enabled":    enabled,
		"remote_access_tailscale_serve_https_port": opts.httpsPort,
	})
}

func loadRemoteAccessCLIConfig(opts remoteAccessOptions) (config.Config, error) {
	path := config.ResolveConfigPath(opts.configPath)
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, err
	}
	if strings.TrimSpace(opts.workspaceDir) != "" {
		workspaceDir, err := resolveWorkspaceDir(opts.workspaceDir)
		if err != nil {
			return config.Config{}, err
		}
		cfg.WorkspaceDir = workspaceDir
	}
	return cfg, nil
}
