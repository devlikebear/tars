package config

import (
	"os"
	"strings"
)

type configInputField struct {
	yamlKey string
	envKeys []string
	apply   func(*Config, string)
	merge   func(*Config, Config)
}

var configInputFields = []configInputField{
	stringField("mode", []string{"TARS_MODE"}, func(cfg *Config) *string { return &cfg.Mode }, identityString),
	stringField("workspace_dir", []string{"TARS_WORKSPACE_DIR"}, func(cfg *Config) *string { return &cfg.WorkspaceDir }, identityString),
	stringField("session_default_id", []string{"SESSION_DEFAULT_ID", "TARS_SESSION_DEFAULT_ID"}, func(cfg *Config) *string { return &cfg.SessionDefaultID }, strings.TrimSpace),
	stringField("session_telegram_scope", []string{"SESSION_TELEGRAM_SCOPE", "TARS_SESSION_TELEGRAM_SCOPE"}, func(cfg *Config) *string { return &cfg.SessionTelegramScope }, lowerTrimmedString),
	stringField("log_level", []string{"LOG_LEVEL", "TARS_LOG_LEVEL"}, func(cfg *Config) *string { return &cfg.LogLevel }, lowerTrimmedString),
	stringField("log_file", []string{"LOG_FILE", "TARS_LOG_FILE"}, func(cfg *Config) *string { return &cfg.LogFile }, strings.TrimSpace),
	intField("log_rotate_max_size_mb", []string{"LOG_ROTATE_MAX_SIZE_MB", "TARS_LOG_ROTATE_MAX_SIZE_MB"}, func(cfg *Config) *int { return &cfg.LogRotateMaxSizeMB }, parsePositiveInt),
	intField("log_rotate_max_days", []string{"LOG_ROTATE_MAX_DAYS", "TARS_LOG_ROTATE_MAX_DAYS"}, func(cfg *Config) *int { return &cfg.LogRotateMaxDays }, parsePositiveInt),
	intField("log_rotate_max_backups", []string{"LOG_ROTATE_MAX_BACKUPS", "TARS_LOG_ROTATE_MAX_BACKUPS"}, func(cfg *Config) *int { return &cfg.LogRotateMaxBackups }, parsePositiveInt),
	stringField("api_auth_mode", []string{"API_AUTH_MODE", "TARS_API_AUTH_MODE"}, func(cfg *Config) *string { return &cfg.APIAuthMode }, strings.TrimSpace),
	stringField("dashboard_auth_mode", []string{"DASHBOARD_AUTH_MODE", "TARS_DASHBOARD_AUTH_MODE"}, func(cfg *Config) *string { return &cfg.DashboardAuthMode }, strings.TrimSpace),
	stringField("api_auth_token", []string{"API_AUTH_TOKEN", "TARS_API_AUTH_TOKEN"}, func(cfg *Config) *string { return &cfg.APIAuthToken }, strings.TrimSpace),
	stringField("api_user_token", []string{"API_USER_TOKEN", "TARS_API_USER_TOKEN"}, func(cfg *Config) *string { return &cfg.APIUserToken }, strings.TrimSpace),
	stringField("api_admin_token", []string{"API_ADMIN_TOKEN", "TARS_API_ADMIN_TOKEN"}, func(cfg *Config) *string { return &cfg.APIAdminToken }, strings.TrimSpace),
	boolField("api_allow_insecure_local_auth", []string{"API_ALLOW_INSECURE_LOCAL_AUTH", "TARS_API_ALLOW_INSECURE_LOCAL_AUTH"}, func(cfg *Config) *bool { return &cfg.APIAllowInsecureLocalAuth }),
	intField("api_max_inflight_chat", []string{"API_MAX_INFLIGHT_CHAT", "TARS_API_MAX_INFLIGHT_CHAT"}, func(cfg *Config) *int { return &cfg.APIMaxInflightChat }, parsePositiveInt),
	intField("api_max_inflight_agent_runs", []string{"API_MAX_INFLIGHT_AGENT_RUNS", "TARS_API_MAX_INFLIGHT_AGENT_RUNS"}, func(cfg *Config) *int { return &cfg.APIMaxInflightAgentRuns }, parsePositiveInt),
	stringField("llm_provider", []string{"LLM_PROVIDER", "TARS_LLM_PROVIDER"}, func(cfg *Config) *string { return &cfg.LLMProvider }, identityString),
	stringField("llm_auth_mode", []string{"LLM_AUTH_MODE", "TARS_LLM_AUTH_MODE"}, func(cfg *Config) *string { return &cfg.LLMAuthMode }, identityString),
	stringField("llm_oauth_provider", []string{"LLM_OAUTH_PROVIDER", "TARS_LLM_OAUTH_PROVIDER"}, func(cfg *Config) *string { return &cfg.LLMOAuthProvider }, identityString),
	stringField("llm_base_url", []string{"LLM_BASE_URL", "TARS_LLM_BASE_URL"}, func(cfg *Config) *string { return &cfg.LLMBaseURL }, identityString),
	stringField("llm_api_key", []string{"LLM_API_KEY", "TARS_LLM_API_KEY"}, func(cfg *Config) *string { return &cfg.LLMAPIKey }, identityString),
	stringField("llm_model", []string{"LLM_MODEL", "TARS_LLM_MODEL"}, func(cfg *Config) *string { return &cfg.LLMModel }, identityString),
	stringField("llm_reasoning_effort", []string{"LLM_REASONING_EFFORT", "TARS_LLM_REASONING_EFFORT"}, func(cfg *Config) *string { return &cfg.LLMReasoningEffort }, identityString),
	intField("llm_thinking_budget", []string{"LLM_THINKING_BUDGET", "TARS_LLM_THINKING_BUDGET"}, func(cfg *Config) *int { return &cfg.LLMThinkingBudget }, parsePositiveInt),
	stringField("llm_service_tier", []string{"LLM_SERVICE_TIER", "TARS_LLM_SERVICE_TIER"}, func(cfg *Config) *string { return &cfg.LLMServiceTier }, identityString),
	boolField("memory_semantic_enabled", []string{"MEMORY_SEMANTIC_ENABLED", "TARS_MEMORY_SEMANTIC_ENABLED"}, func(cfg *Config) *bool { return &cfg.MemorySemanticEnabled }),
	stringField("memory_embed_provider", []string{"MEMORY_EMBED_PROVIDER", "TARS_MEMORY_EMBED_PROVIDER"}, func(cfg *Config) *string { return &cfg.MemoryEmbedProvider }, lowerTrimmedString),
	stringField("memory_embed_base_url", []string{"MEMORY_EMBED_BASE_URL", "TARS_MEMORY_EMBED_BASE_URL"}, func(cfg *Config) *string { return &cfg.MemoryEmbedBaseURL }, strings.TrimSpace),
	stringField("memory_embed_api_key", []string{"MEMORY_EMBED_API_KEY", "TARS_MEMORY_EMBED_API_KEY"}, func(cfg *Config) *string { return &cfg.MemoryEmbedAPIKey }, strings.TrimSpace),
	stringField("memory_embed_model", []string{"MEMORY_EMBED_MODEL", "TARS_MEMORY_EMBED_MODEL"}, func(cfg *Config) *string { return &cfg.MemoryEmbedModel }, strings.TrimSpace),
	intField("memory_embed_dimensions", []string{"MEMORY_EMBED_DIMENSIONS", "TARS_MEMORY_EMBED_DIMENSIONS"}, func(cfg *Config) *int { return &cfg.MemoryEmbedDimensions }, parsePositiveInt),
	floatField("usage_limit_daily_usd", []string{"USAGE_LIMIT_DAILY_USD", "TARS_USAGE_LIMIT_DAILY_USD"}, func(cfg *Config) *float64 { return &cfg.UsageLimitDailyUSD }, parsePositiveFloat),
	floatField("usage_limit_weekly_usd", []string{"USAGE_LIMIT_WEEKLY_USD", "TARS_USAGE_LIMIT_WEEKLY_USD"}, func(cfg *Config) *float64 { return &cfg.UsageLimitWeeklyUSD }, parsePositiveFloat),
	floatField("usage_limit_monthly_usd", []string{"USAGE_LIMIT_MONTHLY_USD", "TARS_USAGE_LIMIT_MONTHLY_USD"}, func(cfg *Config) *float64 { return &cfg.UsageLimitMonthlyUSD }, parsePositiveFloat),
	stringField("usage_limit_mode", []string{"USAGE_LIMIT_MODE", "TARS_USAGE_LIMIT_MODE"}, func(cfg *Config) *string { return &cfg.UsageLimitMode }, lowerTrimmedString),
	usagePriceOverridesField("usage_price_overrides_json", []string{"USAGE_PRICE_OVERRIDES_JSON", "TARS_USAGE_PRICE_OVERRIDES_JSON"}),
	intField("agent_max_iterations", []string{"AGENT_MAX_ITERATIONS", "TARS_AGENT_MAX_ITERATIONS"}, func(cfg *Config) *int { return &cfg.AgentMaxIterations }, parsePositiveInt),
	boolField("pulse_enabled", []string{"PULSE_ENABLED", "TARS_PULSE_ENABLED"}, func(cfg *Config) *bool { return &cfg.PulseEnabled }),
	stringField("pulse_interval", []string{"PULSE_INTERVAL", "TARS_PULSE_INTERVAL"}, func(cfg *Config) *string { return &cfg.PulseInterval }, strings.TrimSpace),
	stringField("pulse_timeout", []string{"PULSE_TIMEOUT", "TARS_PULSE_TIMEOUT"}, func(cfg *Config) *string { return &cfg.PulseTimeout }, strings.TrimSpace),
	stringField("pulse_active_hours", []string{"PULSE_ACTIVE_HOURS", "TARS_PULSE_ACTIVE_HOURS"}, func(cfg *Config) *string { return &cfg.PulseActiveHours }, strings.TrimSpace),
	stringField("pulse_timezone", []string{"PULSE_TIMEZONE", "TARS_PULSE_TIMEZONE"}, func(cfg *Config) *string { return &cfg.PulseTimezone }, strings.TrimSpace),
	stringField("pulse_min_severity", []string{"PULSE_MIN_SEVERITY", "TARS_PULSE_MIN_SEVERITY"}, func(cfg *Config) *string { return &cfg.PulseMinSeverity }, lowerTrimmedString),
	stringListField("pulse_allowed_autofixes_json", []string{"PULSE_ALLOWED_AUTOFIXES_JSON", "TARS_PULSE_ALLOWED_AUTOFIXES_JSON"}, func(cfg *Config) *[]string { return &cfg.PulseAllowedAutofixes }, parseJSONStringList),
	boolField("pulse_notify_telegram", []string{"PULSE_NOTIFY_TELEGRAM", "TARS_PULSE_NOTIFY_TELEGRAM"}, func(cfg *Config) *bool { return &cfg.PulseNotifyTelegram }),
	boolField("pulse_notify_session_events", []string{"PULSE_NOTIFY_SESSION_EVENTS", "TARS_PULSE_NOTIFY_SESSION_EVENTS"}, func(cfg *Config) *bool { return &cfg.PulseNotifySessionEvents }),
	intField("pulse_cron_failure_threshold", []string{"PULSE_CRON_FAILURE_THRESHOLD", "TARS_PULSE_CRON_FAILURE_THRESHOLD"}, func(cfg *Config) *int { return &cfg.PulseCronFailureThreshold }, parsePositiveInt),
	intField("pulse_stuck_run_minutes", []string{"PULSE_STUCK_RUN_MINUTES", "TARS_PULSE_STUCK_RUN_MINUTES"}, func(cfg *Config) *int { return &cfg.PulseStuckRunMinutes }, parsePositiveInt),
	floatField("pulse_disk_warn_percent", []string{"PULSE_DISK_WARN_PERCENT", "TARS_PULSE_DISK_WARN_PERCENT"}, func(cfg *Config) *float64 { return &cfg.PulseDiskWarnPercent }, parsePositiveFloat),
	floatField("pulse_disk_critical_percent", []string{"PULSE_DISK_CRITICAL_PERCENT", "TARS_PULSE_DISK_CRITICAL_PERCENT"}, func(cfg *Config) *float64 { return &cfg.PulseDiskCriticalPercent }, parsePositiveFloat),
	intField("pulse_delivery_failure_threshold", []string{"PULSE_DELIVERY_FAILURE_THRESHOLD", "TARS_PULSE_DELIVERY_FAILURE_THRESHOLD"}, func(cfg *Config) *int { return &cfg.PulseDeliveryFailureThreshold }, parsePositiveInt),
	stringField("pulse_delivery_failure_window", []string{"PULSE_DELIVERY_FAILURE_WINDOW", "TARS_PULSE_DELIVERY_FAILURE_WINDOW"}, func(cfg *Config) *string { return &cfg.PulseDeliveryFailureWindow }, strings.TrimSpace),
	intField("cron_run_history_limit", []string{"CRON_RUN_HISTORY_LIMIT", "TARS_CRON_RUN_HISTORY_LIMIT"}, func(cfg *Config) *int { return &cfg.CronRunHistoryLimit }, parsePositiveInt),
	stringField("notify_command", []string{"TARS_NOTIFY_COMMAND", "NOTIFY_COMMAND"}, func(cfg *Config) *string { return &cfg.NotifyCommand }, strings.TrimSpace),
	boolField("notify_when_no_clients", []string{"TARS_NOTIFY_WHEN_NO_CLIENTS", "NOTIFY_WHEN_NO_CLIENTS"}, func(cfg *Config) *bool { return &cfg.NotifyWhenNoClients }),
	boolField("assistant_enabled", []string{"ASSISTANT_ENABLED", "TARS_ASSISTANT_ENABLED"}, func(cfg *Config) *bool { return &cfg.AssistantEnabled }),
	stringField("assistant_hotkey", []string{"ASSISTANT_HOTKEY", "TARS_ASSISTANT_HOTKEY"}, func(cfg *Config) *string { return &cfg.AssistantHotkey }, strings.TrimSpace),
	stringField("assistant_whisper_bin", []string{"ASSISTANT_WHISPER_BIN", "TARS_ASSISTANT_WHISPER_BIN"}, func(cfg *Config) *string { return &cfg.AssistantWhisperBin }, strings.TrimSpace),
	stringField("assistant_ffmpeg_bin", []string{"ASSISTANT_FFMPEG_BIN", "TARS_ASSISTANT_FFMPEG_BIN"}, func(cfg *Config) *string { return &cfg.AssistantFFmpegBin }, strings.TrimSpace),
	stringField("assistant_tts_bin", []string{"ASSISTANT_TTS_BIN", "TARS_ASSISTANT_TTS_BIN"}, func(cfg *Config) *string { return &cfg.AssistantTTSBin }, strings.TrimSpace),
	stringField("schedule_timezone", []string{"SCHEDULE_TIMEZONE", "TARS_SCHEDULE_TIMEZONE"}, func(cfg *Config) *string { return &cfg.ScheduleTimezone }, strings.TrimSpace),
	mcpServersField("mcp_servers_json", []string{"MCP_SERVERS_JSON", "TARS_MCP_SERVERS_JSON"}),
	boolField("tools_web_search_enabled", []string{"TOOLS_WEB_SEARCH_ENABLED", "TARS_TOOLS_WEB_SEARCH_ENABLED"}, func(cfg *Config) *bool { return &cfg.ToolsWebSearchEnabled }),
	boolField("tools_web_fetch_enabled", []string{"TOOLS_WEB_FETCH_ENABLED", "TARS_TOOLS_WEB_FETCH_ENABLED"}, func(cfg *Config) *bool { return &cfg.ToolsWebFetchEnabled }),
	stringField("tools_default_set", []string{"TOOLS_DEFAULT_SET", "TARS_TOOLS_DEFAULT_SET"}, func(cfg *Config) *string { return &cfg.ToolsDefaultSet }, lowerTrimmedString),
	boolField("tools_allow_high_risk_user", []string{"TOOLS_ALLOW_HIGH_RISK_USER", "TARS_TOOLS_ALLOW_HIGH_RISK_USER"}, func(cfg *Config) *bool { return &cfg.ToolsAllowHighRiskUser }),
	stringField("tools_web_search_api_key", []string{"TOOLS_WEB_SEARCH_API_KEY", "TARS_TOOLS_WEB_SEARCH_API_KEY"}, func(cfg *Config) *string { return &cfg.ToolsWebSearchAPIKey }, strings.TrimSpace),
	stringField("tools_web_search_provider", []string{"TOOLS_WEB_SEARCH_PROVIDER", "TARS_TOOLS_WEB_SEARCH_PROVIDER"}, func(cfg *Config) *string { return &cfg.ToolsWebSearchProvider }, lowerTrimmedString),
	stringField("tools_web_search_perplexity_api_key", []string{"TOOLS_WEB_SEARCH_PERPLEXITY_API_KEY", "TARS_TOOLS_WEB_SEARCH_PERPLEXITY_API_KEY"}, func(cfg *Config) *string { return &cfg.ToolsWebSearchPerplexityAPIKey }, strings.TrimSpace),
	stringField("tools_web_search_perplexity_model", []string{"TOOLS_WEB_SEARCH_PERPLEXITY_MODEL", "TARS_TOOLS_WEB_SEARCH_PERPLEXITY_MODEL"}, func(cfg *Config) *string { return &cfg.ToolsWebSearchPerplexityModel }, strings.TrimSpace),
	stringField("tools_web_search_perplexity_base_url", []string{"TOOLS_WEB_SEARCH_PERPLEXITY_BASE_URL", "TARS_TOOLS_WEB_SEARCH_PERPLEXITY_BASE_URL"}, func(cfg *Config) *string { return &cfg.ToolsWebSearchPerplexityBaseURL }, strings.TrimSpace),
	intField("tools_web_search_cache_ttl_seconds", []string{"TOOLS_WEB_SEARCH_CACHE_TTL_SECONDS", "TARS_TOOLS_WEB_SEARCH_CACHE_TTL_SECONDS"}, func(cfg *Config) *int { return &cfg.ToolsWebSearchCacheTTLSeconds }, parsePositiveInt),
	stringListField("tools_web_fetch_private_host_allowlist_json", []string{"TOOLS_WEB_FETCH_PRIVATE_HOST_ALLOWLIST_JSON", "TARS_TOOLS_WEB_FETCH_PRIVATE_HOST_ALLOWLIST_JSON"}, func(cfg *Config) *[]string { return &cfg.ToolsWebFetchPrivateHostAllowlist }, parseJSONStringList),
	boolField("tools_web_fetch_allow_private_hosts", []string{"TOOLS_WEB_FETCH_ALLOW_PRIVATE_HOSTS", "TARS_TOOLS_WEB_FETCH_ALLOW_PRIVATE_HOSTS"}, func(cfg *Config) *bool { return &cfg.ToolsWebFetchAllowPrivateHosts }),
	boolField("tools_apply_patch_enabled", []string{"TOOLS_APPLY_PATCH_ENABLED", "TARS_TOOLS_APPLY_PATCH_ENABLED"}, func(cfg *Config) *bool { return &cfg.ToolsApplyPatchEnabled }),
	boolField("vault_enabled", []string{"VAULT_ENABLED", "TARS_VAULT_ENABLED"}, func(cfg *Config) *bool { return &cfg.VaultEnabled }),
	stringField("vault_addr", []string{"VAULT_ADDR", "TARS_VAULT_ADDR"}, func(cfg *Config) *string { return &cfg.VaultAddr }, strings.TrimSpace),
	stringField("vault_auth_mode", []string{"VAULT_AUTH_MODE", "TARS_VAULT_AUTH_MODE"}, func(cfg *Config) *string { return &cfg.VaultAuthMode }, lowerTrimmedString),
	stringField("vault_token", []string{"VAULT_TOKEN", "TARS_VAULT_TOKEN"}, func(cfg *Config) *string { return &cfg.VaultToken }, strings.TrimSpace),
	stringField("vault_namespace", []string{"VAULT_NAMESPACE", "TARS_VAULT_NAMESPACE"}, func(cfg *Config) *string { return &cfg.VaultNamespace }, strings.TrimSpace),
	intField("vault_timeout_ms", []string{"VAULT_TIMEOUT_MS", "TARS_VAULT_TIMEOUT_MS"}, func(cfg *Config) *int { return &cfg.VaultTimeoutMS }, parsePositiveInt),
	stringField("vault_kv_mount", []string{"VAULT_KV_MOUNT", "TARS_VAULT_KV_MOUNT"}, func(cfg *Config) *string { return &cfg.VaultKVMount }, strings.TrimSpace),
	intField("vault_kv_version", []string{"VAULT_KV_VERSION", "TARS_VAULT_KV_VERSION"}, func(cfg *Config) *int { return &cfg.VaultKVVersion }, parsePositiveInt),
	stringField("vault_approle_mount", []string{"VAULT_APPROLE_MOUNT", "TARS_VAULT_APPROLE_MOUNT"}, func(cfg *Config) *string { return &cfg.VaultAppRoleMount }, strings.TrimSpace),
	stringField("vault_approle_role_id", []string{"VAULT_APPROLE_ROLE_ID", "TARS_VAULT_APPROLE_ROLE_ID"}, func(cfg *Config) *string { return &cfg.VaultAppRoleRoleID }, strings.TrimSpace),
	stringField("vault_approle_secret_id", []string{"VAULT_APPROLE_SECRET_ID", "TARS_VAULT_APPROLE_SECRET_ID"}, func(cfg *Config) *string { return &cfg.VaultAppRoleSecretID }, strings.TrimSpace),
	stringListField("vault_secret_path_allowlist_json", []string{"VAULT_SECRET_PATH_ALLOWLIST_JSON", "TARS_VAULT_SECRET_PATH_ALLOWLIST_JSON"}, func(cfg *Config) *[]string { return &cfg.VaultSecretPathAllowlist }, parseJSONStringList),
	boolField("browser_runtime_enabled", []string{"BROWSER_RUNTIME_ENABLED", "TARS_BROWSER_RUNTIME_ENABLED"}, func(cfg *Config) *bool { return &cfg.BrowserRuntimeEnabled }),
	stringField("browser_default_profile", []string{"BROWSER_DEFAULT_PROFILE", "TARS_BROWSER_DEFAULT_PROFILE"}, func(cfg *Config) *string { return &cfg.BrowserDefaultProfile }, lowerTrimmedString),
	boolField("browser_managed_headless", []string{"BROWSER_MANAGED_HEADLESS", "TARS_BROWSER_MANAGED_HEADLESS"}, func(cfg *Config) *bool { return &cfg.BrowserManagedHeadless }),
	stringField("browser_managed_executable_path", []string{"BROWSER_MANAGED_EXECUTABLE_PATH", "TARS_BROWSER_MANAGED_EXECUTABLE_PATH"}, func(cfg *Config) *string { return &cfg.BrowserManagedExecutablePath }, strings.TrimSpace),
	stringField("browser_managed_user_data_dir", []string{"BROWSER_MANAGED_USER_DATA_DIR", "TARS_BROWSER_MANAGED_USER_DATA_DIR"}, func(cfg *Config) *string { return &cfg.BrowserManagedUserDataDir }, strings.TrimSpace),
	stringField("browser_site_flows_dir", []string{"BROWSER_SITE_FLOWS_DIR", "TARS_BROWSER_SITE_FLOWS_DIR"}, func(cfg *Config) *string { return &cfg.BrowserSiteFlowsDir }, strings.TrimSpace),
	stringListField("browser_auto_login_site_allowlist_json", []string{"BROWSER_AUTO_LOGIN_SITE_ALLOWLIST_JSON", "TARS_BROWSER_AUTO_LOGIN_SITE_ALLOWLIST_JSON"}, func(cfg *Config) *[]string { return &cfg.BrowserAutoLoginSiteAllowlist }, parseJSONStringList),
	boolField("gateway_enabled", []string{"GATEWAY_ENABLED", "TARS_GATEWAY_ENABLED"}, func(cfg *Config) *bool { return &cfg.GatewayEnabled }),
	stringField("gateway_default_agent", []string{"GATEWAY_DEFAULT_AGENT", "TARS_GATEWAY_DEFAULT_AGENT"}, func(cfg *Config) *string { return &cfg.GatewayDefaultAgent }, strings.TrimSpace),
	gatewayAgentsField("gateway_agents_json", []string{"GATEWAY_AGENTS_JSON", "TARS_GATEWAY_AGENTS_JSON"}),
	boolField("gateway_agents_watch", []string{"GATEWAY_AGENTS_WATCH", "TARS_GATEWAY_AGENTS_WATCH"}, func(cfg *Config) *bool { return &cfg.GatewayAgentsWatch }),
	intField("gateway_agents_watch_debounce_ms", []string{"GATEWAY_AGENTS_WATCH_DEBOUNCE_MS", "TARS_GATEWAY_AGENTS_WATCH_DEBOUNCE_MS"}, func(cfg *Config) *int { return &cfg.GatewayAgentsWatchDebounceMS }, parsePositiveInt),
	boolField("gateway_persistence_enabled", []string{"GATEWAY_PERSISTENCE_ENABLED", "TARS_GATEWAY_PERSISTENCE_ENABLED"}, func(cfg *Config) *bool { return &cfg.GatewayPersistenceEnabled }),
	boolField("gateway_runs_persistence_enabled", []string{"GATEWAY_RUNS_PERSISTENCE_ENABLED", "TARS_GATEWAY_RUNS_PERSISTENCE_ENABLED"}, func(cfg *Config) *bool { return &cfg.GatewayRunsPersistenceEnabled }),
	boolField("gateway_channels_persistence_enabled", []string{"GATEWAY_CHANNELS_PERSISTENCE_ENABLED", "TARS_GATEWAY_CHANNELS_PERSISTENCE_ENABLED"}, func(cfg *Config) *bool { return &cfg.GatewayChannelsPersistenceEnabled }),
	intField("gateway_runs_max_records", []string{"GATEWAY_RUNS_MAX_RECORDS", "TARS_GATEWAY_RUNS_MAX_RECORDS"}, func(cfg *Config) *int { return &cfg.GatewayRunsMaxRecords }, parsePositiveInt),
	intField("gateway_channels_max_messages_per_channel", []string{"GATEWAY_CHANNELS_MAX_MESSAGES_PER_CHANNEL", "TARS_GATEWAY_CHANNELS_MAX_MESSAGES_PER_CHANNEL"}, func(cfg *Config) *int { return &cfg.GatewayChannelsMaxMessagesPerChannel }, parsePositiveInt),
	intField("gateway_subagents_max_threads", []string{"GATEWAY_SUBAGENTS_MAX_THREADS", "TARS_GATEWAY_SUBAGENTS_MAX_THREADS"}, func(cfg *Config) *int { return &cfg.GatewaySubagentsMaxThreads }, parsePositiveInt),
	intField("gateway_subagents_max_depth", []string{"GATEWAY_SUBAGENTS_MAX_DEPTH", "TARS_GATEWAY_SUBAGENTS_MAX_DEPTH"}, func(cfg *Config) *int { return &cfg.GatewaySubagentsMaxDepth }, parsePositiveInt),
	stringField("gateway_persistence_dir", []string{"GATEWAY_PERSISTENCE_DIR", "TARS_GATEWAY_PERSISTENCE_DIR"}, func(cfg *Config) *string { return &cfg.GatewayPersistenceDir }, strings.TrimSpace),
	boolField("gateway_restore_on_startup", []string{"GATEWAY_RESTORE_ON_STARTUP", "TARS_GATEWAY_RESTORE_ON_STARTUP"}, func(cfg *Config) *bool { return &cfg.GatewayRestoreOnStartup }),
	boolField("gateway_report_summary_enabled", []string{"GATEWAY_REPORT_SUMMARY_ENABLED", "TARS_GATEWAY_REPORT_SUMMARY_ENABLED"}, func(cfg *Config) *bool { return &cfg.GatewayReportSummaryEnabled }),
	boolField("gateway_archive_enabled", []string{"GATEWAY_ARCHIVE_ENABLED", "TARS_GATEWAY_ARCHIVE_ENABLED"}, func(cfg *Config) *bool { return &cfg.GatewayArchiveEnabled }),
	stringField("gateway_archive_dir", []string{"GATEWAY_ARCHIVE_DIR", "TARS_GATEWAY_ARCHIVE_DIR"}, func(cfg *Config) *string { return &cfg.GatewayArchiveDir }, strings.TrimSpace),
	intField("gateway_archive_retention_days", []string{"GATEWAY_ARCHIVE_RETENTION_DAYS", "TARS_GATEWAY_ARCHIVE_RETENTION_DAYS"}, func(cfg *Config) *int { return &cfg.GatewayArchiveRetentionDays }, parsePositiveInt),
	intField("gateway_archive_max_file_bytes", []string{"GATEWAY_ARCHIVE_MAX_FILE_BYTES", "TARS_GATEWAY_ARCHIVE_MAX_FILE_BYTES"}, func(cfg *Config) *int { return &cfg.GatewayArchiveMaxFileBytes }, parsePositiveInt),
	boolField("channels_local_enabled", []string{"CHANNELS_LOCAL_ENABLED", "TARS_CHANNELS_LOCAL_ENABLED"}, func(cfg *Config) *bool { return &cfg.ChannelsLocalEnabled }),
	boolField("channels_webhook_enabled", []string{"CHANNELS_WEBHOOK_ENABLED", "TARS_CHANNELS_WEBHOOK_ENABLED"}, func(cfg *Config) *bool { return &cfg.ChannelsWebhookEnabled }),
	boolField("channels_telegram_enabled", []string{"CHANNELS_TELEGRAM_ENABLED", "TARS_CHANNELS_TELEGRAM_ENABLED"}, func(cfg *Config) *bool { return &cfg.ChannelsTelegramEnabled }),
	stringField("channels_telegram_dm_policy", []string{"CHANNELS_TELEGRAM_DM_POLICY", "TARS_CHANNELS_TELEGRAM_DM_POLICY"}, func(cfg *Config) *string { return &cfg.ChannelsTelegramDMPolicy }, lowerTrimmedString),
	boolField("channels_telegram_polling_enabled", []string{"CHANNELS_TELEGRAM_POLLING_ENABLED", "TARS_CHANNELS_TELEGRAM_POLLING_ENABLED"}, func(cfg *Config) *bool { return &cfg.ChannelsTelegramPollingEnabled }),
	stringField("telegram_bot_token", []string{"TELEGRAM_BOT_TOKEN", "TARS_TELEGRAM_BOT_TOKEN"}, func(cfg *Config) *string { return &cfg.TelegramBotToken }, strings.TrimSpace),
	boolField("tools_message_enabled", []string{"TOOLS_MESSAGE_ENABLED", "TARS_TOOLS_MESSAGE_ENABLED"}, func(cfg *Config) *bool { return &cfg.ToolsMessageEnabled }),
	boolField("tools_browser_enabled", []string{"TOOLS_BROWSER_ENABLED", "TARS_TOOLS_BROWSER_ENABLED"}, func(cfg *Config) *bool { return &cfg.ToolsBrowserEnabled }),
	boolField("tools_nodes_enabled", []string{"TOOLS_NODES_ENABLED", "TARS_TOOLS_NODES_ENABLED"}, func(cfg *Config) *bool { return &cfg.ToolsNodesEnabled }),
	boolField("tools_gateway_enabled", []string{"TOOLS_GATEWAY_ENABLED", "TARS_TOOLS_GATEWAY_ENABLED"}, func(cfg *Config) *bool { return &cfg.ToolsGatewayEnabled }),
	boolField("skills_enabled", []string{"SKILLS_ENABLED", "TARS_SKILLS_ENABLED"}, func(cfg *Config) *bool { return &cfg.SkillsEnabled }),
	boolField("skills_watch", []string{"SKILLS_WATCH", "TARS_SKILLS_WATCH"}, func(cfg *Config) *bool { return &cfg.SkillsWatch }),
	intField("skills_watch_debounce_ms", []string{"SKILLS_WATCH_DEBOUNCE_MS", "TARS_SKILLS_WATCH_DEBOUNCE_MS"}, func(cfg *Config) *int { return &cfg.SkillsWatchDebounceMS }, parsePositiveInt),
	stringListField("skills_extra_dirs_json", []string{"SKILLS_EXTRA_DIRS_JSON", "TARS_SKILLS_EXTRA_DIRS_JSON"}, func(cfg *Config) *[]string { return &cfg.SkillsExtraDirs }, parseJSONStringList),
	stringField("skills_bundled_dir", []string{"SKILLS_BUNDLED_DIR", "TARS_SKILLS_BUNDLED_DIR"}, func(cfg *Config) *string { return &cfg.SkillsBundledDir }, strings.TrimSpace),
	boolField("plugins_enabled", []string{"PLUGINS_ENABLED", "TARS_PLUGINS_ENABLED"}, func(cfg *Config) *bool { return &cfg.PluginsEnabled }),
	boolField("plugins_watch", []string{"PLUGINS_WATCH", "TARS_PLUGINS_WATCH"}, func(cfg *Config) *bool { return &cfg.PluginsWatch }),
	intField("plugins_watch_debounce_ms", []string{"PLUGINS_WATCH_DEBOUNCE_MS", "TARS_PLUGINS_WATCH_DEBOUNCE_MS"}, func(cfg *Config) *int { return &cfg.PluginsWatchDebounceMS }, parsePositiveInt),
	stringListField("plugins_extra_dirs_json", []string{"PLUGINS_EXTRA_DIRS_JSON", "TARS_PLUGINS_EXTRA_DIRS_JSON"}, func(cfg *Config) *[]string { return &cfg.PluginsExtraDirs }, parseJSONStringList),
	stringField("plugins_bundled_dir", []string{"PLUGINS_BUNDLED_DIR", "TARS_PLUGINS_BUNDLED_DIR"}, func(cfg *Config) *string { return &cfg.PluginsBundledDir }, strings.TrimSpace),
	boolField("plugins_allow_mcp_servers", []string{"PLUGINS_ALLOW_MCP_SERVERS", "TARS_PLUGINS_ALLOW_MCP_SERVERS"}, func(cfg *Config) *bool { return &cfg.PluginsAllowMCPServers }),
	stringListField("mcp_command_allowlist_json", []string{"MCP_COMMAND_ALLOWLIST_JSON", "TARS_MCP_COMMAND_ALLOWLIST_JSON"}, func(cfg *Config) *[]string { return &cfg.MCPCommandAllowlist }, parseJSONStringList),
}

