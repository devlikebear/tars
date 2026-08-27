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

type RemoteAccessConfig struct {
	RemoteAccessTailscaleServeEnabled   bool
	RemoteAccessTailscaleServeHTTPSPort int
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

	// ClaudeCodeCLIPermissionMode selects the value passed to
	// `claude -p --permission-mode` when the active tier uses the
	// claude-code-cli provider. Allowed values: "auto" (default), "default",
	// "acceptEdits", "plan", "dontAsk", "bypassPermissions".
	// Empty/unknown values degrade to "auto" inside the provider. Other
	// providers ignore this.
	ClaudeCodeCLIPermissionMode string
}

// LLMProviderSettings is one entry in the named provider pool. It holds
// "where to call + how to authenticate" but NOT "what model to call".
// Models are bound at the tier level (LLMTierBinding.Model) so that one
// provider can serve multiple models.
//
// Kind identifies the provider type ("anthropic", "openai", "openai-codex",
// "gemini", "gemini-native", "kimi", "claude-code-cli",
// "antigravity-cli") and maps
// to the value passed to llm.NewProvider.Provider. The config package does not
// validate Kind against a closed list — llm.NewProvider returns a clear
// error for unknown kinds, keeping the config package free of an
// internal/llm import.
//
// OAuth token source (formerly the user-facing oauth_provider field) is
// derived internally from Kind via llmdefaults.OAuthProvider — there is
// no per-config override. ServiceTier is set per tier binding only
// (LLMTierBinding.ServiceTier) — there is no provider-level default.
type LLMProviderSettings struct {
	Kind     string `json:"kind"      yaml:"kind"`
	AuthMode string `json:"auth_mode" yaml:"auth_mode"`
	BaseURL  string `json:"base_url"  yaml:"base_url"`
	APIKey   string `json:"api_key"   yaml:"api_key"`
}

// LLMTierBinding binds a tier to a provider alias + concrete model +
// per-call knobs. Provider must be a key in cfg.LLMProviders — the
// resolver rejects unknown aliases with a loud error.
//
// ReasoningEffort, ThinkingBudget, ServiceTier, MaxTokens, and BetaFeatures
// are per-tier knobs; they are not configurable at the provider level.
//
// MaxTokens caps the response length. Zero means "unset" — the provider
// applies a per-model default (llmdefaults.MaxOutputTokens) and falls back
// to a conservative floor for models it does not recognize. Set it
// explicitly on gateway-hosted models, which never match that table.
//
// BetaFeatures opts the tier into provider beta features. The values are
// passed through verbatim (Anthropic joins them into one anthropic-beta
// header) and are deliberately not validated against a closed list, so a
// newly announced flag needs no TARS release. The field is named
// provider-agnostically: other providers may adopt the same mechanism.
// Empty means no beta header is sent at all, which is what a third-party
// Anthropic-compatible gateway wants.
type LLMTierBinding struct {
	Provider        string   `json:"provider"         yaml:"provider"`
	Model           string   `json:"model"            yaml:"model"`
	ReasoningEffort string   `json:"reasoning_effort" yaml:"reasoning_effort"`
	ThinkingBudget  int      `json:"thinking_budget"  yaml:"thinking_budget"`
	ServiceTier     string   `json:"service_tier"     yaml:"service_tier"`
	MaxTokens       int      `json:"max_tokens"       yaml:"max_tokens"`
	BetaFeatures    []string `json:"beta_features"    yaml:"beta_features"`
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
	// ToolsExecMaxTimeoutMS caps the per-call timeout the LLM can pass to
	// the exec tool. Long-running commands (`make build`, `gh pr checks
	// --watch`, `npm install`) need more than the historical 30s default;
	// raise this to give them headroom while still preventing infinite
	// hangs. 0 falls back to defaults.go.
	ToolsExecMaxTimeoutMS int
	// ToolsProcessMaxTimeoutMS caps background (process-managed)
	// commands started via `exec background:true`, plus the per-call
	// timeout for the `process` tool's `wait` action. Independent of the
	// foreground cap so watchers like `gh pr checks --watch` can run for
	// tens of minutes. 0 falls back to defaults.go.
	ToolsProcessMaxTimeoutMS int
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

// WorkLedgerConfig controls the additive durable work projection. The
// unexported presence bit lets an explicit YAML false override the enabled
// default without changing the legacy bool-field merge behavior.
type WorkLedgerConfig struct {
	Enabled                                 bool
	SchedulerEnabled                        bool
	SchedulerMaxWorkers                     int
	SchedulerLeaseSeconds                   int
	SchedulerHeartbeatSeconds               int
	SchedulerPollMilliseconds               int
	SchedulerExecutionEnvironment           string
	SchedulerExecutionDataDir               string
	SchedulerArtifactPaths                  []string
	SchedulerExternalHarnessConfigPath      string
	SchedulerRemoteWorkersEnabled           bool
	SchedulerRemoteWorkersGatewayConfigPath string
	SchedulerA2AEnabled                     bool
	SchedulerA2ADiscoveryURL                string
	SchedulerA2ABearerToken                 string
	SchedulerA2AAllowedHosts                []string
	SchedulerA2AAllowPrivateHosts           bool
	SchedulerA2AAllowInsecureLoopback       bool
	SchedulerA2APollMilliseconds            int
	SchedulerA2AMaxPollSeconds              int
	enabledSet                              bool
	schedulerEnabledSet                     bool
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

type CompanionConfig struct {
	Enabled    bool
	enabledSet bool
}

type EmbodimentConfig struct {
	Enabled   bool
	Providers []EmbodimentProviderConfig
}

type EmbodimentProviderConfig struct {
	Name                  string   `json:"name" yaml:"name"`
	Enabled               bool     `json:"enabled" yaml:"enabled"`
	Transport             string   `json:"transport" yaml:"transport"`
	Endpoint              string   `json:"endpoint" yaml:"endpoint"`
	Capabilities          []string `json:"capabilities" yaml:"capabilities"`
	SessionID             string   `json:"session_id" yaml:"session_id"`
	Agent                 string   `json:"agent" yaml:"agent"`
	OwnerOnlyDirective    bool     `json:"owner_only_directive" yaml:"owner_only_directive"`
	SalienceMinSoundLevel float64  `json:"salience_min_sound_level" yaml:"salience_min_sound_level"`
	MinTriggerInterval    string   `json:"min_trigger_interval" yaml:"min_trigger_interval"`
	MaxTriggersPerHour    int      `json:"max_triggers_per_hour" yaml:"max_triggers_per_hour"`
	TriggerObservations   bool     `json:"trigger_observations" yaml:"trigger_observations"`
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
	RemoteAccessConfig
	LLMConfig
	MemoryConfig
	UsageConfig
	AutomationConfig
	AssistantConfig
	CompactionConfig
	ToolConfig
	AgentRuntimeConfig
	WorkLedger WorkLedgerConfig
	ChannelConfig
	Companion  CompanionConfig
	Embodiment EmbodimentConfig
	ExtensionConfig
}

const DefaultConfigFilename = "config/default.yaml"

// Default returns safe baseline settings for local execution.
func Default() Config {
	return defaultConfigValues()
}
