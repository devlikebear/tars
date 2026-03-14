package config

import (
	"os"
	"path/filepath"
	"strings"
)

func applyLLMDefaults(cfg *Config) {
	applyDefaults(cfg)
}

func applyDefaults(cfg *Config) {
	if cfg == nil {
		return
	}
	defaults := defaultConfigValues()
	applyCoreDefaults(cfg, defaults)
	applyToolDefaults(cfg, defaults)
	applyVaultDefaults(cfg, defaults)
	applyBrowserDefaults(cfg, defaults)
	applyGatewayDefaults(cfg, defaults)
	applyProviderDefaults(cfg, defaults)
}

func applyCoreDefaults(cfg *Config, defaults Config) {
	if strings.TrimSpace(cfg.Mode) == "" {
		cfg.Mode = defaults.Mode
	}
	if strings.TrimSpace(cfg.WorkspaceDir) == "" {
		cfg.WorkspaceDir = defaults.WorkspaceDir
	}
	cfg.APIAuthMode = strings.TrimSpace(strings.ToLower(cfg.APIAuthMode))
	switch cfg.APIAuthMode {
	case "off", "external-required", "required":
	default:
		cfg.APIAuthMode = defaults.APIAuthMode
	}
	cfg.APIAuthToken = strings.TrimSpace(cfg.APIAuthToken)
	cfg.APIUserToken = strings.TrimSpace(cfg.APIUserToken)
	cfg.APIAdminToken = strings.TrimSpace(cfg.APIAdminToken)
	if cfg.APIMaxInflightChat <= 0 {
		cfg.APIMaxInflightChat = defaults.APIMaxInflightChat
	}
	if cfg.APIMaxInflightAgentRuns <= 0 {
		cfg.APIMaxInflightAgentRuns = defaults.APIMaxInflightAgentRuns
	}
	cfg.SessionDefaultID = strings.TrimSpace(cfg.SessionDefaultID)
	cfg.SessionTelegramScope = strings.TrimSpace(strings.ToLower(cfg.SessionTelegramScope))
	switch cfg.SessionTelegramScope {
	case "main", "per-user":
	default:
		cfg.SessionTelegramScope = defaults.SessionTelegramScope
	}
	cfg.LLMProvider = strings.TrimSpace(strings.ToLower(cfg.LLMProvider))
	if cfg.LLMProvider == "" {
		cfg.LLMProvider = defaults.LLMProvider
	}
	cfg.LLMReasoningEffort = normalizeLLMReasoningEffort(cfg.LLMReasoningEffort)
	if cfg.LLMThinkingBudget < 0 {
		cfg.LLMThinkingBudget = 0
	}
	cfg.LLMServiceTier = normalizeLLMServiceTier(cfg.LLMServiceTier)
	cfg.ChannelsTelegramDMPolicy = strings.TrimSpace(strings.ToLower(cfg.ChannelsTelegramDMPolicy))
	switch cfg.ChannelsTelegramDMPolicy {
	case "pairing", "allowlist", "open", "disabled":
	default:
		cfg.ChannelsTelegramDMPolicy = defaults.ChannelsTelegramDMPolicy
	}
	cfg.TelegramBotToken = strings.TrimSpace(cfg.TelegramBotToken)
	cfg.LLMAuthMode = strings.TrimSpace(strings.ToLower(cfg.LLMAuthMode))
	if cfg.LLMAuthMode == "" {
		switch cfg.LLMProvider {
		case "openai-codex":
			cfg.LLMAuthMode = "oauth"
		case "claude-code-cli":
			cfg.LLMAuthMode = "cli"
		default:
			cfg.LLMAuthMode = defaults.LLMAuthMode
		}
	}
	if cfg.LLMProvider == "openai-codex" && cfg.LLMAuthMode == "api-key" && strings.TrimSpace(cfg.LLMAPIKey) == "" {
		cfg.LLMAuthMode = "oauth"
	}
	if cfg.LLMProvider == "claude-code-cli" && cfg.LLMAuthMode == "api-key" && strings.TrimSpace(cfg.LLMAPIKey) == "" {
		cfg.LLMAuthMode = "cli"
	}
	cfg.LLMOAuthProvider = strings.TrimSpace(strings.ToLower(cfg.LLMOAuthProvider))
	if cfg.LLMAuthMode == "oauth" && cfg.LLMOAuthProvider == "" {
		if provider := defaultOAuthProvider(cfg.LLMProvider); provider != "" {
			cfg.LLMOAuthProvider = provider
		}
	}
	if cfg.AgentMaxIterations <= 0 {
		cfg.AgentMaxIterations = defaults.AgentMaxIterations
	}
	if cfg.UsageLimitDailyUSD <= 0 {
		cfg.UsageLimitDailyUSD = defaults.UsageLimitDailyUSD
	}
	if cfg.UsageLimitWeeklyUSD <= 0 {
		cfg.UsageLimitWeeklyUSD = defaults.UsageLimitWeeklyUSD
	}
	if cfg.UsageLimitMonthlyUSD <= 0 {
		cfg.UsageLimitMonthlyUSD = defaults.UsageLimitMonthlyUSD
	}
	cfg.UsageLimitMode = strings.TrimSpace(strings.ToLower(cfg.UsageLimitMode))
	switch cfg.UsageLimitMode {
	case "soft", "hard":
	default:
		cfg.UsageLimitMode = defaults.UsageLimitMode
	}
	if cfg.UsagePriceOverrides == nil {
		cfg.UsagePriceOverrides = map[string]UsagePrice{}
	}
	if cfg.CronRunHistoryLimit <= 0 {
		cfg.CronRunHistoryLimit = defaults.CronRunHistoryLimit
	}
	cfg.AssistantHotkey = strings.TrimSpace(cfg.AssistantHotkey)
	if cfg.AssistantHotkey == "" {
		cfg.AssistantHotkey = defaults.AssistantHotkey
	}
	cfg.AssistantWhisperBin = strings.TrimSpace(cfg.AssistantWhisperBin)
	if cfg.AssistantWhisperBin == "" {
		cfg.AssistantWhisperBin = defaults.AssistantWhisperBin
	}
	cfg.AssistantFFmpegBin = strings.TrimSpace(cfg.AssistantFFmpegBin)
	if cfg.AssistantFFmpegBin == "" {
		cfg.AssistantFFmpegBin = defaults.AssistantFFmpegBin
	}
	cfg.AssistantTTSBin = strings.TrimSpace(cfg.AssistantTTSBin)
	if cfg.AssistantTTSBin == "" {
		cfg.AssistantTTSBin = defaults.AssistantTTSBin
	}
	cfg.ScheduleTimezone = strings.TrimSpace(cfg.ScheduleTimezone)
	if cfg.ScheduleTimezone == "" {
		cfg.ScheduleTimezone = defaults.ScheduleTimezone
	}
}

