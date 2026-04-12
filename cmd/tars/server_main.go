package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/devlikebear/tars/internal/tarsserver"
	"github.com/spf13/cobra"
)

type serveOptions struct {
	configPath   string
	mode         string
	workspaceDir string
	logFile      string
	verbose      bool
	runOnce      bool
	runLoop      bool
	serveAPI     bool
	apiAddr      string
}

var serveRunner = runServeCommand

const defaultServeLogFile = ".logs/tars-debug.log"

func defaultServeOptions() serveOptions {
	return serveOptions{
		logFile:  defaultServeLogFile,
		serveAPI: true,
		apiAddr:  tarsserver.DefaultAPIAddr,
	}
}

func newServeCommand(stdout, stderr io.Writer) *cobra.Command {
	opts := defaultServeOptions()
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run TARS daemon server mode",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.runOnce && opts.runLoop {
				return fmt.Errorf("--run-once and --run-loop are mutually exclusive")
			}
			if opts.runOnce || opts.runLoop {
				fmt.Fprintln(stderr, "warning: --run-once and --run-loop are deprecated no-ops; pulse runs automatically when the server is up")
			}
			return serveRunner(cmd.Context(), opts, stdout, stderr)
		},
	}
	cmd.Flags().StringVar(&opts.configPath, "config", "", "path to config file")
	cmd.Flags().StringVar(&opts.mode, "mode", "", "runtime mode override")
	cmd.Flags().StringVar(&opts.workspaceDir, "workspace-dir", "", "workspace directory override")
	cmd.Flags().StringVar(&opts.logFile, "log-file", opts.logFile, "append json logs to file")
	cmd.Flags().BoolVar(&opts.verbose, "verbose", false, "enable verbose debug logging")
	cmd.Flags().BoolVar(&opts.runOnce, "run-once", false, "deprecated — pulse runs automatically; flag is a no-op")
	cmd.Flags().BoolVar(&opts.runLoop, "run-loop", false, "deprecated — pulse runs automatically; flag is a no-op")
	cmd.Flags().BoolVar(&opts.serveAPI, "serve-api", opts.serveAPI, "serve tars http api")
	cmd.Flags().StringVar(&opts.apiAddr, "api-addr", opts.apiAddr, "http api listen address")
	return cmd
}

func runServeCommand(ctx context.Context, opts serveOptions, stdout, stderr io.Writer) error {
	return tarsserver.Serve(ctx, tarsserver.ServeOptions{
		ConfigPath:   strings.TrimSpace(opts.configPath),
		Mode:         strings.TrimSpace(opts.mode),
		WorkspaceDir: strings.TrimSpace(opts.workspaceDir),
		LogFile:      strings.TrimSpace(opts.logFile),
		Verbose:      opts.verbose,
		RunOnce:      opts.runOnce,
		RunLoop:      opts.runLoop,
		ServeAPI:     opts.serveAPI,
		APIAddr:      strings.TrimSpace(opts.apiAddr),
	}, stdout, stderr)
}
