package config

type MCPServer struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type UsagePrice struct {
	InputPer1MUSD      float64 `json:"input_per_1m_usd"`
	OutputPer1MUSD     float64 `json:"output_per_1m_usd"`
	CacheReadPer1MUSD  float64 `json:"cache_read_per_1m_usd,omitempty"`
	CacheWritePer1MUSD float64 `json:"cache_write_per_1m_usd,omitempty"`
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
	SessionDefaultID                     string
	SessionTelegramScope                 string
	APIAuthMode                          string
	APIAuthToken                         string
	APIUserToken                         string
	APIAdminToken                        string
	APIAllowInsecureLocalAuth            bool
	APIMaxInflightChat                   int
	APIMaxInflightAgentRuns              int
	LLMProvider                          string
	LLMAuthMode                          string
	LLMOAuthProvider                     string
	LLMBaseURL                           string
	LLMAPIKey                            string
	LLMModel                             string
	LLMReasoningEffort                   string
	LLMThinkingBudget                    int
	LLMServiceTier                       string
	UsageLimitDailyUSD                   float64
	UsageLimitWeeklyUSD                  float64
	UsageLimitMonthlyUSD                 float64
	UsageLimitMode                       string
	UsagePriceOverrides                  map[string]UsagePrice
	AgentMaxIterations                   int
	HeartbeatActiveHours                 string
	HeartbeatTimezone                    string
	CronRunHistoryLimit                  int
	NotifyCommand                        string
	NotifyWhenNoClients                  bool
	AssistantEnabled                     bool
	AssistantHotkey                      string
	AssistantWhisperBin                  string
	AssistantFFmpegBin                   string
	AssistantTTSBin                      string
	ScheduleTimezone                     string
	BifrostBase                          string
	BifrostAPIKey                        string
	BifrostModel                         string
	ToolsWebSearchEnabled                bool
	ToolsWebFetchEnabled                 bool
	ToolsDefaultSet                      string
	ToolsAllowHighRiskUser               bool
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
	BrowserRelayToken                    string
	BrowserRelayAllowQueryToken          bool
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
	ChannelsTelegramDMPolicy             string
	ChannelsTelegramPollingEnabled       bool
	TelegramBotToken                     string
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
	PluginsAllowMCPServers               bool
	MCPServers                           []MCPServer
	MCPCommandAllowlist                  []string
}

const DefaultConfigFilename = "config/standalone.yaml"

// Default returns safe baseline settings for local standalone execution.
func Default() Config {
	return defaultConfigValues()
}