func normalizeLLMReasoningEffort(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "":
		return ""
	case "none", "off", "disabled":
		return "none"
	case "minimal", "min":
		return "minimal"
	case "low":
		return "low"
	case "medium", "med":
		return "medium"
	case "high":
		return "high"
	case "veryhigh", "very-high", "very_high", "xhigh":
		return "high"
	default:
		return ""
	}
}

func normalizeLLMServiceTier(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "":
		return ""
	case "auto", "default", "flex", "priority":
		return strings.TrimSpace(strings.ToLower(raw))
	default:
		return ""
	}
}

func applyToolDefaults(cfg *Config, defaults Config) {
	cfg.ToolsWebSearchProvider = strings.TrimSpace(strings.ToLower(cfg.ToolsWebSearchProvider))
	if cfg.ToolsWebSearchProvider == "" {
		cfg.ToolsWebSearchProvider = defaults.ToolsWebSearchProvider
	}
	cfg.ToolsDefaultSet = strings.TrimSpace(strings.ToLower(cfg.ToolsDefaultSet))
	switch cfg.ToolsDefaultSet {
	case "", "standard":
		cfg.ToolsDefaultSet = defaults.ToolsDefaultSet
	case "minimal":
	default:
		cfg.ToolsDefaultSet = defaults.ToolsDefaultSet
	}
	if cfg.ToolsWebSearchPerplexityModel == "" {
		cfg.ToolsWebSearchPerplexityModel = defaults.ToolsWebSearchPerplexityModel
	}
	if cfg.ToolsWebSearchPerplexityBaseURL == "" {
		cfg.ToolsWebSearchPerplexityBaseURL = defaults.ToolsWebSearchPerplexityBaseURL
	}
	if cfg.ToolsWebSearchCacheTTLSeconds <= 0 {
		cfg.ToolsWebSearchCacheTTLSeconds = defaults.ToolsWebSearchCacheTTLSeconds
	}
}

