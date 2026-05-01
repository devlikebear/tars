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

type AgentRuntimeAgent struct {
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
	WorkspaceDir           string
	SessionDefaultID       string
	SessionTelegramScope   string
	StyleDirectnessDefault int
	StyleHumorDefault      int
	StyleCautionDefault    int
	StyleAutonomyDefault   int
	LogLevel               string
	LogFile                string
	LogRotateMaxSizeMB     int
	LogRotateMaxDays       int
	LogRotateMaxBackups    int
	// PlanClarifyMode controls whether the LLM asks clarifying questions
	// before drafting a plan. One of "smart" (default), "auto", or "ask".
	// See internal/prompt/builder.go for behavior per mode.
	PlanClarifyMode string
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

// LLMConfig holds the named provider pool + tier bindings that together
// describe every LLM endpoint TARS will call. See
// docs/plans/llm-provider-pool.md for the schema rationale.
//
// Legacy flat fields (LLMProvider, LLMAuthMode, ..., LLMTierHeavy/Standard/
// Light) were removed in the cutover commit — user configs must migrate
// to llm_providers + llm_tiers.
type LLMConfig struct {
	// LLMProviders is the named provider pool. Each entry describes
	// "where to call + how to authenticate" — credentials, base URL,
	// auth mode. It does NOT carry a model; models are bound at the
	// tier level via LLMTierBinding. One provider can therefore serve
	// multiple models by being referenced from multiple tiers.
	LLMProviders map[string]LLMProviderSettings

	// LLMTiers binds each tier name (typically "heavy"/"standard"/"light")
	// to a provider alias (key in LLMProviders) + a concrete model + the
	// optional per-call knobs. A tier's binding.Provider must exist in
	// LLMProviders or resolution errors.
	LLMTiers map[string]LLMTierBinding

	// LLMDefaultTier is the tier used when a role has no explicit
	// mapping in LLMRoleDefaults. Must be a key in LLMTiers.
	LLMDefaultTier string

	// LLMRoleDefaults maps a canonical role name (e.g. "chat_main",
	// "pulse_decider") to a tier name ("heavy"|"standard"|"light"). Roles
	// absent from the map fall back to LLMDefaultTier. Role names are
	// validated at router build time via llm.ParseRole — this package
	// does not import internal/llm.
	LLMRoleDefaults map[string]string
}

// LLMProviderSettings is one entry in the named provider pool. It holds
// "where to call + how to authenticate" but NOT "what model to call".
// Models are bound at the tier level (LLMTierBinding.Model) so that one
// provider can serve multiple models.
//
// Kind identifies the provider type ("anthropic", "openai", "openai-codex",
// "gemini", "gemini-native", "kimi", "claude-code-cli") and maps to the value
// passed to llm.NewProvider.Provider. The config package does not
// validate Kind against a closed list — llm.NewProvider returns a clear
// error for unknown kinds, keeping the config package free of an
// internal/llm import.
//
// ServiceTier is the provider-level default applied when a tier binding
// does not set its own. Tier-level ServiceTier (LLMTierBinding) takes
// precedence when non-empty; see ResolveLLMTier for the merge order.
type LLMProviderSettings struct {
	Kind          string `json:"kind"           yaml:"kind"`
	AuthMode      string `json:"auth_mode"      yaml:"auth_mode"`
	OAuthProvider string `json:"oauth_provider" yaml:"oauth_provider"`
	BaseURL       string `json:"base_url"       yaml:"base_url"`
	APIKey        string `json:"api_key"        yaml:"api_key"`
	ServiceTier   string `json:"service_tier"   yaml:"service_tier"`
}

// LLMTierBinding binds a tier to a provider alias + concrete model +
// per-call knobs. Provider must be a key in cfg.LLMProviders — the
// resolver rejects unknown aliases with a loud error.
//
// ServiceTier here overrides the provider-level default when non-empty.
// ReasoningEffort and ThinkingBudget are pure per-tier values (providers
// do not set them at the pool level).
type LLMTierBinding struct {
	Provider        string `json:"provider"         yaml:"provider"`
	Model           string `json:"model"            yaml:"model"`
	ReasoningEffort string `json:"reasoning_effort" yaml:"reasoning_effort"`
	ThinkingBudget  int    `json:"thinking_budget"  yaml:"thinking_budget"`
	ServiceTier     string `json:"service_tier"     yaml:"service_tier"`
}

type MemoryConfig struct {
	MemoryBackend         string
	MemorySemanticEnabled bool
	MemoryEmbedProvider   string
	MemoryEmbedBaseURL    string
	MemoryEmbedAPIKey     string
	MemoryEmbedModel      string
	MemoryEmbedDimensions int
}

type UsageConfig struct {
	UsageLimitDailyUSD    float64
	UsageLimitWeeklyUSD   float64
	UsageLimitMonthlyUSD  float64
	UsageDailyTokenBudget int
	UsageLimitMode        string
	UsagePriceOverrides   map[string]UsagePrice
}

type AutomationConfig struct {
	AgentMaxIterations  int
	CronRunHistoryLimit int
	NotifyCommand       string
	NotifyWhenNoClients bool
	ScheduleTimezone    string

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
	// PulseReflectionFailureThreshold is the number of consecutive
	// reflection run failures that causes pulse to emit a reflection-
	// failure signal.
	PulseReflectionFailureThreshold int

	// Reflection is the nightly batch runner (memory + KB cleanup).
	ReflectionEnabled             bool
	ReflectionSleepWindow         string // "HH:MM-HH:MM" in ReflectionTimezone
	ReflectionTimezone            string
	ReflectionTickInterval        string // duration string, e.g. "5m"
	ReflectionEmptySessionAge     string // duration string, e.g. "24h"
	ReflectionMemoryLookbackHours int
	ReflectionMaxTurnsPerSession  int
}

type AssistantConfig struct {
	AssistantEnabled    bool
	AssistantHotkey     string
	AssistantWhisperBin string
	AssistantFFmpegBin  string
	AssistantTTSBin     string
}

type CompactionConfig struct {
	CompactionTriggerTokens      int
	CompactionKeepRecentTokens   int
	CompactionKeepRecentFraction float64
	CompactionLLMMode            string
	CompactionLLMTimeoutSeconds  int
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
	ToolsAgentRuntimeEnabled          bool
}

type AgentRuntimeConfig struct {
	AgentRuntimeEnabled                       bool
	AgentRuntimeDefaultAgent                  string
	AgentRuntimeAgents                        []AgentRuntimeAgent
	AgentRuntimeTaskOverride                  AgentRuntimeTaskOverrideConfig
	AgentRuntimeAgentsWatch                   bool
	AgentRuntimeAgentsWatchDebounceMS         int
	AgentRuntimePersistenceEnabled            bool
	AgentRuntimeRunsPersistenceEnabled        bool
	AgentRuntimeChannelsPersistenceEnabled    bool
	AgentRuntimeRunsMaxRecords                int
	AgentRuntimeChannelsMaxMessagesPerChannel int
	AgentRuntimeSubagentsMaxThreads           int
	AgentRuntimeSubagentsMaxDepth             int
	AgentRuntimeConsensusEnabled              bool
	AgentRuntimeConsensusMaxFanout            int
	AgentRuntimeConsensusBudgetTokens         int
	AgentRuntimeConsensusBudgetUSD            float64
	AgentRuntimeConsensusTimeoutSeconds       int
	AgentRuntimeConsensusAllowedAliases       []string
	AgentRuntimeConsensusConcurrentRuns       int
	AgentRuntimePersistenceDir                string
	AgentRuntimeRestoreOnStartup              bool
	AgentRuntimeReportSummaryEnabled          bool
	AgentRuntimeArchiveEnabled                bool
	AgentRuntimeArchiveDir                    string
	AgentRuntimeArchiveRetentionDays          int
	AgentRuntimeArchiveMaxFileBytes           int
}

type AgentRuntimeTaskOverrideConfig struct {
	Enabled        bool     `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	AllowedAliases []string `json:"allowed_aliases,omitempty" yaml:"allowed_aliases,omitempty"`
	AllowedModels  []string `json:"allowed_models,omitempty" yaml:"allowed_models,omitempty"`
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
	CompactionConfig
	ToolConfig
	AgentRuntimeConfig
	ChannelConfig
	ExtensionConfig
}

const DefaultConfigFilename = "config/default.yaml"

// Default returns safe baseline settings for local execution.
func Default() Config {
	return defaultConfigValues()
}
