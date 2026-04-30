package main

import (
	"context"
	"io"
	"strings"

	"github.com/devlikebear/tars/internal/tarsserver"
	"github.com/spf13/cobra"
)

type serveOptions struct {
	configPath   string
	workspaceDir string
	logFile      string
	verbose      bool
	configCheck  bool
	apiAddr      string
}

var serveRunner = runServeCommand

const defaultServeLogFile = ".logs/tars-debug.log"

func defaultServeOptions() serveOptions {
	return serveOptions{
		logFile: defaultServeLogFile,
		apiAddr: tarsserver.DefaultAPIAddr,
	}
}

func newServeCommand(stdout, stderr io.Writer) *cobra.Command {
	opts := defaultServeOptions()
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run TARS daemon server mode",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serveRunner(cmd.Context(), opts, stdout, stderr)
		},
	}
	cmd.Flags().StringVar(&opts.configPath, "config", "", "path to config file")
	cmd.Flags().StringVar(&opts.workspaceDir, "workspace-dir", "", "workspace directory override")
	cmd.Flags().StringVar(&opts.logFile, "log-file", opts.logFile, "append json logs to file")
	cmd.Flags().BoolVar(&opts.verbose, "verbose", false, "enable verbose debug logging")
	cmd.Flags().BoolVar(&opts.configCheck, "config-check", opts.configCheck, "validate config and runtime dependencies, then exit without starting the http api")
	cmd.Flags().StringVar(&opts.apiAddr, "api-addr", opts.apiAddr, "http api listen address")
	return cmd
}

func runServeCommand(ctx context.Context, opts serveOptions, stdout, stderr io.Writer) error {
	return tarsserver.Serve(ctx, tarsserver.ServeOptions{
		ConfigPath:   strings.TrimSpace(opts.configPath),
		WorkspaceDir: strings.TrimSpace(opts.workspaceDir),
		LogFile:      strings.TrimSpace(opts.logFile),
		Verbose:      opts.verbose,
		ConfigCheck:  opts.configCheck,
		APIAddr:      strings.TrimSpace(opts.apiAddr),
	}, stdout, stderr)
}
