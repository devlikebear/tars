package config

import (
	"os"
	"path/filepath"
)

// TarsHomeDir returns the base directory for TARS data (~/.tars).
func TarsHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".tars")
	}
	return filepath.Join(home, ".tars")
}

// FixedConfigPath returns the fixed config file path (~/.tars/config/config.yaml).
// This path is not user-overridable; all commands use it.
func FixedConfigPath() string {
	return filepath.Join(TarsHomeDir(), "config", "config.yaml")
}

// DefaultWorkspaceDir returns the default workspace directory (~/.tars/workspace).
func DefaultWorkspaceDir() string {
	return filepath.Join(TarsHomeDir(), "workspace")
}

// Plan clarify mode constants — keep in sync with the values rendered by
// the Settings dropdown and the prompt branches in internal/prompt/builder.go.
const (
	PlanClarifyModeSmart = "smart"
	PlanClarifyModeAuto  = "auto"
	PlanClarifyModeAsk   = "ask"
)

const (
	defaultPlanClarifyMode                     = PlanClarifyModeSmart
	defaultSessionTelegramScope                = "main"
	defaultAPIAuthMode                         = "required"
	defaultDashboardAuthMode                   = "inherit"
	defaultAPIMaxInflightChat                  = 2
	defaultAPIMaxInflightAgentRuns             = 4
	defaultMemoryEmbedProvider                 = "gemini"
	defaultMemoryEmbedModel                    = "gemini-embedding-2-preview"
	defaultMemoryEmbedDimensions               = 768
	defaultUsageLimitDailyUSD                  = 10.0
	defaultUsageLimitWeeklyUSD                 = 50.0
	defaultUsageLimitMonthlyUSD                = 150.0
	defaultUsageLimitMode                      = "soft"
	defaultAgentMaxIterations                  = 8
	defaultCronRunHistoryLimit                 = 200
	defaultAssistantHotkey                     = "Ctrl+Option+Space"
	defaultAssistantWhisperBin                 = "whisper-cli"
	defaultAssistantFFmpegBin                  = "ffmpeg"
	defaultAssistantTTSBin                     = "say"
	defaultCompactionTriggerTokens             = 100000
	defaultCompactionKeepRecentTokens          = 12000
	defaultCompactionKeepRecentFraction        = 0.30
	defaultCompactionLLMMode                   = "auto"
	defaultCompactionLLMTimeoutSeconds         = 15
	defaultScheduleTimezone                    = "Asia/Seoul"
	defaultToolsDefaultSet                     = "standard"
	defaultToolsWebSearchProvider              = "brave"
	defaultPerplexityModel                     = "sonar"
	defaultPerplexityBaseURL                   = "https://api.perplexity.ai/chat/completions"
	defaultToolsWebSearchCacheTTLSeconds       = 60
	defaultAgentRuntimeWatchDebounceMS         = 200
	defaultAgentRuntimeRunsMaxRecords          = 2000
	defaultAgentRuntimeChannelsMaxMessages     = 500
	defaultAgentRuntimeSubagentsMaxThreads     = 4
	defaultAgentRuntimeSubagentsMaxDepth       = 1
	defaultAgentRuntimeConsensusMaxFanout      = 3
	defaultAgentRuntimeConsensusBudgetTokens   = 20000
	defaultAgentRuntimeConsensusBudgetUSD      = 0.50
	defaultAgentRuntimeConsensusTimeoutSecs    = 120
	defaultAgentRuntimeConsensusConcurrentRuns = 1
	defaultAgentRuntimeArchiveRetentionDays    = 30
	defaultAgentRuntimeArchiveMaxFileBytes     = 10485760
	defaultChannelsTelegramDMPolicy            = "pairing"
	defaultSkillsBundledDir                    = "./skills"
	defaultPluginsBundledDir                   = "./plugins"
	defaultOpenAIBaseURL                       = "https://api.openai.com/v1"
	defaultOpenAIModel                         = "gpt-4o-mini"
	defaultOpenAICodexBaseURL                  = "https://chatgpt.com/backend-api"
	defaultOpenAICodexModel                    = "gpt-5.3-codex"
	defaultClaudeCodeCLIModel                  = "sonnet"
	defaultGeminiBaseURL                       = "https://generativelanguage.googleapis.com/v1beta/openai"
	defaultGeminiNativeBaseURL                 = "https://generativelanguage.googleapis.com/v1beta"
	defaultGeminiModel                         = "gemini-2.5-flash"
	defaultAnthropicBaseURL                    = "https://api.anthropic.com"
	defaultAnthropicModel                      = "claude-haiku-4-5-20251001"
	defaultOpenAICodexOAuthProvider            = "openai-codex"
	defaultClaudeOAuthProvider                 = "claude-code"
	defaultGeminiOAuthProvider                 = "google-antigravity"
)

