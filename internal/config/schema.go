package config

import "github.com/devlikebear/tars/internal/memory"

// FieldMeta describes a single configuration field for UI rendering.
type FieldMeta struct {
	Key             string   `json:"key"`
	Path            string   `json:"path"`
	Section         string   `json:"section"`
	Type            string   `json:"type"` // "string", "int", "float", "bool", "json"
	Label           string   `json:"label"`
	Description     string   `json:"description"`
	Impact          []string `json:"impact,omitempty"`
	DefaultValue    any      `json:"default_value,omitempty"`
	RequiresRestart bool     `json:"requires_restart"`
	Sensitive       bool     `json:"sensitive,omitempty"`
	Options         []string `json:"options,omitempty"`
}

func f(key, section, typ, label, desc string) FieldMeta {
	return FieldMeta{Key: key, Path: preferredYAMLPathForKey(key), Section: section, Type: typ, Label: label, Description: desc}
}

func fs(key, section, label, desc string, sensitive bool) FieldMeta {
	return FieldMeta{Key: key, Path: preferredYAMLPathForKey(key), Section: section, Type: "string", Label: label, Description: desc, Sensitive: sensitive}
}

func fsel(key, section, label, desc string, options []string) FieldMeta {
	return FieldMeta{Key: key, Path: preferredYAMLPathForKey(key), Section: section, Type: "select", Label: label, Description: desc, Options: options}
}

func fjson(key, section, label, desc string) FieldMeta {
	return FieldMeta{Key: key, Path: preferredYAMLPathForKey(key), Section: section, Type: "json", Label: label, Description: desc}
}

