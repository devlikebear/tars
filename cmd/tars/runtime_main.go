package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	protocol "github.com/devlikebear/tars/pkg/tarsclient"
	"github.com/spf13/cobra"
)

var (
	statusCommandRunner = runStatusCommand
	healthCommandRunner = runHealthCommand
)

func newStatusCommand(stdout, stderr io.Writer) *cobra.Command {
	opts := defaultClientOptions()
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show server workspace and session status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return statusCommandRunner(cmd.Context(), stdout, stderr, opts)
		},
	}
	bindReadOnlyRuntimeFlags(cmd, &opts)
	return cmd
}

func newHealthCommand(stdout, stderr io.Writer) *cobra.Command {
	opts := defaultClientOptions()
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check server health",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return healthCommandRunner(cmd.Context(), stdout, stderr, opts)
		},
	}
	bindReadOnlyRuntimeFlags(cmd, &opts)
	return cmd
}

func bindReadOnlyRuntimeFlags(cmd *cobra.Command, opts *clientOptions) {
	if cmd == nil || opts == nil {
		return
	}
	cmd.Flags().StringVar(&opts.serverURL, "server-url", opts.serverURL, "tars server url")
	cmd.Flags().StringVar(&opts.apiToken, "api-token", opts.apiToken, "api token")
}

func runStatusCommand(ctx context.Context, stdout, _ io.Writer, opts clientOptions) error {
	info, err := newProtocolClient(opts).Status(ctx)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "SYSTEM > workspace=%s sessions=%d", info.WorkspaceDir, info.SessionCount); err != nil {
		return err
	}
	if strings.TrimSpace(info.MainSessionID) != "" {
		if _, err := fmt.Fprintf(stdout, " main_session=%s", strings.TrimSpace(info.MainSessionID)); err != nil {
			return err
		}
	}
	if strings.TrimSpace(info.AuthRole) != "" {
		if _, err := fmt.Fprintf(stdout, " auth_role=%s", strings.TrimSpace(info.AuthRole)); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(stdout)
	return err
}

func runHealthCommand(ctx context.Context, stdout, _ io.Writer, opts clientOptions) error {
	info, err := newProtocolClient(opts).Healthz(ctx)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "SYSTEM > ok=%t component=%s time=%s\n", info.OK, info.Component, info.Time)
	return err
}

func newProtocolClient(opts clientOptions) *protocol.Client {
	return protocol.New(protocol.Config{
		ServerURL:     strings.TrimSpace(opts.serverURL),
		APIToken:      strings.TrimSpace(opts.apiToken),
		AdminAPIToken: strings.TrimSpace(opts.adminToken),
	})
}
