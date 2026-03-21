package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/cli"
	"github.com/devlikebear/tars/internal/envloader"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Serve executes tars runtime directly with structured options.
func Serve(ctx context.Context, serveOpts ServeOptions, stdout, stderr io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	envloader.Load(".env", ".env.secret")

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

	logger, cleanup := setupRuntimeLogger(loggerConfig{
		FilePath: opts.LogFile,
	}, stderr)
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

func applyOptionDefaults(opts *options) {
	if opts == nil {
		return
	}
	if strings.TrimSpace(opts.APIAddr) == "" {
		opts.APIAddr = DefaultAPIAddr
	}
	if opts.HeartbeatInterval <= 0 {
		opts.HeartbeatInterval = DefaultHeartbeatInterval
	}
}

type loggerConfig struct {
	FilePath       string
	Level          string
	RotateMaxSizeMB  int
	RotateMaxDays    int
	RotateMaxBackups int
}

func setupRuntimeLogger(cfg loggerConfig, stderr io.Writer) (zerolog.Logger, func()) {
	consoleWriter := zerolog.ConsoleWriter{
		Out:        stderr,
		TimeFormat: "15:04:05",
		NoColor:    false,
	}
	logWriter := io.Writer(consoleWriter)

	trimmedLogPath := strings.TrimSpace(cfg.FilePath)
	// If the path looks like a directory (ends with /), append default filename.
	if trimmedLogPath != "" && strings.HasSuffix(trimmedLogPath, "/") {
		trimmedLogPath = trimmedLogPath + "tars.log"
	}

	var closers []func()
	if trimmedLogPath != "" {
		// Ensure parent directory exists before lumberjack opens the file.
		if dir := filepath.Dir(trimmedLogPath); dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0o755)
		}
		maxSize := cfg.RotateMaxSizeMB
		if maxSize <= 0 {
			maxSize = 100 // default 100MB
		}
		maxDays := cfg.RotateMaxDays
		if maxDays <= 0 {
			maxDays = 30 // default 30 days
		}
		maxBackups := cfg.RotateMaxBackups
		if maxBackups <= 0 {
			maxBackups = 5 // default 5 backups
		}
		lj := &lumberjack.Logger{
			Filename:   trimmedLogPath,
			MaxSize:    maxSize,
			MaxAge:     maxDays,
			MaxBackups: maxBackups,
			LocalTime:  true,
			Compress:   true,
		}
		logWriter = zerolog.MultiLevelWriter(consoleWriter, lj)
		closers = append(closers, func() { _ = lj.Close() })
	}

	level := parseLogLevel(cfg.Level)
	logger := zerolog.New(logWriter).With().Timestamp().Str("component", "tars").Logger().Level(level)

	cleanup := func() {
		for _, fn := range closers {
			fn()
		}
	}
	return logger, cleanup
}

func parseLogLevel(s string) zerolog.Level {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.DebugLevel
	}
}
