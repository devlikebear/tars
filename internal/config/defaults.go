package config

const (
	defaultMode                          = "standalone"
	defaultWorkspaceDir                  = "./workspace"
	defaultSessionTelegramScope          = "main"
	defaultAPIAuthMode                   = "required"
	defaultDashboardAuthMode             = "inherit"
	defaultAPIMaxInflightChat            = 2
	defaultAPIMaxInflightAgentRuns       = 4
	defaultLLMProvider                   = "bifrost"
	defaultLLMAuthMode                   = "api-key"
	defaultMemoryEmbedProvider           = "gemini"
	defaultMemoryEmbedModel              = "gemini-embedding-2-preview"
	defaultMemoryEmbedDimensions         = 768
	defaultUsageLimitDailyUSD            = 10.0
	defaultUsageLimitWeeklyUSD           = 50.0
	defaultUsageLimitMonthlyUSD          = 150.0
	defaultUsageLimitMode                = "soft"
	defaultBifrostModel                  = "openai/gpt-4o-mini"
	defaultAgentMaxIterations            = 8
	defaultCronRunHistoryLimit           = 200
	defaultAssistantHotkey               = "Ctrl+Option+Space"
	defaultAssistantWhisperBin           = "whisper-cli"
	defaultAssistantFFmpegBin            = "ffmpeg"
	defaultAssistantTTSBin               = "say"
	defaultScheduleTimezone              = "Asia/Seoul"
	defaultToolsDefaultSet               = "standard"
	defaultToolsWebSearchProvider        = "brave"
	defaultPerplexityModel               = "sonar"
	defaultPerplexityBaseURL             = "https://api.perplexity.ai/chat/completions"
	defaultToolsWebSearchCacheTTLSeconds = 60
	defaultVaultAddr                     = "http://127.0.0.1:8200"
	defaultVaultAuthMode                 = "token"
	defaultVaultTimeoutMS                = 1500
	defaultVaultKVMount                  = "secret"
	defaultVaultKVVersion                = 2
	defaultVaultAppRoleMount             = "approle"
	defaultBrowserDefaultProfile         = "managed"
	defaultBrowserRelayAddr              = "127.0.0.1:43182"
	defaultGatewayWatchDebounceMS        = 200
	defaultGatewayRunsMaxRecords         = 2000
	defaultGatewayChannelsMaxMessages    = 500
	defaultGatewaySubagentsMaxThreads    = 4
	defaultGatewaySubagentsMaxDepth      = 1
	defaultGatewayArchiveRetentionDays   = 30
	defaultGatewayArchiveMaxFileBytes    = 10485760
	defaultChannelsTelegramDMPolicy      = "pairing"
	defaultSkillsBundledDir              = "./skills"
	defaultPluginsBundledDir             = "./plugins"
	defaultOpenAIBaseURL                 = "https://api.openai.com/v1"
	defaultOpenAIModel                   = "gpt-4o-mini"
	defaultOpenAICodexBaseURL            = "https://chatgpt.com/backend-api"
	defaultOpenAICodexModel              = "gpt-5.3-codex"
	defaultClaudeCodeCLIModel            = "sonnet"
	defaultGeminiBaseURL                 = "https://generativelanguage.googleapis.com/v1beta/openai"
	defaultGeminiNativeBaseURL           = "https://generativelanguage.googleapis.com/v1beta"
	defaultGeminiModel                   = "gemini-2.5-flash"
	defaultAnthropicBaseURL              = "https://api.anthropic.com"
	defaultAnthropicModel                = "claude-3-5-haiku-latest"
	defaultOpenAICodexOAuthProvider      = "openai-codex"
	defaultClaudeOAuthProvider           = "claude-code"
	defaultGeminiOAuthProvider           = "google-antigravity"
	defaultChromeExtensionOrigin         = "chrome-extension://*"
)

