package config

import (
	"os"
	"path/filepath"
	"strings"
)

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
	cfg.SessionDefaultID = strings.TrimSpace(cfg.SessionDefaultID)
	cfg.SessionTelegramScope = strings.TrimSpace(strings.ToLower(cfg.SessionTelegramScope))
	switch cfg.SessionTelegramScope {
	case "main", "per-user":
	default:
		cfg.SessionTelegramScope = "main"
	}
	cfg.LLMProvider = strings.TrimSpace(strings.ToLower(cfg.LLMProvider))
	if cfg.LLMProvider == "" {
		cfg.LLMProvider = "bifrost"
	}
	cfg.ChannelsTelegramDMPolicy = strings.TrimSpace(strings.ToLower(cfg.ChannelsTelegramDMPolicy))
	switch cfg.ChannelsTelegramDMPolicy {
	case "pairing", "allowlist", "open", "disabled":
	default:
		cfg.ChannelsTelegramDMPolicy = "pairing"
	}
	cfg.TelegramBotToken = strings.TrimSpace(cfg.TelegramBotToken)
	cfg.LLMAuthMode = strings.TrimSpace(strings.ToLower(cfg.LLMAuthMode))
	if cfg.LLMAuthMode == "" {
		switch cfg.LLMProvider {
		case "openai-codex":
			cfg.LLMAuthMode = "oauth"
		default:
			cfg.LLMAuthMode = "api-key"
		}
	}
	if cfg.LLMProvider == "openai-codex" && cfg.LLMAuthMode == "api-key" && strings.TrimSpace(cfg.LLMAPIKey) == "" {
		cfg.LLMAuthMode = "oauth"
	}
	cfg.LLMOAuthProvider = strings.TrimSpace(strings.ToLower(cfg.LLMOAuthProvider))
	if cfg.LLMAuthMode == "oauth" && cfg.LLMOAuthProvider == "" {
		switch cfg.LLMProvider {
		case "anthropic":
			cfg.LLMOAuthProvider = "claude-code"
		case "gemini", "gemini-native":
			cfg.LLMOAuthProvider = "google-antigravity"
		case "openai-codex":
			cfg.LLMOAuthProvider = "openai-codex"
		}
	}
	if cfg.AgentMaxIterations <= 0 {
		cfg.AgentMaxIterations = 8
	}
	if cfg.UsageLimitDailyUSD <= 0 {
		cfg.UsageLimitDailyUSD = 10.0
	}
	if cfg.UsageLimitWeeklyUSD <= 0 {
		cfg.UsageLimitWeeklyUSD = 50.0
	}
	if cfg.UsageLimitMonthlyUSD <= 0 {
		cfg.UsageLimitMonthlyUSD = 150.0
	}
	cfg.UsageLimitMode = strings.TrimSpace(strings.ToLower(cfg.UsageLimitMode))
	switch cfg.UsageLimitMode {
	case "soft", "hard":
	default:
		cfg.UsageLimitMode = "soft"
	}
	if cfg.UsagePriceOverrides == nil {
		cfg.UsagePriceOverrides = map[string]UsagePrice{}
	}
	if cfg.CronRunHistoryLimit <= 0 {
		cfg.CronRunHistoryLimit = 200
	}
	cfg.AssistantHotkey = strings.TrimSpace(cfg.AssistantHotkey)
	if cfg.AssistantHotkey == "" {
		cfg.AssistantHotkey = "Ctrl+Option+Space"
	}
	cfg.AssistantWhisperBin = strings.TrimSpace(cfg.AssistantWhisperBin)
	if cfg.AssistantWhisperBin == "" {
		cfg.AssistantWhisperBin = "whisper-cli"
	}
	cfg.AssistantFFmpegBin = strings.TrimSpace(cfg.AssistantFFmpegBin)
	if cfg.AssistantFFmpegBin == "" {
		cfg.AssistantFFmpegBin = "ffmpeg"
	}
	cfg.AssistantTTSBin = strings.TrimSpace(cfg.AssistantTTSBin)
	if cfg.AssistantTTSBin == "" {
		cfg.AssistantTTSBin = "say"
	}
	cfg.ScheduleTimezone = strings.TrimSpace(cfg.ScheduleTimezone)
	if cfg.ScheduleTimezone == "" {
		cfg.ScheduleTimezone = "Asia/Seoul"
	}
	cfg.ToolsWebSearchProvider = strings.TrimSpace(strings.ToLower(cfg.ToolsWebSearchProvider))
	if cfg.ToolsWebSearchProvider == "" {
		cfg.ToolsWebSearchProvider = "brave"
	}
	cfg.ToolsDefaultSet = strings.TrimSpace(strings.ToLower(cfg.ToolsDefaultSet))
	switch cfg.ToolsDefaultSet {
	case "", "standard":
		cfg.ToolsDefaultSet = "standard"
	case "minimal":
	default:
		cfg.ToolsDefaultSet = "standard"
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
	cfg.BrowserRelayToken = strings.TrimSpace(cfg.BrowserRelayToken)
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
		case "openai-codex":
			if cfg.LLMBaseURL == "" {
				cfg.LLMBaseURL = "https://chatgpt.com/backend-api"
			}
			if cfg.LLMModel == "" {
				cfg.LLMModel = "gpt-5.3-codex"
			}
			if cfg.LLMAPIKey == "" {
				cfg.LLMAPIKey = firstNonEmpty(os.Getenv("OPENAI_CODEX_OAUTH_TOKEN"), os.Getenv("TARS_OPENAI_CODEX_OAUTH_TOKEN"))
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
