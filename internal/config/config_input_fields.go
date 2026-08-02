package config

import (
	"os"
	"strings"
)

type configInputField struct {
	yamlKey  string
	yamlPath string
	envKeys  []string
	apply    func(*Config, string)
	merge    func(*Config, Config)
}

var configInputFields = []configInputField{
	stringField("workspace_dir", []string{"TARS_WORKSPACE_DIR"}, func(cfg *Config) *string { return &cfg.WorkspaceDir }, identityString),
	stringField("plan_clarify_mode", []string{"PLAN_CLARIFY_MODE", "TARS_PLAN_CLARIFY_MODE"}, func(cfg *Config) *string { return &cfg.PlanClarifyMode }, lowerTrimmedString),
	stringField("session_default_id", []string{"SESSION_DEFAULT_ID", "TARS_SESSION_DEFAULT_ID"}, func(cfg *Config) *string { return &cfg.SessionDefaultID }, strings.TrimSpace),
	stringField("session_telegram_scope", []string{"SESSION_TELEGRAM_SCOPE", "TARS_SESSION_TELEGRAM_SCOPE"}, func(cfg *Config) *string { return &cfg.SessionTelegramScope }, lowerTrimmedString),
	withYAMLPath(styleDefaultField("style_directness_default", []string{"STYLE_DIRECTNESS_DEFAULT", "TARS_STYLE_DIRECTNESS_DEFAULT"}, func(cfg *Config) *int { return &cfg.StyleDirectnessDefault }), "runtime.style.directness_default"),
	withYAMLPath(styleDefaultField("style_humor_default", []string{"STYLE_HUMOR_DEFAULT", "TARS_STYLE_HUMOR_DEFAULT"}, func(cfg *Config) *int { return &cfg.StyleHumorDefault }), "runtime.style.humor_default"),
	withYAMLPath(styleDefaultField("style_caution_default", []string{"STYLE_CAUTION_DEFAULT", "TARS_STYLE_CAUTION_DEFAULT"}, func(cfg *Config) *int { return &cfg.StyleCautionDefault }), "runtime.style.caution_default"),
	withYAMLPath(styleDefaultField("style_autonomy_default", []string{"STYLE_AUTONOMY_DEFAULT", "TARS_STYLE_AUTONOMY_DEFAULT"}, func(cfg *Config) *int { return &cfg.StyleAutonomyDefault }), "runtime.style.autonomy_default"),
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
	withYAMLPath(intField("api_max_inflight_chat", []string{"API_MAX_INFLIGHT_CHAT", "TARS_API_MAX_INFLIGHT_CHAT"}, func(cfg *Config) *int { return &cfg.APIMaxInflightChat }, parsePositiveInt), "api.max_inflight.chat"),
	withYAMLPath(intField("api_max_inflight_agent_runs", []string{"API_MAX_INFLIGHT_AGENT_RUNS", "TARS_API_MAX_INFLIGHT_AGENT_RUNS"}, func(cfg *Config) *int { return &cfg.APIMaxInflightAgentRuns }, parsePositiveInt), "api.max_inflight.agent_runs"),
	withYAMLPath(boolField("remote_access_tailscale_serve_enabled", []string{"REMOTE_ACCESS_TAILSCALE_SERVE_ENABLED", "TARS_REMOTE_ACCESS_TAILSCALE_SERVE_ENABLED"}, func(cfg *Config) *bool { return &cfg.RemoteAccessTailscaleServeEnabled }), "remote_access.tailscale_serve.enabled"),
	withYAMLPath(intField("remote_access_tailscale_serve_https_port", []string{"REMOTE_ACCESS_TAILSCALE_SERVE_HTTPS_PORT", "TARS_REMOTE_ACCESS_TAILSCALE_SERVE_HTTPS_PORT"}, func(cfg *Config) *int { return &cfg.RemoteAccessTailscaleServeHTTPSPort }, parsePositiveInt), "remote_access.tailscale_serve.https_port"),
	// Named provider pool + tier bindings. See docs/plans/llm-provider-pool.md
	// and internal/config/llm_resolve.go.
	withYAMLPath(llmProvidersField("llm_providers", []string{"LLM_PROVIDERS_JSON", "TARS_LLM_PROVIDERS_JSON"}), "llm.providers"),
	withYAMLPath(llmTiersField("llm_tiers", []string{"LLM_TIERS_JSON", "TARS_LLM_TIERS_JSON"}), "llm.tiers"),
	withYAMLPath(stringField("llm_default_tier", []string{"LLM_DEFAULT_TIER", "TARS_LLM_DEFAULT_TIER"}, func(cfg *Config) *string { return &cfg.LLMDefaultTier }, lowerTrimmedString), "llm.default_tier"),
	withYAMLPath(llmRoleDefaultsField("llm_role_defaults", []string{"LLM_ROLE_DEFAULTS_JSON", "TARS_LLM_ROLE_DEFAULTS_JSON"}), "llm.role_defaults"),
	withYAMLPath(stringField("claude_code_cli_permission_mode", []string{"CLAUDE_CODE_CLI_PERMISSION_MODE", "TARS_CLAUDE_CODE_CLI_PERMISSION_MODE"}, func(cfg *Config) *string { return &cfg.ClaudeCodeCLIPermissionMode }, strings.TrimSpace), "llm.claude_code_cli.permission_mode"),
	stringField("memory_backend", []string{"MEMORY_BACKEND", "TARS_MEMORY_BACKEND"}, func(cfg *Config) *string { return &cfg.MemoryBackend }, lowerTrimmedString),
	boolField("memory_semantic_enabled", []string{"MEMORY_SEMANTIC_ENABLED", "TARS_MEMORY_SEMANTIC_ENABLED"}, func(cfg *Config) *bool { return &cfg.MemorySemanticEnabled }),
	stringField("memory_embed_provider", []string{"MEMORY_EMBED_PROVIDER", "TARS_MEMORY_EMBED_PROVIDER"}, func(cfg *Config) *string { return &cfg.MemoryEmbedProvider }, lowerTrimmedString),
	stringField("memory_embed_base_url", []string{"MEMORY_EMBED_BASE_URL", "TARS_MEMORY_EMBED_BASE_URL"}, func(cfg *Config) *string { return &cfg.MemoryEmbedBaseURL }, strings.TrimSpace),
	stringField("memory_embed_api_key", []string{"MEMORY_EMBED_API_KEY", "TARS_MEMORY_EMBED_API_KEY"}, func(cfg *Config) *string { return &cfg.MemoryEmbedAPIKey }, strings.TrimSpace),
	stringField("memory_embed_model", []string{"MEMORY_EMBED_MODEL", "TARS_MEMORY_EMBED_MODEL"}, func(cfg *Config) *string { return &cfg.MemoryEmbedModel }, strings.TrimSpace),
	intField("memory_embed_dimensions", []string{"MEMORY_EMBED_DIMENSIONS", "TARS_MEMORY_EMBED_DIMENSIONS"}, func(cfg *Config) *int { return &cfg.MemoryEmbedDimensions }, parsePositiveInt),
	floatField("usage_limit_daily_usd", []string{"USAGE_LIMIT_DAILY_USD", "TARS_USAGE_LIMIT_DAILY_USD"}, func(cfg *Config) *float64 { return &cfg.UsageLimitDailyUSD }, parsePositiveFloat),
	floatField("usage_limit_weekly_usd", []string{"USAGE_LIMIT_WEEKLY_USD", "TARS_USAGE_LIMIT_WEEKLY_USD"}, func(cfg *Config) *float64 { return &cfg.UsageLimitWeeklyUSD }, parsePositiveFloat),
	floatField("usage_limit_monthly_usd", []string{"USAGE_LIMIT_MONTHLY_USD", "TARS_USAGE_LIMIT_MONTHLY_USD"}, func(cfg *Config) *float64 { return &cfg.UsageLimitMonthlyUSD }, parsePositiveFloat),
	withYAMLPath(intField("usage_daily_token_budget", []string{"USAGE_DAILY_TOKEN_BUDGET", "TARS_USAGE_DAILY_TOKEN_BUDGET"}, func(cfg *Config) *int { return &cfg.UsageDailyTokenBudget }, parseNonNegativeInt), "usage.limits.daily_tokens"),
	stringField("usage_limit_mode", []string{"USAGE_LIMIT_MODE", "TARS_USAGE_LIMIT_MODE"}, func(cfg *Config) *string { return &cfg.UsageLimitMode }, lowerTrimmedString),
	usagePriceOverridesField("usage_price_overrides_json", []string{"USAGE_PRICE_OVERRIDES_JSON", "TARS_USAGE_PRICE_OVERRIDES_JSON"}),
	intField("agent_max_iterations", []string{"AGENT_MAX_ITERATIONS", "TARS_AGENT_MAX_ITERATIONS"}, func(cfg *Config) *int { return &cfg.AgentMaxIterations }, parsePositiveInt),
	boolField("pulse_enabled", []string{"PULSE_ENABLED", "TARS_PULSE_ENABLED"}, func(cfg *Config) *bool { return &cfg.PulseEnabled }),
	stringField("pulse_interval", []string{"PULSE_INTERVAL", "TARS_PULSE_INTERVAL"}, func(cfg *Config) *string { return &cfg.PulseInterval }, strings.TrimSpace),
	stringField("pulse_timeout", []string{"PULSE_TIMEOUT", "TARS_PULSE_TIMEOUT"}, func(cfg *Config) *string { return &cfg.PulseTimeout }, strings.TrimSpace),
	stringField("pulse_active_hours", []string{"PULSE_ACTIVE_HOURS", "TARS_PULSE_ACTIVE_HOURS"}, func(cfg *Config) *string { return &cfg.PulseActiveHours }, strings.TrimSpace),
	stringField("pulse_timezone", []string{"PULSE_TIMEZONE", "TARS_PULSE_TIMEZONE"}, func(cfg *Config) *string { return &cfg.PulseTimezone }, strings.TrimSpace),
	stringField("pulse_min_severity", []string{"PULSE_MIN_SEVERITY", "TARS_PULSE_MIN_SEVERITY"}, func(cfg *Config) *string { return &cfg.PulseMinSeverity }, lowerTrimmedString),
	withYAMLPath(stringListField("pulse_allowed_autofixes_json", []string{"PULSE_ALLOWED_AUTOFIXES_JSON", "TARS_PULSE_ALLOWED_AUTOFIXES_JSON"}, func(cfg *Config) *[]string { return &cfg.PulseAllowedAutofixes }, parseJSONStringList), "automation.pulse.allowed_autofixes"),
	boolField("pulse_notify_telegram", []string{"PULSE_NOTIFY_TELEGRAM", "TARS_PULSE_NOTIFY_TELEGRAM"}, func(cfg *Config) *bool { return &cfg.PulseNotifyTelegram }),
	boolField("pulse_notify_session_events", []string{"PULSE_NOTIFY_SESSION_EVENTS", "TARS_PULSE_NOTIFY_SESSION_EVENTS"}, func(cfg *Config) *bool { return &cfg.PulseNotifySessionEvents }),
	intField("pulse_cron_failure_threshold", []string{"PULSE_CRON_FAILURE_THRESHOLD", "TARS_PULSE_CRON_FAILURE_THRESHOLD"}, func(cfg *Config) *int { return &cfg.PulseCronFailureThreshold }, parsePositiveInt),
	intField("pulse_stuck_run_minutes", []string{"PULSE_STUCK_RUN_MINUTES", "TARS_PULSE_STUCK_RUN_MINUTES"}, func(cfg *Config) *int { return &cfg.PulseStuckRunMinutes }, parsePositiveInt),
	floatField("pulse_disk_warn_percent", []string{"PULSE_DISK_WARN_PERCENT", "TARS_PULSE_DISK_WARN_PERCENT"}, func(cfg *Config) *float64 { return &cfg.PulseDiskWarnPercent }, parsePositiveFloat),
	floatField("pulse_disk_critical_percent", []string{"PULSE_DISK_CRITICAL_PERCENT", "TARS_PULSE_DISK_CRITICAL_PERCENT"}, func(cfg *Config) *float64 { return &cfg.PulseDiskCriticalPercent }, parsePositiveFloat),
	intField("pulse_delivery_failure_threshold", []string{"PULSE_DELIVERY_FAILURE_THRESHOLD", "TARS_PULSE_DELIVERY_FAILURE_THRESHOLD"}, func(cfg *Config) *int { return &cfg.PulseDeliveryFailureThreshold }, parsePositiveInt),
	stringField("pulse_delivery_failure_window", []string{"PULSE_DELIVERY_FAILURE_WINDOW", "TARS_PULSE_DELIVERY_FAILURE_WINDOW"}, func(cfg *Config) *string { return &cfg.PulseDeliveryFailureWindow }, strings.TrimSpace),
	intField("pulse_reflection_failure_threshold", []string{"PULSE_REFLECTION_FAILURE_THRESHOLD", "TARS_PULSE_REFLECTION_FAILURE_THRESHOLD"}, func(cfg *Config) *int { return &cfg.PulseReflectionFailureThreshold }, parsePositiveInt),
	boolField("reflection_enabled", []string{"REFLECTION_ENABLED", "TARS_REFLECTION_ENABLED"}, func(cfg *Config) *bool { return &cfg.ReflectionEnabled }),
	stringField("reflection_sleep_window", []string{"REFLECTION_SLEEP_WINDOW", "TARS_REFLECTION_SLEEP_WINDOW"}, func(cfg *Config) *string { return &cfg.ReflectionSleepWindow }, strings.TrimSpace),
	stringField("reflection_timezone", []string{"REFLECTION_TIMEZONE", "TARS_REFLECTION_TIMEZONE"}, func(cfg *Config) *string { return &cfg.ReflectionTimezone }, strings.TrimSpace),
	stringField("reflection_tick_interval", []string{"REFLECTION_TICK_INTERVAL", "TARS_REFLECTION_TICK_INTERVAL"}, func(cfg *Config) *string { return &cfg.ReflectionTickInterval }, strings.TrimSpace),
	stringField("reflection_empty_session_age", []string{"REFLECTION_EMPTY_SESSION_AGE", "TARS_REFLECTION_EMPTY_SESSION_AGE"}, func(cfg *Config) *string { return &cfg.ReflectionEmptySessionAge }, strings.TrimSpace),
	intField("reflection_memory_lookback_hours", []string{"REFLECTION_MEMORY_LOOKBACK_HOURS", "TARS_REFLECTION_MEMORY_LOOKBACK_HOURS"}, func(cfg *Config) *int { return &cfg.ReflectionMemoryLookbackHours }, parsePositiveInt),
	intField("reflection_max_turns_per_session", []string{"REFLECTION_MAX_TURNS_PER_SESSION", "TARS_REFLECTION_MAX_TURNS_PER_SESSION"}, func(cfg *Config) *int { return &cfg.ReflectionMaxTurnsPerSession }, parsePositiveInt),
	intField("cron_run_history_limit", []string{"CRON_RUN_HISTORY_LIMIT", "TARS_CRON_RUN_HISTORY_LIMIT"}, func(cfg *Config) *int { return &cfg.CronRunHistoryLimit }, parsePositiveInt),
	stringField("notify_command", []string{"TARS_NOTIFY_COMMAND", "NOTIFY_COMMAND"}, func(cfg *Config) *string { return &cfg.NotifyCommand }, strings.TrimSpace),
	boolField("notify_when_no_clients", []string{"TARS_NOTIFY_WHEN_NO_CLIENTS", "NOTIFY_WHEN_NO_CLIENTS"}, func(cfg *Config) *bool { return &cfg.NotifyWhenNoClients }),
	boolField("assistant_enabled", []string{"ASSISTANT_ENABLED", "TARS_ASSISTANT_ENABLED"}, func(cfg *Config) *bool { return &cfg.AssistantEnabled }),
	stringField("assistant_hotkey", []string{"ASSISTANT_HOTKEY", "TARS_ASSISTANT_HOTKEY"}, func(cfg *Config) *string { return &cfg.AssistantHotkey }, strings.TrimSpace),
	stringField("assistant_whisper_bin", []string{"ASSISTANT_WHISPER_BIN", "TARS_ASSISTANT_WHISPER_BIN"}, func(cfg *Config) *string { return &cfg.AssistantWhisperBin }, strings.TrimSpace),
	stringField("assistant_ffmpeg_bin", []string{"ASSISTANT_FFMPEG_BIN", "TARS_ASSISTANT_FFMPEG_BIN"}, func(cfg *Config) *string { return &cfg.AssistantFFmpegBin }, strings.TrimSpace),
	stringField("assistant_tts_bin", []string{"ASSISTANT_TTS_BIN", "TARS_ASSISTANT_TTS_BIN"}, func(cfg *Config) *string { return &cfg.AssistantTTSBin }, strings.TrimSpace),
	intField("compaction_trigger_tokens", []string{"COMPACTION_TRIGGER_TOKENS", "TARS_COMPACTION_TRIGGER_TOKENS"}, func(cfg *Config) *int { return &cfg.CompactionTriggerTokens }, parsePositiveInt),
	intField("compaction_keep_recent_tokens", []string{"COMPACTION_KEEP_RECENT_TOKENS", "TARS_COMPACTION_KEEP_RECENT_TOKENS"}, func(cfg *Config) *int { return &cfg.CompactionKeepRecentTokens }, parsePositiveInt),
	floatField("compaction_keep_recent_fraction", []string{"COMPACTION_KEEP_RECENT_FRACTION", "TARS_COMPACTION_KEEP_RECENT_FRACTION"}, func(cfg *Config) *float64 { return &cfg.CompactionKeepRecentFraction }, parsePositiveFloat),
	stringField("compaction_llm_mode", []string{"COMPACTION_LLM_MODE", "TARS_COMPACTION_LLM_MODE"}, func(cfg *Config) *string { return &cfg.CompactionLLMMode }, lowerTrimmedString),
	intField("compaction_llm_timeout_seconds", []string{"COMPACTION_LLM_TIMEOUT_SECONDS", "TARS_COMPACTION_LLM_TIMEOUT_SECONDS"}, func(cfg *Config) *int { return &cfg.CompactionLLMTimeoutSeconds }, parsePositiveInt),
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
	intField("tools_exec_max_timeout_ms", []string{"TOOLS_EXEC_MAX_TIMEOUT_MS", "TARS_TOOLS_EXEC_MAX_TIMEOUT_MS"}, func(cfg *Config) *int { return &cfg.ToolsExecMaxTimeoutMS }, parsePositiveInt),
	intField("tools_process_max_timeout_ms", []string{"TOOLS_PROCESS_MAX_TIMEOUT_MS", "TARS_TOOLS_PROCESS_MAX_TIMEOUT_MS"}, func(cfg *Config) *int { return &cfg.ToolsProcessMaxTimeoutMS }, parsePositiveInt),
	stringListField("tools_web_fetch_private_host_allowlist_json", []string{"TOOLS_WEB_FETCH_PRIVATE_HOST_ALLOWLIST_JSON", "TARS_TOOLS_WEB_FETCH_PRIVATE_HOST_ALLOWLIST_JSON"}, func(cfg *Config) *[]string { return &cfg.ToolsWebFetchPrivateHostAllowlist }, parseJSONStringList),
	boolField("tools_web_fetch_allow_private_hosts", []string{"TOOLS_WEB_FETCH_ALLOW_PRIVATE_HOSTS", "TARS_TOOLS_WEB_FETCH_ALLOW_PRIVATE_HOSTS"}, func(cfg *Config) *bool { return &cfg.ToolsWebFetchAllowPrivateHosts }),
	boolField("tools_apply_patch_enabled", []string{"TOOLS_APPLY_PATCH_ENABLED", "TARS_TOOLS_APPLY_PATCH_ENABLED"}, func(cfg *Config) *bool { return &cfg.ToolsApplyPatchEnabled }),
	boolField("agentruntime_enabled", []string{"AGENTRUNTIME_ENABLED", "TARS_AGENTRUNTIME_ENABLED"}, func(cfg *Config) *bool { return &cfg.AgentRuntimeEnabled }),
	stringField("agentruntime_default_agent", []string{"AGENTRUNTIME_DEFAULT_AGENT", "TARS_AGENTRUNTIME_DEFAULT_AGENT"}, func(cfg *Config) *string { return &cfg.AgentRuntimeDefaultAgent }, strings.TrimSpace),
	agentRuntimeTaskOverrideField("agentruntime_task_override", []string{"AGENTRUNTIME_TASK_OVERRIDE_JSON", "TARS_AGENTRUNTIME_TASK_OVERRIDE_JSON"}),
	agentRuntimeAgentsField("agentruntime_agents_json", []string{"AGENTRUNTIME_AGENTS_JSON", "TARS_AGENTRUNTIME_AGENTS_JSON"}),
	boolField("agentruntime_agents_watch", []string{"AGENTRUNTIME_AGENTS_WATCH", "TARS_AGENTRUNTIME_AGENTS_WATCH"}, func(cfg *Config) *bool { return &cfg.AgentRuntimeAgentsWatch }),
	intField("agentruntime_agents_watch_debounce_ms", []string{"AGENTRUNTIME_AGENTS_WATCH_DEBOUNCE_MS", "TARS_AGENTRUNTIME_AGENTS_WATCH_DEBOUNCE_MS"}, func(cfg *Config) *int { return &cfg.AgentRuntimeAgentsWatchDebounceMS }, parsePositiveInt),
	boolField("agentruntime_persistence_enabled", []string{"AGENTRUNTIME_PERSISTENCE_ENABLED", "TARS_AGENTRUNTIME_PERSISTENCE_ENABLED"}, func(cfg *Config) *bool { return &cfg.AgentRuntimePersistenceEnabled }),
	boolField("agentruntime_runs_persistence_enabled", []string{"AGENTRUNTIME_RUNS_PERSISTENCE_ENABLED", "TARS_AGENTRUNTIME_RUNS_PERSISTENCE_ENABLED"}, func(cfg *Config) *bool { return &cfg.AgentRuntimeRunsPersistenceEnabled }),
	boolField("agentruntime_channels_persistence_enabled", []string{"AGENTRUNTIME_CHANNELS_PERSISTENCE_ENABLED", "TARS_AGENTRUNTIME_CHANNELS_PERSISTENCE_ENABLED"}, func(cfg *Config) *bool { return &cfg.AgentRuntimeChannelsPersistenceEnabled }),
	intField("agentruntime_runs_max_records", []string{"AGENTRUNTIME_RUNS_MAX_RECORDS", "TARS_AGENTRUNTIME_RUNS_MAX_RECORDS"}, func(cfg *Config) *int { return &cfg.AgentRuntimeRunsMaxRecords }, parsePositiveInt),
	intField("agentruntime_channels_max_messages_per_channel", []string{"AGENTRUNTIME_CHANNELS_MAX_MESSAGES_PER_CHANNEL", "TARS_AGENTRUNTIME_CHANNELS_MAX_MESSAGES_PER_CHANNEL"}, func(cfg *Config) *int { return &cfg.AgentRuntimeChannelsMaxMessagesPerChannel }, parsePositiveInt),
	intField("agentruntime_subagents_max_threads", []string{"AGENTRUNTIME_SUBAGENTS_MAX_THREADS", "TARS_AGENTRUNTIME_SUBAGENTS_MAX_THREADS"}, func(cfg *Config) *int { return &cfg.AgentRuntimeSubagentsMaxThreads }, parsePositiveInt),
	intField("agentruntime_subagents_max_depth", []string{"AGENTRUNTIME_SUBAGENTS_MAX_DEPTH", "TARS_AGENTRUNTIME_SUBAGENTS_MAX_DEPTH"}, func(cfg *Config) *int { return &cfg.AgentRuntimeSubagentsMaxDepth }, parsePositiveInt),
	boolField("agentruntime_consensus_enabled", []string{"AGENTRUNTIME_CONSENSUS_ENABLED", "TARS_AGENTRUNTIME_CONSENSUS_ENABLED"}, func(cfg *Config) *bool { return &cfg.AgentRuntimeConsensusEnabled }),
	intField("agentruntime_consensus_max_fanout", []string{"AGENTRUNTIME_CONSENSUS_MAX_FANOUT", "TARS_AGENTRUNTIME_CONSENSUS_MAX_FANOUT"}, func(cfg *Config) *int { return &cfg.AgentRuntimeConsensusMaxFanout }, parsePositiveInt),
	intField("agentruntime_consensus_budget_tokens", []string{"AGENTRUNTIME_CONSENSUS_BUDGET_TOKENS", "TARS_AGENTRUNTIME_CONSENSUS_BUDGET_TOKENS"}, func(cfg *Config) *int { return &cfg.AgentRuntimeConsensusBudgetTokens }, parsePositiveInt),
	floatField("agentruntime_consensus_budget_usd", []string{"AGENTRUNTIME_CONSENSUS_BUDGET_USD", "TARS_AGENTRUNTIME_CONSENSUS_BUDGET_USD"}, func(cfg *Config) *float64 { return &cfg.AgentRuntimeConsensusBudgetUSD }, parsePositiveFloat),
	intField("agentruntime_consensus_timeout_seconds", []string{"AGENTRUNTIME_CONSENSUS_TIMEOUT_SECONDS", "TARS_AGENTRUNTIME_CONSENSUS_TIMEOUT_SECONDS"}, func(cfg *Config) *int { return &cfg.AgentRuntimeConsensusTimeoutSeconds }, parsePositiveInt),
	stringListField("agentruntime_consensus_allowed_aliases_json", []string{"AGENTRUNTIME_CONSENSUS_ALLOWED_ALIASES_JSON", "TARS_AGENTRUNTIME_CONSENSUS_ALLOWED_ALIASES_JSON"}, func(cfg *Config) *[]string { return &cfg.AgentRuntimeConsensusAllowedAliases }, parseJSONStringList),
	intField("agentruntime_consensus_concurrent_runs", []string{"AGENTRUNTIME_CONSENSUS_CONCURRENT_RUNS", "TARS_AGENTRUNTIME_CONSENSUS_CONCURRENT_RUNS"}, func(cfg *Config) *int { return &cfg.AgentRuntimeConsensusConcurrentRuns }, parsePositiveInt),
	stringField("agentruntime_persistence_dir", []string{"AGENTRUNTIME_PERSISTENCE_DIR", "TARS_AGENTRUNTIME_PERSISTENCE_DIR"}, func(cfg *Config) *string { return &cfg.AgentRuntimePersistenceDir }, strings.TrimSpace),
	boolField("agentruntime_restore_on_startup", []string{"AGENTRUNTIME_RESTORE_ON_STARTUP", "TARS_AGENTRUNTIME_RESTORE_ON_STARTUP"}, func(cfg *Config) *bool { return &cfg.AgentRuntimeRestoreOnStartup }),
	boolField("agentruntime_report_summary_enabled", []string{"AGENTRUNTIME_REPORT_SUMMARY_ENABLED", "TARS_AGENTRUNTIME_REPORT_SUMMARY_ENABLED"}, func(cfg *Config) *bool { return &cfg.AgentRuntimeReportSummaryEnabled }),
	boolField("agentruntime_archive_enabled", []string{"AGENTRUNTIME_ARCHIVE_ENABLED", "TARS_AGENTRUNTIME_ARCHIVE_ENABLED"}, func(cfg *Config) *bool { return &cfg.AgentRuntimeArchiveEnabled }),
	stringField("agentruntime_archive_dir", []string{"AGENTRUNTIME_ARCHIVE_DIR", "TARS_AGENTRUNTIME_ARCHIVE_DIR"}, func(cfg *Config) *string { return &cfg.AgentRuntimeArchiveDir }, strings.TrimSpace),
	intField("agentruntime_archive_retention_days", []string{"AGENTRUNTIME_ARCHIVE_RETENTION_DAYS", "TARS_AGENTRUNTIME_ARCHIVE_RETENTION_DAYS"}, func(cfg *Config) *int { return &cfg.AgentRuntimeArchiveRetentionDays }, parsePositiveInt),
	intField("agentruntime_archive_max_file_bytes", []string{"AGENTRUNTIME_ARCHIVE_MAX_FILE_BYTES", "TARS_AGENTRUNTIME_ARCHIVE_MAX_FILE_BYTES"}, func(cfg *Config) *int { return &cfg.AgentRuntimeArchiveMaxFileBytes }, parsePositiveInt),
	withYAMLPath(workLedgerEnabledField("work_ledger_enabled", []string{"WORK_LEDGER_ENABLED", "TARS_WORK_LEDGER_ENABLED"}), "work_ledger.enabled"),
	withYAMLPath(workSchedulerEnabledField("work_scheduler_enabled", []string{"WORK_SCHEDULER_ENABLED", "TARS_WORK_SCHEDULER_ENABLED"}), "work_ledger.scheduler.enabled"),
	withYAMLPath(intField("work_scheduler_max_workers", []string{"WORK_SCHEDULER_MAX_WORKERS", "TARS_WORK_SCHEDULER_MAX_WORKERS"}, func(cfg *Config) *int { return &cfg.WorkLedger.SchedulerMaxWorkers }, parsePositiveInt), "work_ledger.scheduler.max_workers"),
	withYAMLPath(intField("work_scheduler_lease_seconds", []string{"WORK_SCHEDULER_LEASE_SECONDS", "TARS_WORK_SCHEDULER_LEASE_SECONDS"}, func(cfg *Config) *int { return &cfg.WorkLedger.SchedulerLeaseSeconds }, parsePositiveInt), "work_ledger.scheduler.lease_seconds"),
	withYAMLPath(intField("work_scheduler_heartbeat_seconds", []string{"WORK_SCHEDULER_HEARTBEAT_SECONDS", "TARS_WORK_SCHEDULER_HEARTBEAT_SECONDS"}, func(cfg *Config) *int { return &cfg.WorkLedger.SchedulerHeartbeatSeconds }, parsePositiveInt), "work_ledger.scheduler.heartbeat_seconds"),
	withYAMLPath(intField("work_scheduler_poll_milliseconds", []string{"WORK_SCHEDULER_POLL_MILLISECONDS", "TARS_WORK_SCHEDULER_POLL_MILLISECONDS"}, func(cfg *Config) *int { return &cfg.WorkLedger.SchedulerPollMilliseconds }, parsePositiveInt), "work_ledger.scheduler.poll_milliseconds"),
	withYAMLPath(stringField("work_scheduler_execution_environment", []string{"WORK_SCHEDULER_EXECUTION_ENVIRONMENT", "TARS_WORK_SCHEDULER_EXECUTION_ENVIRONMENT"}, func(cfg *Config) *string { return &cfg.WorkLedger.SchedulerExecutionEnvironment }, lowerTrimmedString), "work_ledger.scheduler.execution_environment"),
	withYAMLPath(stringField("work_scheduler_execution_data_dir", []string{"WORK_SCHEDULER_EXECUTION_DATA_DIR", "TARS_WORK_SCHEDULER_EXECUTION_DATA_DIR"}, func(cfg *Config) *string { return &cfg.WorkLedger.SchedulerExecutionDataDir }, strings.TrimSpace), "work_ledger.scheduler.execution_data_dir"),
	withYAMLPath(stringListField("work_scheduler_artifact_paths_json", []string{"WORK_SCHEDULER_ARTIFACT_PATHS_JSON", "TARS_WORK_SCHEDULER_ARTIFACT_PATHS_JSON"}, func(cfg *Config) *[]string { return &cfg.WorkLedger.SchedulerArtifactPaths }, parseJSONStringList), "work_ledger.scheduler.artifact_paths"),
	withYAMLPath(boolField("work_scheduler_remote_workers_enabled", []string{"WORK_SCHEDULER_REMOTE_WORKERS_ENABLED", "TARS_WORK_SCHEDULER_REMOTE_WORKERS_ENABLED"}, func(cfg *Config) *bool { return &cfg.WorkLedger.SchedulerRemoteWorkersEnabled }), "work_ledger.scheduler.remote_workers.enabled"),
	withYAMLPath(boolField("work_scheduler_a2a_enabled", []string{"WORK_SCHEDULER_A2A_ENABLED", "TARS_WORK_SCHEDULER_A2A_ENABLED"}, func(cfg *Config) *bool { return &cfg.WorkLedger.SchedulerA2AEnabled }), "work_ledger.scheduler.a2a.enabled"),
	withYAMLPath(stringField("work_scheduler_a2a_discovery_url", []string{"WORK_SCHEDULER_A2A_DISCOVERY_URL", "TARS_WORK_SCHEDULER_A2A_DISCOVERY_URL"}, func(cfg *Config) *string { return &cfg.WorkLedger.SchedulerA2ADiscoveryURL }, strings.TrimSpace), "work_ledger.scheduler.a2a.discovery_url"),
	withYAMLPath(stringField("work_scheduler_a2a_bearer_token", []string{"WORK_SCHEDULER_A2A_BEARER_TOKEN", "TARS_WORK_SCHEDULER_A2A_BEARER_TOKEN"}, func(cfg *Config) *string { return &cfg.WorkLedger.SchedulerA2ABearerToken }, strings.TrimSpace), "work_ledger.scheduler.a2a.bearer_token"),
	withYAMLPath(stringListField("work_scheduler_a2a_allowed_hosts_json", []string{"WORK_SCHEDULER_A2A_ALLOWED_HOSTS_JSON", "TARS_WORK_SCHEDULER_A2A_ALLOWED_HOSTS_JSON"}, func(cfg *Config) *[]string { return &cfg.WorkLedger.SchedulerA2AAllowedHosts }, parseJSONStringList), "work_ledger.scheduler.a2a.allowed_hosts"),
	withYAMLPath(boolField("work_scheduler_a2a_allow_private_hosts", []string{"WORK_SCHEDULER_A2A_ALLOW_PRIVATE_HOSTS", "TARS_WORK_SCHEDULER_A2A_ALLOW_PRIVATE_HOSTS"}, func(cfg *Config) *bool { return &cfg.WorkLedger.SchedulerA2AAllowPrivateHosts }), "work_ledger.scheduler.a2a.allow_private_hosts"),
	withYAMLPath(boolField("work_scheduler_a2a_allow_insecure_loopback", []string{"WORK_SCHEDULER_A2A_ALLOW_INSECURE_LOOPBACK", "TARS_WORK_SCHEDULER_A2A_ALLOW_INSECURE_LOOPBACK"}, func(cfg *Config) *bool { return &cfg.WorkLedger.SchedulerA2AAllowInsecureLoopback }), "work_ledger.scheduler.a2a.allow_insecure_loopback"),
	withYAMLPath(intField("work_scheduler_a2a_poll_milliseconds", []string{"WORK_SCHEDULER_A2A_POLL_MILLISECONDS", "TARS_WORK_SCHEDULER_A2A_POLL_MILLISECONDS"}, func(cfg *Config) *int { return &cfg.WorkLedger.SchedulerA2APollMilliseconds }, parsePositiveInt), "work_ledger.scheduler.a2a.poll_milliseconds"),
	withYAMLPath(intField("work_scheduler_a2a_max_poll_seconds", []string{"WORK_SCHEDULER_A2A_MAX_POLL_SECONDS", "TARS_WORK_SCHEDULER_A2A_MAX_POLL_SECONDS"}, func(cfg *Config) *int { return &cfg.WorkLedger.SchedulerA2AMaxPollSeconds }, parsePositiveInt), "work_ledger.scheduler.a2a.max_poll_seconds"),
	boolField("channels_local_enabled", []string{"CHANNELS_LOCAL_ENABLED", "TARS_CHANNELS_LOCAL_ENABLED"}, func(cfg *Config) *bool { return &cfg.ChannelsLocalEnabled }),
	boolField("channels_webhook_enabled", []string{"CHANNELS_WEBHOOK_ENABLED", "TARS_CHANNELS_WEBHOOK_ENABLED"}, func(cfg *Config) *bool { return &cfg.ChannelsWebhookEnabled }),
	boolField("channels_telegram_enabled", []string{"CHANNELS_TELEGRAM_ENABLED", "TARS_CHANNELS_TELEGRAM_ENABLED"}, func(cfg *Config) *bool { return &cfg.ChannelsTelegramEnabled }),
	stringField("channels_telegram_dm_policy", []string{"CHANNELS_TELEGRAM_DM_POLICY", "TARS_CHANNELS_TELEGRAM_DM_POLICY"}, func(cfg *Config) *string { return &cfg.ChannelsTelegramDMPolicy }, lowerTrimmedString),
	boolField("channels_telegram_polling_enabled", []string{"CHANNELS_TELEGRAM_POLLING_ENABLED", "TARS_CHANNELS_TELEGRAM_POLLING_ENABLED"}, func(cfg *Config) *bool { return &cfg.ChannelsTelegramPollingEnabled }),
	stringField("telegram_bot_token", []string{"TELEGRAM_BOT_TOKEN", "TARS_TELEGRAM_BOT_TOKEN"}, func(cfg *Config) *string { return &cfg.TelegramBotToken }, strings.TrimSpace),
	withYAMLPath(companionEnabledField("companion_enabled", []string{"COMPANION_ENABLED", "TARS_COMPANION_ENABLED"}), "companion.enabled"),
	withYAMLPath(boolField("embodiment_enabled", []string{"EMBODIMENT_ENABLED", "TARS_EMBODIMENT_ENABLED"}, func(cfg *Config) *bool { return &cfg.Embodiment.Enabled }), "embodiment.enabled"),
	withYAMLPath(embodimentProvidersField("embodiment_providers_json", []string{"EMBODIMENT_PROVIDERS_JSON", "TARS_EMBODIMENT_PROVIDERS_JSON"}), "embodiment.providers"),
	boolField("tools_message_enabled", []string{"TOOLS_MESSAGE_ENABLED", "TARS_TOOLS_MESSAGE_ENABLED"}, func(cfg *Config) *bool { return &cfg.ToolsMessageEnabled }),
	boolField("tools_agentruntime_enabled", []string{"TOOLS_AGENTRUNTIME_ENABLED", "TARS_TOOLS_AGENTRUNTIME_ENABLED"}, func(cfg *Config) *bool { return &cfg.ToolsAgentRuntimeEnabled }),
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
		if strings.TrimSpace(field.yamlPath) == "" {
			field.yamlPath = inferPreferredYAMLPathForKey(field.yamlKey)
		}
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

type EnvOverrideMeta struct {
	EnvKey string `json:"env_key"`
}

func ActiveEnvOverrides() map[string]EnvOverrideMeta {
	overrides := map[string]EnvOverrideMeta{}
	for _, field := range configInputFields {
		if key, _, ok := firstDefinedEnvKey(field.envKeys); ok {
			overrides[field.yamlKey] = EnvOverrideMeta{EnvKey: key}
		}
	}
	return overrides
}

func configInputFieldByYAMLKey(key string) (configInputField, bool) {
	field, ok := configInputFieldsByYAMLKey[strings.TrimSpace(strings.ToLower(key))]
	return field, ok
}

func withYAMLPath(field configInputField, path string) configInputField {
	field.yamlPath = strings.TrimSpace(path)
	return field
}

func firstDefinedEnv(keys []string) string {
	_, value, ok := firstDefinedEnvKey(keys)
	if !ok {
		return ""
	}
	return value
}

func firstDefinedEnvKey(keys []string) (string, string, bool) {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return key, value, true
		}
	}
	return "", "", false
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

