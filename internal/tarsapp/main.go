package tarsapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/cli"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

// Run executes tars runtime with the provided CLI arguments.
func Run(args []string, stdout, stderr io.Writer) int {
	return run(args, stdout, stderr)
}

// Serve executes tars runtime directly with structured options.
func Serve(ctx context.Context, serveOpts ServeOptions, stdout, stderr io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	loadRuntimeEnvFiles(".env", ".env.secret")

	opts := &options{
		ConfigPath:        strings.TrimSpace(serveOpts.ConfigPath),
		Mode:              strings.TrimSpace(serveOpts.Mode),
		WorkspaceDir:      strings.TrimSpace(serveOpts.WorkspaceDir),
		LogFile:           strings.TrimSpace(serveOpts.LogFile),
		Verbose:           serveOpts.Verbose,
		RunOnce:           serveOpts.RunOnce,
		RunLoop:           serveOpts.RunLoop,
		ServeAPI:          serveOpts.ServeAPI,
		APIAddr:           strings.TrimSpace(serveOpts.APIAddr),
		HeartbeatInterval: serveOpts.HeartbeatInterval,
		MaxHeartbeats:     serveOpts.MaxHeartbeats,
	}
	applyOptionDefaults(opts)

	logger, cleanup := setupRuntimeLogger(opts.LogFile, stderr)
	defer cleanup()
	zlog.Logger = logger

	cmd, _ := newRootCmd(opts, stdout, stderr, time.Now)
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		var ex *cli.ExitError
		if errors.As(err, &ex) {
			if ex.Err != nil {
				return ex.Err
			}
			return fmt.Errorf("serve exited with code %d", ex.Code)
		}
		return err
	}
	return nil
}

func run(args []string, stdout, stderr io.Writer) int {
	loadRuntimeEnvFiles(".env", ".env.secret")
	opts := &options{
		LogFile: flagValue(args, "--log-file"),
	}
	applyOptionDefaults(opts)

	logger, cleanup := setupRuntimeLogger(opts.LogFile, stderr)
	defer cleanup()
	zlog.Logger = logger

	cmd, opts := newRootCmd(opts, stdout, stderr, time.Now)
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

func applyOptionDefaults(opts *options) {
	if opts == nil {
		return
	}
	if strings.TrimSpace(opts.APIAddr) == "" {
		opts.APIAddr = "127.0.0.1:43180"
	}
	if opts.HeartbeatInterval <= 0 {
		opts.HeartbeatInterval = 30 * time.Minute
	}
}

func setupRuntimeLogger(logFilePath string, stderr io.Writer) (zerolog.Logger, func()) {
	consoleWriter := zerolog.ConsoleWriter{
		Out:        stderr,
		TimeFormat: "15:04:05",
		NoColor:    false,
	}
	logWriter := io.Writer(consoleWriter)

	var logFile *os.File
	var logFileErr error
	if strings.TrimSpace(logFilePath) != "" {
		logFile, logFileErr = os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if logFileErr == nil {
			logWriter = zerolog.MultiLevelWriter(consoleWriter, logFile)
		}
	}

	logger := zerolog.New(logWriter).With().Timestamp().Str("component", "tars").Logger()
	if logFileErr != nil {
		logger.Error().
			Err(logFileErr).
			Str("path", strings.TrimSpace(logFilePath)).
			Msg("failed to open log file; using console logging only")
	}

	cleanup := func() {
		if logFile != nil {
			_ = logFile.Close()
		}
	}
	return logger, cleanup
}
