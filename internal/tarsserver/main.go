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
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/envloader"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Serve executes tars runtime directly with structured options.
//
// Bootstrap order: load config first (panic on failure — there is no
// recoverable mode without a workspace config), derive the final logger
// config from CLI overrides + config values, then install the runtime
// logger exactly once. This avoids the previous two-phase setup where a
// CLI-only logger handle leaked when config values triggered a second
// setupRuntimeLogger call.
func Serve(ctx context.Context, serveOpts ServeOptions, stdout, stderr io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	envloader.Load(".env", ".env.secret")

	opts := &options{
		ConfigPath:   strings.TrimSpace(serveOpts.ConfigPath),
		WorkspaceDir: strings.TrimSpace(serveOpts.WorkspaceDir),
		LogFile:      strings.TrimSpace(serveOpts.LogFile),
		Verbose:      serveOpts.Verbose,
		ConfigCheck:  serveOpts.ConfigCheck,
		APIAddr:      strings.TrimSpace(serveOpts.APIAddr),
	}
	applyOptionDefaults(opts)

	cfg, err := loadConfigForServe(opts)
	if err != nil {
		panic(fmt.Sprintf("tars: load config: %v", err))
	}

	logger, cleanup := setupRuntimeLogger(buildLoggerConfig(opts, cfg), stderr)
	defer cleanup()
	zlog.Logger = logger

	cmd, _ := newRootCmd(opts, cfg, stdout, stderr, time.Now)
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

// buildLoggerConfig merges CLI flags and config values into the final
// loggerConfig. Precedence: --verbose forces debug level (highest);
// otherwise config.LogLevel wins when set; LogFile is config-then-CLI
// (config takes precedence to match the previous behavior).
func buildLoggerConfig(opts *options, cfg config.Config) loggerConfig {
	logCfg := loggerConfig{FilePath: opts.LogFile}
	if path := strings.TrimSpace(cfg.LogFile); path != "" {
		logCfg.FilePath = path
	}
	if level := strings.TrimSpace(cfg.LogLevel); level != "" {
		logCfg.Level = level
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
	if opts.Verbose {
		logCfg.Level = "debug"
	}
	return logCfg
}

func applyOptionDefaults(opts *options) {
	if opts == nil {
		return
	}
	if strings.TrimSpace(opts.APIAddr) == "" {
		opts.APIAddr = DefaultAPIAddr
	}
}

type loggerConfig struct {
	FilePath         string
	Level            string
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
