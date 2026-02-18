package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/devlikebear/tarsncase/internal/cli"
	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/sentinel"
	"github.com/devlikebear/tarsncase/internal/serverauth"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type options struct {
	ConfigPath string
	APIAddr    string
	Verbose    bool
}

func run(args []string, stdout, stderr io.Writer) int {
	_ = godotenv.Load(".env")

	logger := zerolog.New(zerolog.ConsoleWriter{
		Out:        stderr,
		TimeFormat: "15:04:05",
		NoColor:    false,
	}).With().Timestamp().Str("component", "cased").Logger()
	zlog.Logger = logger

	cmd, opts := newRootCmd(&options{}, stdout, logger)
	_ = opts
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		var ex *cli.ExitError
		if errors.As(err, &ex) {
			return ex.Code
		}
		logger.Error().Err(err).Msg("failed to parse flags")
		if cli.IsFlagError(err) {
			return 2
		}
		return 1
	}
	return 0
}

func newRootCmd(opts *options, stdout io.Writer, logger zerolog.Logger) (*cobra.Command, *options) {
	if opts == nil {
		opts = &options{}
	}
	cmd := &cobra.Command{
		Use:           "cased",
		Short:         "Sentinel daemon for supervising tarsd",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, _ []string) error {
			log := logger
			if opts.Verbose {
				log = log.Level(zerolog.DebugLevel)
				zlog.Logger = log
				log.Debug().Msg("verbose logging enabled")
			}

			resolvedConfigPath := config.ResolveCasedConfigPath(opts.ConfigPath)
			cfg, err := config.LoadCased(resolvedConfigPath)
			if err != nil {
				log.Error().Err(err).Msg("failed to load cased config")
				return &cli.ExitError{Code: 1, Err: err}
			}
			if resolvedConfigPath != "" {
				log.Debug().Str("config_path", resolvedConfigPath).Msg("resolved config file")
			}
			if opts.APIAddr != "" {
				cfg.APIAddr = opts.APIAddr
			}

			supervisor := sentinel.NewSupervisor(sentinel.Options{
				Enabled:            true,
				TargetCommand:      cfg.TargetCommand,
				TargetArgs:         cfg.TargetArgs,
				TargetWorkingDir:   cfg.TargetWorkingDir,
				TargetEnv:          cfg.TargetEnv,
				ProbeURL:           cfg.ProbeURL,
				ProbeInterval:      time.Duration(cfg.ProbeIntervalMS) * time.Millisecond,
				ProbeTimeout:       time.Duration(cfg.ProbeTimeoutMS) * time.Millisecond,
				ProbeFailThreshold: cfg.ProbeFailThreshold,
				RestartMaxAttempts: cfg.RestartMaxAttempts,
				RestartBackoff:     time.Duration(cfg.RestartBackoffMS) * time.Millisecond,
				RestartBackoffMax:  time.Duration(cfg.RestartBackoffMaxMS) * time.Millisecond,
				RestartCooldown:    time.Duration(cfg.RestartCooldownMS) * time.Millisecond,
				EventBufferSize:    cfg.EventBufferSize,
				Autostart:          cfg.Autostart,
			})
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			if err := supervisor.Start(ctx); err != nil {
				log.Error().Err(err).Msg("failed to start sentinel supervisor")
				return &cli.ExitError{Code: 1, Err: err}
			}

			mux := http.NewServeMux()
			handler := sentinel.NewAPIHandler(supervisor)
			mux.Handle("/v1/sentinel/status", handler)
			mux.Handle("/v1/sentinel/events", handler)
			mux.Handle("/v1/sentinel/restart", handler)
			mux.Handle("/v1/sentinel/pause", handler)
			mux.Handle("/v1/sentinel/resume", handler)
			auth := serverauth.NewMiddleware(serverauth.Options{
				Mode:        cfg.APIAuthMode,
				BearerToken: cfg.APIAuthToken,
			}, io.Discard)
			server := &http.Server{Addr: cfg.APIAddr, Handler: auth(mux)}

			go func() {
				<-ctx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = supervisor.Close(shutdownCtx)
				_ = server.Shutdown(shutdownCtx)
			}()

			log.Info().
				Str("addr", cfg.APIAddr).
				Str("target_command", cfg.TargetCommand).
				Msg("cased api server started")
			if _, err := fmt.Fprintf(stdout, "cased api serving on %s\n", cfg.APIAddr); err != nil {
				return &cli.ExitError{Code: 1, Err: err}
			}
			if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error().Err(err).Msg("failed to serve cased api")
				return &cli.ExitError{Code: 1, Err: err}
			}
			return nil
		},
	}
	cmd.SetOut(stdout)
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "path to cased config file")
	cmd.Flags().StringVar(&opts.APIAddr, "api-addr", "", "cased api listen address override")
	cmd.Flags().BoolVar(&opts.Verbose, "verbose", false, "enable verbose debug logging")
	return cmd, opts
}
