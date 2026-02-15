package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/devlikebear/tarsncase/internal/cli"
	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/heartbeat"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/mcp"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/devlikebear/tarsncase/internal/tool"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	_ = godotenv.Load(".env")
	opts := &options{
		LogFile: flagValue(args, "--log-file"),
	}

	consoleWriter := zerolog.ConsoleWriter{
		Out:        stderr,
		TimeFormat: "15:04:05",
		NoColor:    false,
	}
	logWriter := io.Writer(consoleWriter)

	var logFile *os.File
	var logFileErr error
	if opts.LogFile != "" {
		logFile, logFileErr = os.OpenFile(opts.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if logFileErr == nil {
			logWriter = zerolog.MultiLevelWriter(consoleWriter, logFile)
			defer logFile.Close()
		}
	}

	logger := zerolog.New(logWriter).With().Timestamp().Str("component", "tarsd").Logger()
	zlog.Logger = logger
	if logFileErr != nil {
		logger.Error().
			Err(logFileErr).
			Str("path", opts.LogFile).
			Msg("failed to open log file; using console logging only")
	}

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

type options struct {
	ConfigPath        string
	Mode              string
	WorkspaceDir      string
	LogFile           string
	Verbose           bool
	RunOnce           bool
	RunLoop           bool
	ServeAPI          bool
	APIAddr           string
	HeartbeatInterval time.Duration
	MaxHeartbeats     int
}

const (
	chatHistoryMaxTokens     = 120000
	autoCompactTriggerTokens = 100000
	autoCompactKeepRecent    = 0
	autoCompactKeepTokens    = session.DefaultKeepRecentTokens
)

const memoryToolSystemRule = `
## Memory Tool Policy
- If the user asks about past facts, preferences, prior chat context, or "what you remember", you must call memory_search and/or memory_get before answering.
- Do not guess memory-backed facts without first checking tools.
- Tool-call arguments must be valid JSON.
`

func newRootCmd(opts *options, stdout, stderr io.Writer, nowFn func() time.Time) (*cobra.Command, *options) {
	if opts == nil {
		opts = &options{}
	}

	cmd := &cobra.Command{
		Use:           "tarsd",
		Short:         "Main daemon for TARS",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, _ []string) error {
			logger := zlog.Logger
			if opts.Verbose {
				logger = logger.Level(zerolog.DebugLevel)
				zlog.Logger = logger
				logger.Debug().Msg("verbose logging enabled")
			}

			if opts.RunOnce && opts.RunLoop {
				return &cli.ExitError{Code: 2, Err: fmt.Errorf("--run-once and --run-loop are mutually exclusive")}
			}

			cfg, err := config.Load(opts.ConfigPath)
			if err != nil {
				logger.Error().Err(err).Msg("failed to load config")
				return &cli.ExitError{Code: 1, Err: err}
			}

			if opts.Mode != "" {
				cfg.Mode = opts.Mode
			}
			if opts.WorkspaceDir != "" {
				cfg.WorkspaceDir = opts.WorkspaceDir
			}

			if err := memory.EnsureWorkspace(cfg.WorkspaceDir); err != nil {
				logger.Error().Err(err).Msg("failed to initialize workspace")
				return &cli.ExitError{Code: 1, Err: err}
			}
			if err := memory.AppendDailyLog(cfg.WorkspaceDir, nowFn(), "tarsd startup complete"); err != nil {
				logger.Error().Err(err).Msg("failed to write daily log")
				return &cli.ExitError{Code: 1, Err: err}
			}

			var ask heartbeat.AskFunc
			var llmClient llm.Client
			needLLM := opts.RunOnce || opts.RunLoop || opts.ServeAPI
			if needLLM {
				client, err := llm.NewProvider(llm.ProviderOptions{
					Provider:      cfg.LLMProvider,
					AuthMode:      cfg.LLMAuthMode,
					OAuthProvider: cfg.LLMOAuthProvider,
					BaseURL:       cfg.LLMBaseURL,
					Model:         cfg.LLMModel,
					APIKey:        cfg.LLMAPIKey,
				})
				if err != nil {
					logger.Error().Err(err).Msg("failed to initialize llm provider")
					return &cli.ExitError{Code: 1, Err: err}
				}
				llmClient = client
				ask = newAgentAskFunc(cfg.WorkspaceDir, llmClient, cfg.AgentMaxIterations, logger)
				logger.Debug().
					Str("provider", cfg.LLMProvider).
					Str("auth_mode", cfg.LLMAuthMode).
					Str("model", cfg.LLMModel).
					Str("base_url", cfg.LLMBaseURL).
					Msg("llm provider initialized")
			}

			if opts.ServeAPI {
				store := session.NewStore(cfg.WorkspaceDir)
				cronStore := cron.NewStore(cfg.WorkspaceDir)
				var mcpClient *mcp.Client
				var mcpTools []tool.Tool
				if len(cfg.MCPServers) > 0 {
					mcpClient = mcp.NewClient(cfg.MCPServers)
					buildCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					tools, err := mcpClient.BuildTools(buildCtx)
					cancel()
					if err != nil {
						logger.Warn().Err(err).Msg("mcp tool discovery failed")
					} else {
						mcpTools = tools
						logger.Info().Int("mcp_tool_count", len(mcpTools)).Msg("mcp tools discovered")
					}
				}

				mux := http.NewServeMux()
				heartbeatHandler := newHeartbeatAPIHandler(cfg.WorkspaceDir, nowFn, ask, logger)
				mux.Handle("/v1/heartbeat/", heartbeatHandler)
				chatHandler := newChatAPIHandlerWithOptions(
					cfg.WorkspaceDir,
					store,
					llmClient,
					logger,
					cfg.AgentMaxIterations,
					mcpTools...,
				)
				mux.Handle("/v1/chat", chatHandler)
				sessionHandler := newSessionAPIHandler(store, logger)
				mux.Handle("/v1/sessions", sessionHandler)
				mux.Handle("/v1/sessions/", sessionHandler)
				statusHandler := newStatusAPIHandler(cfg.WorkspaceDir, store, logger)
				mux.Handle("/v1/status", statusHandler)
				compactHandler := newCompactAPIHandler(cfg.WorkspaceDir, store, llmClient, logger)
				mux.Handle("/v1/compact", compactHandler)
				cronHandler := newCronAPIHandler(cronStore, ask, logger)
				mux.Handle("/v1/cron/jobs", cronHandler)
				mux.Handle("/v1/cron/jobs/", cronHandler)
				mcpHandler := newMCPAPIHandler(mcpClient, logger)
				mux.Handle("/v1/mcp/servers", mcpHandler)
				mux.Handle("/v1/mcp/tools", mcpHandler)

				server := &http.Server{
					Addr:    opts.APIAddr,
					Handler: requestDebugMiddleware(logger, mux),
				}

				ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
				defer stop()

				go func() {
					<-ctx.Done()
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_ = server.Shutdown(shutdownCtx)
				}()
				cronManager := cron.NewManager(cronStore, ask, 30*time.Second, nowFn)
				go func() {
					if err := cronManager.Start(ctx); err != nil {
						logger.Error().Err(err).Msg("cron manager stopped with error")
					}
				}()

				logger.Info().Str("addr", opts.APIAddr).Msg("tarsd api server started")
				if _, err := fmt.Fprintf(stdout, "tarsd api serving on %s\n", opts.APIAddr); err != nil {
					return &cli.ExitError{Code: 1, Err: err}
				}
				if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Error().Err(err).Msg("failed to serve api")
					return &cli.ExitError{Code: 1, Err: err}
				}
				return nil
			}

			if opts.RunOnce {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := heartbeat.RunOnceWithLLM(ctx, cfg.WorkspaceDir, nowFn(), ask); err != nil {
					logger.Error().Err(err).Msg("failed to run heartbeat once")
					return &cli.ExitError{Code: 1, Err: err}
				}
				logger.Info().Msg("heartbeat run-once complete")
			}
			if opts.RunLoop {
				count, err := heartbeat.RunLoopWithLLM(
					context.Background(),
					cfg.WorkspaceDir,
					opts.HeartbeatInterval,
					opts.MaxHeartbeats,
					nowFn,
					ask,
				)
				if err != nil {
					logger.Error().Err(err).Msg("failed to run heartbeat loop")
					return &cli.ExitError{Code: 1, Err: err}
				}
				logger.Info().Int("heartbeat_count", count).Msg("heartbeat run-loop complete")
			}

			logger.Info().
				Str("mode", cfg.Mode).
				Str("workspace_dir", cfg.WorkspaceDir).
				Msg("tarsd startup complete")

			fmt.Fprintf(stdout, "tarsd starting in %s mode\n", cfg.Mode)
			return nil
		},
	}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "path to config file")
	cmd.Flags().StringVar(&opts.Mode, "mode", "", "runtime mode override")
	cmd.Flags().StringVar(&opts.WorkspaceDir, "workspace-dir", "", "workspace directory override")
	cmd.Flags().StringVar(&opts.LogFile, "log-file", opts.LogFile, "append json logs to file")
	cmd.Flags().BoolVar(&opts.Verbose, "verbose", false, "enable verbose debug logging")
	cmd.Flags().BoolVar(&opts.RunOnce, "run-once", false, "run heartbeat once and exit")
	cmd.Flags().BoolVar(&opts.RunLoop, "run-loop", false, "run heartbeat loop")
	cmd.Flags().BoolVar(&opts.ServeAPI, "serve-api", false, "serve tarsd http api")
	cmd.Flags().StringVar(&opts.APIAddr, "api-addr", "127.0.0.1:18080", "http api listen address")
	cmd.Flags().DurationVar(&opts.HeartbeatInterval, "heartbeat-interval", 30*time.Minute, "heartbeat interval (e.g. 30m, 5s)")
	cmd.Flags().IntVar(&opts.MaxHeartbeats, "max-heartbeats", 0, "maximum heartbeat count in loop (0 means unlimited)")

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