func companionEnabledField(yamlKey string, envKeys []string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			cfg.Companion.Enabled = parseBool(raw, cfg.Companion.Enabled)
			cfg.Companion.enabledSet = true
		},
		merge: func(dst *Config, src Config) {
			if src.Companion.enabledSet {
				dst.Companion.Enabled = src.Companion.Enabled
				dst.Companion.enabledSet = true
			}
		},
	}
}

func workLedgerEnabledField(yamlKey string, envKeys []string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			cfg.WorkLedger.Enabled = parseBool(raw, cfg.WorkLedger.Enabled)
			cfg.WorkLedger.enabledSet = true
		},
		merge: func(dst *Config, src Config) {
			if src.WorkLedger.enabledSet {
				dst.WorkLedger.Enabled = src.WorkLedger.Enabled
				dst.WorkLedger.enabledSet = true
			}
		},
	}
}

func workSchedulerEnabledField(yamlKey string, envKeys []string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			cfg.WorkLedger.SchedulerEnabled = parseBool(raw, cfg.WorkLedger.SchedulerEnabled)
			cfg.WorkLedger.schedulerEnabledSet = true
		},
		merge: func(dst *Config, src Config) {
			if src.WorkLedger.schedulerEnabledSet {
				dst.WorkLedger.SchedulerEnabled = src.WorkLedger.SchedulerEnabled
				dst.WorkLedger.schedulerEnabledSet = true
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

func styleDefaultField(yamlKey string, envKeys []string, accessor func(*Config) *int) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			ptr := accessor(cfg)
			*ptr = parseNonNegativeInt(raw, *ptr)
		},
		merge: func(dst *Config, src Config) {
			value := *accessor(&src)
			if value >= 0 {
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

func agentRuntimeAgentsField(yamlKey string, envKeys []string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			cfg.AgentRuntimeAgents = parseAgentRuntimeAgentsJSON(raw, cfg.AgentRuntimeAgents)
		},
		merge: func(dst *Config, src Config) {
			if len(src.AgentRuntimeAgents) == 0 {
				return
			}
			dst.AgentRuntimeAgents = append([]AgentRuntimeAgent(nil), src.AgentRuntimeAgents...)
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
