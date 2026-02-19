package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

type MCPServer struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type GatewayAgent struct {
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	WorkingDir     string            `json:"working_dir,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	Enabled        bool              `json:"enabled,omitempty"`
}

// Config holds top-level runtime settings.
type Config struct {
	Mode                                 string
	WorkspaceDir                         string
	APIAuthMode                          string
	APIAuthToken                         string
	APIUserToken                         string
	APIAdminToken                        string
	APIWorkspaceHeader                   string
	APIUserWorkspaceIDs                  []string
	APIAdminWorkspaceIDs                 []string
	LLMProvider                          string
	LLMAuthMode                          string
	LLMOAuthProvider                     string
	LLMBaseURL                           string
	LLMAPIKey                            string
	LLMModel                             string
	AgentMaxIterations                   int
	HeartbeatActiveHours                 string
	HeartbeatTimezone                    string
	CronRunHistoryLimit                  int
	NotifyCommand                        string
	NotifyWhenNoClients                  bool
	BifrostBase                          string
	BifrostAPIKey                        string
	BifrostModel                         string
	ToolsWebSearchEnabled                bool
	ToolsWebFetchEnabled                 bool
	ToolsWebSearchAPIKey                 string
	ToolsWebSearchProvider               string
	ToolsWebSearchPerplexityAPIKey       string
	ToolsWebSearchPerplexityModel        string
	ToolsWebSearchPerplexityBaseURL      string
	ToolsWebSearchCacheTTLSeconds        int
	ToolsWebFetchPrivateHostAllowlist    []string
	ToolsWebFetchAllowPrivateHosts       bool
	ToolsApplyPatchEnabled               bool
	VaultEnabled                         bool
	VaultAddr                            string
	VaultAuthMode                        string
	VaultToken                           string
	VaultNamespace                       string
	VaultTimeoutMS                       int
	VaultKVMount                         string
	VaultKVVersion                       int
	VaultAppRoleMount                    string
	VaultAppRoleRoleID                   string
	VaultAppRoleSecretID                 string
	VaultSecretPathAllowlist             []string
	BrowserRuntimeEnabled                bool
	BrowserDefaultProfile                string
	BrowserManagedHeadless               bool
	BrowserManagedExecutablePath         string
	BrowserManagedUserDataDir            string
	BrowserRelayEnabled                  bool
	BrowserRelayAddr                     string
	BrowserRelayOriginAllowlist          []string
	BrowserSiteFlowsDir                  string
	BrowserAutoLoginSiteAllowlist        []string
	GatewayEnabled                       bool
	GatewayDefaultAgent                  string
	GatewayAgents                        []GatewayAgent
	GatewayAgentsWatch                   bool
	GatewayAgentsWatchDebounceMS         int
	GatewayPersistenceEnabled            bool
	GatewayRunsPersistenceEnabled        bool
	GatewayChannelsPersistenceEnabled    bool
	GatewayRunsMaxRecords                int
	GatewayChannelsMaxMessagesPerChannel int
	GatewayPersistenceDir                string
	GatewayRestoreOnStartup              bool
	GatewayReportSummaryEnabled          bool
	GatewayArchiveEnabled                bool
	GatewayArchiveDir                    string
	GatewayArchiveRetentionDays          int
	GatewayArchiveMaxFileBytes           int
	ChannelsLocalEnabled                 bool
	ChannelsWebhookEnabled               bool
	ChannelsTelegramEnabled              bool
	ToolsMessageEnabled                  bool
	ToolsBrowserEnabled                  bool
	ToolsNodesEnabled                    bool
	ToolsGatewayEnabled                  bool
	SkillsEnabled                        bool
	SkillsWatch                          bool
	SkillsWatchDebounceMS                int
	SkillsExtraDirs                      []string
	SkillsBundledDir                     string
	PluginsEnabled                       bool
	PluginsWatch                         bool
	PluginsWatchDebounceMS               int
	PluginsExtraDirs                     []string
	PluginsBundledDir                    string
	MCPServers                           []MCPServer
}

const DefaultTarsdConfigFilename = "config/standalone.yaml"

// Default returns safe baseline settings for local standalone execution.
func Default() Config {
	return Config{
		Mode:                                 "standalone",
		WorkspaceDir:                         "./workspace",
		APIAuthMode:                          "external-required",
		APIWorkspaceHeader:                   "Tars-Workspace-Id",
		LLMProvider:                          "bifrost",
		LLMAuthMode:                          "api-key",
		BifrostModel:                         "openai/gpt-4o-mini",
		AgentMaxIterations:                   8,
		CronRunHistoryLimit:                  200,
		NotifyWhenNoClients:                  true,
		ToolsWebSearchProvider:               "brave",
		ToolsWebSearchPerplexityModel:        "sonar",
		ToolsWebSearchPerplexityBaseURL:      "https://api.perplexity.ai/chat/completions",
		ToolsWebSearchCacheTTLSeconds:        60,
		VaultEnabled:                         false,
		VaultAddr:                            "http://127.0.0.1:8200",
		VaultAuthMode:                        "token",
		VaultTimeoutMS:                       1500,
		VaultKVMount:                         "secret",
		VaultKVVersion:                       2,
		VaultAppRoleMount:                    "approle",
		BrowserRuntimeEnabled:                true,
		BrowserDefaultProfile:                "managed",
		BrowserRelayEnabled:                  true,
		BrowserRelayAddr:                     "127.0.0.1:43182",
		BrowserRelayOriginAllowlist:          []string{"chrome-extension://*"},
		GatewayAgentsWatch:                   true,
		GatewayAgentsWatchDebounceMS:         200,
		GatewayPersistenceEnabled:            true,
		GatewayRunsPersistenceEnabled:        true,
		GatewayChannelsPersistenceEnabled:    true,
		GatewayRunsMaxRecords:                2000,
		GatewayChannelsMaxMessagesPerChannel: 500,
		GatewayRestoreOnStartup:              true,
		GatewayReportSummaryEnabled:          true,
		GatewayArchiveEnabled:                false,
		GatewayArchiveRetentionDays:          30,
		GatewayArchiveMaxFileBytes:           10485760,
		SkillsEnabled:                        true,
		SkillsWatch:                          true,
		SkillsWatchDebounceMS:                200,
		SkillsBundledDir:                     "./skills",
		PluginsEnabled:                       true,
		PluginsWatch:                         true,
		PluginsWatchDebounceMS:               200,
		PluginsBundledDir:                    "./plugins",
	}
}

// Load resolves runtime settings with the following precedence:
// defaults < YAML file < environment variables.
func Load(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		fileCfg, err := loadYAML(path)
		if err != nil {
			return Config{}, err
		}
		merge(&cfg, fileCfg)
	}

	applyEnv(&cfg)
	applyLLMDefaults(&cfg)
	return cfg, nil
}

func ResolveTarsdConfigPath(raw string) string {
	if v := strings.TrimSpace(raw); v != "" {
		return os.ExpandEnv(v)
	}
	if v := strings.TrimSpace(firstNonEmpty(os.Getenv("TARSD_CONFIG"), os.Getenv("TARSD_CONFIG_PATH"))); v != "" {
		return os.ExpandEnv(v)
	}
	if _, err := os.Stat(DefaultTarsdConfigFilename); err == nil {
		return DefaultTarsdConfigFilename
	}
	return ""
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("TARSD_MODE"); v != "" {
		cfg.Mode = v
	}
	if v := os.Getenv("TARSD_WORKSPACE_DIR"); v != "" {
		cfg.WorkspaceDir = v
	}
	if v := firstNonEmpty(os.Getenv("API_AUTH_MODE"), os.Getenv("TARSD_API_AUTH_MODE")); v != "" {
		cfg.APIAuthMode = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("API_AUTH_TOKEN"), os.Getenv("TARSD_API_AUTH_TOKEN")); v != "" {
		cfg.APIAuthToken = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("API_USER_TOKEN"), os.Getenv("TARSD_API_USER_TOKEN")); v != "" {
		cfg.APIUserToken = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("API_ADMIN_TOKEN"), os.Getenv("TARSD_API_ADMIN_TOKEN")); v != "" {
		cfg.APIAdminToken = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("API_WORKSPACE_HEADER"), os.Getenv("TARSD_API_WORKSPACE_HEADER")); v != "" {
		cfg.APIWorkspaceHeader = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("API_USER_WORKSPACE_IDS_JSON"), os.Getenv("TARSD_API_USER_WORKSPACE_IDS_JSON")); v != "" {
		cfg.APIUserWorkspaceIDs = parseJSONStringList(v, cfg.APIUserWorkspaceIDs)
	}
	if v := firstNonEmpty(os.Getenv("API_ADMIN_WORKSPACE_IDS_JSON"), os.Getenv("TARSD_API_ADMIN_WORKSPACE_IDS_JSON")); v != "" {
		cfg.APIAdminWorkspaceIDs = parseJSONStringList(v, cfg.APIAdminWorkspaceIDs)
	}
	if v := firstNonEmpty(os.Getenv("BIFROST_BASE_URL"), os.Getenv("TARSD_BIFROST_BASE_URL")); v != "" {
		cfg.BifrostBase = v
	}
	if v := firstNonEmpty(os.Getenv("BIFROST_API_KEY"), os.Getenv("TARSD_BIFROST_API_KEY")); v != "" {
		cfg.BifrostAPIKey = v
	}
	if v := firstNonEmpty(os.Getenv("BIFROST_MODEL"), os.Getenv("TARSD_BIFROST_MODEL")); v != "" {
		cfg.BifrostModel = v
	}
	if v := firstNonEmpty(os.Getenv("LLM_PROVIDER"), os.Getenv("TARSD_LLM_PROVIDER")); v != "" {
		cfg.LLMProvider = v
	}
	if v := firstNonEmpty(os.Getenv("LLM_AUTH_MODE"), os.Getenv("TARSD_LLM_AUTH_MODE")); v != "" {
		cfg.LLMAuthMode = v
	}
	if v := firstNonEmpty(os.Getenv("LLM_OAUTH_PROVIDER"), os.Getenv("TARSD_LLM_OAUTH_PROVIDER")); v != "" {
		cfg.LLMOAuthProvider = v
	}
	if v := firstNonEmpty(os.Getenv("LLM_BASE_URL"), os.Getenv("TARSD_LLM_BASE_URL")); v != "" {
		cfg.LLMBaseURL = v
	}
	if v := firstNonEmpty(os.Getenv("LLM_API_KEY"), os.Getenv("TARSD_LLM_API_KEY")); v != "" {
		cfg.LLMAPIKey = v
	}
	if v := firstNonEmpty(os.Getenv("LLM_MODEL"), os.Getenv("TARSD_LLM_MODEL")); v != "" {
		cfg.LLMModel = v
	}
	if v := firstNonEmpty(os.Getenv("AGENT_MAX_ITERATIONS"), os.Getenv("TARSD_AGENT_MAX_ITERATIONS")); v != "" {
		cfg.AgentMaxIterations = parsePositiveInt(v, cfg.AgentMaxIterations)
	}
	if v := firstNonEmpty(os.Getenv("HEARTBEAT_ACTIVE_HOURS"), os.Getenv("TARSD_HEARTBEAT_ACTIVE_HOURS")); v != "" {
		cfg.HeartbeatActiveHours = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("HEARTBEAT_TIMEZONE"), os.Getenv("TARSD_HEARTBEAT_TIMEZONE")); v != "" {
		cfg.HeartbeatTimezone = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("CRON_RUN_HISTORY_LIMIT"), os.Getenv("TARSD_CRON_RUN_HISTORY_LIMIT")); v != "" {
		cfg.CronRunHistoryLimit = parsePositiveInt(v, cfg.CronRunHistoryLimit)
	}
	if v := firstNonEmpty(os.Getenv("TARSD_NOTIFY_COMMAND"), os.Getenv("NOTIFY_COMMAND")); v != "" {
		cfg.NotifyCommand = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("TARSD_NOTIFY_WHEN_NO_CLIENTS"), os.Getenv("NOTIFY_WHEN_NO_CLIENTS")); v != "" {
		cfg.NotifyWhenNoClients = parseBool(v, cfg.NotifyWhenNoClients)
	}
	if v := firstNonEmpty(os.Getenv("MCP_SERVERS_JSON"), os.Getenv("TARSD_MCP_SERVERS_JSON")); v != "" {
		cfg.MCPServers = parseMCPServersJSON(v, cfg.MCPServers)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_SEARCH_ENABLED"), os.Getenv("TARSD_TOOLS_WEB_SEARCH_ENABLED")); v != "" {
		cfg.ToolsWebSearchEnabled = parseBool(v, cfg.ToolsWebSearchEnabled)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_FETCH_ENABLED"), os.Getenv("TARSD_TOOLS_WEB_FETCH_ENABLED")); v != "" {
		cfg.ToolsWebFetchEnabled = parseBool(v, cfg.ToolsWebFetchEnabled)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_SEARCH_API_KEY"), os.Getenv("TARSD_TOOLS_WEB_SEARCH_API_KEY")); v != "" {
		cfg.ToolsWebSearchAPIKey = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_SEARCH_PROVIDER"), os.Getenv("TARSD_TOOLS_WEB_SEARCH_PROVIDER")); v != "" {
		cfg.ToolsWebSearchProvider = strings.TrimSpace(strings.ToLower(v))
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_SEARCH_PERPLEXITY_API_KEY"), os.Getenv("TARSD_TOOLS_WEB_SEARCH_PERPLEXITY_API_KEY")); v != "" {
		cfg.ToolsWebSearchPerplexityAPIKey = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_SEARCH_PERPLEXITY_MODEL"), os.Getenv("TARSD_TOOLS_WEB_SEARCH_PERPLEXITY_MODEL")); v != "" {
		cfg.ToolsWebSearchPerplexityModel = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_SEARCH_PERPLEXITY_BASE_URL"), os.Getenv("TARSD_TOOLS_WEB_SEARCH_PERPLEXITY_BASE_URL")); v != "" {
		cfg.ToolsWebSearchPerplexityBaseURL = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_SEARCH_CACHE_TTL_SECONDS"), os.Getenv("TARSD_TOOLS_WEB_SEARCH_CACHE_TTL_SECONDS")); v != "" {
		cfg.ToolsWebSearchCacheTTLSeconds = parsePositiveInt(v, cfg.ToolsWebSearchCacheTTLSeconds)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_FETCH_PRIVATE_HOST_ALLOWLIST_JSON"), os.Getenv("TARSD_TOOLS_WEB_FETCH_PRIVATE_HOST_ALLOWLIST_JSON")); v != "" {
		cfg.ToolsWebFetchPrivateHostAllowlist = parseJSONStringList(v, cfg.ToolsWebFetchPrivateHostAllowlist)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_WEB_FETCH_ALLOW_PRIVATE_HOSTS"), os.Getenv("TARSD_TOOLS_WEB_FETCH_ALLOW_PRIVATE_HOSTS")); v != "" {
		cfg.ToolsWebFetchAllowPrivateHosts = parseBool(v, cfg.ToolsWebFetchAllowPrivateHosts)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_APPLY_PATCH_ENABLED"), os.Getenv("TARSD_TOOLS_APPLY_PATCH_ENABLED")); v != "" {
		cfg.ToolsApplyPatchEnabled = parseBool(v, cfg.ToolsApplyPatchEnabled)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_ENABLED"), os.Getenv("TARSD_VAULT_ENABLED")); v != "" {
		cfg.VaultEnabled = parseBool(v, cfg.VaultEnabled)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_ADDR"), os.Getenv("TARSD_VAULT_ADDR")); v != "" {
		cfg.VaultAddr = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_AUTH_MODE"), os.Getenv("TARSD_VAULT_AUTH_MODE")); v != "" {
		cfg.VaultAuthMode = strings.TrimSpace(strings.ToLower(v))
	}
	if v := firstNonEmpty(os.Getenv("VAULT_TOKEN"), os.Getenv("TARSD_VAULT_TOKEN")); v != "" {
		cfg.VaultToken = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_NAMESPACE"), os.Getenv("TARSD_VAULT_NAMESPACE")); v != "" {
		cfg.VaultNamespace = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_TIMEOUT_MS"), os.Getenv("TARSD_VAULT_TIMEOUT_MS")); v != "" {
		cfg.VaultTimeoutMS = parsePositiveInt(v, cfg.VaultTimeoutMS)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_KV_MOUNT"), os.Getenv("TARSD_VAULT_KV_MOUNT")); v != "" {
		cfg.VaultKVMount = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_KV_VERSION"), os.Getenv("TARSD_VAULT_KV_VERSION")); v != "" {
		cfg.VaultKVVersion = parsePositiveInt(v, cfg.VaultKVVersion)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_APPROLE_MOUNT"), os.Getenv("TARSD_VAULT_APPROLE_MOUNT")); v != "" {
		cfg.VaultAppRoleMount = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_APPROLE_ROLE_ID"), os.Getenv("TARSD_VAULT_APPROLE_ROLE_ID")); v != "" {
		cfg.VaultAppRoleRoleID = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_APPROLE_SECRET_ID"), os.Getenv("TARSD_VAULT_APPROLE_SECRET_ID")); v != "" {
		cfg.VaultAppRoleSecretID = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("VAULT_SECRET_PATH_ALLOWLIST_JSON"), os.Getenv("TARSD_VAULT_SECRET_PATH_ALLOWLIST_JSON")); v != "" {
		cfg.VaultSecretPathAllowlist = parseJSONStringList(v, cfg.VaultSecretPathAllowlist)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_RUNTIME_ENABLED"), os.Getenv("TARSD_BROWSER_RUNTIME_ENABLED")); v != "" {
		cfg.BrowserRuntimeEnabled = parseBool(v, cfg.BrowserRuntimeEnabled)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_DEFAULT_PROFILE"), os.Getenv("TARSD_BROWSER_DEFAULT_PROFILE")); v != "" {
		cfg.BrowserDefaultProfile = strings.TrimSpace(strings.ToLower(v))
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_MANAGED_HEADLESS"), os.Getenv("TARSD_BROWSER_MANAGED_HEADLESS")); v != "" {
		cfg.BrowserManagedHeadless = parseBool(v, cfg.BrowserManagedHeadless)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_MANAGED_EXECUTABLE_PATH"), os.Getenv("TARSD_BROWSER_MANAGED_EXECUTABLE_PATH")); v != "" {
		cfg.BrowserManagedExecutablePath = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_MANAGED_USER_DATA_DIR"), os.Getenv("TARSD_BROWSER_MANAGED_USER_DATA_DIR")); v != "" {
		cfg.BrowserManagedUserDataDir = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_RELAY_ENABLED"), os.Getenv("TARSD_BROWSER_RELAY_ENABLED")); v != "" {
		cfg.BrowserRelayEnabled = parseBool(v, cfg.BrowserRelayEnabled)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_RELAY_ADDR"), os.Getenv("TARSD_BROWSER_RELAY_ADDR")); v != "" {
		cfg.BrowserRelayAddr = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_RELAY_ORIGIN_ALLOWLIST_JSON"), os.Getenv("TARSD_BROWSER_RELAY_ORIGIN_ALLOWLIST_JSON")); v != "" {
		cfg.BrowserRelayOriginAllowlist = parseJSONStringList(v, cfg.BrowserRelayOriginAllowlist)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_SITE_FLOWS_DIR"), os.Getenv("TARSD_BROWSER_SITE_FLOWS_DIR")); v != "" {
		cfg.BrowserSiteFlowsDir = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("BROWSER_AUTO_LOGIN_SITE_ALLOWLIST_JSON"), os.Getenv("TARSD_BROWSER_AUTO_LOGIN_SITE_ALLOWLIST_JSON")); v != "" {
		cfg.BrowserAutoLoginSiteAllowlist = parseJSONStringList(v, cfg.BrowserAutoLoginSiteAllowlist)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_ENABLED"), os.Getenv("TARSD_GATEWAY_ENABLED")); v != "" {
		cfg.GatewayEnabled = parseBool(v, cfg.GatewayEnabled)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_DEFAULT_AGENT"), os.Getenv("TARSD_GATEWAY_DEFAULT_AGENT")); v != "" {
		cfg.GatewayDefaultAgent = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_AGENTS_JSON"), os.Getenv("TARSD_GATEWAY_AGENTS_JSON")); v != "" {
		cfg.GatewayAgents = parseGatewayAgentsJSON(v, cfg.GatewayAgents)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_AGENTS_WATCH"), os.Getenv("TARSD_GATEWAY_AGENTS_WATCH")); v != "" {
		cfg.GatewayAgentsWatch = parseBool(v, cfg.GatewayAgentsWatch)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_AGENTS_WATCH_DEBOUNCE_MS"), os.Getenv("TARSD_GATEWAY_AGENTS_WATCH_DEBOUNCE_MS")); v != "" {
		cfg.GatewayAgentsWatchDebounceMS = parsePositiveInt(v, cfg.GatewayAgentsWatchDebounceMS)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_PERSISTENCE_ENABLED"), os.Getenv("TARSD_GATEWAY_PERSISTENCE_ENABLED")); v != "" {
		cfg.GatewayPersistenceEnabled = parseBool(v, cfg.GatewayPersistenceEnabled)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_RUNS_PERSISTENCE_ENABLED"), os.Getenv("TARSD_GATEWAY_RUNS_PERSISTENCE_ENABLED")); v != "" {
		cfg.GatewayRunsPersistenceEnabled = parseBool(v, cfg.GatewayRunsPersistenceEnabled)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_CHANNELS_PERSISTENCE_ENABLED"), os.Getenv("TARSD_GATEWAY_CHANNELS_PERSISTENCE_ENABLED")); v != "" {
		cfg.GatewayChannelsPersistenceEnabled = parseBool(v, cfg.GatewayChannelsPersistenceEnabled)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_RUNS_MAX_RECORDS"), os.Getenv("TARSD_GATEWAY_RUNS_MAX_RECORDS")); v != "" {
		cfg.GatewayRunsMaxRecords = parsePositiveInt(v, cfg.GatewayRunsMaxRecords)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_CHANNELS_MAX_MESSAGES_PER_CHANNEL"), os.Getenv("TARSD_GATEWAY_CHANNELS_MAX_MESSAGES_PER_CHANNEL")); v != "" {
		cfg.GatewayChannelsMaxMessagesPerChannel = parsePositiveInt(v, cfg.GatewayChannelsMaxMessagesPerChannel)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_PERSISTENCE_DIR"), os.Getenv("TARSD_GATEWAY_PERSISTENCE_DIR")); v != "" {
		cfg.GatewayPersistenceDir = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_RESTORE_ON_STARTUP"), os.Getenv("TARSD_GATEWAY_RESTORE_ON_STARTUP")); v != "" {
		cfg.GatewayRestoreOnStartup = parseBool(v, cfg.GatewayRestoreOnStartup)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_REPORT_SUMMARY_ENABLED"), os.Getenv("TARSD_GATEWAY_REPORT_SUMMARY_ENABLED")); v != "" {
		cfg.GatewayReportSummaryEnabled = parseBool(v, cfg.GatewayReportSummaryEnabled)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_ARCHIVE_ENABLED"), os.Getenv("TARSD_GATEWAY_ARCHIVE_ENABLED")); v != "" {
		cfg.GatewayArchiveEnabled = parseBool(v, cfg.GatewayArchiveEnabled)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_ARCHIVE_DIR"), os.Getenv("TARSD_GATEWAY_ARCHIVE_DIR")); v != "" {
		cfg.GatewayArchiveDir = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_ARCHIVE_RETENTION_DAYS"), os.Getenv("TARSD_GATEWAY_ARCHIVE_RETENTION_DAYS")); v != "" {
		cfg.GatewayArchiveRetentionDays = parsePositiveInt(v, cfg.GatewayArchiveRetentionDays)
	}
	if v := firstNonEmpty(os.Getenv("GATEWAY_ARCHIVE_MAX_FILE_BYTES"), os.Getenv("TARSD_GATEWAY_ARCHIVE_MAX_FILE_BYTES")); v != "" {
		cfg.GatewayArchiveMaxFileBytes = parsePositiveInt(v, cfg.GatewayArchiveMaxFileBytes)
	}
	if v := firstNonEmpty(os.Getenv("CHANNELS_LOCAL_ENABLED"), os.Getenv("TARSD_CHANNELS_LOCAL_ENABLED")); v != "" {
		cfg.ChannelsLocalEnabled = parseBool(v, cfg.ChannelsLocalEnabled)
	}
	if v := firstNonEmpty(os.Getenv("CHANNELS_WEBHOOK_ENABLED"), os.Getenv("TARSD_CHANNELS_WEBHOOK_ENABLED")); v != "" {
		cfg.ChannelsWebhookEnabled = parseBool(v, cfg.ChannelsWebhookEnabled)
	}
	if v := firstNonEmpty(os.Getenv("CHANNELS_TELEGRAM_ENABLED"), os.Getenv("TARSD_CHANNELS_TELEGRAM_ENABLED")); v != "" {
		cfg.ChannelsTelegramEnabled = parseBool(v, cfg.ChannelsTelegramEnabled)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_MESSAGE_ENABLED"), os.Getenv("TARSD_TOOLS_MESSAGE_ENABLED")); v != "" {
		cfg.ToolsMessageEnabled = parseBool(v, cfg.ToolsMessageEnabled)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_BROWSER_ENABLED"), os.Getenv("TARSD_TOOLS_BROWSER_ENABLED")); v != "" {
		cfg.ToolsBrowserEnabled = parseBool(v, cfg.ToolsBrowserEnabled)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_NODES_ENABLED"), os.Getenv("TARSD_TOOLS_NODES_ENABLED")); v != "" {
		cfg.ToolsNodesEnabled = parseBool(v, cfg.ToolsNodesEnabled)
	}
	if v := firstNonEmpty(os.Getenv("TOOLS_GATEWAY_ENABLED"), os.Getenv("TARSD_TOOLS_GATEWAY_ENABLED")); v != "" {
		cfg.ToolsGatewayEnabled = parseBool(v, cfg.ToolsGatewayEnabled)
	}
	if v := firstNonEmpty(os.Getenv("SKILLS_ENABLED"), os.Getenv("TARSD_SKILLS_ENABLED")); v != "" {
		cfg.SkillsEnabled = parseBool(v, cfg.SkillsEnabled)
	}
	if v := firstNonEmpty(os.Getenv("SKILLS_WATCH"), os.Getenv("TARSD_SKILLS_WATCH")); v != "" {
		cfg.SkillsWatch = parseBool(v, cfg.SkillsWatch)
	}
	if v := firstNonEmpty(os.Getenv("SKILLS_WATCH_DEBOUNCE_MS"), os.Getenv("TARSD_SKILLS_WATCH_DEBOUNCE_MS")); v != "" {
		cfg.SkillsWatchDebounceMS = parsePositiveInt(v, cfg.SkillsWatchDebounceMS)
	}
	if v := firstNonEmpty(os.Getenv("SKILLS_EXTRA_DIRS_JSON"), os.Getenv("TARSD_SKILLS_EXTRA_DIRS_JSON")); v != "" {
		cfg.SkillsExtraDirs = parseJSONStringList(v, cfg.SkillsExtraDirs)
	}
	if v := firstNonEmpty(os.Getenv("SKILLS_BUNDLED_DIR"), os.Getenv("TARSD_SKILLS_BUNDLED_DIR")); v != "" {
		cfg.SkillsBundledDir = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("PLUGINS_ENABLED"), os.Getenv("TARSD_PLUGINS_ENABLED")); v != "" {
		cfg.PluginsEnabled = parseBool(v, cfg.PluginsEnabled)
	}
	if v := firstNonEmpty(os.Getenv("PLUGINS_WATCH"), os.Getenv("TARSD_PLUGINS_WATCH")); v != "" {
		cfg.PluginsWatch = parseBool(v, cfg.PluginsWatch)
	}
	if v := firstNonEmpty(os.Getenv("PLUGINS_WATCH_DEBOUNCE_MS"), os.Getenv("TARSD_PLUGINS_WATCH_DEBOUNCE_MS")); v != "" {
		cfg.PluginsWatchDebounceMS = parsePositiveInt(v, cfg.PluginsWatchDebounceMS)
	}
	if v := firstNonEmpty(os.Getenv("PLUGINS_EXTRA_DIRS_JSON"), os.Getenv("TARSD_PLUGINS_EXTRA_DIRS_JSON")); v != "" {
		cfg.PluginsExtraDirs = parseJSONStringList(v, cfg.PluginsExtraDirs)
	}
	if v := firstNonEmpty(os.Getenv("PLUGINS_BUNDLED_DIR"), os.Getenv("TARSD_PLUGINS_BUNDLED_DIR")); v != "" {
		cfg.PluginsBundledDir = strings.TrimSpace(v)
	}
}

func loadYAML(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config file %q: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Config{}, fmt.Errorf("invalid config format at line %d", lineNum)
		}
		key = strings.TrimSpace(key)
		value = cleanYAMLValue(value)

		switch key {
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
		case "api_workspace_header":
			cfg.APIWorkspaceHeader = strings.TrimSpace(value)
		case "api_user_workspace_ids_json":
			cfg.APIUserWorkspaceIDs = parseJSONStringList(value, cfg.APIUserWorkspaceIDs)
		case "api_admin_workspace_ids_json":
			cfg.APIAdminWorkspaceIDs = parseJSONStringList(value, cfg.APIAdminWorkspaceIDs)
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
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("read config file %q: %w", path, err)
	}

	return cfg, nil
}

func merge(dst *Config, src Config) {
	if src.Mode != "" {
		dst.Mode = src.Mode
	}
	if src.WorkspaceDir != "" {
		dst.WorkspaceDir = src.WorkspaceDir
	}
	if src.APIAuthMode != "" {
		dst.APIAuthMode = src.APIAuthMode
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
	if src.APIWorkspaceHeader != "" {
		dst.APIWorkspaceHeader = src.APIWorkspaceHeader
	}
	if len(src.APIUserWorkspaceIDs) > 0 {
		dst.APIUserWorkspaceIDs = append([]string(nil), src.APIUserWorkspaceIDs...)
	}
	if len(src.APIAdminWorkspaceIDs) > 0 {
		dst.APIAdminWorkspaceIDs = append([]string(nil), src.APIAdminWorkspaceIDs...)
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
	if len(src.MCPServers) > 0 {
		dst.MCPServers = src.MCPServers
	}
	if src.ToolsWebSearchEnabled {
		dst.ToolsWebSearchEnabled = true
	}
	if src.ToolsWebFetchEnabled {
		dst.ToolsWebFetchEnabled = true
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
}

func applyLLMDefaults(cfg *Config) {
	cfg.APIAuthMode = strings.TrimSpace(strings.ToLower(cfg.APIAuthMode))
	switch cfg.APIAuthMode {
	case "off", "external-required", "required":
	default:
		cfg.APIAuthMode = "external-required"
	}
	cfg.APIAuthToken = strings.TrimSpace(cfg.APIAuthToken)
	cfg.APIUserToken = strings.TrimSpace(cfg.APIUserToken)
	cfg.APIAdminToken = strings.TrimSpace(cfg.APIAdminToken)
	cfg.APIWorkspaceHeader = strings.TrimSpace(cfg.APIWorkspaceHeader)
	if cfg.APIWorkspaceHeader == "" {
		cfg.APIWorkspaceHeader = "Tars-Workspace-Id"
	}

	cfg.LLMProvider = strings.TrimSpace(strings.ToLower(cfg.LLMProvider))
	if cfg.LLMProvider == "" {
		cfg.LLMProvider = "bifrost"
	}
	cfg.LLMAuthMode = strings.TrimSpace(strings.ToLower(cfg.LLMAuthMode))
	if cfg.LLMAuthMode == "" {
		cfg.LLMAuthMode = "api-key"
	}
	cfg.LLMOAuthProvider = strings.TrimSpace(strings.ToLower(cfg.LLMOAuthProvider))
	if cfg.LLMAuthMode == "oauth" && cfg.LLMOAuthProvider == "" {
		switch cfg.LLMProvider {
		case "anthropic":
			cfg.LLMOAuthProvider = "claude-code"
		case "gemini", "gemini-native":
			cfg.LLMOAuthProvider = "google-antigravity"
		}
	}
	if cfg.AgentMaxIterations <= 0 {
		cfg.AgentMaxIterations = 8
	}
	if cfg.CronRunHistoryLimit <= 0 {
		cfg.CronRunHistoryLimit = 200
	}
	cfg.ToolsWebSearchProvider = strings.TrimSpace(strings.ToLower(cfg.ToolsWebSearchProvider))
	if cfg.ToolsWebSearchProvider == "" {
		cfg.ToolsWebSearchProvider = "brave"
	}
	if cfg.ToolsWebSearchPerplexityModel == "" {
		cfg.ToolsWebSearchPerplexityModel = "sonar"
	}
	if cfg.ToolsWebSearchPerplexityBaseURL == "" {
		cfg.ToolsWebSearchPerplexityBaseURL = "https://api.perplexity.ai/chat/completions"
	}
	if cfg.ToolsWebSearchCacheTTLSeconds <= 0 {
		cfg.ToolsWebSearchCacheTTLSeconds = 60
	}
	cfg.VaultAuthMode = strings.TrimSpace(strings.ToLower(cfg.VaultAuthMode))
	if cfg.VaultAuthMode == "" {
		cfg.VaultAuthMode = "token"
	}
	if cfg.VaultAddr == "" {
		cfg.VaultAddr = "http://127.0.0.1:8200"
	}
	if cfg.VaultTimeoutMS <= 0 {
		cfg.VaultTimeoutMS = 1500
	}
	if cfg.VaultKVMount == "" {
		cfg.VaultKVMount = "secret"
	}
	if cfg.VaultKVVersion <= 0 {
		cfg.VaultKVVersion = 2
	}
	if cfg.VaultAppRoleMount == "" {
		cfg.VaultAppRoleMount = "approle"
	}
	cfg.BrowserDefaultProfile = strings.TrimSpace(strings.ToLower(cfg.BrowserDefaultProfile))
	if cfg.BrowserDefaultProfile == "" {
		cfg.BrowserDefaultProfile = "managed"
	}
	if cfg.BrowserRelayAddr == "" {
		cfg.BrowserRelayAddr = "127.0.0.1:43182"
	}
	if len(cfg.BrowserRelayOriginAllowlist) == 0 {
		cfg.BrowserRelayOriginAllowlist = []string{"chrome-extension://*"}
	}
	if strings.TrimSpace(cfg.BrowserSiteFlowsDir) == "" {
		cfg.BrowserSiteFlowsDir = filepath.Join(strings.TrimSpace(cfg.WorkspaceDir), "automation", "sites")
	}
	if strings.TrimSpace(cfg.BrowserManagedUserDataDir) == "" {
		cfg.BrowserManagedUserDataDir = filepath.Join(strings.TrimSpace(cfg.WorkspaceDir), "_shared", "browser", "managed")
	}
	if cfg.GatewayAgentsWatchDebounceMS <= 0 {
		cfg.GatewayAgentsWatchDebounceMS = 200
	}
	if cfg.GatewayRunsMaxRecords <= 0 {
		cfg.GatewayRunsMaxRecords = 2000
	}
	if cfg.GatewayChannelsMaxMessagesPerChannel <= 0 {
		cfg.GatewayChannelsMaxMessagesPerChannel = 500
	}
	if strings.TrimSpace(cfg.GatewayPersistenceDir) == "" {
		cfg.GatewayPersistenceDir = filepath.Join(strings.TrimSpace(cfg.WorkspaceDir), "_shared", "gateway")
	}
	if cfg.GatewayArchiveRetentionDays <= 0 {
		cfg.GatewayArchiveRetentionDays = 30
	}
	if cfg.GatewayArchiveMaxFileBytes <= 0 {
		cfg.GatewayArchiveMaxFileBytes = 10485760
	}
	if strings.TrimSpace(cfg.GatewayArchiveDir) == "" {
		cfg.GatewayArchiveDir = filepath.Join(strings.TrimSpace(cfg.WorkspaceDir), "_shared", "gateway", "archive")
	}
	if cfg.LLMBaseURL == "" || cfg.LLMModel == "" || cfg.LLMAPIKey == "" {
		switch cfg.LLMProvider {
		case "bifrost":
			if cfg.LLMBaseURL == "" {
				cfg.LLMBaseURL = cfg.BifrostBase
			}
			if cfg.LLMModel == "" {
				cfg.LLMModel = cfg.BifrostModel
			}
			if cfg.LLMAPIKey == "" {
				cfg.LLMAPIKey = cfg.BifrostAPIKey
			}
		case "openai":
			if cfg.LLMBaseURL == "" {
				cfg.LLMBaseURL = "https://api.openai.com/v1"
			}
			if cfg.LLMModel == "" {
				cfg.LLMModel = "gpt-4o-mini"
			}
			if cfg.LLMAPIKey == "" {
				cfg.LLMAPIKey = os.Getenv("OPENAI_API_KEY")
			}
		case "gemini":
			if cfg.LLMBaseURL == "" {
				cfg.LLMBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
			}
			if cfg.LLMModel == "" {
				cfg.LLMModel = "gemini-2.5-flash"
			}
			if cfg.LLMAPIKey == "" {
				cfg.LLMAPIKey = os.Getenv("GEMINI_API_KEY")
			}
		case "gemini-native":
			if cfg.LLMBaseURL == "" {
				cfg.LLMBaseURL = "https://generativelanguage.googleapis.com/v1beta"
			}
			if cfg.LLMModel == "" {
				cfg.LLMModel = "gemini-2.5-flash"
			}
			if cfg.LLMAPIKey == "" {
				cfg.LLMAPIKey = os.Getenv("GEMINI_API_KEY")
			}
		case "anthropic":
			if cfg.LLMBaseURL == "" {
				cfg.LLMBaseURL = "https://api.anthropic.com"
			}
			if cfg.LLMModel == "" {
				cfg.LLMModel = "claude-3-5-haiku-latest"
			}
			if cfg.LLMAPIKey == "" {
				cfg.LLMAPIKey = os.Getenv("ANTHROPIC_API_KEY")
			}
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func cleanYAMLValue(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	noComment := stripYAMLInlineComment(trimmed)
	return os.ExpandEnv(strings.Trim(strings.TrimSpace(noComment), `"'`))
}

