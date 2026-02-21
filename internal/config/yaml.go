package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func loadYAML(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config file %q: %w", path, err)
	}

	parsed := map[string]any{}
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return Config{}, fmt.Errorf("parse config file %q: %w", path, err)
	}

	var cfg Config
	for key, rawValue := range parsed {
		value := yamlValueString(rawValue)
		switch strings.TrimSpace(strings.ToLower(key)) {
		case "mode":
			cfg.Mode = value
		case "workspace_dir":
			cfg.WorkspaceDir = value
		case "api_auth_mode":
			cfg.APIAuthMode = strings.TrimSpace(value)
		case "api_auth_token":
			cfg.APIAuthToken = strings.TrimSpace(value)
		case "api_user_token":
			cfg.APIUserToken = strings.TrimSpace(value)
		case "api_admin_token":
			cfg.APIAdminToken = strings.TrimSpace(value)
		case "bifrost_base_url":
			cfg.BifrostBase = value
		case "bifrost_api_key":
			cfg.BifrostAPIKey = value
		case "bifrost_model":
			cfg.BifrostModel = value
		case "llm_provider":
			cfg.LLMProvider = value
		case "llm_auth_mode":
			cfg.LLMAuthMode = value
		case "llm_oauth_provider":
			cfg.LLMOAuthProvider = value
		case "llm_base_url":
			cfg.LLMBaseURL = value
		case "llm_api_key":
			cfg.LLMAPIKey = value
		case "llm_model":
			cfg.LLMModel = value
		case "agent_max_iterations":
			cfg.AgentMaxIterations = parsePositiveInt(value, cfg.AgentMaxIterations)
		case "heartbeat_active_hours":
			cfg.HeartbeatActiveHours = strings.TrimSpace(value)
		case "heartbeat_timezone":
			cfg.HeartbeatTimezone = strings.TrimSpace(value)
		case "cron_run_history_limit":
			cfg.CronRunHistoryLimit = parsePositiveInt(value, cfg.CronRunHistoryLimit)
		case "notify_command":
			cfg.NotifyCommand = strings.TrimSpace(value)
		case "mcp_servers_json":
			cfg.MCPServers = parseMCPServersJSON(value, cfg.MCPServers)
		case "tools_web_search_enabled":
			cfg.ToolsWebSearchEnabled = parseBool(value, cfg.ToolsWebSearchEnabled)
		case "tools_web_fetch_enabled":
			cfg.ToolsWebFetchEnabled = parseBool(value, cfg.ToolsWebFetchEnabled)
		case "tools_web_search_api_key":
			cfg.ToolsWebSearchAPIKey = strings.TrimSpace(value)
		case "tools_web_search_provider":
			cfg.ToolsWebSearchProvider = strings.TrimSpace(strings.ToLower(value))
		case "tools_web_search_perplexity_api_key":
			cfg.ToolsWebSearchPerplexityAPIKey = strings.TrimSpace(value)
		case "tools_web_search_perplexity_model":
			cfg.ToolsWebSearchPerplexityModel = strings.TrimSpace(value)
		case "tools_web_search_perplexity_base_url":
			cfg.ToolsWebSearchPerplexityBaseURL = strings.TrimSpace(value)
		case "tools_web_search_cache_ttl_seconds":
			cfg.ToolsWebSearchCacheTTLSeconds = parsePositiveInt(value, cfg.ToolsWebSearchCacheTTLSeconds)
		case "tools_web_fetch_private_host_allowlist_json":
			cfg.ToolsWebFetchPrivateHostAllowlist = parseJSONStringList(value, cfg.ToolsWebFetchPrivateHostAllowlist)
		case "tools_web_fetch_allow_private_hosts":
			cfg.ToolsWebFetchAllowPrivateHosts = parseBool(value, cfg.ToolsWebFetchAllowPrivateHosts)
		case "tools_apply_patch_enabled":
			cfg.ToolsApplyPatchEnabled = parseBool(value, cfg.ToolsApplyPatchEnabled)
		case "vault_enabled":
			cfg.VaultEnabled = parseBool(value, cfg.VaultEnabled)
		case "vault_addr":
			cfg.VaultAddr = strings.TrimSpace(value)
		case "vault_auth_mode":
			cfg.VaultAuthMode = strings.TrimSpace(strings.ToLower(value))
		case "vault_token":
			cfg.VaultToken = strings.TrimSpace(value)
		case "vault_namespace":
			cfg.VaultNamespace = strings.TrimSpace(value)
		case "vault_timeout_ms":
			cfg.VaultTimeoutMS = parsePositiveInt(value, cfg.VaultTimeoutMS)
		case "vault_kv_mount":
			cfg.VaultKVMount = strings.TrimSpace(value)
		case "vault_kv_version":
			cfg.VaultKVVersion = parsePositiveInt(value, cfg.VaultKVVersion)
		case "vault_approle_mount":
			cfg.VaultAppRoleMount = strings.TrimSpace(value)
		case "vault_approle_role_id":
			cfg.VaultAppRoleRoleID = strings.TrimSpace(value)
		case "vault_approle_secret_id":
			cfg.VaultAppRoleSecretID = strings.TrimSpace(value)
		case "vault_secret_path_allowlist_json":
			cfg.VaultSecretPathAllowlist = parseJSONStringList(value, cfg.VaultSecretPathAllowlist)
		case "browser_runtime_enabled":
			cfg.BrowserRuntimeEnabled = parseBool(value, cfg.BrowserRuntimeEnabled)
		case "browser_default_profile":
			cfg.BrowserDefaultProfile = strings.TrimSpace(strings.ToLower(value))
		case "browser_managed_headless":
			cfg.BrowserManagedHeadless = parseBool(value, cfg.BrowserManagedHeadless)
		case "browser_managed_executable_path":
			cfg.BrowserManagedExecutablePath = strings.TrimSpace(value)
		case "browser_managed_user_data_dir":
			cfg.BrowserManagedUserDataDir = strings.TrimSpace(value)
		case "browser_relay_enabled":
			cfg.BrowserRelayEnabled = parseBool(value, cfg.BrowserRelayEnabled)
		case "browser_relay_addr":
			cfg.BrowserRelayAddr = strings.TrimSpace(value)
		case "browser_relay_origin_allowlist_json":
			cfg.BrowserRelayOriginAllowlist = parseJSONStringList(value, cfg.BrowserRelayOriginAllowlist)
		case "browser_site_flows_dir":
			cfg.BrowserSiteFlowsDir = strings.TrimSpace(value)
		case "browser_auto_login_site_allowlist_json":
			cfg.BrowserAutoLoginSiteAllowlist = parseJSONStringList(value, cfg.BrowserAutoLoginSiteAllowlist)
		case "gateway_enabled":
			cfg.GatewayEnabled = parseBool(value, cfg.GatewayEnabled)
		case "gateway_default_agent":
			cfg.GatewayDefaultAgent = strings.TrimSpace(value)
		case "gateway_agents_json":
			cfg.GatewayAgents = parseGatewayAgentsJSON(value, cfg.GatewayAgents)
		case "gateway_agents_watch":
			cfg.GatewayAgentsWatch = parseBool(value, cfg.GatewayAgentsWatch)
		case "gateway_agents_watch_debounce_ms":
			cfg.GatewayAgentsWatchDebounceMS = parsePositiveInt(value, cfg.GatewayAgentsWatchDebounceMS)
		case "gateway_persistence_enabled":
			cfg.GatewayPersistenceEnabled = parseBool(value, cfg.GatewayPersistenceEnabled)
		case "gateway_runs_persistence_enabled":
			cfg.GatewayRunsPersistenceEnabled = parseBool(value, cfg.GatewayRunsPersistenceEnabled)
		case "gateway_channels_persistence_enabled":
			cfg.GatewayChannelsPersistenceEnabled = parseBool(value, cfg.GatewayChannelsPersistenceEnabled)
		case "gateway_runs_max_records":
			cfg.GatewayRunsMaxRecords = parsePositiveInt(value, cfg.GatewayRunsMaxRecords)
		case "gateway_channels_max_messages_per_channel":
			cfg.GatewayChannelsMaxMessagesPerChannel = parsePositiveInt(value, cfg.GatewayChannelsMaxMessagesPerChannel)
		case "gateway_persistence_dir":
			cfg.GatewayPersistenceDir = strings.TrimSpace(value)
		case "gateway_restore_on_startup":
			cfg.GatewayRestoreOnStartup = parseBool(value, cfg.GatewayRestoreOnStartup)
		case "gateway_report_summary_enabled":
			cfg.GatewayReportSummaryEnabled = parseBool(value, cfg.GatewayReportSummaryEnabled)
		case "gateway_archive_enabled":
			cfg.GatewayArchiveEnabled = parseBool(value, cfg.GatewayArchiveEnabled)
		case "gateway_archive_dir":
			cfg.GatewayArchiveDir = strings.TrimSpace(value)
		case "gateway_archive_retention_days":
			cfg.GatewayArchiveRetentionDays = parsePositiveInt(value, cfg.GatewayArchiveRetentionDays)
		case "gateway_archive_max_file_bytes":
			cfg.GatewayArchiveMaxFileBytes = parsePositiveInt(value, cfg.GatewayArchiveMaxFileBytes)
		case "channels_local_enabled":
			cfg.ChannelsLocalEnabled = parseBool(value, cfg.ChannelsLocalEnabled)
		case "channels_webhook_enabled":
			cfg.ChannelsWebhookEnabled = parseBool(value, cfg.ChannelsWebhookEnabled)
		case "channels_telegram_enabled":
			cfg.ChannelsTelegramEnabled = parseBool(value, cfg.ChannelsTelegramEnabled)
		case "channels_telegram_dm_policy":
			cfg.ChannelsTelegramDMPolicy = strings.TrimSpace(strings.ToLower(value))
		case "channels_telegram_polling_enabled":
			cfg.ChannelsTelegramPollingEnabled = parseBool(value, cfg.ChannelsTelegramPollingEnabled)
		case "telegram_bot_token":
			cfg.TelegramBotToken = strings.TrimSpace(value)
		case "tools_message_enabled":
			cfg.ToolsMessageEnabled = parseBool(value, cfg.ToolsMessageEnabled)
		case "tools_browser_enabled":
			cfg.ToolsBrowserEnabled = parseBool(value, cfg.ToolsBrowserEnabled)
		case "tools_nodes_enabled":
			cfg.ToolsNodesEnabled = parseBool(value, cfg.ToolsNodesEnabled)
		case "tools_gateway_enabled":
			cfg.ToolsGatewayEnabled = parseBool(value, cfg.ToolsGatewayEnabled)
		case "skills_enabled":
			cfg.SkillsEnabled = parseBool(value, cfg.SkillsEnabled)
		case "skills_watch":
			cfg.SkillsWatch = parseBool(value, cfg.SkillsWatch)
		case "skills_watch_debounce_ms":
			cfg.SkillsWatchDebounceMS = parsePositiveInt(value, cfg.SkillsWatchDebounceMS)
		case "skills_extra_dirs_json":
			cfg.SkillsExtraDirs = parseJSONStringList(value, cfg.SkillsExtraDirs)
		case "skills_bundled_dir":
			cfg.SkillsBundledDir = strings.TrimSpace(value)
		case "plugins_enabled":
			cfg.PluginsEnabled = parseBool(value, cfg.PluginsEnabled)
		case "plugins_watch":
			cfg.PluginsWatch = parseBool(value, cfg.PluginsWatch)
		case "plugins_watch_debounce_ms":
			cfg.PluginsWatchDebounceMS = parsePositiveInt(value, cfg.PluginsWatchDebounceMS)
		case "plugins_extra_dirs_json":
			cfg.PluginsExtraDirs = parseJSONStringList(value, cfg.PluginsExtraDirs)
		case "plugins_bundled_dir":
			cfg.PluginsBundledDir = strings.TrimSpace(value)
		}
	}
	return cfg, nil
}

func yamlValueString(raw any) string {
	switch value := raw.(type) {
	case nil:
		return ""
	case string:
		return os.ExpandEnv(strings.TrimSpace(value))
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return strings.TrimSpace(fmt.Sprint(value))
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return strings.TrimSpace(fmt.Sprint(value))
		}
		return strings.TrimSpace(string(encoded))
	}
}
