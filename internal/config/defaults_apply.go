package config

import (
	"os"
	"path/filepath"
	"strings"
)

func applyDefaults(cfg *Config) {
	if cfg == nil {
		return
	}
	defaults := defaultConfigValues()
	applyCoreDefaults(cfg, defaults)
	applyMemoryDefaults(cfg, defaults)
	applyCompactionDefaults(cfg, defaults)
	applyToolDefaults(cfg, defaults)
	applyAgentRuntimeDefaults(cfg, defaults)
	applyLLMPoolDefaults(cfg)
}

func applyCoreDefaults(cfg *Config, defaults Config) {
	if strings.TrimSpace(cfg.WorkspaceDir) == "" {
		cfg.WorkspaceDir = defaults.WorkspaceDir
	}
	cfg.APIAuthMode = strings.TrimSpace(strings.ToLower(cfg.APIAuthMode))
	switch cfg.APIAuthMode {
	case "off", "external-required", "required":
	default:
		cfg.APIAuthMode = defaults.APIAuthMode
	}
	cfg.DashboardAuthMode = strings.TrimSpace(strings.ToLower(cfg.DashboardAuthMode))
	switch cfg.DashboardAuthMode {
	case "inherit", "off":
	default:
		cfg.DashboardAuthMode = defaults.DashboardAuthMode
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
	cfg.ChannelsTelegramDMPolicy = strings.TrimSpace(strings.ToLower(cfg.ChannelsTelegramDMPolicy))
	switch cfg.ChannelsTelegramDMPolicy {
	case "pairing", "allowlist", "open", "disabled":
	default:
		cfg.ChannelsTelegramDMPolicy = defaults.ChannelsTelegramDMPolicy
	}
	cfg.TelegramBotToken = strings.TrimSpace(cfg.TelegramBotToken)
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

func applyMemoryDefaults(cfg *Config, defaults Config) {
	cfg.MemoryEmbedProvider = strings.TrimSpace(strings.ToLower(cfg.MemoryEmbedProvider))
	if cfg.MemoryEmbedProvider == "" {
		cfg.MemoryEmbedProvider = defaults.MemoryEmbedProvider
	}
	cfg.MemoryEmbedBaseURL = strings.TrimSpace(cfg.MemoryEmbedBaseURL)
	if cfg.MemoryEmbedBaseURL == "" {
		cfg.MemoryEmbedBaseURL = defaults.MemoryEmbedBaseURL
	}
	cfg.MemoryEmbedAPIKey = strings.TrimSpace(cfg.MemoryEmbedAPIKey)
	cfg.MemoryEmbedModel = strings.TrimSpace(cfg.MemoryEmbedModel)
	if cfg.MemoryEmbedModel == "" {
		cfg.MemoryEmbedModel = defaults.MemoryEmbedModel
	}
	if cfg.MemoryEmbedDimensions <= 0 {
		cfg.MemoryEmbedDimensions = defaults.MemoryEmbedDimensions
	}
}

func applyCompactionDefaults(cfg *Config, defaults Config) {
	if cfg.CompactionTriggerTokens <= 0 {
		cfg.CompactionTriggerTokens = defaults.CompactionTriggerTokens
	}
	if cfg.CompactionKeepRecentTokens <= 0 {
		cfg.CompactionKeepRecentTokens = defaults.CompactionKeepRecentTokens
	}
	if cfg.CompactionKeepRecentFraction <= 0 {
		cfg.CompactionKeepRecentFraction = defaults.CompactionKeepRecentFraction
	}
	cfg.CompactionLLMMode = strings.TrimSpace(strings.ToLower(cfg.CompactionLLMMode))
	switch cfg.CompactionLLMMode {
	case "", "auto":
		cfg.CompactionLLMMode = defaults.CompactionLLMMode
	case "deterministic":
	default:
		cfg.CompactionLLMMode = defaults.CompactionLLMMode
	}
	if cfg.CompactionLLMTimeoutSeconds <= 0 {
		cfg.CompactionLLMTimeoutSeconds = defaults.CompactionLLMTimeoutSeconds
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

func applyAgentRuntimeDefaults(cfg *Config, defaults Config) {
	if cfg.AgentRuntimeAgentsWatchDebounceMS <= 0 {
		cfg.AgentRuntimeAgentsWatchDebounceMS = defaults.AgentRuntimeAgentsWatchDebounceMS
	}
	if cfg.AgentRuntimeRunsMaxRecords <= 0 {
		cfg.AgentRuntimeRunsMaxRecords = defaults.AgentRuntimeRunsMaxRecords
	}
	if cfg.AgentRuntimeChannelsMaxMessagesPerChannel <= 0 {
		cfg.AgentRuntimeChannelsMaxMessagesPerChannel = defaults.AgentRuntimeChannelsMaxMessagesPerChannel
	}
	if cfg.AgentRuntimeSubagentsMaxThreads <= 0 {
		cfg.AgentRuntimeSubagentsMaxThreads = defaults.AgentRuntimeSubagentsMaxThreads
	}
	if cfg.AgentRuntimeSubagentsMaxDepth <= 0 {
		cfg.AgentRuntimeSubagentsMaxDepth = defaults.AgentRuntimeSubagentsMaxDepth
	}
	if cfg.AgentRuntimeConsensusMaxFanout <= 0 {
		cfg.AgentRuntimeConsensusMaxFanout = defaults.AgentRuntimeConsensusMaxFanout
	}
	if cfg.AgentRuntimeConsensusBudgetTokens <= 0 {
		cfg.AgentRuntimeConsensusBudgetTokens = defaults.AgentRuntimeConsensusBudgetTokens
	}
	if cfg.AgentRuntimeConsensusBudgetUSD <= 0 {
		cfg.AgentRuntimeConsensusBudgetUSD = defaults.AgentRuntimeConsensusBudgetUSD
	}
	if cfg.AgentRuntimeConsensusTimeoutSeconds <= 0 {
		cfg.AgentRuntimeConsensusTimeoutSeconds = defaults.AgentRuntimeConsensusTimeoutSeconds
	}
	if cfg.AgentRuntimeConsensusAllowedAliases == nil {
		cfg.AgentRuntimeConsensusAllowedAliases = append([]string{}, defaults.AgentRuntimeConsensusAllowedAliases...)
	}
	if cfg.AgentRuntimeConsensusConcurrentRuns <= 0 {
		cfg.AgentRuntimeConsensusConcurrentRuns = defaults.AgentRuntimeConsensusConcurrentRuns
	}
	if strings.TrimSpace(cfg.AgentRuntimePersistenceDir) == "" {
		cfg.AgentRuntimePersistenceDir = filepath.Join(strings.TrimSpace(cfg.WorkspaceDir), "_shared", "agentruntime")
	}
	if cfg.AgentRuntimeArchiveRetentionDays <= 0 {
		cfg.AgentRuntimeArchiveRetentionDays = defaults.AgentRuntimeArchiveRetentionDays
	}
	if cfg.AgentRuntimeArchiveMaxFileBytes <= 0 {
		cfg.AgentRuntimeArchiveMaxFileBytes = defaults.AgentRuntimeArchiveMaxFileBytes
	}
	if strings.TrimSpace(cfg.AgentRuntimeArchiveDir) == "" {
		cfg.AgentRuntimeArchiveDir = filepath.Join(strings.TrimSpace(cfg.WorkspaceDir), "_shared", "agentruntime", "archive")
	}
	if cfg.MCPCommandAllowlist == nil {
		cfg.MCPCommandAllowlist = append([]string{}, defaults.MCPCommandAllowlist...)
	}
}

// applyLLMPoolDefaults fills in Kind-specific defaults for each entry in
// cfg.LLMProviders and normalizes per-tier knobs in cfg.LLMTiers.
//
// For each provider pool entry:
//   - AuthMode defaults based on Kind (openai-codex → oauth,
//     claude-code-cli → cli, everything else → api-key)
//   - BaseURL defaults to the canonical endpoint for the Kind
//   - APIKey defaults to the conventional env var for the Kind
//     when the user did not set one explicitly
//   - OAuthProvider defaults when AuthMode is oauth
//
// For each tier binding:
//   - ReasoningEffort is normalized (aliases like "med" → "medium")
//   - ServiceTier is normalized to a canonical value
//   - negative ThinkingBudget is clamped to 0
//
// This function is the new-schema equivalent of the deleted
// applyProviderDefaults + the LLM section of applyCoreDefaults.
//
// Validation of tier-to-provider references and missing tiers happens
// at router build time via ResolveAllLLMTiers — this function never
// errors, it only fills in blanks.
func applyLLMPoolDefaults(cfg *Config) {
	for alias, p := range cfg.LLMProviders {
		p.Kind = strings.ToLower(strings.TrimSpace(p.Kind))
		p.AuthMode = strings.ToLower(strings.TrimSpace(p.AuthMode))
		p.OAuthProvider = strings.ToLower(strings.TrimSpace(p.OAuthProvider))
		p.BaseURL = strings.TrimSpace(p.BaseURL)
		p.APIKey = strings.TrimSpace(p.APIKey)
		p.ServiceTier = normalizeLLMServiceTier(p.ServiceTier)

		if p.AuthMode == "" {
			switch p.Kind {
			case "openai-codex":
				p.AuthMode = "oauth"
			case "claude-code-cli":
				p.AuthMode = "cli"
			default:
				p.AuthMode = "api-key"
			}
		}

		switch p.Kind {
		case "openai":
			if p.BaseURL == "" {
				p.BaseURL = defaultOpenAIBaseURL
			}
			if p.APIKey == "" {
				p.APIKey = os.Getenv("OPENAI_API_KEY")
			}
		case "openai-codex":
			if p.BaseURL == "" {
				p.BaseURL = defaultOpenAICodexBaseURL
			}
			if p.APIKey == "" {
				p.APIKey = firstNonEmpty(os.Getenv("OPENAI_CODEX_OAUTH_TOKEN"), os.Getenv("TARS_OPENAI_CODEX_OAUTH_TOKEN"))
			}
		case "claude-code-cli":
			// claude-code-cli resolves credentials through the local CLI,
			// nothing to inject here.
		case "gemini":
			if p.BaseURL == "" {
				p.BaseURL = defaultGeminiBaseURL
			}
			if p.APIKey == "" {
				p.APIKey = os.Getenv("GEMINI_API_KEY")
			}
		case "gemini-native":
			if p.BaseURL == "" {
				p.BaseURL = defaultGeminiNativeBaseURL
			}
			if p.APIKey == "" {
				p.APIKey = os.Getenv("GEMINI_API_KEY")
			}
		case "anthropic":
			if p.BaseURL == "" {
				p.BaseURL = defaultAnthropicBaseURL
			}
			if p.APIKey == "" {
				p.APIKey = os.Getenv("ANTHROPIC_API_KEY")
			}
		}

		if p.AuthMode == "oauth" && p.OAuthProvider == "" {
			if provider := defaultOAuthProvider(p.Kind); provider != "" {
				p.OAuthProvider = provider
			}
		}
		// openai-codex falling back when api-key is requested but no key
		// is present — preserve the old behavior of promoting to oauth.
		if p.Kind == "openai-codex" && p.AuthMode == "api-key" && p.APIKey == "" {
			p.AuthMode = "oauth"
			if p.OAuthProvider == "" {
				p.OAuthProvider = defaultOpenAICodexOAuthProvider
			}
		}
		if p.Kind == "claude-code-cli" && p.AuthMode == "api-key" && p.APIKey == "" {
			p.AuthMode = "cli"
		}

		cfg.LLMProviders[alias] = p
	}

	for tier, b := range cfg.LLMTiers {
		b.Provider = strings.TrimSpace(b.Provider)
		b.Model = strings.TrimSpace(b.Model)
		b.ReasoningEffort = normalizeLLMReasoningEffort(b.ReasoningEffort)
		if b.ThinkingBudget < 0 {
			b.ThinkingBudget = 0
		}
		b.ServiceTier = normalizeLLMServiceTier(b.ServiceTier)
		cfg.LLMTiers[tier] = b
	}

	cfg.LLMDefaultTier = strings.ToLower(strings.TrimSpace(cfg.LLMDefaultTier))
	if cfg.LLMDefaultTier == "" {
		cfg.LLMDefaultTier = "standard"
	}

	cfg.LLMRoleDefaults = normalizeLLMRoleDefaults(cfg.LLMRoleDefaults)
}

// normalizeLLMRoleDefaults lowercases + trims both keys and values and
// drops empty entries. Unknown role names are NOT rejected here —
// validation happens at router build time via llm.ParseRole so that
// this package does not import internal/llm.
func normalizeLLMRoleDefaults(src map[string]string) map[string]string {
	if len(src) == 0 {
		return src
	}
	out := make(map[string]string, len(src))
	for role, tier := range src {
		role = strings.ToLower(strings.TrimSpace(role))
		tier = strings.ToLower(strings.TrimSpace(tier))
		if role == "" || tier == "" {
			continue
		}
		out[role] = tier
	}
	return out
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
