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

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/cli"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/mcp"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/pulse"
	"github.com/devlikebear/tars/internal/reflection"
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
	agentRuntime            *agentruntime.Runtime
	agentRuntimeAgentsWatch *agentRuntimeAgentsWatcher
	cronManager             *workspaceCronManager
	watchdogManager         *workspaceWatchdogManager
	pulseRuntime            *pulse.Runtime
	reflectionRuntime       *reflection.Runtime
	telegramPoller          *telegramUpdatePoller
}

type apiRouteHandlers struct {
	pulse           http.Handler
	reflection      http.Handler
	chat            http.Handler
	sessions        http.Handler
	memory          http.Handler
	console         http.Handler
	usage           http.Handler
	ops             http.Handler
	status          http.Handler
	auth            http.Handler
	healthz         http.Handler
	providersModels http.Handler
	compact         http.Handler
	cron            http.Handler
	mcp             http.Handler
	extensions      http.Handler
	agentRuns       http.Handler
	agentSubagents  http.Handler
	agentRuntime    http.Handler
	channels        http.Handler
	events          http.Handler
	config          http.Handler
	skillhub        http.Handler
	skillCreator    http.Handler
	mcpCreator      http.Handler
	filesystem      http.Handler
	workspaceFiles  http.Handler
	terminal        http.Handler
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
	cronStoreResolver := newWorkspaceCronStoreResolver(cfg.WorkspaceDir, cfg.CronRunHistoryLimit, cronStore)
	activity := &runtimeActivity{}
	broker := newEventBroker()
	notificationStore, err := newNotificationStore(
		filepath.Join(strings.TrimSpace(cfg.AgentRuntimePersistenceDir), "notifications.json"),
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
	if deps.llmRouter == nil {
		return nil, fmt.Errorf("llm router is not configured")
	}
	_, chatResolution, err := deps.llmRouter.ClientFor(llm.RoleChatMain)
	if err != nil {
		return nil, err
	}
	logger.Debug().
		Str("tier", string(chatResolution.Tier)).
		Str("provider", chatResolution.Provider).
		Str("model", chatResolution.Model).
		Str("source", chatResolution.Source).
		Msg("llm router resolved chat_main tier")
	attachUsageWarningNotifier(deps.llmRouter, func(ctx context.Context, message string) {
		dispatcher.Emit(ctx, newNotificationEvent("usage", "warn", "Usage limit warning", message))
	})
	telegramPairings, err := newTelegramPairingStore(telegramPairingStorePath(cfg), nowFn)
	if err != nil {
		return nil, err
	}
	telegramDeliveryCounter := newTelegramDeliveryCounter(100)
	telegramSender := newTelegramCountingSender(
		newTelegramSender(cfg.TelegramBotToken),
		telegramDeliveryCounter,
	)
	// The telegram_send tool closure intentionally captures this pointer.
	// It is assigned after agent runtime construction and used at runtime
	// to append outbound Telegram records to agent runtime channel history.
	var agentRuntimeForTelegram *agentruntime.Runtime
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
		if agentRuntimeForTelegram != nil {
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
			if _, recordErr := agentRuntimeForTelegram.OutboundTelegram(req.BotID, req.ChatID, req.ThreadID, req.Text, recordPayload); recordErr != nil {
				logger.Debug().Err(recordErr).Str("chat_id", strings.TrimSpace(req.ChatID)).Msg("telegram_send tool agent runtime record failed")
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
			cfg,
			cfg.WorkspaceDir,
			nil,
			deps.llmRouter,
			deps.usageTracker,
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
			return apiRunPromptWithTools(ctx, runLabel, prompt, nil, "", nil)
		}
	}
	watchdogState := newWatchdogWorkspaceState()
	watchdogRunner := newWorkspaceWatchdogRunnerWithNotify(
		cfg.WorkspaceDir,
		cronStoreResolver,
		nowFn,
		watchdogState,
		dispatcher.Emit,
	)
	var cronRunner func(ctx context.Context, job cron.Job) (string, error)

	mux := http.NewServeMux()

	processManager := tool.NewProcessManager()
	mcpClient := mcp.NewClient(cfg.MCPServers)
	mcpClient.SetCommandAllowlist(cfg.MCPCommandAllowlist)
	extensionsManager, err := buildExtensionsManager(cfg, mcpClient, nil)
	if err != nil {
		return nil, err
	}
	agentRuntime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:                                   cfg.AgentRuntimeEnabled,
		WorkspaceDir:                              cfg.WorkspaceDir,
		SessionStore:                              sessionStore,
		SessionStoreForWorkspace:                  sessionStoreResolver,
		RunPrompt:                                 apiRunPrompt,
		Executors:                                 nil,
		DefaultAgent:                              strings.TrimSpace(cfg.AgentRuntimeDefaultAgent),
		AgentRuntimeAgentsWatchEnabled:            false,
		ChannelsLocalEnabled:                      cfg.ChannelsLocalEnabled,
		ChannelsWebhookEnabled:                    cfg.ChannelsWebhookEnabled,
		ChannelsTelegramEnabled:                   cfg.ChannelsTelegramEnabled,
		AgentRuntimePersistenceEnabled:            cfg.AgentRuntimePersistenceEnabled,
		AgentRuntimeRunsPersistenceEnabled:        cfg.AgentRuntimeRunsPersistenceEnabled,
		AgentRuntimeChannelsPersistenceEnabled:    cfg.AgentRuntimeChannelsPersistenceEnabled,
		AgentRuntimeRunsMaxRecords:                cfg.AgentRuntimeRunsMaxRecords,
		AgentRuntimeChannelsMaxMessagesPerChannel: cfg.AgentRuntimeChannelsMaxMessagesPerChannel,
		AgentRuntimeSubagentsMaxThreads:           cfg.AgentRuntimeSubagentsMaxThreads,
		AgentRuntimeSubagentsMaxDepth:             cfg.AgentRuntimeSubagentsMaxDepth,
		AgentRuntimeConsensusEnabled:              cfg.AgentRuntimeConsensusEnabled,
		AgentRuntimeConsensusMaxFanout:            cfg.AgentRuntimeConsensusMaxFanout,
		AgentRuntimeConsensusBudgetTokens:         cfg.AgentRuntimeConsensusBudgetTokens,
		AgentRuntimeConsensusBudgetUSD:            cfg.AgentRuntimeConsensusBudgetUSD,
		AgentRuntimeConsensusTimeoutSeconds:       cfg.AgentRuntimeConsensusTimeoutSeconds,
		AgentRuntimeConsensusAllowedAliases:       append([]string(nil), cfg.AgentRuntimeConsensusAllowedAliases...),
		AgentRuntimeConsensusConcurrentRuns:       cfg.AgentRuntimeConsensusConcurrentRuns,
		AgentRuntimePersistenceDir:                cfg.AgentRuntimePersistenceDir,
		AgentRuntimeRestoreOnStartup:              cfg.AgentRuntimeRestoreOnStartup,
		AgentRuntimeReportSummaryEnabled:          cfg.AgentRuntimeReportSummaryEnabled,
		AgentRuntimeArchiveEnabled:                cfg.AgentRuntimeArchiveEnabled,
		AgentRuntimeArchiveDir:                    cfg.AgentRuntimeArchiveDir,
		AgentRuntimeArchiveRetentionDays:          cfg.AgentRuntimeArchiveRetentionDays,
		AgentRuntimeArchiveMaxFileBytes:           cfg.AgentRuntimeArchiveMaxFileBytes,
		ResolveProviderOverride: func(tier string, override *agentruntime.ProviderOverride) (agentruntime.ResolvedProviderOverride, error) {
			if override == nil {
				return agentruntime.ResolvedProviderOverride{}, nil
			}
			resolved, err := resolveProviderOverrideClient(cfg, cfg.WorkspaceDir, deps.usageTracker, tier, override)
			if err != nil {
				return agentruntime.ResolvedProviderOverride{}, err
			}
			return agentruntime.ResolvedProviderOverride{Alias: strings.TrimSpace(override.Alias), Kind: resolved.provider, Model: resolved.model, Tier: resolved.tier}, nil
		},
		EstimateTokensCost: func(provider, model string, inputTokens, outputTokens int) (float64, bool) {
			if deps.usageTracker == nil {
				return 0, false
			}
			return deps.usageTracker.EstimateCost(provider, model, llm.Usage{InputTokens: inputTokens, OutputTokens: outputTokens})
		},
		UsageTracker: deps.usageTracker,
		Now:          nowFn,
	})
	agentRuntimeForTelegram = agentRuntime

	reflectionSetup := buildReflectionRuntime(reflectionSetupInputs{
		Config:       cfg,
		WorkspaceDir: cfg.WorkspaceDir,
		Router:       deps.llmRouter,
		SessionStore: sessionStore,
		Logger:       logger,
	})

	pulseSetup := buildPulseRuntime(pulseSetupInputs{
		Config:           cfg,
		WorkspaceDir:     cfg.WorkspaceDir,
		Router:           deps.llmRouter,
		CronStore:        cronStore,
		AgentRuntime:     agentRuntime,
		OpsManager:       opsManager,
		DeliveryCounter:  telegramDeliveryCounter,
		ReflectionHealth: reflectionHealthFromSetup(reflectionSetup),
		NotifyEmit:       dispatcher.Emit,
		Logger:           logger,
	})

	refreshAgentRuntimeExecutors := func(reason string) int {
		executors := buildAgentRuntimeExecutors(cfg, apiRunPromptWithTools, logger)
		agentRuntime.SetExecutors(executors, strings.TrimSpace(cfg.AgentRuntimeDefaultAgent))
		agents := len(agentRuntime.Agents())
		logger.Debug().Str("reason", reason).Int("agentruntime_agents", agents).Msg("agent runtime executors refreshed")
		return agents
	}
	_ = refreshAgentRuntimeExecutors("startup")

	chatTooling := buildChatToolingOptions(
		processManager,
		extensionsManager,
		agentRuntime,
		chatCompactionOptions{
			TriggerTokens:      cfg.CompactionTriggerTokens,
			KeepRecentTokens:   cfg.CompactionKeepRecentTokens,
			KeepRecentFraction: cfg.CompactionKeepRecentFraction,
			LLMMode:            cfg.CompactionLLMMode,
			LLMTimeoutSeconds:  cfg.CompactionLLMTimeoutSeconds,
		},
		cfg.ToolsDefaultSet,
		cfg.ToolsAllowHighRiskUser,
		semanticMemoryConfigFromConfig(cfg),
		cfg.APIMaxInflightChat,
		deps.usageTracker,
		cfg.PlanClarifyMode,
	)
	chatTooling.OpsManager = opsManager
	chatTooling.AutomationToolsForWorkspace = func(workspaceID string) []tool.Tool {
		resolvedStore, err := cronStoreResolver.Resolve(defaultWorkspaceID)
		if err != nil {
			logger.Warn().Err(err).Msg("resolve cron store failed for chat tools")
			resolvedStore = cronStore
		}
		return buildAutomationTools(resolvedStore, cronRunner)
	}
	chatTools := buildOptionalChatTools(cfg, agentRuntime)
	if cfg.ChannelsTelegramEnabled {
		chatTools = append(chatTools, telegramSendTool)
	}
	cronPromptDeps := chatHandlerDeps{
		workspaceDir:  cfg.WorkspaceDir,
		store:         sessionStore,
		router:        deps.llmRouter,
		logger:        logger,
		maxIters:      cfg.AgentMaxIterations,
		mainSessionID: mainSessionID,
		tooling:       chatTooling,
		extraTools:    chatTools,
	}
	baseCronRunner := newCronJobRunnerWithNotify(
		cfg.WorkspaceDir,
		sessionStore,
		newCronPromptRunnerWithSessionContext(apiRunPromptWithTools, cronPromptDeps),
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
		func(ctx context.Context, job cron.Job, reminderText string) error {
			if telegramSender == nil || strings.TrimSpace(reminderText) == "" {
				return nil
			}
			meta, _ := cron.ExtractPayloadMeta(job.Payload)
			chatID := strings.TrimSpace(meta.TelegramChatID)
			threadID := strings.TrimSpace(meta.TelegramThreadID)
			botID := strings.TrimSpace(meta.TelegramBotID)
			if chatID == "" && telegramPairings != nil {
				resolvedChatID, err := telegramPairings.resolveDefaultChatID()
				if err != nil {
					if strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "multiple paired telegram chats") {
						logger.Debug().Str("job_id", strings.TrimSpace(job.ID)).Msg("skip cron reminder telegram send: multiple paired chats")
						return nil
					}
					return err
				}
				chatID = strings.TrimSpace(resolvedChatID)
			}
			if chatID == "" {
				return nil
			}
			sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			sendResult, err := telegramSender.Send(sendCtx, telegramSendRequest{
				BotID:    botID,
				ChatID:   chatID,
				ThreadID: threadID,
				Text:     reminderText,
			})
			if err != nil {
				return err
			}
			if agentRuntime != nil {
				recordPayload := map[string]any{
					"provider":      "telegram",
					"cron_job_id":   strings.TrimSpace(job.ID),
					"cron_job_name": strings.TrimSpace(job.Name),
				}
				if botID != "" {
					recordPayload["bot_id"] = botID
				}
				if sendResult.MessageID > 0 {
					recordPayload["message_id"] = sendResult.MessageID
				}
				if sendResult.ChatID != "" {
					recordPayload["provider_chat_id"] = sendResult.ChatID
				}
				if _, recordErr := agentRuntime.OutboundTelegram(botID, chatID, threadID, reminderText, recordPayload); recordErr != nil {
					logger.Debug().Err(recordErr).Str("job_id", strings.TrimSpace(job.ID)).Str("chat_id", chatID).Msg("record cron reminder telegram outbound failed")
				}
			}
			return nil
		},
	)
	cronRunner = func(ctx context.Context, job cron.Job) (string, error) {
		if baseCronRunner != nil {
			return baseCronRunner(ctx, job)
		}
		return "", fmt.Errorf("cron runner not configured")
	}
	chatHandler := newChatAPIHandlerWithRuntimeConfig(
		cfg.WorkspaceDir,
		sessionStore,
		nil,
		deps.llmRouter,
		logger,
		cfg.AgentMaxIterations,
		activity,
		mainSessionID,
		chatTooling,
		chatTools...,
	)
	sessionHandler := newSessionAPIHandlerWithUsage(sessionStore, logger, deps.usageTracker)
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
	compactHandler := newCompactAPIHandler(cfg.WorkspaceDir, sessionStore, deps.llmRouter, logger)
	cronHandler := newCronAPIHandlerWithRunnerAndResolver(cronStoreResolver, cronRunner, logger)
	mcpHandler := newMCPAPIHandler(mcpClient, logger)
	extensionsHandler := newExtensionsAPIHandler(extensionsManager, logger, func() (bool, int) {
		if agentRuntime == nil {
			return false, 0
		}
		return true, refreshAgentRuntimeExecutors("extensions_reload")
	})
	agentRunsHandler := newAgentRunsAPIHandlerWithInflightLimit(agentRuntime, logger, cfg.APIMaxInflightAgentRuns)
	agentSubagentsHandler := newAgentRuntimeSubagentsAPIHandler(agentRuntime, cfg, func() {
		_ = refreshAgentRuntimeExecutors("agentruntime_subagents_update")
	}, deps.llmRouter)
	agentRuntimeHandler := newAgentRuntimeAPIHandler(agentRuntime, logger, func() {
		_ = refreshAgentRuntimeExecutors("agentruntime_reload")
	})
	telegramInbound := newTelegramInboundHandler(
		cfg.WorkspaceDir,
		sessionStore,
		nil,
		telegramSender,
		agentRuntime,
		telegramPairings,
		cfg.ChannelsTelegramDMPolicy,
		logger,
	)
	telegramInbound.mainSessionID = strings.TrimSpace(mainSessionID)
	telegramInbound.llmRouter = deps.llmRouter
	telegramInbound.sessionScope = normalizeTelegramSessionScope(cfg.SessionTelegramScope)
	telegramInbound.maxIterations = cfg.AgentMaxIterations
	telegramInbound.tooling = chatTooling
	telegramInbound.extraTools = append([]tool.Tool(nil), chatTools...)
	telegramInbound.commands = newTelegramCommandHandler(telegramCommandHandlerOptions{
		Store:          sessionStore,
		CronResolver:   cronStoreResolver,
		Runtime:        agentRuntime,
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
		agentRuntime,
		telegramSender,
		telegramPairings,
		cfg.ChannelsTelegramDMPolicy,
		cfg.ChannelsTelegramPollingEnabled,
		logger,
	)
	hubInstaller := skillhub.NewInstaller(cfg.WorkspaceDir)
	skillhubHandler := newSkillhubAPIHandler(hubInstaller, extensionsManager, logger)
	skillCreatorHandler := newSkillCreatorAPIHandler(cfg.WorkspaceDir, logger, nil)
	mcpCreatorHandler := newMCPServerCreatorAPIHandler(cfg.WorkspaceDir, logger, nil)
	eventsHandler := newEventsAPIHandler(broker, notificationStore, logger)
	resolvedConfigPath := config.ResolveConfigPath(opts.ConfigPath)
	configHandler := newConfigAPIHandler(resolvedConfigPath, cfg, cfg.WorkspaceDir, logger)
	filesystemHandler := newFilesystemBrowseHandler(logger)
	workspaceFilesHandler := newWorkspaceFilesHandler(cfg.WorkspaceDir, logger)
	terminalHandler := newTerminalAPIHandler(cfg.WorkspaceDir, sessionStore, logger)
	memoryHandler := newMemoryAPIHandler(cfg.WorkspaceDir, buildMemoryBackend(cfg.WorkspaceDir, semanticMemoryConfigFromConfig(cfg), cfg.MemoryBackend), logger)
	registerAPIRoutes(mux, apiRouteHandlers{
		pulse:           pulseSetup.Handler,
		reflection:      reflectionSetup.Handler,
		chat:            chatHandler,
		sessions:        sessionHandler,
		memory:          memoryHandler,
		console:         consoleHandler,
		usage:           usageHandler,
		ops:             opsHandler,
		status:          statusHandler,
		auth:            authHandler,
		healthz:         healthzHandler,
		providersModels: providersModelsHandler,
		compact:         compactHandler,
		cron:            cronHandler,
		mcp:             mcpHandler,
		extensions:      extensionsHandler,
		agentRuns:       agentRunsHandler,
		agentSubagents:  agentSubagentsHandler,
		agentRuntime:    agentRuntimeHandler,
		channels:        channelsHandler,
		events:          eventsHandler,
		config:          configHandler,
		skillhub:        skillhubHandler,
		skillCreator:    skillCreatorHandler,
		mcpCreator:      mcpCreatorHandler,
		filesystem:      filesystemHandler,
		workspaceFiles:  workspaceFilesHandler,
		terminal:        terminalHandler,
	})

	server := &http.Server{
		Addr:    opts.APIAddr,
		Handler: applyAPIMiddleware(cfg, logger, mux, stderr),
	}
	agentRuntimeAgentsWatch := newAgentRuntimeAgentsWatcher(agentRuntimeAgentsWatcherOptions{
		WorkspaceDir: cfg.WorkspaceDir,
		Debounce:     time.Duration(cfg.AgentRuntimeAgentsWatchDebounceMS) * time.Millisecond,
		Logger:       logger,
		Refresh: func(reason string) {
			_ = refreshAgentRuntimeExecutors(reason)
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
		agentRuntime:            agentRuntime,
		agentRuntimeAgentsWatch: agentRuntimeAgentsWatch,
		cronManager:             cronManager,
		watchdogManager:         watchdogManager,
		pulseRuntime:            pulseSetup.Runtime,
		reflectionRuntime:       reflectionSetup.Runtime,
		telegramPoller:          telegramPoller,
	}, nil
}

func attachUsageWarningNotifier(router llm.Router, notifier usage.WarningNotifier) {
	if router == nil || notifier == nil {
		return
	}
	seen := map[*usage.TrackedClient]struct{}{}
	for _, tier := range []llm.Tier{llm.TierHeavy, llm.TierStandard, llm.TierLight} {
		client, _, err := router.ClientForTier(tier)
		if err != nil {
			continue
		}
		tracked, ok := client.(*usage.TrackedClient)
		if !ok || tracked == nil {
			continue
		}
		if _, ok := seen[tracked]; ok {
			continue
		}
		seen[tracked] = struct{}{}
		tracked.SetNotifier(notifier)
	}
}

func registerAPIRoutes(mux *http.ServeMux, handlers apiRouteHandlers) {
	if mux == nil {
		return
	}
	mux.Handle("/v1/pulse/", handlers.pulse)
	mux.Handle("/v1/reflection/", handlers.reflection)
	mux.Handle("/v1/chat", handlers.chat)
	mux.Handle("/v1/chat/", handlers.chat)
	mux.Handle("/v1/sessions", handlers.sessions)
	mux.Handle("/v1/sessions/", handlers.sessions)
	mux.Handle("/v1/admin/sessions", handlers.sessions)
	mux.Handle("/v1/admin/sessions/", handlers.sessions)
	mux.Handle("/v1/memory/assets", handlers.memory)
	mux.Handle("/v1/memory/file", handlers.memory)
	mux.Handle("/v1/memory/search", handlers.memory)
	mux.Handle("/v1/memory/prefetch", handlers.memory)
	mux.Handle("/v1/memory/kb/notes", handlers.memory)
	mux.Handle("/v1/memory/kb/notes/", handlers.memory)
	mux.Handle("/v1/memory/kb/graph", handlers.memory)
	mux.Handle("/v1/workspace/sysprompt/files", handlers.memory)
	mux.Handle("/v1/workspace/sysprompt/file", handlers.memory)
	mux.Handle("/v1/admin/sysprompt/preview", handlers.memory)
	mux.Handle("/console", handlers.console)
	mux.Handle("/console/", handlers.console)
	if viteProxy := newConsoleDevViteHandler(); viteProxy != nil {
		for _, prefix := range []string{"/@vite/", "/@fs/", "/src/", "/node_modules/"} {
			mux.Handle(prefix, viteProxy)
		}
	}
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
	mux.Handle("/v1/mcp/servers", handlers.mcp)
	mux.Handle("/v1/mcp/tools", handlers.mcp)
	mux.Handle("/v1/skills", handlers.extensions)
	mux.Handle("/v1/skills/", handlers.extensions)
	mux.Handle("/v1/plugins", handlers.extensions)
	mux.Handle("/v1/runtime/extensions/reload", handlers.extensions)
	mux.Handle("/v1/runtime/extensions/disabled", handlers.extensions)
	mux.Handle("/v1/agentruntime/agents", handlers.agentRuns)
	mux.Handle("/v1/agentruntime/runs", handlers.agentRuns)
	mux.Handle("/v1/agentruntime/runs/", handlers.agentRuns)
	mux.Handle("/v1/agent/agents", handlers.agentRuns)
	mux.Handle("/v1/agent/runs", handlers.agentRuns)
	mux.Handle("/v1/agent/runs/", handlers.agentRuns)
	mux.Handle("/v1/agentruntime/subagents", handlers.agentSubagents)
	mux.Handle("/v1/agentruntime/subagents/", handlers.agentSubagents)
	mux.Handle("/v1/agentruntime/status", handlers.agentRuntime)
	mux.Handle("/v1/agentruntime/reload", handlers.agentRuntime)
	mux.Handle("/v1/agentruntime/restart", handlers.agentRuntime)
	mux.Handle("/v1/agentruntime/reports/summary", handlers.agentRuntime)
	mux.Handle("/v1/agentruntime/reports/runs", handlers.agentRuntime)
	mux.Handle("/v1/agentruntime/reports/channels", handlers.agentRuntime)
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
	mux.Handle("/v1/admin/skills/draft", handlers.skillCreator)
	mux.Handle("/v1/admin/skills/save-local", handlers.skillCreator)
	mux.Handle("/v1/admin/skills/test", handlers.skillCreator)
	mux.Handle("/v1/admin/skills/submit-pr", handlers.skillCreator)
	mux.Handle("/v1/admin/mcp-servers/draft", handlers.mcpCreator)
	mux.Handle("/v1/admin/mcp-servers/save-local", handlers.mcpCreator)
	mux.Handle("/v1/admin/mcp-servers/test", handlers.mcpCreator)
	mux.Handle("/v1/admin/mcp-servers/submit-pr", handlers.mcpCreator)
	mux.Handle("/v1/filesystem/browse", handlers.filesystem)
	mux.Handle("/v1/workspace/files", handlers.workspaceFiles)
	mux.Handle("/v1/workspace/files/", handlers.workspaceFiles)
	mux.Handle("/v1/terminal/open", handlers.terminal)
	mux.Handle("/v1/terminal/ws", handlers.terminal)
}

func startBackgrounds(ctx context.Context, runtime *serveAPIRuntime, logger zerolog.Logger) error {
	if runtime == nil {
		return fmt.Errorf("serve runtime is required")
	}
	cfg := runtime.cfg

	if runtime.agentRuntime != nil {
		runtime.agentRuntime.SetAgentsWatchEnabled(false)
	}
	if cfg.AgentRuntimeEnabled && cfg.AgentRuntimeAgentsWatch && runtime.agentRuntime != nil && runtime.agentRuntimeAgentsWatch != nil {
		started, watchErr := runtime.agentRuntimeAgentsWatch.Start(ctx)
		if watchErr != nil {
			logger.Warn().Err(watchErr).Msg("agent runtime agents watcher start failed")
		}
		runtime.agentRuntime.SetAgentsWatchEnabled(started)
		if started {
			logger.Info().Int("debounce_ms", cfg.AgentRuntimeAgentsWatchDebounceMS).Msg("agent runtime agents watcher started")
		} else {
			logger.Debug().Msg("agent runtime agents watcher skipped (workspace agents dir not found)")
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
	if runtime.pulseRuntime != nil {
		runtime.pulseRuntime.Start(ctx)
	}
	if runtime.reflectionRuntime != nil {
		runtime.reflectionRuntime.Start(ctx)
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
	return nil
}

func shutdownRuntime(ctx context.Context, runtime *serveAPIRuntime) {
	if runtime == nil {
		return
	}
	if runtime.pulseRuntime != nil {
		runtime.pulseRuntime.Stop()
	}
	if runtime.reflectionRuntime != nil {
		runtime.reflectionRuntime.Stop()
	}
	if runtime.extensionsManager != nil {
		runtime.extensionsManager.Close()
	}
	if runtime.agentRuntimeAgentsWatch != nil {
		runtime.agentRuntimeAgentsWatch.Close()
	}
	if runtime.agentRuntime != nil {
		_ = runtime.agentRuntime.Close(ctx)
	}
	if runtime.server != nil {
		_ = runtime.server.Shutdown(ctx)
	}
}
