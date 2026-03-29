package config

// FieldMeta describes a single configuration field for UI rendering.
type FieldMeta struct {
	Key         string   `json:"key"`
	Section     string   `json:"section"`
	Type        string   `json:"type"` // "string", "int", "float", "bool", "json"
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Sensitive   bool     `json:"sensitive,omitempty"`
	Options     []string `json:"options,omitempty"`
}

func f(key, section, typ, label, desc string) FieldMeta {
	return FieldMeta{Key: key, Section: section, Type: typ, Label: label, Description: desc}
}

func fs(key, section, label, desc string, sensitive bool) FieldMeta {
	return FieldMeta{Key: key, Section: section, Type: "string", Label: label, Description: desc, Sensitive: sensitive}
}

func fsel(key, section, label, desc string, options []string) FieldMeta {
	return FieldMeta{Key: key, Section: section, Type: "select", Label: label, Description: desc, Options: options}
}

// Schema returns metadata for all configuration fields, grouped for UI display.
func Schema() []FieldMeta {
	return []FieldMeta{
		// ── Runtime ──────────────────────────────
		fsel("mode", "Runtime", "Mode", "Runtime mode", []string{"standalone", "server"}),
		f("workspace_dir", "Runtime", "string", "Workspace Directory", "Directory for workspace data and sessions"),
		f("session_default_id", "Runtime", "string", "Default Session ID", "Override the default session identifier"),
		fsel("session_telegram_scope", "Runtime", "Telegram Session Scope", "Session scoping for Telegram messages", []string{"main", "per-chat"}),
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
		fs("api_admin_token", "API", "Admin Token", "Admin-tier bearer token (control operations, gateway, config)", true),
		f("api_allow_insecure_local_auth", "API", "bool", "Allow Insecure Local Auth", "Allow loopback (127.0.0.1) requests without auth token"),
		f("api_max_inflight_chat", "API", "int", "Max Inflight Chat", "Maximum concurrent chat requests"),
		f("api_max_inflight_agent_runs", "API", "int", "Max Inflight Agent Runs", "Maximum concurrent agent run requests"),

		// ── LLM ──────────────────────────────────
		fsel("llm_provider", "LLM", "Provider", "LLM provider backend", []string{"anthropic", "openai", "openai-codex", "gemini", "bifrost"}),
		fsel("llm_auth_mode", "LLM", "Auth Mode", "LLM authentication mode", []string{"api-key", "oauth"}),
		fsel("llm_oauth_provider", "LLM", "OAuth Provider", "OAuth provider name when auth_mode is oauth", []string{"", "openai-codex"}),
		f("llm_base_url", "LLM", "string", "Base URL", "Custom base URL for the LLM API endpoint"),
		fs("llm_api_key", "LLM", "API Key", "API key for the LLM provider", true),
		f("llm_model", "LLM", "string", "Model", "Model identifier (e.g. claude-sonnet-4-20250514, gpt-4o)"),
		fsel("llm_reasoning_effort", "LLM", "Reasoning Effort", "Reasoning effort level", []string{"", "low", "medium", "high"}),
		f("llm_thinking_budget", "LLM", "int", "Thinking Budget", "Max tokens for extended thinking (0 = disabled)"),
		f("llm_service_tier", "LLM", "string", "Service Tier", "Service tier hint for the provider"),
		f("bifrost_base_url", "LLM", "string", "Bifrost Base URL", "Base URL for Bifrost gateway"),
		fs("bifrost_api_key", "LLM", "Bifrost API Key", "API key for Bifrost gateway", true),
		f("bifrost_model", "LLM", "string", "Bifrost Model", "Default model for Bifrost (e.g. openai/gpt-4o-mini)"),

		// ── Memory ───────────────────────────────
		f("memory_semantic_enabled", "Memory", "bool", "Semantic Memory", "Enable semantic memory with vector embeddings"),
		fsel("memory_embed_provider", "Memory", "Embed Provider", "Embedding provider", []string{"gemini", "openai"}),
		f("memory_embed_base_url", "Memory", "string", "Embed Base URL", "Base URL for embedding API"),
		fs("memory_embed_api_key", "Memory", "Embed API Key", "API key for the embedding provider", true),
		f("memory_embed_model", "Memory", "string", "Embed Model", "Embedding model identifier"),
		f("memory_embed_dimensions", "Memory", "int", "Embed Dimensions", "Vector dimensions for embeddings"),

		// ── Usage ────────────────────────────────
		f("usage_limit_daily_usd", "Usage", "float", "Daily Limit (USD)", "Maximum daily LLM spend in USD"),
		f("usage_limit_weekly_usd", "Usage", "float", "Weekly Limit (USD)", "Maximum weekly LLM spend in USD"),
		f("usage_limit_monthly_usd", "Usage", "float", "Monthly Limit (USD)", "Maximum monthly LLM spend in USD"),
		fsel("usage_limit_mode", "Usage", "Limit Mode", "Enforcement mode", []string{"soft", "hard"}),

		// ── Automation ───────────────────────────
		f("agent_max_iterations", "Automation", "int", "Max Iterations", "Maximum agent loop iterations per request"),
		f("heartbeat_active_hours", "Automation", "string", "Active Hours", "Heartbeat active hours range (e.g. 09:00-18:00)"),
		f("heartbeat_timezone", "Automation", "string", "Timezone", "Timezone for active hours evaluation"),
		f("cron_run_history_limit", "Automation", "int", "Cron History Limit", "Maximum run records kept per cron job"),
		f("notify_command", "Automation", "string", "Notify Command", "Shell command executed for notifications"),
		f("notify_when_no_clients", "Automation", "bool", "Notify When No Clients", "Send notifications even when no SSE clients connected"),
		f("schedule_timezone", "Automation", "string", "Schedule Timezone", "Default timezone for scheduled triggers"),

		// ── Assistant ────────────────────────────
		f("assistant_enabled", "Assistant", "bool", "Enabled", "Enable voice assistant feature"),
		f("assistant_hotkey", "Assistant", "string", "Hotkey", "Global hotkey to activate assistant"),
		f("assistant_whisper_bin", "Assistant", "string", "Whisper Binary", "Path to whisper CLI for speech-to-text"),
		f("assistant_ffmpeg_bin", "Assistant", "string", "FFmpeg Binary", "Path to ffmpeg for audio processing"),
		f("assistant_tts_bin", "Assistant", "string", "TTS Binary", "Path to text-to-speech binary"),

		// ── Tools ────────────────────────────────
		f("tools_web_search_enabled", "Tools", "bool", "Web Search", "Enable web search tool"),
		f("tools_web_fetch_enabled", "Tools", "bool", "Web Fetch", "Enable web fetch tool"),
		f("tools_allow_high_risk_user", "Tools", "bool", "Allow High-Risk Tools", "Allow user-level access to high-risk tools"),
		fs("tools_web_search_api_key", "Tools", "Search API Key", "API key for web search provider", true),
		fsel("tools_web_search_provider", "Tools", "Search Provider", "Web search backend", []string{"brave", "perplexity"}),
		f("tools_web_search_cache_ttl_seconds", "Tools", "int", "Search Cache TTL", "Cache duration for search results in seconds"),
		f("tools_web_fetch_allow_private_hosts", "Tools", "bool", "Allow Private Hosts", "Allow fetching from private/internal hosts"),
		f("tools_apply_patch_enabled", "Tools", "bool", "Apply Patch", "Enable apply-patch tool"),
		f("tools_message_enabled", "Tools", "bool", "Message Tool", "Enable message/notification tool"),
		f("tools_browser_enabled", "Tools", "bool", "Browser Tool", "Enable browser automation tool"),
		f("tools_nodes_enabled", "Tools", "bool", "Nodes Tool", "Enable sub-agent nodes tool"),
		f("tools_gateway_enabled", "Tools", "bool", "Gateway Tool", "Enable gateway dispatch tool"),

		// ── MCP ──────────────────────────────────
		f("mcp_command_allowlist_json", "MCP", "json", "Command Allowlist", "JSON array of allowed commands for MCP servers (e.g. [\"npx\",\"node\",\"uvx\",\"python3\"])"),

		// ── Vault ────────────────────────────────
		f("vault_enabled", "Vault", "bool", "Enabled", "Enable HashiCorp Vault integration"),
		f("vault_addr", "Vault", "string", "Address", "Vault server address"),
		fsel("vault_auth_mode", "Vault", "Auth Mode", "Vault auth method", []string{"token", "approle"}),
		fs("vault_token", "Vault", "Token", "Vault authentication token", true),
		f("vault_namespace", "Vault", "string", "Namespace", "Vault namespace for enterprise"),
		f("vault_timeout_ms", "Vault", "int", "Timeout (ms)", "Vault request timeout in milliseconds"),
		f("vault_kv_mount", "Vault", "string", "KV Mount", "KV secrets engine mount path"),
		f("vault_kv_version", "Vault", "int", "KV Version", "KV secrets engine version (1 or 2)"),

		// ── Browser ──────────────────────────────
		f("browser_runtime_enabled", "Browser", "bool", "Runtime Enabled", "Enable browser runtime for automation"),
		fsel("browser_default_profile", "Browser", "Default Profile", "Browser profile mode", []string{"managed", "system"}),
		f("browser_managed_headless", "Browser", "bool", "Headless Mode", "Run managed browser without GUI"),
		f("browser_managed_executable_path", "Browser", "string", "Executable Path", "Path to browser executable"),
		f("browser_managed_user_data_dir", "Browser", "string", "User Data Dir", "Browser user data directory"),
		f("browser_site_flows_dir", "Browser", "string", "Site Flows Dir", "Directory for browser site flow definitions"),

		// ── Gateway ──────────────────────────────
		f("gateway_enabled", "Gateway", "bool", "Enabled", "Enable agent gateway for multi-agent orchestration"),
		f("gateway_default_agent", "Gateway", "string", "Default Agent", "Default agent name for dispatched tasks"),
		f("gateway_agents_watch", "Gateway", "bool", "Watch Agent Files", "Auto-reload agents when definition files change"),
		f("gateway_persistence_enabled", "Gateway", "bool", "Persistence", "Enable gateway state persistence"),
		f("gateway_runs_persistence_enabled", "Gateway", "bool", "Runs Persistence", "Persist agent run records"),
		f("gateway_channels_persistence_enabled", "Gateway", "bool", "Channels Persistence", "Persist channel message history"),
		f("gateway_runs_max_records", "Gateway", "int", "Max Run Records", "Maximum stored run records"),
		f("gateway_channels_max_messages_per_channel", "Gateway", "int", "Max Messages/Channel", "Maximum messages retained per channel"),
		f("gateway_subagents_max_threads", "Gateway", "int", "Max Subagent Threads", "Maximum concurrent subagent threads"),
		f("gateway_subagents_max_depth", "Gateway", "int", "Max Subagent Depth", "Maximum subagent nesting depth"),
		f("gateway_restore_on_startup", "Gateway", "bool", "Restore on Startup", "Restore persisted runs when server starts"),
		f("gateway_archive_enabled", "Gateway", "bool", "Archive Enabled", "Enable run archival to disk"),
		f("gateway_archive_dir", "Gateway", "string", "Archive Dir", "Directory for archived run files"),
		f("gateway_archive_retention_days", "Gateway", "int", "Archive Retention (days)", "Days to retain archived runs"),

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
		f("skills_bundled_dir", "Extensions", "string", "Skills Directory", "Directory for bundled skill files"),
		f("plugins_enabled", "Extensions", "bool", "Plugins Enabled", "Load and serve plugin definitions"),
		f("plugins_watch", "Extensions", "bool", "Watch Plugins", "Auto-reload plugins when files change"),
		f("plugins_bundled_dir", "Extensions", "string", "Plugins Directory", "Directory for bundled plugin files"),
		f("plugins_allow_mcp_servers", "Extensions", "bool", "Allow MCP in Plugins", "Allow plugins to register MCP servers"),
	}
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
	case "mode":
		return cfg.Mode
	case "workspace_dir":
		return cfg.WorkspaceDir
	case "session_default_id":
		return cfg.SessionDefaultID
	case "session_telegram_scope":
		return cfg.SessionTelegramScope
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
	case "llm_provider":
		return cfg.LLMProvider
	case "llm_auth_mode":
		return cfg.LLMAuthMode
	case "llm_oauth_provider":
		return cfg.LLMOAuthProvider
	case "llm_base_url":
		return cfg.LLMBaseURL
	case "llm_api_key":
		return cfg.LLMAPIKey
	case "llm_model":
		return cfg.LLMModel
	case "llm_reasoning_effort":
		return cfg.LLMReasoningEffort
	case "llm_thinking_budget":
		return cfg.LLMThinkingBudget
	case "llm_service_tier":
		return cfg.LLMServiceTier
	case "bifrost_base_url":
		return cfg.BifrostBase
	case "bifrost_api_key":
		return cfg.BifrostAPIKey
	case "bifrost_model":
		return cfg.BifrostModel
	// Memory
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
	case "usage_limit_mode":
		return cfg.UsageLimitMode
	// Automation
	case "agent_max_iterations":
		return cfg.AgentMaxIterations
	case "heartbeat_active_hours":
		return cfg.HeartbeatActiveHours
	case "heartbeat_timezone":
		return cfg.HeartbeatTimezone
	case "cron_run_history_limit":
		return cfg.CronRunHistoryLimit
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
	// Tools
	case "tools_web_search_enabled":
		return cfg.ToolsWebSearchEnabled
	case "tools_web_fetch_enabled":
		return cfg.ToolsWebFetchEnabled
	case "tools_allow_high_risk_user":
		return cfg.ToolsAllowHighRiskUser
	case "tools_web_search_api_key":
		return cfg.ToolsWebSearchAPIKey
	case "tools_web_search_provider":
		return cfg.ToolsWebSearchProvider
	case "tools_web_search_cache_ttl_seconds":
		return cfg.ToolsWebSearchCacheTTLSeconds
	case "tools_web_fetch_allow_private_hosts":
		return cfg.ToolsWebFetchAllowPrivateHosts
	case "tools_apply_patch_enabled":
		return cfg.ToolsApplyPatchEnabled
	case "tools_message_enabled":
		return cfg.ToolsMessageEnabled
	case "tools_browser_enabled":
		return cfg.ToolsBrowserEnabled
	case "tools_nodes_enabled":
		return cfg.ToolsNodesEnabled
	case "tools_gateway_enabled":
		return cfg.ToolsGatewayEnabled
	// MCP
	case "mcp_command_allowlist_json":
		return cfg.MCPCommandAllowlist
	// Vault
	case "vault_enabled":
		return cfg.VaultEnabled
	case "vault_addr":
		return cfg.VaultAddr
	case "vault_auth_mode":
		return cfg.VaultAuthMode
	case "vault_token":
		return cfg.VaultToken
	case "vault_namespace":
		return cfg.VaultNamespace
	case "vault_timeout_ms":
		return cfg.VaultTimeoutMS
	case "vault_kv_mount":
		return cfg.VaultKVMount
	case "vault_kv_version":
		return cfg.VaultKVVersion
	// Browser
	case "browser_runtime_enabled":
		return cfg.BrowserRuntimeEnabled
	case "browser_default_profile":
		return cfg.BrowserDefaultProfile
	case "browser_managed_headless":
		return cfg.BrowserManagedHeadless
	case "browser_managed_executable_path":
		return cfg.BrowserManagedExecutablePath
	case "browser_managed_user_data_dir":
		return cfg.BrowserManagedUserDataDir
	case "browser_site_flows_dir":
		return cfg.BrowserSiteFlowsDir
	// Gateway
	case "gateway_enabled":
		return cfg.GatewayEnabled
	case "gateway_default_agent":
		return cfg.GatewayDefaultAgent
	case "gateway_agents_watch":
		return cfg.GatewayAgentsWatch
	case "gateway_persistence_enabled":
		return cfg.GatewayPersistenceEnabled
	case "gateway_runs_persistence_enabled":
		return cfg.GatewayRunsPersistenceEnabled
	case "gateway_channels_persistence_enabled":
		return cfg.GatewayChannelsPersistenceEnabled
	case "gateway_runs_max_records":
		return cfg.GatewayRunsMaxRecords
	case "gateway_channels_max_messages_per_channel":
		return cfg.GatewayChannelsMaxMessagesPerChannel
	case "gateway_subagents_max_threads":
		return cfg.GatewaySubagentsMaxThreads
	case "gateway_subagents_max_depth":
		return cfg.GatewaySubagentsMaxDepth
	case "gateway_restore_on_startup":
		return cfg.GatewayRestoreOnStartup
	case "gateway_archive_enabled":
		return cfg.GatewayArchiveEnabled
	case "gateway_archive_dir":
		return cfg.GatewayArchiveDir
	case "gateway_archive_retention_days":
		return cfg.GatewayArchiveRetentionDays
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
	case "skills_bundled_dir":
		return cfg.SkillsBundledDir
	case "plugins_enabled":
		return cfg.PluginsEnabled
	case "plugins_watch":
		return cfg.PluginsWatch
	case "plugins_bundled_dir":
		return cfg.PluginsBundledDir
	case "plugins_allow_mcp_servers":
		return cfg.PluginsAllowMCPServers
	default:
		return nil
	}
}
