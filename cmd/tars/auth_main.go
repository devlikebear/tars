package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/spf13/cobra"
)

type authOptions struct {
	workspaceDir string
	password     string
	role         string
	ttl          time.Duration
}

func defaultAuthOptions() authOptions {
	return authOptions{
		workspaceDir: defaultWorkspaceDir(),
		role:         consoleauth.RoleUser,
		ttl:          5 * time.Minute,
	}
}

func newAuthCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage browser login accounts and pairing codes",
	}

	initOpts := defaultAuthOptions()
	initCmd := &cobra.Command{
		Use:          "init",
		Short:        "Create the initial admin browser login",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthInit(cmd.Context(), stdin, stdout, stderr, initOpts)
		},
	}
	initCmd.Flags().StringVar(&initOpts.workspaceDir, "workspace-dir", initOpts.workspaceDir, "workspace directory")
	initCmd.Flags().StringVar(&initOpts.password, "password", "", "admin password (or TARS_INITIAL_ADMIN_PASSWORD)")

	passwdOpts := defaultAuthOptions()
	passwdCmd := &cobra.Command{
		Use:          "passwd [admin|user]",
		Short:        "Set or change a browser login password",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			passwdOpts.role = args[0]
			return runAuthPasswd(cmd.Context(), stdin, stdout, stderr, passwdOpts)
		},
	}
	passwdCmd.Flags().StringVar(&passwdOpts.workspaceDir, "workspace-dir", passwdOpts.workspaceDir, "workspace directory")
	passwdCmd.Flags().StringVar(&passwdOpts.password, "password", "", "password")

	pairingOpts := defaultAuthOptions()
	pairingCmd := &cobra.Command{
		Use:          "pairing-code",
		Short:        "Create a one-time browser pairing code",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthPairingCode(cmd.Context(), stdout, pairingOpts)
		},
	}
	pairingCmd.Flags().StringVar(&pairingOpts.workspaceDir, "workspace-dir", pairingOpts.workspaceDir, "workspace directory")
	pairingCmd.Flags().StringVar(&pairingOpts.role, "role", pairingOpts.role, "role for the pairing code")
	pairingCmd.Flags().DurationVar(&pairingOpts.ttl, "ttl", pairingOpts.ttl, "pairing code TTL")

	cmd.AddCommand(initCmd, passwdCmd, pairingCmd)
	return cmd
}

func runAuthInit(_ context.Context, stdin io.Reader, stdout, stderr io.Writer, opts authOptions) error {
	workspaceDir, err := resolveWorkspaceDir(opts.workspaceDir)
	if err != nil {
		return err
	}
	store := consoleauth.NewStore(workspaceDir)
	hasAdmin, err := store.HasPassword(consoleauth.RoleAdmin)
	if err != nil {
		return err
	}
	if hasAdmin {
		return fmt.Errorf("admin password already configured; use `tars auth passwd admin`")
	}
	password, err := resolveAuthPassword(stdin, stderr, opts.password, os.Getenv("TARS_INITIAL_ADMIN_PASSWORD"))
	if err != nil {
		return err
	}
	if err := store.SetPassword(consoleauth.RoleAdmin, password); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "admin account initialized for %s\n", workspaceDir)
	return err
}

func runAuthPasswd(_ context.Context, stdin io.Reader, stdout, stderr io.Writer, opts authOptions) error {
	role, err := normalizeAuthCommandRole(opts.role)
	if err != nil {
		return err
	}
	workspaceDir, err := resolveWorkspaceDir(opts.workspaceDir)
	if err != nil {
		return err
	}
	password, err := resolveAuthPassword(stdin, stderr, opts.password, "")
	if err != nil {
		return err
	}
	if err := consoleauth.NewStore(workspaceDir).SetPassword(role, password); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%s password updated for %s\n", role, workspaceDir)
	return err
}

func runAuthPairingCode(_ context.Context, stdout io.Writer, opts authOptions) error {
	role, err := normalizeAuthCommandRole(opts.role)
	if err != nil {
		return err
	}
	workspaceDir, err := resolveWorkspaceDir(opts.workspaceDir)
	if err != nil {
		return err
	}
	code, err := consoleauth.NewStore(workspaceDir).CreatePairingCode(role, opts.ttl)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "pairing code (%s, expires %s): %s\n", code.Role, code.ExpiresAt.Format(time.RFC3339), code.Code)
	return err
}

func resolveAuthPassword(stdin io.Reader, stderr io.Writer, explicit, fallback string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return explicit, nil
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback, nil
	}
	if stderr != nil {
		if _, err := fmt.Fprint(stderr, "Password: "); err != nil {
			return "", err
		}
	}
	reader := bufio.NewReader(stdin)
	password, err := reader.ReadString('\n')
	if err != nil && len(password) == 0 {
		return "", fmt.Errorf("password is required")
	}
	password = strings.TrimRight(password, "\r\n")
	if strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("password is required")
	}
	return password, nil
}

func normalizeAuthCommandRole(role string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case consoleauth.RoleAdmin:
		return consoleauth.RoleAdmin, nil
	case consoleauth.RoleUser:
		return consoleauth.RoleUser, nil
	default:
		return "", fmt.Errorf("unsupported role %q (expected admin or user)", role)
	}
}
