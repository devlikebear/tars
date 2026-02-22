package config

import (
	"os"
	"strings"
)

func applyEnv(cfg *Config) {
	if v := os.Getenv("TARS_MODE"); v != "" {
		cfg.Mode = v
	}
	if v := os.Getenv("TARS_WORKSPACE_DIR"); v != "" {
		cfg.WorkspaceDir = v
	}
	if v := firstNonEmpty(os.Getenv("SESSION_DEFAULT_ID"), os.Getenv("TARS_SESSION_DEFAULT_ID")); v != "" {
		cfg.SessionDefaultID = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("SESSION_TELEGRAM_SCOPE"), os.Getenv("TARS_SESSION_TELEGRAM_SCOPE")); v != "" {
		cfg.SessionTelegramScope = strings.TrimSpace(strings.ToLower(v))
	}
	if v := firstNonEmpty(os.Getenv("API_AUTH_MODE"), os.Getenv("TARS_API_AUTH_MODE")); v != "" {
		cfg.APIAuthMode = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("API_AUTH_TOKEN"), os.Getenv("TARS_API_AUTH_TOKEN")); v != "" {
		cfg.APIAuthToken = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("API_USER_TOKEN"), os.Getenv("TARS_API_USER_TOKEN")); v != "" {
		cfg.APIUserToken = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("API_ADMIN_TOKEN"), os.Getenv("TARS_API_ADMIN_TOKEN")); v != "" {
		cfg.APIAdminToken = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("BIFROST_BASE_URL"), os.Getenv("TARS_BIFROST_BASE_URL")); v != "" {
		cfg.BifrostBase = v
	}
	if v := firstNonEmpty(os.Getenv("BIFROST_API_KEY"), os.Getenv("TARS_BIFROST_API_KEY")); v != "" {
		cfg.BifrostAPIKey = v
	}
	if v := firstNonEmpty(os.Getenv("BIFROST_MODEL"), os.Getenv("TARS_BIFROST_MODEL")); v != "" {
		cfg.BifrostModel = v
	}
	if v := firstNonEmpty(os.Getenv("LLM_PROVIDER"), os.Getenv("TARS_LLM_PROVIDER")); v != "" {
		cfg.LLMProvider = v
	}
	if v := firstNonEmpty(os.Getenv("LLM_AUTH_MODE"), os.Getenv("TARS_LLM_AUTH_MODE")); v != "" {
		cfg.LLMAuthMode = v
	}
	if v := firstNonEmpty(os.Getenv("LLM_OAUTH_PROVIDER"), os.Getenv("TARS_LLM_OAUTH_PROVIDER")); v != "" {
		cfg.LLMOAuthProvider = v
	}
	if v := firstNonEmpty(os.Getenv("LLM_BASE_URL"), os.Getenv("TARS_LLM_BASE_URL")); v != "" {
		cfg.LLMBaseURL = v
	}
	if v := firstNonEmpty(os.Getenv("LLM_API_KEY"), os.Getenv("TARS_LLM_API_KEY")); v != "" {
		cfg.LLMAPIKey = v
	}
	if v := firstNonEmpty(os.Getenv("LLM_MODEL"), os.Getenv("TARS_LLM_MODEL")); v != "" {
		cfg.LLMModel = v
	}
	if v := firstNonEmpty(os.Getenv("AGENT_MAX_ITERATIONS"), os.Getenv("TARS_AGENT_MAX_ITERATIONS")); v != "" {
		cfg.AgentMaxIterations = parsePositiveInt(v, cfg.AgentMaxIterations)
	}
	if v := firstNonEmpty(os.Getenv("HEARTBEAT_ACTIVE_HOURS"), os.Getenv("TARS_HEARTBEAT_ACTIVE_HOURS")); v != "" {
		cfg.HeartbeatActiveHours = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("HEARTBEAT_TIMEZONE"), os.Getenv("TARS_HEARTBEAT_TIMEZONE")); v != "" {
		cfg.HeartbeatTimezone = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("CRON_RUN_HISTORY_LIMIT"), os.Getenv("TARS_CRON_RUN_HISTORY_LIMIT")); v != "" {
		cfg.CronRunHistoryLimit = parsePositiveInt(v, cfg.CronRunHistoryLimit)
	}
	if v := firstNonEmpty(os.Getenv("TARS_NOTIFY_COMMAND"), os.Getenv("NOTIFY_COMMAND")); v != "" {
		cfg.NotifyCommand = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("TARS_NOTIFY_WHEN_NO_CLIENTS"), os.Getenv("NOTIFY_WHEN_NO_CLIENTS")); v != "" {
		cfg.NotifyWhenNoClients = parseBool(v, cfg.NotifyWhenNoClients)
	}
	if v := firstNonEmpty(os.Getenv("MCP_SERVERS_JSON"), os.Getenv("TARS_MCP_SERVERS_JSON")); v != "" {
		cfg.MCPServers = parseMCPServersJSON(v, cfg.MCPServers)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_SEARCH_ENABLED"), os.Getenv("TARS_TOOLS_WEB_SEARCH_ENABLED")); v != "" {
		cfg.ToolsWebSearchEnabled = parseBool(v, cfg.ToolsWebSearchEnabled)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_FETCH_ENABLED"), os.Getenv("TARS_TOOLS_WEB_FETCH_ENABLED")); v != "" {
		cfg.ToolsWebFetchEnabled = parseBool(v, cfg.ToolsWebFetchEnabled)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_SEARCH_API_KEY"), os.Getenv("TARS_TOOLS_WEB_SEARCH_API_KEY")); v != "" {
		cfg.ToolsWebSearchAPIKey = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_SEARCH_PROVIDER"), os.Getenv("TARS_TOOLS_WEB_SEARCH_PROVIDER")); v != "" {
		cfg.ToolsWebSearchProvider = strings.TrimSpace(strings.ToLower(v))
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_SEARCH_PERPLEXITY_API_KEY"), os.Getenv("TARS_TOOLS_WEB_SEARCH_PERPLEXITY_API_KEY")); v != "" {
		cfg.ToolsWebSearchPerplexityAPIKey = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_SEARCH_PERPLEXITY_MODEL"), os.Getenv("TARS_TOOLS_WEB_SEARCH_PERPLEXITY_MODEL")); v != "" {
		cfg.ToolsWebSearchPerplexityModel = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_SEARCH_PERPLEXITY_BASE_URL"), os.Getenv("TARS_TOOLS_WEB_SEARCH_PERPLEXITY_BASE_URL")); v != "" {
		cfg.ToolsWebSearchPerplexityBaseURL = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_SEARCH_CACHE_TTL_SECONDS"), os.Getenv("TARS_TOOLS_WEB_SEARCH_CACHE_TTL_SECONDS")); v != "" {
		cfg.ToolsWebSearchCacheTTLSeconds = parsePositiveInt(v, cfg.ToolsWebSearchCacheTTLSeconds)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_FETCH_PRIVATE_HOST_ALLOWLIST_JSON"), os.Getenv("TARS_TOOLS_WEB_FETCH_PRIVATE_HOST_ALLOWLIST_JSON")); v != "" {
		cfg.ToolsWebFetchPrivateHostAllowlist = parseJSONStringList(v, cfg.ToolsWebFetchPrivateHostAllowlist)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_FETCH_ALLOW_PRIVATE_HOSTS"), os.Getenv("TARS_TOOLS_WEB_FETCH_ALLOW_PRIVATE_HOSTS")); v != "" {
		cfg.ToolsWebFetchAllowPrivateHosts = parseBool(v, cfg.ToolsWebFetchAllowPrivateHosts)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_APPLY_PATCH_ENABLED"), os.Getenv("TARS_TOOLS_APPLY_PATCH_ENABLED")); v != "" {
		cfg.ToolsApplyPatchEnabled = parseBool(v, cfg.ToolsApplyPatchEnabled)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_ENABLED"), os.Getenv("TARS_VAULT_ENABLED")); v != "" {
		cfg.VaultEnabled = parseBool(v, cfg.VaultEnabled)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_ADDR"), os.Getenv("TARS_VAULT_ADDR")); v != "" {
		cfg.VaultAddr = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_AUTH_MODE"), os.Getenv("TARS_VAULT_AUTH_MODE")); v != "" {
		cfg.VaultAuthMode = strings.TrimSpace(strings.ToLower(v))
	}
	if v := firstNonEmpty(os.Getenv("VAULT_TOKEN"), os.Getenv("TARS_VAULT_TOKEN")); v != "" {
		cfg.VaultToken = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_NAMESPACE"), os.Getenv("TARS_VAULT_NAMESPACE")); v != "" {
		cfg.VaultNamespace = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_TIMEOUT_MS"), os.Getenv("TARS_VAULT_TIMEOUT_MS")); v != "" {
		cfg.VaultTimeoutMS = parsePositiveInt(v, cfg.VaultTimeoutMS)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_KV_MOUNT"), os.Getenv("TARS_VAULT_KV_MOUNT")); v != "" {
		cfg.VaultKVMount = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_KV_VERSION"), os.Getenv("TARS_VAULT_KV_VERSION")); v != "" {
		cfg.VaultKVVersion = parsePositiveInt(v, cfg.VaultKVVersion)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_APPROLE_MOUNT"), os.Getenv("TARS_VAULT_APPROLE_MOUNT")); v != "" {
		cfg.VaultAppRoleMount = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_APPROLE_ROLE_ID"), os.Getenv("TARS_VAULT_APPROLE_ROLE_ID")); v != "" {
		cfg.VaultAppRoleRoleID = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_APPROLE_SECRET_ID"), os.Getenv("TARS_VAULT_APPROLE_SECRET_ID")); v != "" {
		cfg.VaultAppRoleSecretID = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_SECRET_PATH_ALLOWLIST_JSON"), os.Getenv("TARS_VAULT_SECRET_PATH_ALLOWLIST_JSON")); v != "" {
		cfg.VaultSecretPathAllowlist = parseJSONStringList(v, cfg.VaultSecretPathAllowlist)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_RUNTIME_ENABLED"), os.Getenv("TARS_BROWSER_RUNTIME_ENABLED")); v != "" {
		cfg.BrowserRuntimeEnabled = parseBool(v, cfg.BrowserRuntimeEnabled)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_DEFAULT_PROFILE"), os.Getenv("TARS_BROWSER_DEFAULT_PROFILE")); v != "" {
		cfg.BrowserDefaultProfile = strings.TrimSpace(strings.ToLower(v))
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_MANAGED_HEADLESS"), os.Getenv("TARS_BROWSER_MANAGED_HEADLESS")); v != "" {
		cfg.BrowserManagedHeadless = parseBool(v, cfg.BrowserManagedHeadless)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_MANAGED_EXECUTABLE_PATH"), os.Getenv("TARS_BROWSER_MANAGED_EXECUTABLE_PATH")); v != "" {
		cfg.BrowserManagedExecutablePath = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_MANAGED_USER_DATA_DIR"), os.Getenv("TARS_BROWSER_MANAGED_USER_DATA_DIR")); v != "" {
		cfg.BrowserManagedUserDataDir = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_RELAY_ENABLED"), os.Getenv("TARS_BROWSER_RELAY_ENABLED")); v != "" {
		cfg.BrowserRelayEnabled = parseBool(v, cfg.BrowserRelayEnabled)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_RELAY_ADDR"), os.Getenv("TARS_BROWSER_RELAY_ADDR")); v != "" {
		cfg.BrowserRelayAddr = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_RELAY_TOKEN"), os.Getenv("TARS_BROWSER_RELAY_TOKEN")); v != "" {
		cfg.BrowserRelayToken = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_RELAY_ORIGIN_ALLOWLIST_JSON"), os.Getenv("TARS_BROWSER_RELAY_ORIGIN_ALLOWLIST_JSON")); v != "" {
		cfg.BrowserRelayOriginAllowlist = parseJSONStringList(v, cfg.BrowserRelayOriginAllowlist)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_SITE_FLOWS_DIR"), os.Getenv("TARS_BROWSER_SITE_FLOWS_DIR")); v != "" {
		cfg.BrowserSiteFlowsDir = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_AUTO_LOGIN_SITE_ALLOWLIST_JSON"), os.Getenv("TARS_BROWSER_AUTO_LOGIN_SITE_ALLOWLIST_JSON")); v != "" {
		cfg.BrowserAutoLoginSiteAllowlist = parseJSONStringList(v, cfg.BrowserAutoLoginSiteAllowlist)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_ENABLED"), os.Getenv("TARS_GATEWAY_ENABLED")); v != "" {
		cfg.GatewayEnabled = parseBool(v, cfg.GatewayEnabled)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_DEFAULT_AGENT"), os.Getenv("TARS_GATEWAY_DEFAULT_AGENT")); v != "" {
		cfg.GatewayDefaultAgent = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_AGENTS_JSON"), os.Getenv("TARS_GATEWAY_AGENTS_JSON")); v != "" {
		cfg.GatewayAgents = parseGatewayAgentsJSON(v, cfg.GatewayAgents)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_AGENTS_WATCH"), os.Getenv("TARS_GATEWAY_AGENTS_WATCH")); v != "" {
		cfg.GatewayAgentsWatch = parseBool(v, cfg.GatewayAgentsWatch)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_AGENTS_WATCH_DEBOUNCE_MS"), os.Getenv("TARS_GATEWAY_AGENTS_WATCH_DEBOUNCE_MS")); v != "" {
		cfg.GatewayAgentsWatchDebounceMS = parsePositiveInt(v, cfg.GatewayAgentsWatchDebounceMS)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_PERSISTENCE_ENABLED"), os.Getenv("TARS_GATEWAY_PERSISTENCE_ENABLED")); v != "" {
		cfg.GatewayPersistenceEnabled = parseBool(v, cfg.GatewayPersistenceEnabled)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_RUNS_PERSISTENCE_ENABLED"), os.Getenv("TARS_GATEWAY_RUNS_PERSISTENCE_ENABLED")); v != "" {
		cfg.GatewayRunsPersistenceEnabled = parseBool(v, cfg.GatewayRunsPersistenceEnabled)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_CHANNELS_PERSISTENCE_ENABLED"), os.Getenv("TARS_GATEWAY_CHANNELS_PERSISTENCE_ENABLED")); v != "" {
		cfg.GatewayChannelsPersistenceEnabled = parseBool(v, cfg.GatewayChannelsPersistenceEnabled)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_RUNS_MAX_RECORDS"), os.Getenv("TARS_GATEWAY_RUNS_MAX_RECORDS")); v != "" {
		cfg.GatewayRunsMaxRecords = parsePositiveInt(v, cfg.GatewayRunsMaxRecords)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_CHANNELS_MAX_MESSAGES_PER_CHANNEL"), os.Getenv("TARS_GATEWAY_CHANNELS_MAX_MESSAGES_PER_CHANNEL")); v != "" {
		cfg.GatewayChannelsMaxMessagesPerChannel = parsePositiveInt(v, cfg.GatewayChannelsMaxMessagesPerChannel)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_PERSISTENCE_DIR"), os.Getenv("TARS_GATEWAY_PERSISTENCE_DIR")); v != "" {
		cfg.GatewayPersistenceDir = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_RESTORE_ON_STARTUP"), os.Getenv("TARS_GATEWAY_RESTORE_ON_STARTUP")); v != "" {
		cfg.GatewayRestoreOnStartup = parseBool(v, cfg.GatewayRestoreOnStartup)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_REPORT_SUMMARY_ENABLED"), os.Getenv("TARS_GATEWAY_REPORT_SUMMARY_ENABLED")); v != "" {
		cfg.GatewayReportSummaryEnabled = parseBool(v, cfg.GatewayReportSummaryEnabled)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_ARCHIVE_ENABLED"), os.Getenv("TARS_GATEWAY_ARCHIVE_ENABLED")); v != "" {
		cfg.GatewayArchiveEnabled = parseBool(v, cfg.GatewayArchiveEnabled)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_ARCHIVE_DIR"), os.Getenv("TARS_GATEWAY_ARCHIVE_DIR")); v != "" {
		cfg.GatewayArchiveDir = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_ARCHIVE_RETENTION_DAYS"), os.Getenv("TARS_GATEWAY_ARCHIVE_RETENTION_DAYS")); v != "" {
		cfg.GatewayArchiveRetentionDays = parsePositiveInt(v, cfg.GatewayArchiveRetentionDays)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_ARCHIVE_MAX_FILE_BYTES"), os.Getenv("TARS_GATEWAY_ARCHIVE_MAX_FILE_BYTES")); v != "" {
		cfg.GatewayArchiveMaxFileBytes = parsePositiveInt(v, cfg.GatewayArchiveMaxFileBytes)
	}
	if v := firstNonEmpty(os.Getenv("CHANNELS_LOCAL_ENABLED"), os.Getenv("TARS_CHANNELS_LOCAL_ENABLED")); v != "" {
		cfg.ChannelsLocalEnabled = parseBool(v, cfg.ChannelsLocalEnabled)
	}
	if v := firstNonEmpty(os.Getenv("CHANNELS_WEBHOOK_ENABLED"), os.Getenv("TARS_CHANNELS_WEBHOOK_ENABLED")); v != "" {
		cfg.ChannelsWebhookEnabled = parseBool(v, cfg.ChannelsWebhookEnabled)
	}
	if v := firstNonEmpty(os.Getenv("CHANNELS_TELEGRAM_ENABLED"), os.Getenv("TARS_CHANNELS_TELEGRAM_ENABLED")); v != "" {
		cfg.ChannelsTelegramEnabled = parseBool(v, cfg.ChannelsTelegramEnabled)
	}
	if v := firstNonEmpty(os.Getenv("CHANNELS_TELEGRAM_DM_POLICY"), os.Getenv("TARS_CHANNELS_TELEGRAM_DM_POLICY")); v != "" {
		cfg.ChannelsTelegramDMPolicy = strings.TrimSpace(strings.ToLower(v))
	}
	if v := firstNonEmpty(os.Getenv("CHANNELS_TELEGRAM_POLLING_ENABLED"), os.Getenv("TARS_CHANNELS_TELEGRAM_POLLING_ENABLED")); v != "" {
		cfg.ChannelsTelegramPollingEnabled = parseBool(v, cfg.ChannelsTelegramPollingEnabled)
	}
	if v := firstNonEmpty(os.Getenv("TELEGRAM_BOT_TOKEN"), os.Getenv("TARS_TELEGRAM_BOT_TOKEN")); v != "" {
		cfg.TelegramBotToken = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_MESSAGE_ENABLED"), os.Getenv("TARS_TOOLS_MESSAGE_ENABLED")); v != "" {
		cfg.ToolsMessageEnabled = parseBool(v, cfg.ToolsMessageEnabled)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_BROWSER_ENABLED"), os.Getenv("TARS_TOOLS_BROWSER_ENABLED")); v != "" {
		cfg.ToolsBrowserEnabled = parseBool(v, cfg.ToolsBrowserEnabled)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_NODES_ENABLED"), os.Getenv("TARS_TOOLS_NODES_ENABLED")); v != "" {
		cfg.ToolsNodesEnabled = parseBool(v, cfg.ToolsNodesEnabled)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_GATEWAY_ENABLED"), os.Getenv("TARS_TOOLS_GATEWAY_ENABLED")); v != "" {
		cfg.ToolsGatewayEnabled = parseBool(v, cfg.ToolsGatewayEnabled)
	}
	if v := firstNonEmpty(os.Getenv("SKILLS_ENABLED"), os.Getenv("TARS_SKILLS_ENABLED")); v != "" {
		cfg.SkillsEnabled = parseBool(v, cfg.SkillsEnabled)
	}
	if v := firstNonEmpty(os.Getenv("SKILLS_WATCH"), os.Getenv("TARS_SKILLS_WATCH")); v != "" {
		cfg.SkillsWatch = parseBool(v, cfg.SkillsWatch)
	}
	if v := firstNonEmpty(os.Getenv("SKILLS_WATCH_DEBOUNCE_MS"), os.Getenv("TARS_SKILLS_WATCH_DEBOUNCE_MS")); v != "" {
		cfg.SkillsWatchDebounceMS = parsePositiveInt(v, cfg.SkillsWatchDebounceMS)
	}
	if v := firstNonEmpty(os.Getenv("SKILLS_EXTRA_DIRS_JSON"), os.Getenv("TARS_SKILLS_EXTRA_DIRS_JSON")); v != "" {
		cfg.SkillsExtraDirs = parseJSONStringList(v, cfg.SkillsExtraDirs)
	}
	if v := firstNonEmpty(os.Getenv("SKILLS_BUNDLED_DIR"), os.Getenv("TARS_SKILLS_BUNDLED_DIR")); v != "" {
		cfg.SkillsBundledDir = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("PLUGINS_ENABLED"), os.Getenv("TARS_PLUGINS_ENABLED")); v != "" {
		cfg.PluginsEnabled = parseBool(v, cfg.PluginsEnabled)
	}
	if v := firstNonEmpty(os.Getenv("PLUGINS_WATCH"), os.Getenv("TARS_PLUGINS_WATCH")); v != "" {
		cfg.PluginsWatch = parseBool(v, cfg.PluginsWatch)
	}
	if v := firstNonEmpty(os.Getenv("PLUGINS_WATCH_DEBOUNCE_MS"), os.Getenv("TARS_PLUGINS_WATCH_DEBOUNCE_MS")); v != "" {
		cfg.PluginsWatchDebounceMS = parsePositiveInt(v, cfg.PluginsWatchDebounceMS)
	}
	if v := firstNonEmpty(os.Getenv("PLUGINS_EXTRA_DIRS_JSON"), os.Getenv("TARS_PLUGINS_EXTRA_DIRS_JSON")); v != "" {
		cfg.PluginsExtraDirs = parseJSONStringList(v, cfg.PluginsExtraDirs)
	}
	if v := firstNonEmpty(os.Getenv("PLUGINS_BUNDLED_DIR"), os.Getenv("TARS_PLUGINS_BUNDLED_DIR")); v != "" {
		cfg.PluginsBundledDir = strings.TrimSpace(v)
	}
}