var configInputFieldsByYAMLKey = func() map[string]configInputField {
	index := make(map[string]configInputField, len(configInputFields))
	for _, field := range configInputFields {
		index[field.yamlKey] = field
	}
	return index
}()

func applyConfigInputFieldsFromEnv(cfg *Config, fields []configInputField) {
	for _, field := range fields {
		if value := firstDefinedEnv(field.envKeys); value != "" {
			field.apply(cfg, value)
		}
	}
}

func mergeConfigInputFields(dst *Config, src Config, fields []configInputField) {
	for _, field := range fields {
		field.merge(dst, src)
	}
}

func configInputFieldByYAMLKey(key string) (configInputField, bool) {
	field, ok := configInputFieldsByYAMLKey[strings.TrimSpace(strings.ToLower(key))]
	return field, ok
}

func firstDefinedEnv(keys []string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func stringField(yamlKey string, envKeys []string, accessor func(*Config) *string, normalize func(string) string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			*accessor(cfg) = normalize(raw)
		},
		merge: func(dst *Config, src Config) {
			value := *accessor(&src)
			if value != "" {
				*accessor(dst) = value
			}
		},
	}
}

func boolField(yamlKey string, envKeys []string, accessor func(*Config) *bool) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			ptr := accessor(cfg)
			*ptr = parseBool(raw, *ptr)
		},
		merge: func(dst *Config, src Config) {
			if *accessor(&src) {
				*accessor(dst) = true
			}
		},
	}
}