func applyVaultDefaults(cfg *Config, defaults Config) {
	cfg.VaultAuthMode = strings.TrimSpace(strings.ToLower(cfg.VaultAuthMode))
	if cfg.VaultAuthMode == "" {
		cfg.VaultAuthMode = defaults.VaultAuthMode
	}
	if cfg.VaultAddr == "" {
		cfg.VaultAddr = defaults.VaultAddr
	}
	if cfg.VaultTimeoutMS <= 0 {
		cfg.VaultTimeoutMS = defaults.VaultTimeoutMS
	}
	if cfg.VaultKVMount == "" {
		cfg.VaultKVMount = defaults.VaultKVMount
	}
	if cfg.VaultKVVersion <= 0 {
		cfg.VaultKVVersion = defaults.VaultKVVersion
	}
	if cfg.VaultAppRoleMount == "" {
		cfg.VaultAppRoleMount = defaults.VaultAppRoleMount
	}
}

func applyBrowserDefaults(cfg *Config, defaults Config) {
	cfg.BrowserDefaultProfile = strings.TrimSpace(strings.ToLower(cfg.BrowserDefaultProfile))
	if cfg.BrowserDefaultProfile == "" {
		cfg.BrowserDefaultProfile = defaults.BrowserDefaultProfile
	}
	if cfg.BrowserRelayAddr == "" {
		cfg.BrowserRelayAddr = defaults.BrowserRelayAddr
	}
	cfg.BrowserRelayToken = strings.TrimSpace(cfg.BrowserRelayToken)
	if len(cfg.BrowserRelayOriginAllowlist) == 0 {
		cfg.BrowserRelayOriginAllowlist = append([]string{}, defaults.BrowserRelayOriginAllowlist...)
	}
	if strings.TrimSpace(cfg.BrowserSiteFlowsDir) == "" {
		cfg.BrowserSiteFlowsDir = filepath.Join(strings.TrimSpace(cfg.WorkspaceDir), "automation", "sites")
	}
	if strings.TrimSpace(cfg.BrowserManagedUserDataDir) == "" {
		cfg.BrowserManagedUserDataDir = filepath.Join(strings.TrimSpace(cfg.WorkspaceDir), "_shared", "browser", "managed")
	}
}

