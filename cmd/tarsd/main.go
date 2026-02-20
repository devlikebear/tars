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
	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/heartbeat"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/mcp"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/serverauth"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/devlikebear/tarsncase/internal/tool"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	loadRuntimeEnvFiles(".env", ".env.secret")
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

## Automation Tool Policy
- If the user asks about cron jobs managed by this app, call cron (preferred) or cron_list / cron_get / cron_runs / cron_create / cron_update / cron_delete / cron_run instead of OS commands like crontab.
- If the user asks about heartbeat status or asks to trigger heartbeat, call heartbeat (preferred) or heartbeat_status / heartbeat_run_once instead of inferring from process or file guesses.

## Runtime Tool Policy
- For async background agent tasks across sessions, use sessions_spawn and sessions_runs.
- For channel or gateway runtime operations, use message / gateway tools when available.
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

			resolvedConfigPath := config.ResolveTarsdConfigPath(opts.ConfigPath)
			cfg, err := config.Load(resolvedConfigPath)
			if err != nil {
				logger.Error().Err(err).Msg("failed to load config")
				return &cli.ExitError{Code: 1, Err: err}
			}
			if strings.TrimSpace(resolvedConfigPath) != "" {
				logger.Debug().Str("config_path", resolvedConfigPath).Msg("resolved config file")
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
			sessionStore := session.NewStore(cfg.WorkspaceDir)
			sessionStoreResolver := newWorkspaceSessionStoreResolver(cfg.WorkspaceDir, sessionStore)

			var ask heartbeat.AskFunc
			var runPrompt func(ctx context.Context, runLabel string, prompt string) (string, error)
			var runPromptWithTools gatewayPromptRunner
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
				runPrompt = newAgentPromptRunner(cfg.WorkspaceDir, llmClient, cfg.AgentMaxIterations, logger)
				runPromptWithTools = newAgentPromptRunnerWithTools(cfg.WorkspaceDir, llmClient, cfg.AgentMaxIterations, logger)
				ask = newAgentAskFunc(cfg.WorkspaceDir, llmClient, cfg.AgentMaxIterations, logger)
				logger.Debug().
					Str("provider", cfg.LLMProvider).
					Str("auth_mode", cfg.LLMAuthMode).
					Str("model", cfg.LLMModel).
					Str("base_url", cfg.LLMBaseURL).
					Msg("llm provider initialized")
			}

			if opts.ServeAPI {
				cronStore := cron.NewStoreWithOptions(cfg.WorkspaceDir, cron.StoreOptions{
					RunHistoryLimit: cfg.CronRunHistoryLimit,
				})
				cronStoreResolver := newWorkspaceCronStoreResolver(cfg.WorkspaceDir, cfg.CronRunHistoryLimit, cronStore)
				activity := &runtimeActivity{}
				heartbeatState := newHeartbeatWorkspaceState()
				heartbeatPolicyForWorkspace := func(workspaceID string) heartbeat.Policy {
					return buildHeartbeatPolicy(
						sessionStoreResolver(workspaceID),
						cfg.HeartbeatActiveHours,
						cfg.HeartbeatTimezone,
						activity,
					)
				}
				broker := newEventBroker()
				dispatcher := newNotificationDispatcher(
					broker,
					newCommandNotifier(cfg.NotifyCommand, logger),
					cfg.NotifyWhenNoClients,
					logger,
				)
				heartbeatRunner := newWorkspaceHeartbeatRunnerWithNotify(
					cfg.WorkspaceDir,
					nowFn,
					ask,
					heartbeatPolicyForWorkspace,
					heartbeatState,
					dispatcher.Emit,
				)
				cronRunner := newCronJobRunnerWithNotify(
					cfg.WorkspaceDir,
					sessionStore,
					runPrompt,
					logger,
					dispatcher.Emit,
				)
				mux := http.NewServeMux()
				heartbeatHandler := newHeartbeatAPIHandlerWithRunner(heartbeatRunner, logger)
				mux.Handle("/v1/heartbeat/", heartbeatHandler)
				processManager := tool.NewProcessManager()
				mcpClient := mcp.NewClient(cfg.MCPServers)
				extensionsManager, err := buildExtensionsManager(cfg, mcpClient)
				if err != nil {
					logger.Error().Err(err).Msg("failed to initialize extensions manager")
					return &cli.ExitError{Code: 1, Err: err}
				}
				vaultReader, vaultStatus, vaultErr := buildVaultReader(cfg)
				if vaultErr != nil {
					logger.Warn().Err(vaultErr).Msg("vault client initialization failed; browser auto-login will be unavailable")
				}
				relayServer, err := buildBrowserRelay(cfg)
				if err != nil {
					logger.Error().Err(err).Msg("failed to initialize browser relay")
					return &cli.ExitError{Code: 1, Err: err}
				}
				browserService := buildBrowserService(cfg, relayServer, vaultReader)
				gatewayRuntime := gateway.NewRuntime(gateway.RuntimeOptions{
					Enabled:                              cfg.GatewayEnabled,
					WorkspaceDir:                         cfg.WorkspaceDir,
					SessionStore:                         sessionStore,
					SessionStoreForWorkspace:             sessionStoreResolver,
					RunPrompt:                            runPrompt,
					Executors:                            nil,
					DefaultAgent:                         strings.TrimSpace(cfg.GatewayDefaultAgent),
					GatewayAgentsWatchEnabled:            false,
					ChannelsLocalEnabled:                 cfg.ChannelsLocalEnabled,
					ChannelsWebhookEnabled:               cfg.ChannelsWebhookEnabled,
					ChannelsTelegramEnabled:              cfg.ChannelsTelegramEnabled,
					GatewayPersistenceEnabled:            cfg.GatewayPersistenceEnabled,
					GatewayRunsPersistenceEnabled:        cfg.GatewayRunsPersistenceEnabled,
					GatewayChannelsPersistenceEnabled:    cfg.GatewayChannelsPersistenceEnabled,
					GatewayRunsMaxRecords:                cfg.GatewayRunsMaxRecords,
					GatewayChannelsMaxMessagesPerChannel: cfg.GatewayChannelsMaxMessagesPerChannel,
					GatewayPersistenceDir:                cfg.GatewayPersistenceDir,
					GatewayRestoreOnStartup:              cfg.GatewayRestoreOnStartup,
					GatewayReportSummaryEnabled:          cfg.GatewayReportSummaryEnabled,
					GatewayArchiveEnabled:                cfg.GatewayArchiveEnabled,
					GatewayArchiveDir:                    cfg.GatewayArchiveDir,
					GatewayArchiveRetentionDays:          cfg.GatewayArchiveRetentionDays,
					GatewayArchiveMaxFileBytes:           cfg.GatewayArchiveMaxFileBytes,
					BrowserDefaultProfile:                cfg.BrowserDefaultProfile,
					BrowserManagedUserDataDir:            cfg.BrowserManagedUserDataDir,
					BrowserSiteFlowsDir:                  cfg.BrowserSiteFlowsDir,
					BrowserAutoLoginSiteAllowlist:        cfg.BrowserAutoLoginSiteAllowlist,
					BrowserVaultReader:                   vaultReader,
					BrowserService:                       browserService,
					Now:                                  nowFn,
				})
				refreshGatewayExecutors := func(reason string) int {
					executors := buildGatewayExecutors(cfg, runPromptWithTools, logger)
					gatewayRuntime.SetExecutors(executors, strings.TrimSpace(cfg.GatewayDefaultAgent))
					agents := len(gatewayRuntime.Agents())
					logger.Debug().Str("reason", reason).Int("gateway_agents", agents).Msg("gateway executors refreshed")
					return agents
				}
				_ = refreshGatewayExecutors("startup")
				chatTooling := buildChatToolingOptions(processManager, extensionsManager, gatewayRuntime)
				chatTooling.AutomationToolsForWorkspace = func(workspaceID string) []tool.Tool {
					resolvedStore, err := cronStoreResolver.Resolve(workspaceID)
					if err != nil {
						logger.Warn().Err(err).Str("workspace_id", normalizeWorkspaceID(workspaceID)).Msg("resolve workspace cron store failed for chat tools")
						resolvedStore = cronStore
					}
					return buildAutomationTools(
						resolvedStore,
						cronRunner,
						heartbeatRunner,
						func(ctx context.Context) (tool.HeartbeatStatus, error) {
							targetWorkspaceID := normalizeWorkspaceID(serverauth.WorkspaceIDFromContext(ctx))
							if targetWorkspaceID == defaultWorkspaceID {
								targetWorkspaceID = normalizeWorkspaceID(workspaceID)
							}
							return heartbeatState.snapshot(
								targetWorkspaceID,
								ask != nil,
								cfg.HeartbeatActiveHours,
								cfg.HeartbeatTimezone,
								activity.isChatBusy(),
							), nil
						},
						nowFn,
					)
				}
				chatTools := buildOptionalChatTools(cfg, gatewayRuntime)
				chatHandler := newChatAPIHandlerWithRuntimeConfig(
					cfg.WorkspaceDir,
					sessionStore,
					llmClient,
					logger,
					cfg.AgentMaxIterations,
					activity,
					chatTooling,
					chatTools...,
				)
				mux.Handle("/v1/chat", chatHandler)
				sessionHandler := newSessionAPIHandler(sessionStore, logger)
				mux.Handle("/v1/sessions", sessionHandler)
				mux.Handle("/v1/sessions/", sessionHandler)
				statusHandler := newStatusAPIHandler(cfg.WorkspaceDir, sessionStore, logger)
				mux.Handle("/v1/status", statusHandler)
				authHandler := newAuthAPIHandler(cfg.APIAuthMode)
				mux.Handle("/v1/auth/whoami", authHandler)
				mux.Handle("/v1/healthz", newHealthzAPIHandler(nowFn))
				compactHandler := newCompactAPIHandler(cfg.WorkspaceDir, sessionStore, llmClient, logger)
				mux.Handle("/v1/compact", compactHandler)
				cronHandler := newCronAPIHandlerWithRunnerAndResolver(cronStoreResolver, cronRunner, logger)
				mux.Handle("/v1/cron/jobs", cronHandler)
				mux.Handle("/v1/cron/jobs/", cronHandler)
				mcpHandler := newMCPAPIHandler(mcpClient, logger)
				mux.Handle("/v1/mcp/servers", mcpHandler)
				mux.Handle("/v1/mcp/tools", mcpHandler)
				extensionsHandler := newExtensionsAPIHandler(extensionsManager, logger, func() (bool, int) {
					if gatewayRuntime == nil {
						return false, 0
					}
					return true, refreshGatewayExecutors("extensions_reload")
				})
				mux.Handle("/v1/skills", extensionsHandler)
				mux.Handle("/v1/skills/", extensionsHandler)
				mux.Handle("/v1/plugins", extensionsHandler)
				mux.Handle("/v1/runtime/extensions/reload", extensionsHandler)
				agentRunsHandler := newAgentRunsAPIHandler(gatewayRuntime, logger)
				mux.Handle("/v1/agent/agents", agentRunsHandler)
				mux.Handle("/v1/agent/runs", agentRunsHandler)
				mux.Handle("/v1/agent/runs/", agentRunsHandler)
				gatewayHandler := newGatewayAPIHandler(gatewayRuntime, logger, func() {
					_ = refreshGatewayExecutors("gateway_reload")
				})
				mux.Handle("/v1/gateway/status", gatewayHandler)
				mux.Handle("/v1/gateway/reload", gatewayHandler)
				mux.Handle("/v1/gateway/restart", gatewayHandler)
				mux.Handle("/v1/gateway/reports/summary", gatewayHandler)
				mux.Handle("/v1/gateway/reports/runs", gatewayHandler)
				mux.Handle("/v1/gateway/reports/channels", gatewayHandler)
				browserHandler := newBrowserAPIHandler(gatewayRuntime, vaultStatus, logger)
				mux.Handle("/v1/browser/status", browserHandler)
				mux.Handle("/v1/browser/profiles", browserHandler)
				mux.Handle("/v1/browser/login", browserHandler)
				mux.Handle("/v1/browser/check", browserHandler)
				mux.Handle("/v1/browser/run", browserHandler)
				mux.Handle("/v1/vault/status", browserHandler)
				channelsHandler := newChannelsAPIHandler(gatewayRuntime, logger)
				mux.Handle("/v1/channels/webhook/inbound/", channelsHandler)
				mux.Handle("/v1/channels/telegram/webhook/", channelsHandler)
				eventsHandler := newEventStreamHandler(broker, logger)
				mux.Handle("/v1/events/stream", eventsHandler)

				server := &http.Server{
					Addr:    opts.APIAddr,
					Handler: applyAPIMiddleware(cfg, logger, mux, stderr),
				}

				ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
				defer stop()
				gatewayRuntime.SetAgentsWatchEnabled(false)
				gatewayAgentsWatch := newGatewayAgentsWatcher(gatewayAgentsWatcherOptions{
					WorkspaceDir: cfg.WorkspaceDir,
					Debounce:     time.Duration(cfg.GatewayAgentsWatchDebounceMS) * time.Millisecond,
					Logger:       logger,
					Refresh: func(reason string) {
						_ = refreshGatewayExecutors(reason)
					},
				})

				if cfg.GatewayEnabled && cfg.GatewayAgentsWatch {
					started, watchErr := gatewayAgentsWatch.Start(ctx)
					if watchErr != nil {
						logger.Warn().Err(watchErr).Msg("gateway agents watcher start failed")
					}
					gatewayRuntime.SetAgentsWatchEnabled(started)
					if started {
						logger.Info().Int("debounce_ms", cfg.GatewayAgentsWatchDebounceMS).Msg("gateway agents watcher started")
					} else {
						logger.Debug().Msg("gateway agents watcher skipped (workspace agents dir not found)")
					}
				}
				if relayServer != nil {
					if err := relayServer.Start(ctx); err != nil {
						logger.Warn().Err(err).Msg("browser relay start failed")
					} else {
						logger.Info().Str("addr", relayServer.Addr()).Msg("browser relay started")
					}
				}

				go func() {
					<-ctx.Done()
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					extensionsManager.Close()
					gatewayAgentsWatch.Close()
					if relayServer != nil {
						_ = relayServer.Close(shutdownCtx)
					}
					if gatewayRuntime != nil {
						_ = gatewayRuntime.Close(shutdownCtx)
					}
					_ = server.Shutdown(shutdownCtx)
				}()
				if err := extensionsManager.Start(ctx); err != nil {
					logger.Error().Err(err).Msg("failed to start extensions manager")
					return &cli.ExitError{Code: 1, Err: err}
				}
				cronManager := newWorkspaceCronManager(cronStoreResolver, cronRunner, 30*time.Second, nowFn, logger)
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
				runPolicy := buildHeartbeatPolicy(sessionStore, cfg.HeartbeatActiveHours, cfg.HeartbeatTimezone, nil)
				if _, err := heartbeat.RunOnceWithLLMResultWithPolicy(ctx, cfg.WorkspaceDir, nowFn(), ask, runPolicy); err != nil {
					logger.Error().Err(err).Msg("failed to run heartbeat once")
					return &cli.ExitError{Code: 1, Err: err}
				}
				logger.Info().Msg("heartbeat run-once complete")
			}
			if opts.RunLoop {
				runPolicy := buildHeartbeatPolicy(sessionStore, cfg.HeartbeatActiveHours, cfg.HeartbeatTimezone, nil)
				count, err := heartbeat.RunLoopWithLLMWithPolicy(
					context.Background(),
					cfg.WorkspaceDir,
					opts.HeartbeatInterval,
					opts.MaxHeartbeats,
					nowFn,
					ask,
					runPolicy,
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
	cmd.Flags().StringVar(&opts.APIAddr, "api-addr", "127.0.0.1:43180", "http api listen address")
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