func stripYAMLInlineComment(raw string) string {
	if raw == "" {
		return raw
	}
	var b strings.Builder
	b.Grow(len(raw))
	inSingle := false
	inDouble := false
	escaped := false
	prevSpace := true
	for _, r := range raw {
		if inDouble {
			b.WriteRune(r)
			if escaped {
				escaped = false
				prevSpace = unicode.IsSpace(r)
				continue
			}
			if r == '\\' {
				escaped = true
				prevSpace = false
				continue
			}
			if r == '"' {
				inDouble = false
			}
			prevSpace = unicode.IsSpace(r)
			continue
		}
		if inSingle {
			b.WriteRune(r)
			if r == '\'' {
				inSingle = false
			}
			prevSpace = unicode.IsSpace(r)
			continue
		}
		if r == '#' && prevSpace {
			break
		}
		b.WriteRune(r)
		if r == '"' {
			inDouble = true
		} else if r == '\'' {
			inSingle = true
		}
		prevSpace = unicode.IsSpace(r)
	}
	return strings.TrimRightFunc(b.String(), unicode.IsSpace)
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseBool(value string, fallback bool) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func parseMCPServersJSON(raw string, fallback []MCPServer) []MCPServer {
	var parsed []MCPServer
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		return fallback
	}
	out := make([]MCPServer, 0, len(parsed))
	for _, server := range parsed {
		name := strings.TrimSpace(server.Name)
		command := strings.TrimSpace(server.Command)
		if name == "" || command == "" {
			continue
		}
		s := MCPServer{
			Name:    name,
			Command: command,
			Args:    append([]string(nil), server.Args...),
		}
		if len(server.Env) > 0 {
			s.Env = make(map[string]string, len(server.Env))
			for k, v := range server.Env {
				s.Env[k] = v
			}
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func parseCSVList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func parseJSONStringList(raw string, fallback []string) []string {
	var parsed []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		return fallback
	}
	out := make([]string, 0, len(parsed))
	for _, item := range parsed {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func parseGatewayAgentsJSON(raw string, fallback []GatewayAgent) []GatewayAgent {
	type rawGatewayAgent struct {
		Name           string            `json:"name"`
		Description    string            `json:"description,omitempty"`
		Command        string            `json:"command"`
		Args           []string          `json:"args,omitempty"`
		Env            map[string]string `json:"env,omitempty"`
		WorkingDir     string            `json:"working_dir,omitempty"`
		TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
		Enabled        *bool             `json:"enabled,omitempty"`
	}
	var parsed []rawGatewayAgent
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		return fallback
	}
	out := make([]GatewayAgent, 0, len(parsed))
	for _, agent := range parsed {
		name := strings.TrimSpace(agent.Name)
		command := strings.TrimSpace(agent.Command)
		if name == "" || command == "" {
			continue
		}
		item := GatewayAgent{
			Name:           name,
			Description:    strings.TrimSpace(agent.Description),
			Command:        command,
			Args:           append([]string(nil), agent.Args...),
			WorkingDir:     strings.TrimSpace(agent.WorkingDir),
			TimeoutSeconds: agent.TimeoutSeconds,
			Enabled:        true,
		}
		if agent.Enabled != nil {
			item.Enabled = *agent.Enabled
		}
		if len(agent.Env) > 0 {
			item.Env = make(map[string]string, len(agent.Env))
			for k, v := range agent.Env {
				item.Env[k] = v
			}
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
