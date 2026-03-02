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

	"github.com/devlikebear/tarsncase/internal/approval"
	"github.com/devlikebear/tarsncase/internal/browserrelay"
	"github.com/devlikebear/tarsncase/internal/cli"
	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/extensions"
	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/heartbeat"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/mcp"
	"github.com/devlikebear/tarsncase/internal/ops"
	"github.com/devlikebear/tarsncase/internal/project"
	"github.com/devlikebear/tarsncase/internal/research"
	"github.com/devlikebear/tarsncase/internal/schedule"
	"github.com/devlikebear/tarsncase/internal/tool"
	"github.com/devlikebear/tarsncase/internal/usage"
	"github.com/rs/zerolog"
)

type serveAPIRuntime struct {
	cfg                config.Config
	mainSessionID      string
	server             *http.Server
	extensionsManager  *extensions.Manager
	gatewayRuntime     *gateway.Runtime
	gatewayAgentsWatch *gatewayAgentsWatcher
	relayServer        *browserrelay.Server
	cronManager        *workspaceCronManager
	telegramPoller     *telegramUpdatePoller
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
	mainSessionID, err := resolveMainSessionID(sessionStore, cfg.SessionDefaultID)
	if err != nil {
		return nil, err
	}

	cronStore := cron.NewStoreWithOptions(cfg.WorkspaceDir, cron.StoreOptions{
		RunHistoryLimit: cfg.CronRunHistoryLimit,
	})
	opsManager := ops.NewManager(cfg.WorkspaceDir, ops.Options{})
	scheduleStore := schedule.NewStore(cfg.WorkspaceDir, cronStore, schedule.Options{
		Timezone: cfg.ScheduleTimezone,
	})
	researchService := research.NewService(cfg.WorkspaceDir, research.Options{})
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
	providerModelsCache, err := newProviderModelsCache(providerModelsCachePath(cfg), providerModelsCacheTTL, nowFn)
	if err != nil {
		return nil, err
	}
	providerModelsService := newProviderModelsService(cfg, providerModelsCache, llm.NewModelFetcher(), nowFn)
	dispatcher := newNotificationDispatcher(
		broker,
		newCommandNotifier(cfg.NotifyCommand, logger),
		cfg.NotifyWhenNoClients,
		logger,
	)
	dispatcher.store = notificationStore
	if tracked, ok := deps.llmClient.(*usage.TrackedClient); ok {
		tracked.SetNotifier(func(ctx context.Context, message string) {
			dispatcher.Emit(ctx, newNotificationEvent("usage", "warn", "Usage limit warning", message))
		})
	}
	telegramPairings, err := newTelegramPairingStore(telegramPairingStorePath(cfg), nowFn)
	if err != nil {
		return nil, err
	}
	telegramSender := newTelegramSender(cfg.TelegramBotToken)
	// The telegram_send tool closure intentionally captures this pointer.
	// It is assigned after gateway runtime construction and used at runtime
	// to append outbound Telegram records to gateway channel history.
	var gatewayRuntimeForTelegram *gateway.Runtime
	telegramSendTool := tool.NewTelegramSendTool(tool.TelegramSendFunc(func(ctx context.Context, req tool.TelegramSendRequest) (tool.TelegramSendResult, error) {
		if telegramSender == nil {
			return tool.TelegramSendResult{}, fmt.Errorf("telegram sender is not configured")
		}
		sendResult, err := telegramSender.Send(ctx, telegramSendRequest{
			BotID:     strings.TrimSpace(req.BotID),
			ChatID:    strings.TrimSpace(req.ChatID),
			Text:      strings.TrimSpace(req.Text),
			ThreadID:  strings.TrimSpace(req.ThreadID),
			ParseMode: strings.TrimSpace(req.ParseMode),
		})
		if err != nil {
			return tool.TelegramSendResult{}, err
		}
		if gatewayRuntimeForTelegram != nil {
			recordPayload := map[string]any{
				"provider": "telegram",
			}
			if botID := strings.TrimSpace(req.BotID); botID != "" {
				recordPayload["bot_id"] = botID
			}
			if parseMode := strings.TrimSpace(req.ParseMode); parseMode != "" {
				recordPayload["parse_mode"] = parseMode
			}
			if sendResult.MessageID > 0 {
				recordPayload["message_id"] = sendResult.MessageID
			}
			if sendResult.ChatID != "" {
				recordPayload["provider_chat_id"] = sendResult.ChatID
			}
			if sendResult.Text != "" {
				recordPayload["provider_text"] = sendResult.Text
			}
			if _, recordErr := gatewayRuntimeForTelegram.OutboundTelegram(req.BotID, req.ChatID, req.ThreadID, req.Text, recordPayload); recordErr != nil {
				logger.Debug().Err(recordErr).Str("chat_id", strings.TrimSpace(req.ChatID)).Msg("telegram_send tool gateway record failed")
			}
		}
		return tool.TelegramSendResult{
			MessageID: sendResult.MessageID,
			ChatID:    sendResult.ChatID,
			Text:      sendResult.Text,
		}, nil
	}), cfg.ChannelsTelegramEnabled, tool.TelegramDefaultChatIDResolveFunc(func(ctx context.Context) (string, error) {
		_ = ctx
		if telegramPairings == nil {
			return "", nil
		}
		return telegramPairings.resolveDefaultChatID()
	}))
	apiRunPromptWithTools := deps.runPromptWithTools
	if cfg.ChannelsTelegramEnabled {
		if runnerWithTelegram := newAgentPromptRunnerWithTools(
			cfg.WorkspaceDir,
			deps.llmClient,
			cfg.AgentMaxIterations,
			logger,
			telegramSendTool,
		); runnerWithTelegram != nil {
			apiRunPromptWithTools = runnerWithTelegram
		}
	}
	apiRunPrompt := deps.runPrompt
	if apiRunPromptWithTools != nil {
		apiRunPrompt = func(ctx context.Context, runLabel string, prompt string) (string, error) {
			return apiRunPromptWithTools(ctx, runLabel, prompt, nil)
		}
	}
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
		apiRunPrompt,
		logger,
		dispatcher.Emit,
		mainSessionID,
	)

	mux := http.NewServeMux()
	mux.Handle("/v1/heartbeat/", newHeartbeatAPIHandlerWithRunner(heartbeatRunner, logger))

	processManager := tool.NewProcessManager()
	mcpClient := mcp.NewClient(cfg.MCPServers)
	mcpClient.SetCommandAllowlist(cfg.MCPCommandAllowlist)
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
	otpManager := approval.NewOTPManager(nowFn)
	browserService := buildBrowserService(
		cfg,
		relayServer,
		vaultReader,
		newBrowserTelegramOTPRequester(telegramSender, telegramPairings, otpManager),
	)
	gatewayRuntime := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:                              cfg.GatewayEnabled,
		WorkspaceDir:                         cfg.WorkspaceDir,
		SessionStore:                         sessionStore,
		SessionStoreForWorkspace:             sessionStoreResolver,
		RunPrompt:                            apiRunPrompt,
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
		BrowserManagedHeadless:               cfg.BrowserManagedHeadless,
		BrowserManagedExecutablePath:         cfg.BrowserManagedExecutablePath,
		BrowserManagedUserDataDir:            cfg.BrowserManagedUserDataDir,
		BrowserSiteFlowsDir:                  cfg.BrowserSiteFlowsDir,
		BrowserAutoLoginSiteAllowlist:        cfg.BrowserAutoLoginSiteAllowlist,
		BrowserVaultReader:                   vaultReader,
		BrowserService:                       browserService,
		Now:                                  nowFn,
	})
	gatewayRuntimeForTelegram = gatewayRuntime
	refreshGatewayExecutors := func(reason string) int {
		executors := buildGatewayExecutors(cfg, apiRunPromptWithTools, logger)
		gatewayRuntime.SetExecutors(executors, strings.TrimSpace(cfg.GatewayDefaultAgent))
		agents := len(gatewayRuntime.Agents())
		logger.Debug().Str("reason", reason).Int("gateway_agents", agents).Msg("gateway executors refreshed")
		return agents
	}
	_ = refreshGatewayExecutors("startup")

	chatTooling := buildChatToolingOptions(
		processManager,
		extensionsManager,
		gatewayRuntime,
		cfg.ToolsDefaultSet,
		cfg.ToolsAllowHighRiskUser,
		cfg.APIMaxInflightChat,
		deps.usageTracker,
	)
	chatTooling.OpsManager = opsManager
	chatTooling.ScheduleStore = scheduleStore
	chatTooling.ResearchService = researchService
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
	if cfg.ChannelsTelegramEnabled {
		chatTools = append(chatTools, telegramSendTool)
	}
	chatHandler := newChatAPIHandlerWithRuntimeConfig(
		cfg.WorkspaceDir,
		sessionStore,
		deps.llmClient,
		logger,
		cfg.AgentMaxIterations,
		activity,
		mainSessionID,
		chatTooling,
		chatTools...,
	)
	mux.Handle("/v1/chat", chatHandler)
	sessionHandler := newSessionAPIHandler(sessionStore, logger)
	mux.Handle("/v1/sessions", sessionHandler)
	mux.Handle("/v1/sessions/", sessionHandler)
	projectHandler := newProjectAPIHandler(project.NewStore(cfg.WorkspaceDir, nil), sessionStore, mainSessionID, logger)
	mux.Handle("/v1/projects", projectHandler)
	mux.Handle("/v1/projects/", projectHandler)
	usageHandler := newUsageAPIHandler(deps.usageTracker, cfg.APIAuthMode, logger)
	mux.Handle("/v1/usage/summary", usageHandler)
	mux.Handle("/v1/usage/limits", usageHandler)
	opsHandler := newOpsAPIHandler(opsManager, logger, dispatcher.Emit)
	mux.Handle("/v1/ops/status", opsHandler)
	mux.Handle("/v1/ops/cleanup/plan", opsHandler)
	mux.Handle("/v1/ops/cleanup/apply", opsHandler)
	mux.Handle("/v1/ops/approvals", opsHandler)
	mux.Handle("/v1/ops/approvals/", opsHandler)
	mux.Handle("/v1/status", newStatusAPIHandler(cfg.WorkspaceDir, sessionStore, mainSessionID, logger))
	mux.Handle("/v1/auth/whoami", newAuthAPIHandler(cfg.APIAuthMode))
	mux.Handle("/v1/healthz", newHealthzAPIHandler(nowFn))
	providersModelsHandler := newProvidersModelsAPIHandler(providerModelsService, logger)
	mux.Handle("/v1/providers", providersModelsHandler)
	mux.Handle("/v1/models", providersModelsHandler)
	mux.Handle("/v1/compact", newCompactAPIHandler(cfg.WorkspaceDir, sessionStore, deps.llmClient, logger))
	cronHandler := newCronAPIHandlerWithRunnerAndResolver(cronStoreResolver, cronRunner, logger)
	mux.Handle("/v1/cron/jobs", cronHandler)
	mux.Handle("/v1/cron/jobs/", cronHandler)
	scheduleHandler := newScheduleAPIHandler(scheduleStore, logger)
	mux.Handle("/v1/schedules", scheduleHandler)
	mux.Handle("/v1/schedules/", scheduleHandler)
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
	agentRunsHandler := newAgentRunsAPIHandlerWithInflightLimit(gatewayRuntime, logger, cfg.APIMaxInflightAgentRuns)
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
	browserHandler := newBrowserAPIHandler(
		gatewayRuntime,
		vaultStatus,
		relayServer,
		cfg.BrowserRelayEnabled,
		cfg.BrowserRelayOriginAllowlist,
		logger,
	)
	registerBrowserRoutes(mux, browserHandler)
	telegramInbound := newTelegramInboundHandler(
		cfg.WorkspaceDir,
		sessionStore,
		deps.llmClient,
		telegramSender,
		gatewayRuntime,
		telegramPairings,
		cfg.ChannelsTelegramDMPolicy,
		logger,
	)
	telegramInbound.mainSessionID = strings.TrimSpace(mainSessionID)
	telegramInbound.sessionScope = normalizeTelegramSessionScope(cfg.SessionTelegramScope)
	telegramInbound.maxIterations = cfg.AgentMaxIterations
	telegramInbound.tooling = chatTooling
	telegramInbound.extraTools = append([]tool.Tool(nil), chatTools...)
	telegramInbound.otpManager = otpManager
	telegramInbound.commands = newTelegramCommandHandler(telegramCommandHandlerOptions{
		Store:          sessionStore,
		CronResolver:   cronStoreResolver,
		Runtime:        gatewayRuntime,
		MainSession:    mainSessionID,
		SessionScope:   cfg.SessionTelegramScope,
		ProviderModels: providerModelsService,
		Logger:         logger,
	})
	telegramInbound.media = newTelegramMediaDownloader(cfg.TelegramBotToken, cfg.WorkspaceDir)
	telegramPoller := newTelegramUpdatePoller(cfg.TelegramBotToken, logger, telegramInbound.HandleUpdate)
	if telegramPoller != nil {
		telegramPoller = telegramPoller.withOffsetStore(
			telegramPairings.lastUpdateIDValue,
			telegramPairings.setLastUpdateID,
		)
	}
	channelsHandler := newChannelsAPIHandlerWithTelegramPairings(
		gatewayRuntime,
		telegramSender,
		telegramPairings,
		cfg.ChannelsTelegramDMPolicy,
		cfg.ChannelsTelegramPollingEnabled,
		logger,
	)
	mux.Handle("/v1/channels/webhook/inbound/", channelsHandler)
	mux.Handle("/v1/channels/telegram/webhook/", channelsHandler)
	mux.Handle("/v1/channels/telegram/send", channelsHandler)
	mux.Handle("/v1/channels/telegram/pairings", channelsHandler)
	mux.Handle("/v1/channels/telegram/pairings/", channelsHandler)
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
		mainSessionID:      mainSessionID,
		server:             server,
		extensionsManager:  extensionsManager,
		gatewayRuntime:     gatewayRuntime,
		gatewayAgentsWatch: gatewayAgentsWatch,
		relayServer:        relayServer,
		cronManager:        cronManager,
		telegramPoller:     telegramPoller,
	}, nil
}

func registerBrowserRoutes(mux *http.ServeMux, browserHandler http.Handler) {
	if mux == nil || browserHandler == nil {
		return
	}
	mux.Handle("/v1/browser/status", browserHandler)
	mux.Handle("/v1/browser/profiles", browserHandler)
	mux.Handle("/v1/browser/relay", browserHandler)
	mux.Handle("/v1/browser/login", browserHandler)
	mux.Handle("/v1/browser/check", browserHandler)
	mux.Handle("/v1/browser/run", browserHandler)
	mux.Handle("/v1/vault/status", browserHandler)
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
	if cfg.ChannelsTelegramEnabled && cfg.ChannelsTelegramPollingEnabled {
		if runtime.telegramPoller == nil {
			logger.Debug().Msg("telegram polling skipped (token or handler is not configured)")
		} else {
			go runtime.telegramPoller.Run(ctx)
			logger.Info().
				Str("dm_policy", normalizeTelegramDMPolicy(cfg.ChannelsTelegramDMPolicy)).
				Msg("telegram polling started")
		}
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
