package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/devlikebear/tarsncase/internal/browserrelay"
	"github.com/devlikebear/tarsncase/internal/cli"
	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/extensions"
	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/heartbeat"
	"github.com/devlikebear/tarsncase/internal/mcp"
	"github.com/devlikebear/tarsncase/internal/tool"
	"github.com/rs/zerolog"
)

type serveAPIRuntime struct {
	cfg                config.Config
	server             *http.Server
	extensionsManager  *extensions.Manager
	gatewayRuntime     *gateway.Runtime
	gatewayAgentsWatch *gatewayAgentsWatcher
	relayServer        *browserrelay.Server
	cronManager        *workspaceCronManager
}

func runServeAPICommand(
	parentCtx context.Context,
	opts *options,
	deps runtimeDeps,
	nowFn func() time.Time,
	stdout io.Writer,
	stderr io.Writer,
	logger zerolog.Logger,
) error {
	apiRuntime, err := buildAPIMux(opts, deps, nowFn, logger, stderr)
	if err != nil {
		return &cli.ExitError{Code: 1, Err: err}
	}

	ctx, stop := signal.NotifyContext(parentCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := startBackgrounds(ctx, apiRuntime, logger); err != nil {
		logger.Error().Err(err).Msg("failed to start background runtimes")
		return &cli.ExitError{Code: 1, Err: err}
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownRuntime(shutdownCtx, apiRuntime)
	}()

	logger.Info().Str("addr", opts.APIAddr).Msg("tars api server started")
	if _, err := fmt.Fprintf(stdout, "tars api serving on %s\n", opts.APIAddr); err != nil {
		return &cli.ExitError{Code: 1, Err: err}
	}
	if err := apiRuntime.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error().Err(err).Msg("failed to serve api")
		return &cli.ExitError{Code: 1, Err: err}
	}
	return nil
}

