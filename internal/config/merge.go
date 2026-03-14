package config

func merge(dst *Config, src Config) {
	if src.Mode != "" {
		dst.Mode = src.Mode
	}
	if src.WorkspaceDir != "" {
		dst.WorkspaceDir = src.WorkspaceDir
	}
	if src.SessionDefaultID != "" {
		dst.SessionDefaultID = src.SessionDefaultID
	}
	if src.SessionTelegramScope != "" {
		dst.SessionTelegramScope = src.SessionTelegramScope
	}
	if src.APIAuthMode != "" {
		dst.APIAuthMode = src.APIAuthMode
	}
	if src.DashboardAuthMode != "" {
		dst.DashboardAuthMode = src.DashboardAuthMode
	}
	if src.APIAuthToken != "" {
		dst.APIAuthToken = src.APIAuthToken
	}
	if src.APIUserToken != "" {
		dst.APIUserToken = src.APIUserToken
	}
	if src.APIAdminToken != "" {
		dst.APIAdminToken = src.APIAdminToken
	}
	if src.APIAllowInsecureLocalAuth {
		dst.APIAllowInsecureLocalAuth = true
	}
	if src.APIMaxInflightChat > 0 {
		dst.APIMaxInflightChat = src.APIMaxInflightChat
	}
	if src.APIMaxInflightAgentRuns > 0 {
		dst.APIMaxInflightAgentRuns = src.APIMaxInflightAgentRuns
	}
	if src.BifrostBase != "" {
		dst.BifrostBase = src.BifrostBase
	}
	if src.BifrostAPIKey != "" {
		dst.BifrostAPIKey = src.BifrostAPIKey
	}
	if src.BifrostModel != "" {
		dst.BifrostModel = src.BifrostModel
	}
	if src.LLMProvider != "" {
		dst.LLMProvider = src.LLMProvider
	}
	if src.LLMAuthMode != "" {
		dst.LLMAuthMode = src.LLMAuthMode
	}
	if src.LLMOAuthProvider != "" {
		dst.LLMOAuthProvider = src.LLMOAuthProvider
	}
	if src.LLMBaseURL != "" {
		dst.LLMBaseURL = src.LLMBaseURL
	}
	if src.LLMAPIKey != "" {
		dst.LLMAPIKey = src.LLMAPIKey
	}
	if src.LLMModel != "" {
		dst.LLMModel = src.LLMModel
	}
	if src.LLMReasoningEffort != "" {
		dst.LLMReasoningEffort = src.LLMReasoningEffort
	}
	if src.LLMThinkingBudget > 0 {
		dst.LLMThinkingBudget = src.LLMThinkingBudget
	}
	if src.LLMServiceTier != "" {
		dst.LLMServiceTier = src.LLMServiceTier
	}
	if src.UsageLimitDailyUSD > 0 {
		dst.UsageLimitDailyUSD = src.UsageLimitDailyUSD
	}
	if src.UsageLimitWeeklyUSD > 0 {
		dst.UsageLimitWeeklyUSD = src.UsageLimitWeeklyUSD
	}
	if src.UsageLimitMonthlyUSD > 0 {
		dst.UsageLimitMonthlyUSD = src.UsageLimitMonthlyUSD
	}
	if src.UsageLimitMode != "" {
		dst.UsageLimitMode = src.UsageLimitMode
	}
	if len(src.UsagePriceOverrides) > 0 {
		dst.UsagePriceOverrides = map[string]UsagePrice{}
		for key, value := range src.UsagePriceOverrides {
			dst.UsagePriceOverrides[key] = value
		}
	}
	if src.AgentMaxIterations > 0 {
		dst.AgentMaxIterations = src.AgentMaxIterations
	}
	if src.HeartbeatActiveHours != "" {
		dst.HeartbeatActiveHours = src.HeartbeatActiveHours
	}
	if src.HeartbeatTimezone != "" {
		dst.HeartbeatTimezone = src.HeartbeatTimezone
	}
	if src.CronRunHistoryLimit > 0 {
		dst.CronRunHistoryLimit = src.CronRunHistoryLimit
	}
	if src.NotifyCommand != "" {
		dst.NotifyCommand = src.NotifyCommand
	}
	if src.AssistantEnabled {
		dst.AssistantEnabled = true
	}
	if src.AssistantHotkey != "" {
		dst.AssistantHotkey = src.AssistantHotkey
	}
	if src.AssistantWhisperBin != "" {
		dst.AssistantWhisperBin = src.AssistantWhisperBin
	}
	if src.AssistantFFmpegBin != "" {
		dst.AssistantFFmpegBin = src.AssistantFFmpegBin
	}
	if src.AssistantTTSBin != "" {
		dst.AssistantTTSBin = src.AssistantTTSBin
	}
	if src.ScheduleTimezone != "" {
		dst.ScheduleTimezone = src.ScheduleTimezone
	}
	if len(src.MCPServers) > 0 {
		dst.MCPServers = src.MCPServers
	}
	if src.ToolsWebSearchEnabled {
		dst.ToolsWebSearchEnabled = true
	}
	if src.ToolsWebFetchEnabled {
		dst.ToolsWebFetchEnabled = true
	}
	if src.ToolsDefaultSet != "" {
		dst.ToolsDefaultSet = src.ToolsDefaultSet
	}
	if src.ToolsAllowHighRiskUser {
		dst.ToolsAllowHighRiskUser = true
	}
	if src.ToolsWebSearchAPIKey != "" {
		dst.ToolsWebSearchAPIKey = src.ToolsWebSearchAPIKey
	}
	if src.ToolsWebSearchProvider != "" {
		dst.ToolsWebSearchProvider = src.ToolsWebSearchProvider
	}
	if src.ToolsWebSearchPerplexityAPIKey != "" {
		dst.ToolsWebSearchPerplexityAPIKey = src.ToolsWebSearchPerplexityAPIKey
	}
	if src.ToolsWebSearchPerplexityModel != "" {
		dst.ToolsWebSearchPerplexityModel = src.ToolsWebSearchPerplexityModel
	}
	if src.ToolsWebSearchPerplexityBaseURL != "" {
		dst.ToolsWebSearchPerplexityBaseURL = src.ToolsWebSearchPerplexityBaseURL
	}
	if src.ToolsWebSearchCacheTTLSeconds > 0 {
		dst.ToolsWebSearchCacheTTLSeconds = src.ToolsWebSearchCacheTTLSeconds
	}
	if len(src.ToolsWebFetchPrivateHostAllowlist) > 0 {
		dst.ToolsWebFetchPrivateHostAllowlist = append([]string(nil), src.ToolsWebFetchPrivateHostAllowlist...)
	}
	if src.ToolsWebFetchAllowPrivateHosts {
		dst.ToolsWebFetchAllowPrivateHosts = true
	}
	if src.ToolsApplyPatchEnabled {
		dst.ToolsApplyPatchEnabled = true
	}
	if src.VaultEnabled {
		dst.VaultEnabled = true
	}
	if src.VaultAddr != "" {
		dst.VaultAddr = src.VaultAddr
	}
	if src.VaultAuthMode != "" {
		dst.VaultAuthMode = src.VaultAuthMode
	}
	if src.VaultToken != "" {
		dst.VaultToken = src.VaultToken
	}
	if src.VaultNamespace != "" {
		dst.VaultNamespace = src.VaultNamespace
	}
	if src.VaultTimeoutMS > 0 {
		dst.VaultTimeoutMS = src.VaultTimeoutMS
	}
	if src.VaultKVMount != "" {
		dst.VaultKVMount = src.VaultKVMount
	}
	if src.VaultKVVersion > 0 {
		dst.VaultKVVersion = src.VaultKVVersion
	}
	if src.VaultAppRoleMount != "" {
		dst.VaultAppRoleMount = src.VaultAppRoleMount
	}
	if src.VaultAppRoleRoleID != "" {
		dst.VaultAppRoleRoleID = src.VaultAppRoleRoleID
	}
	if src.VaultAppRoleSecretID != "" {
		dst.VaultAppRoleSecretID = src.VaultAppRoleSecretID
	}
	if len(src.VaultSecretPathAllowlist) > 0 {
		dst.VaultSecretPathAllowlist = append([]string(nil), src.VaultSecretPathAllowlist...)
	}
	if src.BrowserRuntimeEnabled {
		dst.BrowserRuntimeEnabled = true
	}
	if src.BrowserDefaultProfile != "" {
		dst.BrowserDefaultProfile = src.BrowserDefaultProfile
	}
	if src.BrowserManagedHeadless {
		dst.BrowserManagedHeadless = true
	}
	if src.BrowserManagedExecutablePath != "" {
		dst.BrowserManagedExecutablePath = src.BrowserManagedExecutablePath
	}
	if src.BrowserManagedUserDataDir != "" {
		dst.BrowserManagedUserDataDir = src.BrowserManagedUserDataDir
	}
	if src.BrowserRelayEnabled {
		dst.BrowserRelayEnabled = true
	}
	if src.BrowserRelayAddr != "" {
		dst.BrowserRelayAddr = src.BrowserRelayAddr
	}
	if src.BrowserRelayToken != "" {
		dst.BrowserRelayToken = src.BrowserRelayToken
	}
	if src.BrowserRelayAllowQueryToken {
		dst.BrowserRelayAllowQueryToken = true
	}
	if len(src.BrowserRelayOriginAllowlist) > 0 {
		dst.BrowserRelayOriginAllowlist = append([]string(nil), src.BrowserRelayOriginAllowlist...)
	}
	if src.BrowserSiteFlowsDir != "" {
		dst.BrowserSiteFlowsDir = src.BrowserSiteFlowsDir
	}
	if len(src.BrowserAutoLoginSiteAllowlist) > 0 {
		dst.BrowserAutoLoginSiteAllowlist = append([]string(nil), src.BrowserAutoLoginSiteAllowlist...)
	}
	if src.GatewayEnabled {
		dst.GatewayEnabled = true
	}
	if src.GatewayDefaultAgent != "" {
		dst.GatewayDefaultAgent = src.GatewayDefaultAgent
	}
	if len(src.GatewayAgents) > 0 {
		dst.GatewayAgents = append([]GatewayAgent(nil), src.GatewayAgents...)
	}
	if src.GatewayAgentsWatch {
		dst.GatewayAgentsWatch = true
	}
	if src.GatewayAgentsWatchDebounceMS > 0 {
		dst.GatewayAgentsWatchDebounceMS = src.GatewayAgentsWatchDebounceMS
	}
	if src.GatewayPersistenceEnabled {
		dst.GatewayPersistenceEnabled = true
	}
	if src.GatewayRunsPersistenceEnabled {
		dst.GatewayRunsPersistenceEnabled = true
	}
	if src.GatewayChannelsPersistenceEnabled {
		dst.GatewayChannelsPersistenceEnabled = true
	}
	if src.GatewayRunsMaxRecords > 0 {
		dst.GatewayRunsMaxRecords = src.GatewayRunsMaxRecords
	}
	if src.GatewayChannelsMaxMessagesPerChannel > 0 {
		dst.GatewayChannelsMaxMessagesPerChannel = src.GatewayChannelsMaxMessagesPerChannel
	}
	if src.GatewayPersistenceDir != "" {
		dst.GatewayPersistenceDir = src.GatewayPersistenceDir
	}
	if src.GatewayRestoreOnStartup {
		dst.GatewayRestoreOnStartup = true
	}
	if src.GatewayReportSummaryEnabled {
		dst.GatewayReportSummaryEnabled = true
	}
	if src.GatewayArchiveEnabled {
		dst.GatewayArchiveEnabled = true
	}
	if src.GatewayArchiveDir != "" {
		dst.GatewayArchiveDir = src.GatewayArchiveDir
	}
	if src.GatewayArchiveRetentionDays > 0 {
		dst.GatewayArchiveRetentionDays = src.GatewayArchiveRetentionDays
	}
	if src.GatewayArchiveMaxFileBytes > 0 {
		dst.GatewayArchiveMaxFileBytes = src.GatewayArchiveMaxFileBytes
	}
	if src.ChannelsLocalEnabled {
		dst.ChannelsLocalEnabled = true
	}
	if src.ChannelsWebhookEnabled {
		dst.ChannelsWebhookEnabled = true
	}
	if src.ChannelsTelegramEnabled {
		dst.ChannelsTelegramEnabled = true
	}
	if src.ChannelsTelegramDMPolicy != "" {
		dst.ChannelsTelegramDMPolicy = src.ChannelsTelegramDMPolicy
	}
	if src.ChannelsTelegramPollingEnabled {
		dst.ChannelsTelegramPollingEnabled = true
	}
	if src.TelegramBotToken != "" {
		dst.TelegramBotToken = src.TelegramBotToken
	}
	if src.ToolsMessageEnabled {
		dst.ToolsMessageEnabled = true
	}
	if src.ToolsBrowserEnabled {
		dst.ToolsBrowserEnabled = true
	}
	if src.ToolsNodesEnabled {
		dst.ToolsNodesEnabled = true
	}
	if src.ToolsGatewayEnabled {
		dst.ToolsGatewayEnabled = true
	}
	if src.SkillsEnabled {
		dst.SkillsEnabled = true
	}
	if src.SkillsWatch {
		dst.SkillsWatch = true
	}
	if src.SkillsWatchDebounceMS > 0 {
		dst.SkillsWatchDebounceMS = src.SkillsWatchDebounceMS
	}
	if len(src.SkillsExtraDirs) > 0 {
		dst.SkillsExtraDirs = append([]string(nil), src.SkillsExtraDirs...)
	}
	if src.SkillsBundledDir != "" {
		dst.SkillsBundledDir = src.SkillsBundledDir
	}
	if src.PluginsEnabled {
		dst.PluginsEnabled = true
	}
	if src.PluginsWatch {
		dst.PluginsWatch = true
	}
	if src.PluginsWatchDebounceMS > 0 {
		dst.PluginsWatchDebounceMS = src.PluginsWatchDebounceMS
	}
	if len(src.PluginsExtraDirs) > 0 {
		dst.PluginsExtraDirs = append([]string(nil), src.PluginsExtraDirs...)
	}
	if src.PluginsBundledDir != "" {
		dst.PluginsBundledDir = src.PluginsBundledDir
	}
	if src.PluginsAllowMCPServers {
		dst.PluginsAllowMCPServers = true
	}
	if len(src.MCPCommandAllowlist) > 0 {
		dst.MCPCommandAllowlist = append([]string(nil), src.MCPCommandAllowlist...)
	}
}
