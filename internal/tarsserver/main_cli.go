package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/devlikebear/tars/internal/cli"
	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// newRootCmd builds the cobra command tree. The caller is expected to
// have already loaded cfg and installed the runtime logger so the RunE
// hook can focus on wiring the rest of the runtime.
func newRootCmd(opts *options, cfg config.Config, stdout, stderr io.Writer, nowFn func() time.Time) (*cobra.Command, *options) {
	if opts == nil {
		opts = &options{}
	}

	cmd := &cobra.Command{
		Use:           "tars",
		Short:         "Main daemon for TARS",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			parentCtx := cmd.Context()
			if parentCtx == nil {
				parentCtx = context.Background()
			}
			logger := zlog.Logger
			if opts.Verbose {
				logger = logger.Level(zerolog.DebugLevel)
				zlog.Logger = logger
				logger.Debug().Msg("verbose logging enabled")
			}

			deps, err := buildRuntimeDeps(opts, cfg, nowFn, logger)
			if err != nil {
				var depErr *runtimeDepsError
				if errors.As(err, &depErr) {
					switch depErr.stage {
					case "ensure_workspace":
						logger.Error().Err(depErr.err).Msg("failed to initialize workspace")
					case "init_llm":
						logger.Error().Err(depErr.err).Msg("failed to initialize llm provider")
					default:
						logger.Error().Err(depErr.err).Msg("failed to initialize runtime dependencies")
					}
				} else {
					logger.Error().Err(err).Msg("failed to initialize runtime dependencies")
				}
				return &cli.ExitError{Code: 1, Err: err}
			}

			if opts.ServeAPI {
				return runServeAPICommand(parentCtx, opts, deps, nowFn, stdout, stderr, logger)
			}

			logger.Info().
				Str("mode", deps.cfg.Mode).
				Str("workspace_dir", deps.cfg.WorkspaceDir).
				Msg("tars startup complete")

			fmt.Fprintf(stdout, "tars starting in %s mode\n", deps.cfg.Mode)
			return nil
		},
	}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.Flags().StringVar(&opts.ConfigPath, "config", opts.ConfigPath, "path to config file")
	cmd.Flags().StringVar(&opts.Mode, "mode", opts.Mode, "runtime mode override")
	cmd.Flags().StringVar(&opts.WorkspaceDir, "workspace-dir", opts.WorkspaceDir, "workspace directory override")
	cmd.Flags().StringVar(&opts.LogFile, "log-file", opts.LogFile, "append json logs to file")
	cmd.Flags().BoolVar(&opts.Verbose, "verbose", opts.Verbose, "enable verbose debug logging")
	cmd.Flags().BoolVar(&opts.ServeAPI, "serve-api", opts.ServeAPI, "serve tars http api")
	cmd.Flags().StringVar(&opts.APIAddr, "api-addr", opts.APIAddr, "http api listen address")

	return cmd, opts
}