func buildAPIMux(
	opts *options,
	deps runtimeDeps,
	nowFn func() time.Time,
	logger zerolog.Logger,
	stderr io.Writer,
) (*serveAPIRuntime, error) {
	cfg := deps.cfg
	sessionStore := deps.sessionStore
	sessionStoreResolver := deps.sessionStoreResolver

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
	notificationStore, err := newNotificationStore(
		filepath.Join(strings.TrimSpace(cfg.GatewayPersistenceDir), "notifications.json"),
		notificationHistoryMax,
	)
	if err != nil {
		return nil, err
	}
	dispatcher := newNotificationDispatcher(
		broker,
		newCommandNotifier(cfg.NotifyCommand, logger),
		cfg.NotifyWhenNoClients,
		logger,
	)
	dispatcher.store = notificationStore
	heartbeatRunner := newWorkspaceHeartbeatRunnerWithNotify(
		cfg.WorkspaceDir,
		nowFn,
		deps.ask,
		heartbeatPolicyForWorkspace,
		heartbeatState,
		dispatcher.Emit,
	)
	cronRunner := newCronJobRunnerWithNotify(
		cfg.WorkspaceDir,
		sessionStore,
		deps.runPrompt,
		logger,
		dispatcher.Emit,
	)

	mux := http.NewServeMux()
	mux.Handle("/v1/heartbeat/", newHeartbeatAPIHandlerWithRunner(heartbeatRunner, logger))

	processManager := tool.NewProcessManager()
	mcpClient := mcp.NewClient(cfg.MCPServers)
	extensionsManager, err := buildExtensionsManager(cfg, mcpClient)
	if err != nil {
		return nil, err
	}
	vaultReader, vaultStatus, vaultErr := buildVaultReader(cfg)
	if vaultErr != nil {
		logger.Warn().Err(vaultErr).Msg("vault client initialization failed; browser auto-login will be unavailable")
	}
	relayServer, err := buildBrowserRelay(cfg)
	if err != nil {
		return nil, err
	}
	browserService := buildBrowserService(cfg, relayServer, vaultReader)
	gatewayRuntime := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:                              cfg.GatewayEnabled,
		WorkspaceDir:                         cfg.WorkspaceDir,
		SessionStore:                         sessionStore,
		SessionStoreForWorkspace:             sessionStoreResolver,
		RunPrompt:                            deps.runPrompt,
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
		executors := buildGatewayExecutors(cfg, deps.runPromptWithTools, logger)
		gatewayRuntime.SetExecutors(executors, strings.TrimSpace(cfg.GatewayDefaultAgent))
		agents := len(gatewayRuntime.Agents())
		logger.Debug().Str("reason", reason).Int("gateway_agents", agents).Msg("gateway executors refreshed")
		return agents
	}
	_ = refreshGatewayExecutors("startup")

	chatTooling := buildChatToolingOptions(processManager, extensionsManager, gatewayRuntime)
	chatTooling.AutomationToolsForWorkspace = func(workspaceID string) []tool.Tool {
		resolvedStore, err := cronStoreResolver.Resolve(defaultWorkspaceID)
		if err != nil {
			logger.Warn().Err(err).Msg("resolve cron store failed for chat tools")
			resolvedStore = cronStore
		}
		return buildAutomationTools(
			resolvedStore,
			cronRunner,
			heartbeatRunner,
			func(ctx context.Context) (tool.HeartbeatStatus, error) {
				return heartbeatState.snapshot(
					defaultWorkspaceID,
					deps.ask != nil,
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
		deps.llmClient,
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
	mux.Handle("/v1/status", newStatusAPIHandler(cfg.WorkspaceDir, sessionStore, logger))
	mux.Handle("/v1/auth/whoami", newAuthAPIHandler(cfg.APIAuthMode))
	mux.Handle("/v1/healthz", newHealthzAPIHandler(nowFn))
	mux.Handle("/v1/compact", newCompactAPIHandler(cfg.WorkspaceDir, sessionStore, deps.llmClient, logger))
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
	channelsHandler := newChannelsAPIHandlerWithTelegramSender(gatewayRuntime, newTelegramSender(cfg.TelegramBotToken), logger)
	mux.Handle("/v1/channels/webhook/inbound/", channelsHandler)
	mux.Handle("/v1/channels/telegram/webhook/", channelsHandler)
	mux.Handle("/v1/channels/telegram/send", channelsHandler)
	eventsHandler := newEventsAPIHandler(broker, notificationStore, logger)
	mux.Handle("/v1/events/stream", eventsHandler)
	mux.Handle("/v1/events/history", eventsHandler)
	mux.Handle("/v1/events/read", eventsHandler)

	server := &http.Server{
		Addr:    opts.APIAddr,
		Handler: applyAPIMiddleware(cfg, logger, mux, stderr),
	}
	gatewayAgentsWatch := newGatewayAgentsWatcher(gatewayAgentsWatcherOptions{
		WorkspaceDir: cfg.WorkspaceDir,
		Debounce:     time.Duration(cfg.GatewayAgentsWatchDebounceMS) * time.Millisecond,
		Logger:       logger,
		Refresh: func(reason string) {
			_ = refreshGatewayExecutors(reason)
		},
	})
	cronManager := newWorkspaceCronManager(cronStoreResolver, cronRunner, 30*time.Second, nowFn, logger)

	return &serveAPIRuntime{
		cfg:                cfg,
		server:             server,
		extensionsManager:  extensionsManager,
		gatewayRuntime:     gatewayRuntime,
		gatewayAgentsWatch: gatewayAgentsWatch,
		relayServer:        relayServer,
		cronManager:        cronManager,
	}, nil
}

func startBackgrounds(ctx context.Context, runtime *serveAPIRuntime, logger zerolog.Logger) error {
	if runtime == nil {
		return fmt.Errorf("serve runtime is required")
	}
	cfg := runtime.cfg

	if runtime.gatewayRuntime != nil {
		runtime.gatewayRuntime.SetAgentsWatchEnabled(false)
	}
	if cfg.GatewayEnabled && cfg.GatewayAgentsWatch && runtime.gatewayRuntime != nil && runtime.gatewayAgentsWatch != nil {
		started, watchErr := runtime.gatewayAgentsWatch.Start(ctx)
		if watchErr != nil {
			logger.Warn().Err(watchErr).Msg("gateway agents watcher start failed")
		}
		runtime.gatewayRuntime.SetAgentsWatchEnabled(started)
		if started {
			logger.Info().Int("debounce_ms", cfg.GatewayAgentsWatchDebounceMS).Msg("gateway agents watcher started")
		} else {
			logger.Debug().Msg("gateway agents watcher skipped (workspace agents dir not found)")
		}
	}

	if runtime.relayServer != nil {
		if err := runtime.relayServer.Start(ctx); err != nil {
			logger.Warn().Err(err).Msg("browser relay start failed")
		} else {
			logger.Info().Str("addr", runtime.relayServer.Addr()).Msg("browser relay started")
		}
	}
	if runtime.extensionsManager != nil {
		if err := runtime.extensionsManager.Start(ctx); err != nil {
			return err
		}
	}
	if runtime.cronManager != nil {
		go func() {
			if err := runtime.cronManager.Start(ctx); err != nil {
				logger.Error().Err(err).Msg("cron manager stopped with error")
			}
		}()
	}
	return nil
}

func shutdownRuntime(ctx context.Context, runtime *serveAPIRuntime) {
	if runtime == nil {
		return
	}
	if runtime.extensionsManager != nil {
		runtime.extensionsManager.Close()
	}
	if runtime.gatewayAgentsWatch != nil {
		runtime.gatewayAgentsWatch.Close()
	}
	if runtime.relayServer != nil {
		_ = runtime.relayServer.Close(ctx)
	}
	if runtime.gatewayRuntime != nil {
		_ = runtime.gatewayRuntime.Close(ctx)
	}
	if runtime.server != nil {
		_ = runtime.server.Shutdown(ctx)
	}
}