// Schema returns metadata for all configuration fields, grouped for UI display.
func Schema() []FieldMeta {
	return withConfigFieldMeta(withConfigImpactHints([]FieldMeta{
		// ── Runtime ──────────────────────────────
		f("workspace_dir", "Runtime", "string", "Workspace Directory", "Directory for workspace data and sessions"),
		fsel("plan_clarify_mode", "Runtime", "Plan Clarify Mode", "How TARS handles ambiguous multi-step requests before drafting a plan: smart = LLM evaluates ambiguity itself; auto = always plan immediately; ask = always ask 1–3 clarifying questions first.", []string{"smart", "auto", "ask"}),
		f("session_default_id", "Runtime", "string", "Default Session ID", "Override the default session identifier"),
		fsel("session_telegram_scope", "Runtime", "Telegram Session Scope", "Session scoping for Telegram messages", []string{"main", "per-chat"}),
		f("style_directness_default", "Runtime", "int", "Style Directness Default", "Global default for the directness slider used in session style controls"),
		f("style_humor_default", "Runtime", "int", "Style Humor Default", "Global default for the humor slider used in session style controls"),
		f("style_caution_default", "Runtime", "int", "Style Caution Default", "Global default for the caution slider used in session style controls"),
		f("style_autonomy_default", "Runtime", "int", "Style Autonomy Default", "Global default for the autonomy slider used in session style controls"),
		fsel("log_level", "Runtime", "Log Level", "Logging verbosity", []string{"debug", "info", "warn", "error"}),
		f("log_file", "Runtime", "string", "Log File", "Path to log file (empty for stderr)"),
		f("log_rotate_max_size_mb", "Runtime", "int", "Log Rotate Max Size (MB)", "Max log file size before rotation"),
		f("log_rotate_max_days", "Runtime", "int", "Log Rotate Max Days", "Max days to retain rotated log files"),
		f("log_rotate_max_backups", "Runtime", "int", "Log Rotate Max Backups", "Max number of rotated log files to retain"),

		// ── API ──────────────────────────────────
		fsel("api_auth_mode", "API", "Auth Mode", "API authentication mode", []string{"off", "required", "external-required"}),
		fsel("dashboard_auth_mode", "API", "Dashboard Auth Mode", "Dashboard auth mode. 'off' disables dashboard auth while keeping /v1/* protected", []string{"inherit", "off"}),
		fs("api_auth_token", "API", "Auth Token (Legacy)", "Legacy single bearer token for API authentication", true),
		fs("api_user_token", "API", "User Token", "User-tier bearer token (read/chat/general operations)", true),
		fs("api_admin_token", "API", "Admin Token", "Admin-tier bearer token (control operations, agent runtime, config)", true),
		f("api_allow_insecure_local_auth", "API", "bool", "Allow Insecure Local Auth", "Allow loopback (127.0.0.1) requests without auth token"),
		f("api_max_inflight_chat", "API", "int", "Max Inflight Chat", "Maximum concurrent chat requests"),
		f("api_max_inflight_agent_runs", "API", "int", "Max Inflight Agent Runs", "Maximum concurrent agent run requests"),

		// ── LLM ──────────────────────────────────
		fjson("llm_providers", "LLM", "Providers", "Named provider pool keyed by alias. Each entry defines kind/auth/base_url/api_key at the provider level."),
		fjson("llm_tiers", "LLM", "Tiers", "Tier bindings keyed by heavy/standard/light (or custom tiers). Each entry binds provider/model/reasoning settings."),
		f("llm_default_tier", "LLM", "string", "Default Tier", "Tier used when a role has no explicit override in llm.role_defaults"),
		fjson("llm_role_defaults", "LLM", "Role Defaults", "Map of runtime role name to tier alias (for example chat_main -> standard)"),

		// ── Memory ───────────────────────────────
		fsel("memory_backend", "Memory", "Backend", "Memory backend implementation", []string{"file"}),
		f("memory_semantic_enabled", "Memory", "bool", "Semantic Memory", "Enable semantic memory with vector embeddings"),
		fsel("memory_embed_provider", "Memory", "Embed Provider", "Embedding provider", memory.SupportedEmbedProviders()),
		f("memory_embed_base_url", "Memory", "string", "Embed Base URL", "Base URL for embedding API"),
		fs("memory_embed_api_key", "Memory", "Embed API Key", "API key for the embedding provider", true),
		f("memory_embed_model", "Memory", "string", "Embed Model", "Embedding model identifier"),
		f("memory_embed_dimensions", "Memory", "int", "Embed Dimensions", "Vector dimensions for embeddings"),

		// ── Usage ────────────────────────────────
		f("usage_limit_daily_usd", "Usage", "float", "Daily Limit (USD)", "Maximum daily LLM spend in USD"),
		f("usage_limit_weekly_usd", "Usage", "float", "Weekly Limit (USD)", "Maximum weekly LLM spend in USD"),
		f("usage_limit_monthly_usd", "Usage", "float", "Monthly Limit (USD)", "Maximum monthly LLM spend in USD"),
		f("usage_daily_token_budget", "Usage", "int", "Daily Token Budget", "Daily input+output token budget for the console usage indicator; 0 disables the chip"),
		fsel("usage_limit_mode", "Usage", "Limit Mode", "Enforcement mode", []string{"soft", "hard"}),
		fjson("usage_price_overrides_json", "Usage", "Price Overrides", "Optional per-model usage price override map"),

		// ── Automation ───────────────────────────
		f("agent_max_iterations", "Automation", "int", "Max Iterations", "Maximum agent loop iterations per request"),
		f("cron_run_history_limit", "Automation", "int", "Cron History Limit", "Maximum run records kept per cron job"),
		// Pulse (system watchdog) schema entries
		f("pulse_enabled", "Automation", "bool", "Pulse Enabled", "Enable the pulse system watchdog"),
		f("pulse_interval", "Automation", "string", "Pulse Interval", "Pulse tick interval (e.g. 1m, 5m)"),
		f("pulse_timeout", "Automation", "string", "Pulse Timeout", "Per-tick LLM call timeout"),
		f("pulse_active_hours", "Automation", "string", "Pulse Active Hours", "Pulse active hours window (HH:MM-HH:MM)"),
		f("pulse_timezone", "Automation", "string", "Pulse Timezone", "Timezone for pulse active hours"),
		fsel("pulse_min_severity", "Automation", "Pulse Min Severity", "Minimum severity for notifications", []string{"info", "warn", "error", "critical"}),
		f("pulse_allowed_autofixes_json", "Automation", "string_list", "Pulse Autofix Allowlist", "Autofixes the decider may invoke"),
		f("pulse_notify_telegram", "Automation", "bool", "Pulse Notify Telegram", "Forward pulse notifications to telegram"),
		f("pulse_notify_session_events", "Automation", "bool", "Pulse Notify Session Events", "Forward pulse notifications to the session event stream"),
		f("pulse_cron_failure_threshold", "Automation", "int", "Pulse Cron Failure Threshold", "Consecutive cron failures before pulse reacts"),
		f("pulse_stuck_run_minutes", "Automation", "int", "Pulse Stuck Run Minutes", "Minutes a agent runtime run may be running before pulse flags it"),
		f("pulse_disk_warn_percent", "Automation", "float", "Pulse Disk Warn %", "Disk usage percent that triggers a warn signal"),
		f("pulse_disk_critical_percent", "Automation", "float", "Pulse Disk Critical %", "Disk usage percent that triggers a critical signal"),
		f("pulse_delivery_failure_threshold", "Automation", "int", "Pulse Delivery Failure Threshold", "Telegram delivery failures in window before pulse reacts"),
		f("pulse_delivery_failure_window", "Automation", "string", "Pulse Delivery Failure Window", "Rolling window for counting delivery failures (e.g. 10m)"),
		f("pulse_reflection_failure_threshold", "Automation", "int", "Pulse Reflection Failure Threshold", "Consecutive nightly reflection failures before pulse reacts"),
		f("reflection_enabled", "Automation", "bool", "Reflection Enabled", "Enable the nightly reflection batch runner"),
		f("reflection_sleep_window", "Automation", "string", "Reflection Sleep Window", "HH:MM-HH:MM window during which reflection is allowed to run"),
		f("reflection_timezone", "Automation", "string", "Reflection Timezone", "Timezone for reflection sleep-window evaluation"),
		f("reflection_tick_interval", "Automation", "string", "Reflection Tick Interval", "How often reflection checks whether to run (e.g. 5m)"),
		f("reflection_empty_session_age", "Automation", "string", "Empty Session Age", "Minimum age a zero-message session must reach before removal (e.g. 24h)"),
		f("reflection_memory_lookback_hours", "Automation", "int", "Memory Lookback Hours", "How far back reflection reads session history for experience extraction"),
		f("reflection_max_turns_per_session", "Automation", "int", "Max Turns Per Session", "Cap on turns reflection processes per session per run"),
		f("notify_command", "Automation", "string", "Notify Command", "Shell command executed for notifications"),
		f("notify_when_no_clients", "Automation", "bool", "Notify When No Clients", "Send notifications even when no SSE clients connected"),
		f("schedule_timezone", "Automation", "string", "Schedule Timezone", "Default timezone for scheduled triggers"),

		// ── Assistant ────────────────────────────
		f("assistant_enabled", "Assistant", "bool", "Enabled", "Enable voice assistant feature"),
		f("assistant_hotkey", "Assistant", "string", "Hotkey", "Global hotkey to activate assistant"),
		f("assistant_whisper_bin", "Assistant", "string", "Whisper Binary", "Path to whisper CLI for speech-to-text"),
		f("assistant_ffmpeg_bin", "Assistant", "string", "FFmpeg Binary", "Path to ffmpeg for audio processing"),
		f("assistant_tts_bin", "Assistant", "string", "TTS Binary", "Path to text-to-speech binary"),

		// ── Compaction ───────────────────────────
		f("compaction_trigger_tokens", "Compaction", "int", "Trigger Tokens", "Estimated transcript tokens that trigger auto-compaction"),
		f("compaction_keep_recent_tokens", "Compaction", "int", "Keep Recent Tokens", "Minimum recent transcript tokens to preserve during compaction"),
		f("compaction_keep_recent_fraction", "Compaction", "float", "Keep Recent Fraction", "Fraction of recent transcript tokens preserved during compaction"),
		fsel("compaction_llm_mode", "Compaction", "LLM Mode", "Whether compaction may call the LLM or stay deterministic", []string{"auto", "deterministic"}),
		f("compaction_llm_timeout_seconds", "Compaction", "int", "LLM Timeout (sec)", "Timeout for LLM-assisted compaction before deterministic fallback"),

		// ── Tools ────────────────────────────────
		f("tools_web_search_enabled", "Tools", "bool", "Web Search", "Enable web search tool"),
		f("tools_web_fetch_enabled", "Tools", "bool", "Web Fetch", "Enable web fetch tool"),
		f("tools_default_set", "Tools", "string", "Default Tool Set", "Default tool policy/profile for chat sessions"),
		f("tools_allow_high_risk_user", "Tools", "bool", "Allow High-Risk Tools", "Allow user-level access to high-risk tools"),
		fs("tools_web_search_api_key", "Tools", "Search API Key", "API key for web search provider", true),
		fsel("tools_web_search_provider", "Tools", "Search Provider", "Web search backend", []string{"brave", "perplexity"}),
		fs("tools_web_search_perplexity_api_key", "Tools", "Perplexity API Key", "API key when the web search provider is perplexity", true),
		f("tools_web_search_perplexity_model", "Tools", "string", "Perplexity Model", "Model name used for Perplexity-backed search"),
		f("tools_web_search_perplexity_base_url", "Tools", "string", "Perplexity Base URL", "Override URL for the Perplexity chat completions API"),
		f("tools_web_search_cache_ttl_seconds", "Tools", "int", "Search Cache TTL", "Cache duration for search results in seconds"),
		f("tools_web_fetch_allow_private_hosts", "Tools", "bool", "Allow Private Hosts", "Allow fetching from private/internal hosts"),
		f("tools_web_fetch_private_host_allowlist_json", "Tools", "string_list", "Private Host Allowlist", "Explicit private hosts allowed for web fetch requests"),
		f("tools_apply_patch_enabled", "Tools", "bool", "Apply Patch", "Enable apply-patch tool"),
		f("tools_message_enabled", "Tools", "bool", "Message Tool", "Enable message/notification tool"),
		f("tools_agentruntime_enabled", "Tools", "bool", "Agent Runtime Tool", "Enable agent runtime dispatch tool"),

		// ── MCP ──────────────────────────────────
		f("mcp_command_allowlist_json", "MCP", "string_list", "Command Allowlist", "Commands that bundled or installed MCP servers may execute"),
		fjson("mcp_servers_json", "MCP", "Servers", "Optional MCP server catalog entries configured directly in YAML"),

		// ── Agent Runtime ───────────────────────
		f("agentruntime_enabled", "Agent Runtime", "bool", "Enabled", "Enable agent runtime for multi-agent orchestration"),
		f("agentruntime_default_agent", "Agent Runtime", "string", "Default Agent", "Default agent name for dispatched tasks"),
		fjson("agentruntime_agents_json", "Agent Runtime", "Agent Catalog", "Agent runtime agent catalog configured directly in YAML"),
		fjson("agentruntime_task_override", "Agent Runtime", "Task Override", "Optional provider/model allowlist for agent runtime task overrides"),
		f("agentruntime_agents_watch", "Agent Runtime", "bool", "Watch Agent Files", "Auto-reload agents when definition files change"),
		f("agentruntime_agents_watch_debounce_ms", "Agent Runtime", "int", "Watch Debounce (ms)", "Debounce window for agent runtime agent catalog reloads"),
		f("agentruntime_persistence_enabled", "Agent Runtime", "bool", "Persistence", "Enable agent runtime state persistence"),
		f("agentruntime_persistence_dir", "Agent Runtime", "string", "Persistence Dir", "Directory used for persisted agent runtime state"),
		f("agentruntime_runs_persistence_enabled", "Agent Runtime", "bool", "Runs Persistence", "Persist agent run records"),
		f("agentruntime_channels_persistence_enabled", "Agent Runtime", "bool", "Channels Persistence", "Persist channel message history"),
		f("agentruntime_runs_max_records", "Agent Runtime", "int", "Max Run Records", "Maximum stored run records"),
		f("agentruntime_channels_max_messages_per_channel", "Agent Runtime", "int", "Max Messages/Channel", "Maximum messages retained per channel"),
		f("agentruntime_subagents_max_threads", "Agent Runtime", "int", "Max Subagent Threads", "Maximum concurrent subagent threads"),
		f("agentruntime_subagents_max_depth", "Agent Runtime", "int", "Max Subagent Depth", "Maximum subagent nesting depth"),
		f("agentruntime_consensus_enabled", "Agent Runtime", "bool", "Consensus Enabled", "Advanced opt-in: expose mode=consensus in subagents_run and allow consensus executions"),
		f("agentruntime_consensus_max_fanout", "Agent Runtime", "int", "Consensus Max Fanout", "Advanced opt-in maximum variants launched for a single consensus task"),
		f("agentruntime_consensus_budget_tokens", "Agent Runtime", "int", "Consensus Token Budget", "Advanced opt-in hard token ceiling for one consensus execution"),
		f("agentruntime_consensus_budget_usd", "Agent Runtime", "float", "Consensus Budget (USD)", "Advanced opt-in cost ceiling for one consensus execution"),
		f("agentruntime_consensus_timeout_seconds", "Agent Runtime", "int", "Consensus Timeout Seconds", "Advanced opt-in maximum wall time for a single consensus execution"),
		f("agentruntime_consensus_allowed_aliases_json", "Agent Runtime", "string_list", "Consensus Allowed Aliases", "Advanced opt-in provider alias allowlist for consensus variants"),
		f("agentruntime_consensus_concurrent_runs", "Agent Runtime", "int", "Consensus Concurrent Runs", "Advanced opt-in maximum number of consensus runs allowed at once"),
		f("agentruntime_restore_on_startup", "Agent Runtime", "bool", "Restore on Startup", "Restore persisted runs when server starts"),
		f("agentruntime_report_summary_enabled", "Agent Runtime", "bool", "Report Summary Enabled", "Emit summarized run reports for agent runtime tasks"),
		f("agentruntime_archive_enabled", "Agent Runtime", "bool", "Archive Enabled", "Enable run archival to disk"),
		f("agentruntime_archive_dir", "Agent Runtime", "string", "Archive Dir", "Directory for archived run files"),
		f("agentruntime_archive_retention_days", "Agent Runtime", "int", "Archive Retention (days)", "Days to retain archived runs"),
		f("agentruntime_archive_max_file_bytes", "Agent Runtime", "int", "Archive Max File Bytes", "Maximum archive file size before rollover"),

		// ── Channels ─────────────────────────────
		f("channels_local_enabled", "Channels", "bool", "Local Channel", "Enable local channel for CLI dispatch"),
		f("channels_webhook_enabled", "Channels", "bool", "Webhook Channel", "Enable inbound webhook channel"),
		f("channels_telegram_enabled", "Channels", "bool", "Telegram Channel", "Enable Telegram bot channel"),
		fsel("channels_telegram_dm_policy", "Channels", "Telegram DM Policy", "DM access policy", []string{"open", "pairing", "deny"}),
		f("channels_telegram_polling_enabled", "Channels", "bool", "Telegram Polling", "Enable Telegram long-polling for updates"),
		fs("telegram_bot_token", "Channels", "Telegram Bot Token", "Bot token from @BotFather", true),

		// ── Extensions ───────────────────────────
		f("skills_enabled", "Extensions", "bool", "Skills Enabled", "Load and serve skill definitions"),
		f("skills_watch", "Extensions", "bool", "Watch Skills", "Auto-reload skills when files change"),
		f("skills_watch_debounce_ms", "Extensions", "int", "Skills Watch Debounce (ms)", "Debounce window for skill reload events"),
		f("skills_extra_dirs_json", "Extensions", "string_list", "Skills Extra Dirs", "Additional directories searched for user skill definitions"),
		f("skills_bundled_dir", "Extensions", "string", "Skills Directory", "Directory for bundled skill files"),
		f("plugins_enabled", "Extensions", "bool", "Plugins Enabled", "Load and serve plugin definitions"),
		f("plugins_watch", "Extensions", "bool", "Watch Plugins", "Auto-reload plugins when files change"),
		f("plugins_watch_debounce_ms", "Extensions", "int", "Plugins Watch Debounce (ms)", "Debounce window for plugin reload events"),
		f("plugins_extra_dirs_json", "Extensions", "string_list", "Plugins Extra Dirs", "Additional directories searched for user plugin definitions"),
		f("plugins_bundled_dir", "Extensions", "string", "Plugins Directory", "Directory for bundled plugin files"),
		f("plugins_allow_mcp_servers", "Extensions", "bool", "Allow MCP in Plugins", "Allow plugins to register MCP servers"),
	}))
}

