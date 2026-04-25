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

// loggerReplacer reconfigures the global runtime logger and closes any
// previously-installed log file handle. It is supplied by the caller so the
// process keeps a single live cleanup at all times.
type loggerReplacer func(loggerConfig) zerolog.Logger

func newRootCmd(opts *options, stdout, stderr io.Writer, nowFn func() time.Time, replaceLogger loggerReplacer) (*cobra.Command, *options) {
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
				// --verbose flag overrides config log_level
				if opts.Verbose {
					logCfg.Level = "debug"
					needReconfigure = true
				}
				if needReconfigure {
					if replaceLogger != nil {
						logger = replaceLogger(logCfg)
					} else {
						newLogger, _ := setupRuntimeLogger(logCfg, stderr)
						zlog.Logger = newLogger
						logger = newLogger
					}
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
