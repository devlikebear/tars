package tarsapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/cli"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func newRootCmd(opts *options, stdout, stderr io.Writer, nowFn func() time.Time) (*cobra.Command, *options) {
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

			if opts.RunOnce && opts.RunLoop {
				return &cli.ExitError{Code: 2, Err: fmt.Errorf("--run-once and --run-loop are mutually exclusive")}
			}

			deps, err := buildRuntimeDeps(opts, nowFn, logger)
			if err != nil {
				var depErr *runtimeDepsError
				if errors.As(err, &depErr) {
					switch depErr.stage {
					case "load_config":
						logger.Error().Err(depErr.err).Msg("failed to load config")
					case "ensure_workspace":
						logger.Error().Err(depErr.err).Msg("failed to initialize workspace")
					case "daily_log":
						logger.Error().Err(depErr.err).Msg("failed to write daily log")
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
			if err := runHeartbeatModes(parentCtx, opts, deps, nowFn, logger); err != nil {
				return err
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
	cmd.Flags().BoolVar(&opts.RunOnce, "run-once", opts.RunOnce, "run heartbeat once and exit")
	cmd.Flags().BoolVar(&opts.RunLoop, "run-loop", opts.RunLoop, "run heartbeat loop")
	cmd.Flags().BoolVar(&opts.ServeAPI, "serve-api", opts.ServeAPI, "serve tars http api")
	cmd.Flags().StringVar(&opts.APIAddr, "api-addr", opts.APIAddr, "http api listen address")
	cmd.Flags().DurationVar(&opts.HeartbeatInterval, "heartbeat-interval", opts.HeartbeatInterval, "heartbeat interval (e.g. 30m, 5s)")
	cmd.Flags().IntVar(&opts.MaxHeartbeats, "max-heartbeats", opts.MaxHeartbeats, "maximum heartbeat count in loop (0 means unlimited)")

	return cmd, opts
}

func flagValue(args []string, name string) string {
	value := ""
	prefix := name + "="
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, prefix) {
			value = strings.TrimPrefix(arg, prefix)
			continue
		}
		if arg == name && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			value = args[i+1]
			i++
		}
	}
	return value
}