func applyGatewayDefaults(cfg *Config, defaults Config) {
	if cfg.GatewayAgentsWatchDebounceMS <= 0 {
		cfg.GatewayAgentsWatchDebounceMS = defaults.GatewayAgentsWatchDebounceMS
	}
	if cfg.GatewayRunsMaxRecords <= 0 {
		cfg.GatewayRunsMaxRecords = defaults.GatewayRunsMaxRecords
	}
	if cfg.GatewayChannelsMaxMessagesPerChannel <= 0 {
		cfg.GatewayChannelsMaxMessagesPerChannel = defaults.GatewayChannelsMaxMessagesPerChannel
	}
	if strings.TrimSpace(cfg.GatewayPersistenceDir) == "" {
		cfg.GatewayPersistenceDir = filepath.Join(strings.TrimSpace(cfg.WorkspaceDir), "_shared", "gateway")
	}
	if cfg.GatewayArchiveRetentionDays <= 0 {
		cfg.GatewayArchiveRetentionDays = defaults.GatewayArchiveRetentionDays
	}
	if cfg.GatewayArchiveMaxFileBytes <= 0 {
		cfg.GatewayArchiveMaxFileBytes = defaults.GatewayArchiveMaxFileBytes
	}
	if strings.TrimSpace(cfg.GatewayArchiveDir) == "" {
		cfg.GatewayArchiveDir = filepath.Join(strings.TrimSpace(cfg.WorkspaceDir), "_shared", "gateway", "archive")
	}
	if cfg.MCPCommandAllowlist == nil {
		cfg.MCPCommandAllowlist = append([]string{}, defaults.MCPCommandAllowlist...)
	}
}

func applyProviderDefaults(cfg *Config, defaults Config) {
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
				cfg.LLMBaseURL = defaultOpenAIBaseURL
			}
			if cfg.LLMModel == "" {
				cfg.LLMModel = defaultOpenAIModel
			}
			if cfg.LLMAPIKey == "" {
				cfg.LLMAPIKey = os.Getenv("OPENAI_API_KEY")
			}
		case "openai-codex":
			if cfg.LLMBaseURL == "" {
				cfg.LLMBaseURL = defaultOpenAICodexBaseURL
			}
			if cfg.LLMModel == "" {
				cfg.LLMModel = defaultOpenAICodexModel
			}
			if cfg.LLMAPIKey == "" {
				cfg.LLMAPIKey = firstNonEmpty(os.Getenv("OPENAI_CODEX_OAUTH_TOKEN"), os.Getenv("TARS_OPENAI_CODEX_OAUTH_TOKEN"))
			}
		case "claude-code-cli":
			if cfg.LLMModel == "" {
				cfg.LLMModel = defaultClaudeCodeCLIModel
			}
		case "gemini":
			if cfg.LLMBaseURL == "" {
				cfg.LLMBaseURL = defaultGeminiBaseURL
			}
			if cfg.LLMModel == "" {
				cfg.LLMModel = defaultGeminiModel
			}
			if cfg.LLMAPIKey == "" {
				cfg.LLMAPIKey = os.Getenv("GEMINI_API_KEY")
			}
		case "gemini-native":
			if cfg.LLMBaseURL == "" {
				cfg.LLMBaseURL = defaultGeminiNativeBaseURL
			}
			if cfg.LLMModel == "" {
				cfg.LLMModel = defaultGeminiModel
			}
			if cfg.LLMAPIKey == "" {
				cfg.LLMAPIKey = os.Getenv("GEMINI_API_KEY")
			}
		case "anthropic":
			if cfg.LLMBaseURL == "" {
				cfg.LLMBaseURL = defaultAnthropicBaseURL
			}
			if cfg.LLMModel == "" {
				cfg.LLMModel = defaultAnthropicModel
			}
			if cfg.LLMAPIKey == "" {
				cfg.LLMAPIKey = os.Getenv("ANTHROPIC_API_KEY")
			}
		}
	}
	if cfg.BifrostModel == "" {
		cfg.BifrostModel = defaults.BifrostModel
	}
}

func defaultOAuthProvider(provider string) string {
	switch strings.TrimSpace(strings.ToLower(provider)) {
	case "anthropic":
		return defaultClaudeOAuthProvider
	case "gemini", "gemini-native":
		return defaultGeminiOAuthProvider
	case "openai-codex":
		return defaultOpenAICodexOAuthProvider
	default:
		return ""
	}
}
