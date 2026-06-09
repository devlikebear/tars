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

// recoverableLLMInitStages enumerate the buildLLMDeps failure stages
// that the CLI may downgrade to setup-only mode on, instead of exiting.
// Anything outside this set (e.g. workspace creation failures) remains
// fatal — onboarding cannot heal it from the wizard.
var recoverableLLMInitStages = map[string]struct{}{
	"init_llm":             {},
	"init_memory_backend":  {},
	"init_semantic_memory": {},
}

func isRecoverableLLMInitError(err error) bool {
	var depErr *runtimeDepsError
	if !errors.As(err, &depErr) {
		return false
	}
	_, ok := recoverableLLMInitStages[depErr.stage]
	return ok
}

func logBootstrapError(logger zerolog.Logger, err error) {
	var depErr *runtimeDepsError
	if errors.As(err, &depErr) {
		switch depErr.stage {
		case "ensure_workspace":
			logger.Error().Err(depErr.err).Msg("failed to initialize workspace")
		case "init_llm":
			logger.Error().Err(depErr.err).Msg("failed to initialize llm provider")
		default:
			logger.Error().Err(depErr.err).Str("stage", depErr.stage).Msg("failed to initialize runtime dependencies")
		}
		return
	}
	logger.Error().Err(err).Msg("failed to initialize runtime dependencies")
}

func runServerRuntime(parentCtx context.Context, opts *options, cfg config.Config, stdout, stderr io.Writer, nowFn func() time.Time, logger zerolog.Logger) error {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	if opts == nil {
		opts = &options{}
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	if opts.Verbose {
		logger = logger.Level(zerolog.DebugLevel)
		zlog.Logger = logger
		logger.Debug().Msg("verbose logging enabled")
	}

	base, err := buildBaseDeps(opts, cfg, nowFn, logger)
	if err != nil {
		logBootstrapError(logger, err)
		return &cli.ExitError{Code: 1, Err: err}
	}

	deps, err := buildLLMDeps(base, cfg, logger)
	if err != nil {
		if isRecoverableLLMInitError(err) {
			logger.Warn().
				Err(err).
				Msg("llm init failed — entering setup-only mode (visit /console to complete setup)")
			deps = base
			deps.LLMReady = false
		} else {
			logBootstrapError(logger, err)
			return &cli.ExitError{Code: 1, Err: err}
		}
	}

	if opts.ConfigCheck {
		if !deps.LLMReady {
			msg := "tars config check requires setup — run tars serve and visit /console"
			logger.Error().Str("workspace_dir", deps.cfg.WorkspaceDir).Msg(msg)
			_, _ = fmt.Fprintln(stdout, msg)
			return &cli.ExitError{Code: 1, Err: fmt.Errorf("%s", msg)}
		}
		logger.Info().
			Str("workspace_dir", deps.cfg.WorkspaceDir).
			Msg("tars config check passed")

		_, _ = fmt.Fprintln(stdout, "tars config check passed")
		return nil
	}

	return runServeAPICommand(parentCtx, opts, deps, nowFn, stdout, stderr, logger)
}

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
			return runServerRuntime(cmd.Context(), opts, cfg, stdout, stderr, nowFn, zlog.Logger)
		},
	}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.Flags().StringVar(&opts.ConfigPath, "config", opts.ConfigPath, "path to config file")
	cmd.Flags().StringVar(&opts.WorkspaceDir, "workspace-dir", opts.WorkspaceDir, "workspace directory override")
	cmd.Flags().StringVar(&opts.LogFile, "log-file", opts.LogFile, "append json logs to file")
	cmd.Flags().BoolVar(&opts.Verbose, "verbose", opts.Verbose, "enable verbose debug logging")
	cmd.Flags().BoolVar(&opts.ConfigCheck, "config-check", opts.ConfigCheck, "validate config and runtime dependencies, then exit without starting the http api")
	cmd.Flags().StringVar(&opts.APIAddr, "api-addr", opts.APIAddr, "http api listen address")

	return cmd, opts
}