func intField(yamlKey string, envKeys []string, accessor func(*Config) *int, parse func(string, int) int) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			ptr := accessor(cfg)
			*ptr = parse(raw, *ptr)
		},
		merge: func(dst *Config, src Config) {
			value := *accessor(&src)
			if value > 0 {
				*accessor(dst) = value
			}
		},
	}
}

func floatField(yamlKey string, envKeys []string, accessor func(*Config) *float64, parse func(string, float64) float64) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			ptr := accessor(cfg)
			*ptr = parse(raw, *ptr)
		},
		merge: func(dst *Config, src Config) {
			value := *accessor(&src)
			if value > 0 {
				*accessor(dst) = value
			}
		},
	}
}

func usagePriceOverridesField(yamlKey string, envKeys []string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			cfg.UsagePriceOverrides = parseUsagePriceOverridesJSON(raw, cfg.UsagePriceOverrides)
		},
		merge: func(dst *Config, src Config) {
			if len(src.UsagePriceOverrides) == 0 {
				return
			}
			dst.UsagePriceOverrides = cloneUsagePriceOverrides(src.UsagePriceOverrides)
		},
	}
}

func stringListField(yamlKey string, envKeys []string, accessor func(*Config) *[]string, parse func(string, []string) []string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			ptr := accessor(cfg)
			*ptr = parse(raw, *ptr)
		},
		merge: func(dst *Config, src Config) {
			value := *accessor(&src)
			if len(value) == 0 {
				return
			}
			*accessor(dst) = append([]string(nil), value...)
		},
	}
}

func mcpServersField(yamlKey string, envKeys []string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			cfg.MCPServers = parseMCPServersJSON(raw, cfg.MCPServers)
		},
		merge: func(dst *Config, src Config) {
			if len(src.MCPServers) == 0 {
				return
			}
			dst.MCPServers = src.MCPServers
		},
	}
}

func gatewayAgentsField(yamlKey string, envKeys []string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			cfg.GatewayAgents = parseGatewayAgentsJSON(raw, cfg.GatewayAgents)
		},
		merge: func(dst *Config, src Config) {
			if len(src.GatewayAgents) == 0 {
				return
			}
			dst.GatewayAgents = append([]GatewayAgent(nil), src.GatewayAgents...)
		},
	}
}

func cloneUsagePriceOverrides(src map[string]UsagePrice) map[string]UsagePrice {
	cloned := make(map[string]UsagePrice, len(src))
	for key, value := range src {
		cloned[key] = value
	}
	return cloned
}

func identityString(value string) string {
	return value
}

func lowerTrimmedString(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}