func defaultConfigValues() Config {
	return Config{
		RuntimeConfig: RuntimeConfig{
			Mode:                 defaultMode,
			WorkspaceDir:         defaultWorkspaceDir,
			SessionTelegramScope: defaultSessionTelegramScope,
		},
		APIConfig: APIConfig{
			APIAuthMode:             defaultAPIAuthMode,
			DashboardAuthMode:       defaultDashboardAuthMode,
			APIMaxInflightChat:      defaultAPIMaxInflightChat,
			APIMaxInflightAgentRuns: defaultAPIMaxInflightAgentRuns,
		},
		LLMConfig: LLMConfig{
			LLMProvider:  defaultLLMProvider,
			LLMAuthMode:  defaultLLMAuthMode,
			BifrostModel: defaultBifrostModel,
		},
		MemoryConfig: MemoryConfig{
			MemorySemanticEnabled: false,
			MemoryEmbedProvider:   defaultMemoryEmbedProvider,
			MemoryEmbedBaseURL:    defaultGeminiNativeBaseURL,
			MemoryEmbedModel:      defaultMemoryEmbedModel,
			MemoryEmbedDimensions: defaultMemoryEmbedDimensions,
		},
		UsageConfig: UsageConfig{
			UsageLimitDailyUSD:   defaultUsageLimitDailyUSD,
			UsageLimitWeeklyUSD:  defaultUsageLimitWeeklyUSD,
			UsageLimitMonthlyUSD: defaultUsageLimitMonthlyUSD,
			UsageLimitMode:       defaultUsageLimitMode,
		},
		AutomationConfig: AutomationConfig{
			AgentMaxIterations:  defaultAgentMaxIterations,
			CronRunHistoryLimit: defaultCronRunHistoryLimit,
			NotifyWhenNoClients: true,
			ScheduleTimezone:    defaultScheduleTimezone,
		},
		AssistantConfig: AssistantConfig{
			AssistantEnabled:    true,
			AssistantHotkey:     defaultAssistantHotkey,
			AssistantWhisperBin: defaultAssistantWhisperBin,
			AssistantFFmpegBin:  defaultAssistantFFmpegBin,
			AssistantTTSBin:     defaultAssistantTTSBin,
		},
		ToolConfig: ToolConfig{
			ToolsDefaultSet:                 defaultToolsDefaultSet,
			ToolsWebSearchProvider:          defaultToolsWebSearchProvider,
			ToolsWebSearchPerplexityModel:   defaultPerplexityModel,
			ToolsWebSearchPerplexityBaseURL: defaultPerplexityBaseURL,
			ToolsWebSearchCacheTTLSeconds:   defaultToolsWebSearchCacheTTLSeconds,
		},
		VaultConfig: VaultConfig{
			VaultEnabled:      false,
			VaultAddr:         defaultVaultAddr,
			VaultAuthMode:     defaultVaultAuthMode,
			VaultTimeoutMS:    defaultVaultTimeoutMS,
			VaultKVMount:      defaultVaultKVMount,
			VaultKVVersion:    defaultVaultKVVersion,
			VaultAppRoleMount: defaultVaultAppRoleMount,
		},
		BrowserConfig: BrowserConfig{
			BrowserRuntimeEnabled:       true,
			BrowserDefaultProfile:       defaultBrowserDefaultProfile,
			BrowserRelayEnabled:         true,
			BrowserRelayAddr:            defaultBrowserRelayAddr,
			BrowserRelayOriginAllowlist: []string{defaultChromeExtensionOrigin},
		},
		GatewayConfig: GatewayConfig{
			GatewayAgentsWatch:                   true,
			GatewayAgentsWatchDebounceMS:         defaultGatewayWatchDebounceMS,
			GatewayPersistenceEnabled:            true,
			GatewayRunsPersistenceEnabled:        true,
			GatewayChannelsPersistenceEnabled:    true,
			GatewayRunsMaxRecords:                defaultGatewayRunsMaxRecords,
			GatewayChannelsMaxMessagesPerChannel: defaultGatewayChannelsMaxMessages,
			GatewaySubagentsMaxThreads:           defaultGatewaySubagentsMaxThreads,
			GatewaySubagentsMaxDepth:             defaultGatewaySubagentsMaxDepth,
			GatewayRestoreOnStartup:              true,
			GatewayReportSummaryEnabled:          true,
			GatewayArchiveEnabled:                false,
			GatewayArchiveRetentionDays:          defaultGatewayArchiveRetentionDays,
			GatewayArchiveMaxFileBytes:           defaultGatewayArchiveMaxFileBytes,
		},
		ChannelConfig: ChannelConfig{
			ChannelsTelegramDMPolicy:       defaultChannelsTelegramDMPolicy,
			ChannelsTelegramPollingEnabled: true,
		},
		ExtensionConfig: ExtensionConfig{
			SkillsEnabled:          true,
			SkillsWatch:            true,
			SkillsWatchDebounceMS:  defaultGatewayWatchDebounceMS,
			SkillsBundledDir:       defaultSkillsBundledDir,
			PluginsEnabled:         true,
			PluginsWatch:           true,
			PluginsWatchDebounceMS: defaultGatewayWatchDebounceMS,
			PluginsBundledDir:      defaultPluginsBundledDir,
			MCPCommandAllowlist:    []string{},
		},
	}
}
