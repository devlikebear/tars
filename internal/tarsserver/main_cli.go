package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/cli"
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
			if err == nil {
				// Reconfigure logger from config values.
				cfg := deps.cfg
				needReconfigure := false
				logCfg := loggerConfig{FilePath: opts.LogFile}
				// Config log_file takes precedence over CLI default.
				if strings.TrimSpace(cfg.LogFile) != "" {
					logCfg.FilePath = cfg.LogFile
					needReconfigure = true
				}
				if strings.TrimSpace(cfg.LogLevel) != "" {
					logCfg.Level = cfg.LogLevel
					needReconfigure = true
				}
				if cfg.LogRotateMaxSizeMB > 0 {
					logCfg.RotateMaxSizeMB = cfg.LogRotateMaxSizeMB
				}
				if cfg.LogRotateMaxDays > 0 {
					logCfg.RotateMaxDays = cfg.LogRotateMaxDays
				}
				if cfg.LogRotateMaxBackups > 0 {
					logCfg.RotateMaxBackups = cfg.LogRotateMaxBackups
				}
				if needReconfigure {
					newLogger, newCleanup := setupRuntimeLogger(logCfg, stderr)
					// Replace global logger; previous cleanup runs via deferred Serve().
					zlog.Logger = newLogger
					logger = newLogger
					_ = newCleanup // cleanup will be handled by the process lifecycle
				}
				logger.Info().
					Str("log_level", logCfg.Level).
					Str("log_file", logCfg.FilePath).
					Int("rotate_max_size_mb", logCfg.RotateMaxSizeMB).
					Int("rotate_max_days", logCfg.RotateMaxDays).
					Int("rotate_max_backups", logCfg.RotateMaxBackups).
					Msg("logger configured")
			}
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
			if opts.RunOnce || opts.RunLoop {
				logger.Warn().Msg("--run-once and --run-loop are deprecated no-ops; pulse runs automatically when the server is up")
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
	cmd.Flags().BoolVar(&opts.RunOnce, "run-once", opts.RunOnce, "deprecated — pulse runs automatically; flag retained for backward compat and is a no-op")
	cmd.Flags().BoolVar(&opts.RunLoop, "run-loop", opts.RunLoop, "deprecated — pulse runs automatically; flag retained for backward compat and is a no-op")
	cmd.Flags().BoolVar(&opts.ServeAPI, "serve-api", opts.ServeAPI, "serve tars http api")
	cmd.Flags().StringVar(&opts.APIAddr, "api-addr", opts.APIAddr, "http api listen address")

	return cmd, opts
}
