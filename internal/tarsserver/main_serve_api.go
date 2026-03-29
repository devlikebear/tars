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

	"github.com/devlikebear/tars/internal/approval"
	"github.com/devlikebear/tars/internal/cli"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/heartbeat"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/mcp"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/research"
	"github.com/devlikebear/tars/internal/schedule"
	"github.com/devlikebear/tars/internal/skillhub"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

type serveAPIRuntime struct {
	cfg                     config.Config
	configPath              string
	mainSessionID           string
	server                  *http.Server
	extensionsManager       *extensions.Manager
	gatewayRuntime          *gateway.Runtime
	gatewayAgentsWatch      *gatewayAgentsWatcher
	cronManager             *workspaceCronManager
	watchdogManager         *workspaceWatchdogManager
	telegramPoller          *telegramUpdatePoller
	restoreProjectAutopilot func() error
}

type apiRouteHandlers struct {
	heartbeat       http.Handler
	chat            http.Handler
	sessions        http.Handler
	projects        http.Handler
	console         http.Handler
	usage           http.Handler
	ops             http.Handler
	status          http.Handler
	auth            http.Handler
	healthz         http.Handler
	providersModels http.Handler
	compact         http.Handler
	cron            http.Handler
	schedules       http.Handler
	mcp             http.Handler
	extensions      http.Handler
	agentRuns       http.Handler
	gateway         http.Handler
	channels        http.Handler
	events          http.Handler
	config          http.Handler
	skillhub        http.Handler
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
		if runnerWithTelegram := newAgentPromptRunnerWithToolsAndMemory(
			cfg.WorkspaceDir,
			deps.llmClient,
			cfg.AgentMaxIterations,
			logger,
			semanticMemoryConfigFromConfig(cfg),
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
	var (
		projectAutopilot        project.PhaseEngine
		ensureProjectAutopilot  func(context.Context) error
		restoreProjectAutopilot func() error
	)
	heartbeatRunner := newWorkspaceHeartbeatRunnerWithNotify(
		cfg.WorkspaceDir,
		nowFn,
		deps.ask,
		heartbeatPolicyForWorkspace,
		heartbeatState,
		dispatcher.Emit,
		func(ctx context.Context) error {
			if ensureProjectAutopilot == nil {
				return nil
			}
			err := ensureProjectAutopilot(ctx)
			if err != nil {
				logger.Error().Err(err).Msg("ensure project autopilot runs after heartbeat failed")
			}
			return err
		},
	)
	watchdogState := newWatchdogWorkspaceState()
	watchdogRunner := newWorkspaceWatchdogRunnerWithNotify(
		cfg.WorkspaceDir,
		cronStoreResolver,
		nowFn,
		watchdogState,
		dispatcher.Emit,
	)
	cronRunner := newCronJobRunnerWithNotify(
		cfg.WorkspaceDir,
		sessionStore,
		apiRunPromptWithTools,
		logger,
		dispatcher.Emit,
		mainSessionID,
		cfg.CronRunHistoryLimit,
		func(ctx context.Context) (string, error) {
			_ = ctx
			if telegramPairings == nil {
				return "", nil
			}
			return telegramPairings.resolveDefaultChatID()
		},
	)

	mux := http.NewServeMux()
	heartbeatHandler := newHeartbeatAPIHandlerWithRunner(heartbeatRunner, logger)

	processManager := tool.NewProcessManager()
	mcpClient := mcp.NewClient(cfg.MCPServers)
	mcpClient.SetCommandAllowlist(cfg.MCPCommandAllowlist)
	vaultReader, vaultStatus, vaultErr := buildVaultReader(cfg)
	if vaultErr != nil {
		logger.Warn().Err(vaultErr).Msg("vault client initialization failed; browser auto-login will be unavailable")
	}
	otpManager := approval.NewOTPManager(nowFn)
	browserPluginConfig := buildBrowserPluginConfig(cfg, vaultReader, vaultStatus,
		newBrowserTelegramOTPRequester(telegramSender, telegramPairings, otpManager))
	extensionsManager, err := buildExtensionsManager(cfg, mcpClient, map[string]map[string]any{
		"tars-browser": browserPluginConfig,
	})
	if err != nil {
		return nil, err
	}
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
		GatewaySubagentsMaxThreads:           cfg.GatewaySubagentsMaxThreads,
		GatewaySubagentsMaxDepth:             cfg.GatewaySubagentsMaxDepth,
		GatewayPersistenceDir:                cfg.GatewayPersistenceDir,
		GatewayRestoreOnStartup:              cfg.GatewayRestoreOnStartup,
		GatewayReportSummaryEnabled:          cfg.GatewayReportSummaryEnabled,
		GatewayArchiveEnabled:                cfg.GatewayArchiveEnabled,
		GatewayArchiveDir:                    cfg.GatewayArchiveDir,
		GatewayArchiveRetentionDays:          cfg.GatewayArchiveRetentionDays,
		GatewayArchiveMaxFileBytes:           cfg.GatewayArchiveMaxFileBytes,
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
		nil,
		cfg.ToolsDefaultSet,
		cfg.ToolsAllowHighRiskUser,
		semanticMemoryConfigFromConfig(cfg),
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
	projectStore := project.NewStore(cfg.WorkspaceDir, nil)
	projectTaskRunner := gateway.NewProjectTaskRunner(gatewayRuntime, "")
	projectAutopilotManager := project.NewAutopilotManager(projectStore, projectTaskRunner, project.DefaultGitHubAuthChecker(), nil)
	projectAutopilot = projectAutopilotManager
	ensureProjectAutopilot = func(ctx context.Context) error {
		if projectAutopilot == nil {
			return nil
		}
		_, err := projectAutopilot.EnsureActiveRuns(ctx)
		return err
	}
	restoreProjectAutopilot = projectAutopilotManager.RestorePersistedRuns
	// NOTE: RestorePersistedRuns is deferred to startBackgrounds() so that
	// autopilot loops do not fire LLM requests before the server is ready.
	chatTooling.ProjectAutopilot = projectAutopilot
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
	sessionHandler := newSessionAPIHandler(sessionStore, logger)
	projectHandler := newProjectAPIHandler(projectStore, sessionStore, mainSessionID, projectTaskRunner, nil, projectAutopilot, logger)
	consoleHandler, err := newConsoleHandler(logger)
	if err != nil {
		return nil, err
	}
	usageHandler := newUsageAPIHandler(deps.usageTracker, cfg.APIAuthMode, logger)
	opsHandler := newOpsAPIHandler(opsManager, logger, dispatcher.Emit)
	statusHandler := newStatusAPIHandler(cfg.WorkspaceDir, sessionStore, mainSessionID, logger)
	authHandler := newAuthAPIHandler(cfg.APIAuthMode)
	healthzHandler := newHealthzAPIHandler(nowFn, dashboardAuthHealthzStatus(cfg))
	providersModelsHandler := newProvidersModelsAPIHandler(providerModelsService, logger)
	compactHandler := newCompactAPIHandler(cfg.WorkspaceDir, sessionStore, deps.llmClient, logger)
	cronHandler := newCronAPIHandlerWithRunnerAndResolver(cronStoreResolver, cronRunner, logger)
	scheduleHandler := newScheduleAPIHandler(scheduleStore, logger)
	mcpHandler := newMCPAPIHandler(mcpClient, logger)
	extensionsHandler := newExtensionsAPIHandler(extensionsManager, logger, func() (bool, int) {
		if gatewayRuntime == nil {
			return false, 0
		}
		return true, refreshGatewayExecutors("extensions_reload")
	})
	agentRunsHandler := newAgentRunsAPIHandlerWithInflightLimit(gatewayRuntime, logger, cfg.APIMaxInflightAgentRuns)
	gatewayHandler := newGatewayAPIHandler(gatewayRuntime, logger, func() {
		_ = refreshGatewayExecutors("gateway_reload")
	})
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
	hubInstaller := skillhub.NewInstaller(cfg.WorkspaceDir)
	skillhubHandler := newSkillhubAPIHandler(hubInstaller, extensionsManager, logger)
	eventsHandler := newEventsAPIHandler(broker, notificationStore, logger)
	resolvedConfigPath := config.ResolveConfigPath(opts.ConfigPath)
	configHandler := newConfigAPIHandler(resolvedConfigPath, cfg, cfg.WorkspaceDir, logger)
	registerAPIRoutes(mux, apiRouteHandlers{
		heartbeat:       heartbeatHandler,
		chat:            chatHandler,
		sessions:        sessionHandler,
		projects:        projectHandler,
		console:         consoleHandler,
		usage:           usageHandler,
		ops:             opsHandler,
		status:          statusHandler,
		auth:            authHandler,
		healthz:         healthzHandler,
		providersModels: providersModelsHandler,
		compact:         compactHandler,
		cron:            cronHandler,
		schedules:       scheduleHandler,
		mcp:             mcpHandler,
		extensions:      extensionsHandler,
		agentRuns:       agentRunsHandler,
		gateway:         gatewayHandler,
		channels:        channelsHandler,
		events:          eventsHandler,
		config:          configHandler,
		skillhub:        skillhubHandler,
	})

	// Register plugin HTTP handlers
	for _, entry := range extensionsManager.CollectHTTPHandlers() {
		mux.Handle(entry.Pattern, entry.Handler)
	}

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
	watchdogManager := newWorkspaceWatchdogManager(watchdogRunner, defaultWatchdogInterval)

	return &serveAPIRuntime{
		cfg:                     cfg,
		configPath:              resolvedConfigPath,
		mainSessionID:           mainSessionID,
		server:                  server,
		extensionsManager:       extensionsManager,
		gatewayRuntime:          gatewayRuntime,
		gatewayAgentsWatch:      gatewayAgentsWatch,
		cronManager:             cronManager,
		watchdogManager:         watchdogManager,
		telegramPoller:          telegramPoller,
		restoreProjectAutopilot: restoreProjectAutopilot,
	}, nil
}

func registerAPIRoutes(mux *http.ServeMux, handlers apiRouteHandlers) {
	if mux == nil {
		return
	}
	legacyDashboard := newLegacyDashboardRedirectHandler()
	mux.Handle("/v1/heartbeat/", handlers.heartbeat)
	mux.Handle("/v1/chat", handlers.chat)
	mux.Handle("/v1/sessions", handlers.sessions)
	mux.Handle("/v1/sessions/", handlers.sessions)
	mux.Handle("/v1/admin/sessions", handlers.sessions)
	mux.Handle("/v1/admin/sessions/", handlers.sessions)
	mux.Handle("/v1/projects", handlers.projects)
	mux.Handle("/v1/projects/", handlers.projects)
	mux.Handle("/v1/project-briefs/", handlers.projects)
	mux.Handle("/console", handlers.console)
	mux.Handle("/console/", handlers.console)
	if viteProxy := newConsoleDevViteHandler(); viteProxy != nil {
		for _, prefix := range []string{"/@vite/", "/@fs/", "/src/", "/node_modules/"} {
			mux.Handle(prefix, viteProxy)
		}
	}
	mux.Handle("/dashboards", legacyDashboard)
	mux.Handle("/dashboards/", legacyDashboard)
	mux.Handle("/ui/projects/", legacyDashboard)
	mux.Handle("/v1/usage/summary", handlers.usage)
	mux.Handle("/v1/usage/limits", handlers.usage)
	mux.Handle("/v1/ops/status", handlers.ops)
	mux.Handle("/v1/ops/cleanup/plan", handlers.ops)
	mux.Handle("/v1/ops/cleanup/apply", handlers.ops)
	mux.Handle("/v1/ops/approvals", handlers.ops)
	mux.Handle("/v1/ops/approvals/", handlers.ops)
	mux.Handle("/v1/status", handlers.status)
	mux.Handle("/v1/auth/whoami", handlers.auth)
	mux.Handle("/v1/healthz", handlers.healthz)
	mux.Handle("/v1/providers", handlers.providersModels)
	mux.Handle("/v1/models", handlers.providersModels)
	mux.Handle("/v1/compact", handlers.compact)
	mux.Handle("/v1/cron/jobs", handlers.cron)
	mux.Handle("/v1/cron/jobs/", handlers.cron)
	mux.Handle("/v1/schedules", handlers.schedules)
	mux.Handle("/v1/schedules/", handlers.schedules)
	mux.Handle("/v1/mcp/servers", handlers.mcp)
	mux.Handle("/v1/mcp/tools", handlers.mcp)
	mux.Handle("/v1/skills", handlers.extensions)
	mux.Handle("/v1/skills/", handlers.extensions)
	mux.Handle("/v1/plugins", handlers.extensions)
	mux.Handle("/v1/runtime/extensions/reload", handlers.extensions)
	mux.Handle("/v1/runtime/extensions/disabled", handlers.extensions)
	mux.Handle("/v1/agent/agents", handlers.agentRuns)
	mux.Handle("/v1/agent/runs", handlers.agentRuns)
	mux.Handle("/v1/agent/runs/", handlers.agentRuns)
	mux.Handle("/v1/gateway/status", handlers.gateway)
	mux.Handle("/v1/gateway/reload", handlers.gateway)
	mux.Handle("/v1/gateway/restart", handlers.gateway)
	mux.Handle("/v1/gateway/reports/summary", handlers.gateway)
	mux.Handle("/v1/gateway/reports/runs", handlers.gateway)
	mux.Handle("/v1/gateway/reports/channels", handlers.gateway)
	// Browser routes are now registered via plugin HTTP handlers
	mux.Handle("/v1/channels/webhook/inbound/", handlers.channels)
	mux.Handle("/v1/channels/telegram/webhook/", handlers.channels)
	mux.Handle("/v1/channels/telegram/send", handlers.channels)
	mux.Handle("/v1/channels/telegram/pairings", handlers.channels)
	mux.Handle("/v1/channels/telegram/pairings/", handlers.channels)
	mux.Handle("/v1/events/stream", handlers.events)
	mux.Handle("/v1/events/history", handlers.events)
	mux.Handle("/v1/events/read", handlers.events)
	mux.Handle("/v1/admin/config", handlers.config)
	mux.Handle("/v1/admin/config/values", handlers.config)
	mux.Handle("/v1/admin/config/schema", handlers.config)
	mux.Handle("/v1/admin/reset/workspace", handlers.config)
	mux.Handle("/v1/admin/restart", handlers.config)
	mux.Handle("/v1/hub/registry", handlers.skillhub)
	mux.Handle("/v1/hub/installed", handlers.skillhub)
	mux.Handle("/v1/hub/install", handlers.skillhub)
	mux.Handle("/v1/hub/uninstall", handlers.skillhub)
	mux.Handle("/v1/hub/update", handlers.skillhub)
	mux.Handle("/v1/hub/skill-content", handlers.skillhub)
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
	if runtime.watchdogManager != nil {
		go func() {
			if err := runtime.watchdogManager.Start(ctx); err != nil {
				logger.Error().Err(err).Msg("watchdog manager stopped with error")
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
	// Restore persisted project autopilot runs after all other backgrounds are up.
	if runtime.restoreProjectAutopilot != nil {
		go func() {
			if err := runtime.restoreProjectAutopilot(); err != nil {
				logger.Error().Err(err).Msg("restore persisted project autopilot runs failed")
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
	if runtime.gatewayRuntime != nil {
		_ = runtime.gatewayRuntime.Close(ctx)
	}
	if runtime.server != nil {
		_ = runtime.server.Shutdown(ctx)
	}
}