func defaultConfigValues() Config {
	return Config{
		RuntimeConfig: RuntimeConfig{
			WorkspaceDir:         DefaultWorkspaceDir(),
			SessionTelegramScope: defaultSessionTelegramScope,
			PlanClarifyMode:      defaultPlanClarifyMode,
		},
		APIConfig: APIConfig{
			APIAuthMode:             defaultAPIAuthMode,
			DashboardAuthMode:       defaultDashboardAuthMode,
			APIMaxInflightChat:      defaultAPIMaxInflightChat,
			APIMaxInflightAgentRuns: defaultAPIMaxInflightAgentRuns,
		},
		// LLMConfig: left empty here. Defaults live in applyLLMPoolDefaults
		// (base_url / api_key / auth_mode auto-fill per Kind) and the
		// checked-in config/default.yaml provides a minimal pool +
		// tiers so first-run works without any local config.
		LLMConfig: LLMConfig{},
		MemoryConfig: MemoryConfig{
			MemoryBackend:         "file",
			MemorySemanticEnabled: false,
			MemoryEmbedProvider:   defaultMemoryEmbedProvider,
			MemoryEmbedBaseURL:    defaultGeminiNativeBaseURL,
			MemoryEmbedModel:      defaultMemoryEmbedModel,
			MemoryEmbedDimensions: defaultMemoryEmbedDimensions,
		},
		UsageConfig: UsageConfig{
			UsageLimitDailyUSD:   defaultUsageLimitDailyUSD,
			UsageLimitWeeklyUSD:  defaultUsageLimitWeeklyUSD,
			UsageLimitMonthlyUSD: defaultUsageLimitMonthlyUSD,
			UsageLimitMode:       defaultUsageLimitMode,
		},
		AutomationConfig: AutomationConfig{
			AgentMaxIterations:  defaultAgentMaxIterations,
			CronRunHistoryLimit: defaultCronRunHistoryLimit,
			NotifyWhenNoClients: true,
			ScheduleTimezone:    defaultScheduleTimezone,

			// Pulse watchdog defaults — see internal/pulse for semantics.
			PulseEnabled:                    true,
			PulseInterval:                   "1m",
			PulseTimeout:                    "2m",
			PulseActiveHours:                "00:00-24:00",
			PulseTimezone:                   "Local",
			PulseMinSeverity:                "warn",
			PulseAllowedAutofixes:           []string{"compress_old_logs", "cleanup_stale_tmp"},
			PulseNotifyTelegram:             false,
			PulseNotifySessionEvents:        true,
			PulseCronFailureThreshold:       3,
			PulseStuckRunMinutes:            60,
			PulseDiskWarnPercent:            85,
			PulseDiskCriticalPercent:        95,
			PulseDeliveryFailureThreshold:   3,
			PulseDeliveryFailureWindow:      "10m",
			PulseReflectionFailureThreshold: 3,

			// Reflection nightly batch defaults — see internal/reflection.
			ReflectionEnabled:             true,
			ReflectionSleepWindow:         "02:00-05:00",
			ReflectionTimezone:            "Local",
			ReflectionTickInterval:        "5m",
			ReflectionEmptySessionAge:     "24h",
			ReflectionMemoryLookbackHours: 24,
			ReflectionMaxTurnsPerSession:  20,
		},
		AssistantConfig: AssistantConfig{
			AssistantEnabled:    true,
			AssistantHotkey:     defaultAssistantHotkey,
			AssistantWhisperBin: defaultAssistantWhisperBin,
			AssistantFFmpegBin:  defaultAssistantFFmpegBin,
			AssistantTTSBin:     defaultAssistantTTSBin,
		},
		CompactionConfig: CompactionConfig{
			CompactionTriggerTokens:      defaultCompactionTriggerTokens,
			CompactionKeepRecentTokens:   defaultCompactionKeepRecentTokens,
			CompactionKeepRecentFraction: defaultCompactionKeepRecentFraction,
			CompactionLLMMode:            defaultCompactionLLMMode,
			CompactionLLMTimeoutSeconds:  defaultCompactionLLMTimeoutSeconds,
		},
		ToolConfig: ToolConfig{
			ToolsDefaultSet:                 defaultToolsDefaultSet,
			ToolsWebSearchProvider:          defaultToolsWebSearchProvider,
			ToolsWebSearchPerplexityModel:   defaultPerplexityModel,
			ToolsWebSearchPerplexityBaseURL: defaultPerplexityBaseURL,
			ToolsWebSearchCacheTTLSeconds:   defaultToolsWebSearchCacheTTLSeconds,
		},
		AgentRuntimeConfig: AgentRuntimeConfig{
			AgentRuntimeAgentsWatch:                   true,
			AgentRuntimeAgentsWatchDebounceMS:         defaultAgentRuntimeWatchDebounceMS,
			AgentRuntimePersistenceEnabled:            true,
			AgentRuntimeRunsPersistenceEnabled:        true,
			AgentRuntimeChannelsPersistenceEnabled:    true,
			AgentRuntimeRunsMaxRecords:                defaultAgentRuntimeRunsMaxRecords,
			AgentRuntimeChannelsMaxMessagesPerChannel: defaultAgentRuntimeChannelsMaxMessages,
			AgentRuntimeSubagentsMaxThreads:           defaultAgentRuntimeSubagentsMaxThreads,
			AgentRuntimeSubagentsMaxDepth:             defaultAgentRuntimeSubagentsMaxDepth,
			AgentRuntimeConsensusEnabled:              false,
			AgentRuntimeConsensusMaxFanout:            defaultAgentRuntimeConsensusMaxFanout,
			AgentRuntimeConsensusBudgetTokens:         defaultAgentRuntimeConsensusBudgetTokens,
			AgentRuntimeConsensusBudgetUSD:            defaultAgentRuntimeConsensusBudgetUSD,
			AgentRuntimeConsensusTimeoutSeconds:       defaultAgentRuntimeConsensusTimeoutSecs,
			AgentRuntimeConsensusAllowedAliases:       []string{},
			AgentRuntimeConsensusConcurrentRuns:       defaultAgentRuntimeConsensusConcurrentRuns,
			AgentRuntimeRestoreOnStartup:              true,
			AgentRuntimeReportSummaryEnabled:          true,
			AgentRuntimeArchiveEnabled:                false,
			AgentRuntimeArchiveRetentionDays:          defaultAgentRuntimeArchiveRetentionDays,
			AgentRuntimeArchiveMaxFileBytes:           defaultAgentRuntimeArchiveMaxFileBytes,
		},
		ChannelConfig: ChannelConfig{
			ChannelsTelegramDMPolicy:       defaultChannelsTelegramDMPolicy,
			ChannelsTelegramPollingEnabled: true,
		},
		ExtensionConfig: ExtensionConfig{
			SkillsEnabled:          true,
			SkillsWatch:            true,
			SkillsWatchDebounceMS:  defaultAgentRuntimeWatchDebounceMS,
			SkillsBundledDir:       defaultSkillsBundledDir,
			PluginsEnabled:         true,
			PluginsWatch:           true,
			PluginsWatchDebounceMS: defaultAgentRuntimeWatchDebounceMS,
			PluginsBundledDir:      defaultPluginsBundledDir,
			MCPCommandAllowlist:    []string{},
		},
	}
}
