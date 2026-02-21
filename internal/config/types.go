package config

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

const DefaultConfigFilename = "config/standalone.yaml"

// Default returns safe baseline settings for local standalone execution.
func Default() Config {
	return Config{
		Mode:                                 "standalone",
		WorkspaceDir:                         "./workspace",
		APIAuthMode:                          "external-required",
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