// ConfigToMap converts a Config to a flat map keyed by YAML keys.
func ConfigToMap(cfg Config) map[string]any {
	m := map[string]any{}
	for _, field := range configInputFields {
		if field.yamlKey == "" {
			continue
		}
		var probe Config
		field.merge(&probe, cfg)
		m[field.yamlKey] = extractValue(field.yamlKey, probe)
	}
	return m
}

func extractValue(yamlKey string, cfg Config) any {
	switch yamlKey {
	// Runtime
	case "workspace_dir":
		return cfg.WorkspaceDir
	case "plan_clarify_mode":
		return cfg.PlanClarifyMode
	case "session_default_id":
		return cfg.SessionDefaultID
	case "session_telegram_scope":
		return cfg.SessionTelegramScope
	case "style_directness_default":
		return cfg.StyleDirectnessDefault
	case "style_humor_default":
		return cfg.StyleHumorDefault
	case "style_caution_default":
		return cfg.StyleCautionDefault
	case "style_autonomy_default":
		return cfg.StyleAutonomyDefault
	case "log_level":
		return cfg.LogLevel
	case "log_file":
		return cfg.LogFile
	case "log_rotate_max_size_mb":
		return cfg.LogRotateMaxSizeMB
	case "log_rotate_max_days":
		return cfg.LogRotateMaxDays
	case "log_rotate_max_backups":
		return cfg.LogRotateMaxBackups
	// API
	case "api_auth_mode":
		return cfg.APIAuthMode
	case "dashboard_auth_mode":
		return cfg.DashboardAuthMode
	case "api_auth_token":
		return cfg.APIAuthToken
	case "api_user_token":
		return cfg.APIUserToken
	case "api_admin_token":
		return cfg.APIAdminToken
	case "api_allow_insecure_local_auth":
		return cfg.APIAllowInsecureLocalAuth
	case "api_max_inflight_chat":
		return cfg.APIMaxInflightChat
	case "api_max_inflight_agent_runs":
		return cfg.APIMaxInflightAgentRuns
	// LLM
	case "llm_providers":
		return sparseProvidersMap(cfg.LLMProviders)
	case "llm_tiers":
		return cloneLLMTiers(cfg.LLMTiers)
	case "llm_default_tier":
		return cfg.LLMDefaultTier
	case "llm_role_defaults":
		return cloneStringMap(cfg.LLMRoleDefaults)
	// Memory
	case "memory_backend":
		return cfg.MemoryBackend
	case "memory_semantic_enabled":
		return cfg.MemorySemanticEnabled
	case "memory_embed_provider":
		return cfg.MemoryEmbedProvider
	case "memory_embed_base_url":
		return cfg.MemoryEmbedBaseURL
	case "memory_embed_api_key":
		return cfg.MemoryEmbedAPIKey
	case "memory_embed_model":
		return cfg.MemoryEmbedModel
	case "memory_embed_dimensions":
		return cfg.MemoryEmbedDimensions
	// Usage
	case "usage_limit_daily_usd":
		return cfg.UsageLimitDailyUSD
	case "usage_limit_weekly_usd":
		return cfg.UsageLimitWeeklyUSD
	case "usage_limit_monthly_usd":
		return cfg.UsageLimitMonthlyUSD
	case "usage_daily_token_budget":
		return cfg.UsageDailyTokenBudget
	case "usage_limit_mode":
		return cfg.UsageLimitMode
	case "usage_price_overrides_json":
		return cloneUsagePriceOverrides(cfg.UsagePriceOverrides)
	// Automation
	case "agent_max_iterations":
		return cfg.AgentMaxIterations
	case "cron_run_history_limit":
		return cfg.CronRunHistoryLimit
	case "pulse_enabled":
		return cfg.PulseEnabled
	case "pulse_interval":
		return cfg.PulseInterval
	case "pulse_timeout":
		return cfg.PulseTimeout
	case "pulse_active_hours":
		return cfg.PulseActiveHours
	case "pulse_timezone":
		return cfg.PulseTimezone
	case "pulse_min_severity":
		return cfg.PulseMinSeverity
	case "pulse_allowed_autofixes_json":
		return cfg.PulseAllowedAutofixes
	case "pulse_notify_telegram":
		return cfg.PulseNotifyTelegram
	case "pulse_notify_session_events":
		return cfg.PulseNotifySessionEvents
	case "pulse_cron_failure_threshold":
		return cfg.PulseCronFailureThreshold
	case "pulse_stuck_run_minutes":
		return cfg.PulseStuckRunMinutes
	case "pulse_disk_warn_percent":
		return cfg.PulseDiskWarnPercent
	case "pulse_disk_critical_percent":
		return cfg.PulseDiskCriticalPercent
	case "pulse_delivery_failure_threshold":
		return cfg.PulseDeliveryFailureThreshold
	case "pulse_delivery_failure_window":
		return cfg.PulseDeliveryFailureWindow
	case "pulse_reflection_failure_threshold":
		return cfg.PulseReflectionFailureThreshold
	case "reflection_enabled":
		return cfg.ReflectionEnabled
	case "reflection_sleep_window":
		return cfg.ReflectionSleepWindow
	case "reflection_timezone":
		return cfg.ReflectionTimezone
	case "reflection_tick_interval":
		return cfg.ReflectionTickInterval
	case "reflection_empty_session_age":
		return cfg.ReflectionEmptySessionAge
	case "reflection_memory_lookback_hours":
		return cfg.ReflectionMemoryLookbackHours
	case "reflection_max_turns_per_session":
		return cfg.ReflectionMaxTurnsPerSession
	case "notify_command":
		return cfg.NotifyCommand
	case "notify_when_no_clients":
		return cfg.NotifyWhenNoClients
	case "schedule_timezone":
		return cfg.ScheduleTimezone
	// Assistant
	case "assistant_enabled":
		return cfg.AssistantEnabled
	case "assistant_hotkey":
		return cfg.AssistantHotkey
	case "assistant_whisper_bin":
		return cfg.AssistantWhisperBin
	case "assistant_ffmpeg_bin":
		return cfg.AssistantFFmpegBin
	case "assistant_tts_bin":
		return cfg.AssistantTTSBin
	// Compaction
	case "compaction_trigger_tokens":
		return cfg.CompactionTriggerTokens
	case "compaction_keep_recent_tokens":
		return cfg.CompactionKeepRecentTokens
	case "compaction_keep_recent_fraction":
		return cfg.CompactionKeepRecentFraction
	case "compaction_llm_mode":
		return cfg.CompactionLLMMode
	case "compaction_llm_timeout_seconds":
		return cfg.CompactionLLMTimeoutSeconds
	// Tools
	case "tools_web_search_enabled":
		return cfg.ToolsWebSearchEnabled
	case "tools_web_fetch_enabled":
		return cfg.ToolsWebFetchEnabled
	case "tools_default_set":
		return cfg.ToolsDefaultSet
	case "tools_allow_high_risk_user":
		return cfg.ToolsAllowHighRiskUser
	case "tools_web_search_api_key":
		return cfg.ToolsWebSearchAPIKey
	case "tools_web_search_provider":
		return cfg.ToolsWebSearchProvider
	case "tools_web_search_perplexity_api_key":
		return cfg.ToolsWebSearchPerplexityAPIKey
	case "tools_web_search_perplexity_model":
		return cfg.ToolsWebSearchPerplexityModel
	case "tools_web_search_perplexity_base_url":
		return cfg.ToolsWebSearchPerplexityBaseURL
	case "tools_web_search_cache_ttl_seconds":
		return cfg.ToolsWebSearchCacheTTLSeconds
	case "tools_web_fetch_private_host_allowlist_json":
		return append([]string(nil), cfg.ToolsWebFetchPrivateHostAllowlist...)
	case "tools_web_fetch_allow_private_hosts":
		return cfg.ToolsWebFetchAllowPrivateHosts
	case "tools_apply_patch_enabled":
		return cfg.ToolsApplyPatchEnabled
	case "tools_message_enabled":
		return cfg.ToolsMessageEnabled
	case "tools_agentruntime_enabled":
		return cfg.ToolsAgentRuntimeEnabled
	// MCP
	case "mcp_command_allowlist_json":
		return append([]string(nil), cfg.MCPCommandAllowlist...)
	case "mcp_servers_json":
		return append([]MCPServer(nil), cfg.MCPServers...)
	// AgentRuntime
	case "agentruntime_enabled":
		return cfg.AgentRuntimeEnabled
	case "agentruntime_default_agent":
		return cfg.AgentRuntimeDefaultAgent
	case "agentruntime_agents_json":
		return append([]AgentRuntimeAgent(nil), cfg.AgentRuntimeAgents...)
	case "agentruntime_task_override":
		return AgentRuntimeTaskOverrideConfig{
			Enabled:        cfg.AgentRuntimeTaskOverride.Enabled,
			AllowedAliases: append([]string(nil), cfg.AgentRuntimeTaskOverride.AllowedAliases...),
			AllowedModels:  append([]string(nil), cfg.AgentRuntimeTaskOverride.AllowedModels...),
		}
	case "agentruntime_agents_watch":
		return cfg.AgentRuntimeAgentsWatch
	case "agentruntime_agents_watch_debounce_ms":
		return cfg.AgentRuntimeAgentsWatchDebounceMS
	case "agentruntime_persistence_enabled":
		return cfg.AgentRuntimePersistenceEnabled
	case "agentruntime_persistence_dir":
		return cfg.AgentRuntimePersistenceDir
	case "agentruntime_runs_persistence_enabled":
		return cfg.AgentRuntimeRunsPersistenceEnabled
	case "agentruntime_channels_persistence_enabled":
		return cfg.AgentRuntimeChannelsPersistenceEnabled
	case "agentruntime_runs_max_records":
		return cfg.AgentRuntimeRunsMaxRecords
	case "agentruntime_channels_max_messages_per_channel":
		return cfg.AgentRuntimeChannelsMaxMessagesPerChannel
	case "agentruntime_subagents_max_threads":
		return cfg.AgentRuntimeSubagentsMaxThreads
	case "agentruntime_subagents_max_depth":
		return cfg.AgentRuntimeSubagentsMaxDepth
	case "agentruntime_consensus_enabled":
		return cfg.AgentRuntimeConsensusEnabled
	case "agentruntime_consensus_max_fanout":
		return cfg.AgentRuntimeConsensusMaxFanout
	case "agentruntime_consensus_budget_tokens":
		return cfg.AgentRuntimeConsensusBudgetTokens
	case "agentruntime_consensus_budget_usd":
		return cfg.AgentRuntimeConsensusBudgetUSD
	case "agentruntime_consensus_timeout_seconds":
		return cfg.AgentRuntimeConsensusTimeoutSeconds
	case "agentruntime_consensus_allowed_aliases_json":
		return append([]string(nil), cfg.AgentRuntimeConsensusAllowedAliases...)
	case "agentruntime_consensus_concurrent_runs":
		return cfg.AgentRuntimeConsensusConcurrentRuns
	case "agentruntime_restore_on_startup":
		return cfg.AgentRuntimeRestoreOnStartup
	case "agentruntime_report_summary_enabled":
		return cfg.AgentRuntimeReportSummaryEnabled
	case "agentruntime_archive_enabled":
		return cfg.AgentRuntimeArchiveEnabled
	case "agentruntime_archive_dir":
		return cfg.AgentRuntimeArchiveDir
	case "agentruntime_archive_retention_days":
		return cfg.AgentRuntimeArchiveRetentionDays
	case "agentruntime_archive_max_file_bytes":
		return cfg.AgentRuntimeArchiveMaxFileBytes
	// Channels
	case "channels_local_enabled":
		return cfg.ChannelsLocalEnabled
	case "channels_webhook_enabled":
		return cfg.ChannelsWebhookEnabled
	case "channels_telegram_enabled":
		return cfg.ChannelsTelegramEnabled
	case "channels_telegram_dm_policy":
		return cfg.ChannelsTelegramDMPolicy
	case "channels_telegram_polling_enabled":
		return cfg.ChannelsTelegramPollingEnabled
	case "telegram_bot_token":
		return cfg.TelegramBotToken
	// Extensions
	case "skills_enabled":
		return cfg.SkillsEnabled
	case "skills_watch":
		return cfg.SkillsWatch
	case "skills_watch_debounce_ms":
		return cfg.SkillsWatchDebounceMS
	case "skills_extra_dirs_json":
		return append([]string(nil), cfg.SkillsExtraDirs...)
	case "skills_bundled_dir":
		return cfg.SkillsBundledDir
	case "plugins_enabled":
		return cfg.PluginsEnabled
	case "plugins_watch":
		return cfg.PluginsWatch
	case "plugins_watch_debounce_ms":
		return cfg.PluginsWatchDebounceMS
	case "plugins_extra_dirs_json":
		return append([]string(nil), cfg.PluginsExtraDirs...)
	case "plugins_bundled_dir":
		return cfg.PluginsBundledDir
	case "plugins_allow_mcp_servers":
		return cfg.PluginsAllowMCPServers
	default:
		return nil
	}
}

// sparseProvidersMap converts an LLMProviders map to a two-level
// map[string]map[string]any that only includes non-empty string fields.
// This is used by extractValue so that PatchYAML's nested-map merge can
// preserve existing fields (especially api_key) that the patch omits.
func sparseProvidersMap(providers map[string]LLMProviderSettings) map[string]map[string]any {
	out := make(map[string]map[string]any, len(providers))
	for alias, p := range providers {
		m := make(map[string]any)
		if p.Kind != "" {
			m["kind"] = p.Kind
		}
		if p.AuthMode != "" {
			m["auth_mode"] = p.AuthMode
		}
		if p.BaseURL != "" {
			m["base_url"] = p.BaseURL
		}
		if p.APIKey != "" {
			m["api_key"] = p.APIKey
		}
		out[alias] = m
	}
	return out
}
