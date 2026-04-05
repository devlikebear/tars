package config

type MCPServer struct {
	Name          string            `json:"name"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Transport     string            `json:"transport,omitempty"`
	URL           string            `json:"url,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	AuthMode      string            `json:"auth_mode,omitempty"`
	AuthTokenEnv  string            `json:"auth_token_env,omitempty"`
	OAuthProvider string            `json:"oauth_provider,omitempty"`
	Source        string            `json:"source,omitempty"`
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

type RuntimeConfig struct {
	Mode                 string
	WorkspaceDir         string
	SessionDefaultID     string
	SessionTelegramScope string
	LogLevel             string
	LogFile              string
	LogRotateMaxSizeMB   int
	LogRotateMaxDays     int
	LogRotateMaxBackups  int
}

type APIConfig struct {
	APIAuthMode               string
	DashboardAuthMode         string
	APIAuthToken              string
	APIUserToken              string
	APIAdminToken             string
	APIAllowInsecureLocalAuth bool
	APIMaxInflightChat        int
	APIMaxInflightAgentRuns   int
}

type LLMConfig struct {
	LLMProvider        string
	LLMAuthMode        string
	LLMOAuthProvider   string
	LLMBaseURL         string
	LLMAPIKey          string
	LLMModel           string
	LLMReasoningEffort string
	LLMThinkingBudget  int
	LLMServiceTier     string
}

type MemoryConfig struct {
	MemorySemanticEnabled bool
	MemoryEmbedProvider   string
	MemoryEmbedBaseURL    string
	MemoryEmbedAPIKey     string
	MemoryEmbedModel      string
	MemoryEmbedDimensions int
}

type UsageConfig struct {
	UsageLimitDailyUSD   float64
	UsageLimitWeeklyUSD  float64
	UsageLimitMonthlyUSD float64
	UsageLimitMode       string
	UsagePriceOverrides  map[string]UsagePrice
}

type AutomationConfig struct {
	AgentMaxIterations   int
	HeartbeatInterval    string
	HeartbeatActiveHours string
	HeartbeatTimezone    string
	CronRunHistoryLimit  int
	NotifyCommand        string
	NotifyWhenNoClients  bool
	ScheduleTimezone     string

	// Pulse is the system-surface watchdog. All fields default to
	// conservative values so it runs silently until signals appear.
	PulseEnabled                  bool
	PulseInterval                 string // duration string, e.g. "1m"
	PulseTimeout                  string // duration string, e.g. "2m"
	PulseActiveHours              string
	PulseTimezone                 string
	PulseMinSeverity              string
	PulseAllowedAutofixes         []string
	PulseNotifyTelegram           bool
	PulseNotifySessionEvents      bool
	PulseCronFailureThreshold     int
	PulseStuckRunMinutes          int
	PulseDiskWarnPercent          float64
	PulseDiskCriticalPercent      float64
	PulseDeliveryFailureThreshold int
	PulseDeliveryFailureWindow    string // duration string, e.g. "10m"
}

type AssistantConfig struct {
	AssistantEnabled    bool
	AssistantHotkey     string
	AssistantWhisperBin string
	AssistantFFmpegBin  string
	AssistantTTSBin     string
}

type ToolConfig struct {
	ToolsWebSearchEnabled             bool
	ToolsWebFetchEnabled              bool
	ToolsDefaultSet                   string
	ToolsAllowHighRiskUser            bool
	ToolsWebSearchAPIKey              string
	ToolsWebSearchProvider            string
	ToolsWebSearchPerplexityAPIKey    string
	ToolsWebSearchPerplexityModel     string
	ToolsWebSearchPerplexityBaseURL   string
	ToolsWebSearchCacheTTLSeconds     int
	ToolsWebFetchPrivateHostAllowlist []string
	ToolsWebFetchAllowPrivateHosts    bool
	ToolsApplyPatchEnabled            bool
	ToolsMessageEnabled               bool
	ToolsBrowserEnabled               bool
	ToolsNodesEnabled                 bool
	ToolsGatewayEnabled               bool
}

type VaultConfig struct {
	VaultEnabled             bool
	VaultAddr                string
	VaultAuthMode            string
	VaultToken               string
	VaultNamespace           string
	VaultTimeoutMS           int
	VaultKVMount             string
	VaultKVVersion           int
	VaultAppRoleMount        string
	VaultAppRoleRoleID       string
	VaultAppRoleSecretID     string
	VaultSecretPathAllowlist []string
}

type BrowserConfig struct {
	BrowserRuntimeEnabled         bool
	BrowserDefaultProfile         string
	BrowserManagedHeadless        bool
	BrowserManagedExecutablePath  string
	BrowserManagedUserDataDir     string
	BrowserSiteFlowsDir           string
	BrowserAutoLoginSiteAllowlist []string
}

type GatewayConfig struct {
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
	GatewaySubagentsMaxThreads           int
	GatewaySubagentsMaxDepth             int
	GatewayPersistenceDir                string
	GatewayRestoreOnStartup              bool
	GatewayReportSummaryEnabled          bool
	GatewayArchiveEnabled                bool
	GatewayArchiveDir                    string
	GatewayArchiveRetentionDays          int
	GatewayArchiveMaxFileBytes           int
}

type ChannelConfig struct {
	ChannelsLocalEnabled           bool
	ChannelsWebhookEnabled         bool
	ChannelsTelegramEnabled        bool
	ChannelsTelegramDMPolicy       string
	ChannelsTelegramPollingEnabled bool
	TelegramBotToken               string
}

type ExtensionConfig struct {
	SkillsEnabled          bool
	SkillsWatch            bool
	SkillsWatchDebounceMS  int
	SkillsExtraDirs        []string
	SkillsBundledDir       string
	PluginsEnabled         bool
	PluginsWatch           bool
	PluginsWatchDebounceMS int
	PluginsExtraDirs       []string
	PluginsBundledDir      string
	PluginsAllowMCPServers bool
	MCPServers             []MCPServer
	MCPCommandAllowlist    []string
}

// Config holds top-level runtime settings grouped by concern.
type Config struct {
	RuntimeConfig
	APIConfig
	LLMConfig
	MemoryConfig
	UsageConfig
	AutomationConfig
	AssistantConfig
	ToolConfig
	VaultConfig
	BrowserConfig
	GatewayConfig
	ChannelConfig
	ExtensionConfig
}

const DefaultConfigFilename = "config/standalone.yaml"

// Default returns safe baseline settings for local standalone execution.
func Default() Config {
	return defaultConfigValues()
}
