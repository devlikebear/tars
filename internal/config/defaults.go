package config

const (
	defaultMode                          = "standalone"
	defaultWorkspaceDir                  = "./workspace"
	defaultSessionTelegramScope          = "main"
	defaultAPIAuthMode                   = "required"
	defaultAPIMaxInflightChat            = 2
	defaultAPIMaxInflightAgentRuns       = 4
	defaultLLMProvider                   = "bifrost"
	defaultLLMAuthMode                   = "api-key"
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
	defaultGatewayArchiveRetentionDays   = 30
	defaultGatewayArchiveMaxFileBytes    = 10485760
	defaultChannelsTelegramDMPolicy      = "pairing"
	defaultSkillsBundledDir              = "./skills"
	defaultPluginsBundledDir             = "./plugins"
	defaultOpenAIBaseURL                 = "https://api.openai.com/v1"
	defaultOpenAIModel                   = "gpt-4o-mini"
	defaultOpenAICodexBaseURL            = "https://chatgpt.com/backend-api"
	defaultOpenAICodexModel              = "gpt-5.3-codex"
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
		Mode:                                 defaultMode,
		WorkspaceDir:                         defaultWorkspaceDir,
		SessionTelegramScope:                 defaultSessionTelegramScope,
		APIAuthMode:                          defaultAPIAuthMode,
		APIMaxInflightChat:                   defaultAPIMaxInflightChat,
		APIMaxInflightAgentRuns:              defaultAPIMaxInflightAgentRuns,
		LLMProvider:                          defaultLLMProvider,
		LLMAuthMode:                          defaultLLMAuthMode,
		UsageLimitDailyUSD:                   defaultUsageLimitDailyUSD,
		UsageLimitWeeklyUSD:                  defaultUsageLimitWeeklyUSD,
		UsageLimitMonthlyUSD:                 defaultUsageLimitMonthlyUSD,
		UsageLimitMode:                       defaultUsageLimitMode,
		BifrostModel:                         defaultBifrostModel,
		AgentMaxIterations:                   defaultAgentMaxIterations,
		CronRunHistoryLimit:                  defaultCronRunHistoryLimit,
		NotifyWhenNoClients:                  true,
		AssistantEnabled:                     true,
		AssistantHotkey:                      defaultAssistantHotkey,
		AssistantWhisperBin:                  defaultAssistantWhisperBin,
		AssistantFFmpegBin:                   defaultAssistantFFmpegBin,
		AssistantTTSBin:                      defaultAssistantTTSBin,
		ScheduleTimezone:                     defaultScheduleTimezone,
		ToolsDefaultSet:                      defaultToolsDefaultSet,
		ToolsWebSearchProvider:               defaultToolsWebSearchProvider,
		ToolsWebSearchPerplexityModel:        defaultPerplexityModel,
		ToolsWebSearchPerplexityBaseURL:      defaultPerplexityBaseURL,
		ToolsWebSearchCacheTTLSeconds:        defaultToolsWebSearchCacheTTLSeconds,
		VaultEnabled:                         false,
		VaultAddr:                            defaultVaultAddr,
		VaultAuthMode:                        defaultVaultAuthMode,
		VaultTimeoutMS:                       defaultVaultTimeoutMS,
		VaultKVMount:                         defaultVaultKVMount,
		VaultKVVersion:                       defaultVaultKVVersion,
		VaultAppRoleMount:                    defaultVaultAppRoleMount,
		BrowserRuntimeEnabled:                true,
		BrowserDefaultProfile:                defaultBrowserDefaultProfile,
		BrowserRelayEnabled:                  true,
		BrowserRelayAddr:                     defaultBrowserRelayAddr,
		BrowserRelayOriginAllowlist:          []string{defaultChromeExtensionOrigin},
		GatewayAgentsWatch:                   true,
		GatewayAgentsWatchDebounceMS:         defaultGatewayWatchDebounceMS,
		GatewayPersistenceEnabled:            true,
		GatewayRunsPersistenceEnabled:        true,
		GatewayChannelsPersistenceEnabled:    true,
		GatewayRunsMaxRecords:                defaultGatewayRunsMaxRecords,
		GatewayChannelsMaxMessagesPerChannel: defaultGatewayChannelsMaxMessages,
		GatewayRestoreOnStartup:              true,
		GatewayReportSummaryEnabled:          true,
		GatewayArchiveEnabled:                false,
		GatewayArchiveRetentionDays:          defaultGatewayArchiveRetentionDays,
		GatewayArchiveMaxFileBytes:           defaultGatewayArchiveMaxFileBytes,
		ChannelsTelegramDMPolicy:             defaultChannelsTelegramDMPolicy,
		ChannelsTelegramPollingEnabled:       true,
		SkillsEnabled:                        true,
		SkillsWatch:                          true,
		SkillsWatchDebounceMS:                defaultGatewayWatchDebounceMS,
		SkillsBundledDir:                     defaultSkillsBundledDir,
		PluginsEnabled:                       true,
		PluginsWatch:                         true,
		PluginsWatchDebounceMS:               defaultGatewayWatchDebounceMS,
		PluginsBundledDir:                    defaultPluginsBundledDir,
		MCPCommandAllowlist:                  []string{},
	}
}
