package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type MCPServer struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Config holds top-level runtime settings.
type Config struct {
	Mode                   string
	WorkspaceDir           string
	LLMProvider            string
	LLMAuthMode            string
	LLMOAuthProvider       string
	LLMBaseURL             string
	LLMAPIKey              string
	LLMModel               string
	AgentMaxIterations     int
	HeartbeatActiveHours   string
	HeartbeatTimezone      string
	CronRunHistoryLimit    int
	NotifyCommand          string
	NotifyWhenNoClients    bool
	BifrostBase            string
	BifrostAPIKey          string
	BifrostModel           string
	ToolsWebSearchEnabled  bool
	ToolsWebFetchEnabled   bool
	ToolsWebSearchAPIKey   string
	ToolsApplyPatchEnabled bool
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
	MCPServers             []MCPServer
}

const DefaultTarsdConfigFilename = "config/standalone.yaml"

// Default returns safe baseline settings for local standalone execution.
func Default() Config {
	return Config{
		Mode:                   "standalone",
		WorkspaceDir:           "./workspace",
		LLMProvider:            "bifrost",
		LLMAuthMode:            "api-key",
		BifrostModel:           "openai/gpt-4o-mini",
		AgentMaxIterations:     8,
		CronRunHistoryLimit:    200,
		NotifyWhenNoClients:    true,
		SkillsEnabled:          true,
		SkillsWatch:            true,
		SkillsWatchDebounceMS:  200,
		SkillsBundledDir:       "./skills",
		PluginsEnabled:         true,
		PluginsWatch:           true,
		PluginsWatchDebounceMS: 200,
		PluginsBundledDir:      "./plugins",
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
	if v := firstNonEmpty(os.Getenv("TOOLS_APPLY_PATCH_ENABLED"), os.Getenv("TARSD_TOOLS_APPLY_PATCH_ENABLED")); v != "" {
		cfg.ToolsApplyPatchEnabled = parseBool(v, cfg.ToolsApplyPatchEnabled)
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
		value = os.ExpandEnv(strings.Trim(strings.TrimSpace(value), `"'`))

		switch key {
		case "mode":
			cfg.Mode = value
		case "workspace_dir":
			cfg.WorkspaceDir = value
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
		case "tools_apply_patch_enabled":
			cfg.ToolsApplyPatchEnabled = parseBool(value, cfg.ToolsApplyPatchEnabled)
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
	if src.ToolsApplyPatchEnabled {
		dst.ToolsApplyPatchEnabled = true
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
