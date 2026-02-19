package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_DefaultOnly(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Mode != "standalone" {
		t.Fatalf("expected mode standalone, got %q", cfg.Mode)
	}

	if cfg.WorkspaceDir != "./workspace" {
		t.Fatalf("expected WorkspaceDir ./workspace, got %q", cfg.WorkspaceDir)
	}
	if cfg.LLMProvider != "bifrost" {
		t.Fatalf("expected LLMProvider bifrost, got %q", cfg.LLMProvider)
	}
	if cfg.LLMAuthMode != "api-key" {
		t.Fatalf("expected LLMAuthMode api-key, got %q", cfg.LLMAuthMode)
	}
	if cfg.LLMModel != "openai/gpt-4o-mini" {
		t.Fatalf("expected LLMModel openai/gpt-4o-mini, got %q", cfg.LLMModel)
	}
	if cfg.BifrostModel != "openai/gpt-4o-mini" {
		t.Fatalf("expected default BifrostModel openai/gpt-4o-mini, got %q", cfg.BifrostModel)
	}
	if cfg.AgentMaxIterations != 8 {
		t.Fatalf("expected default AgentMaxIterations 8, got %d", cfg.AgentMaxIterations)
	}
	if cfg.CronRunHistoryLimit != 200 {
		t.Fatalf("expected default CronRunHistoryLimit 200, got %d", cfg.CronRunHistoryLimit)
	}
	if !cfg.NotifyWhenNoClients {
		t.Fatalf("expected NotifyWhenNoClients=true by default")
	}
}

func TestLoad_YAMLOverridesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "mode: service\nworkspace_dir: ./tenant-workspace\nllm_provider: openai\nllm_auth_mode: oauth\nllm_oauth_provider: claude-code\nllm_base_url: http://localhost:8888/v1\nllm_api_key: llm-yaml-key\nllm_model: llm-yaml-model\nbifrost_base_url: http://localhost:8080/v1\nbifrost_api_key: yaml-key\nbifrost_model: yaml-model\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Mode != "service" {
		t.Fatalf("expected mode service, got %q", cfg.Mode)
	}
	if cfg.WorkspaceDir != "./tenant-workspace" {
		t.Fatalf("expected WorkspaceDir ./tenant-workspace, got %q", cfg.WorkspaceDir)
	}
	if cfg.LLMProvider != "openai" {
		t.Fatalf("expected LLMProvider from yaml, got %q", cfg.LLMProvider)
	}
	if cfg.LLMAuthMode != "oauth" {
		t.Fatalf("expected LLMAuthMode from yaml, got %q", cfg.LLMAuthMode)
	}
	if cfg.LLMOAuthProvider != "claude-code" {
		t.Fatalf("expected LLMOAuthProvider from yaml, got %q", cfg.LLMOAuthProvider)
	}
	if cfg.LLMBaseURL != "http://localhost:8888/v1" {
		t.Fatalf("expected LLMBaseURL from yaml, got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMAPIKey != "llm-yaml-key" {
		t.Fatalf("expected LLMAPIKey from yaml, got %q", cfg.LLMAPIKey)
	}
	if cfg.LLMModel != "llm-yaml-model" {
		t.Fatalf("expected LLMModel from yaml, got %q", cfg.LLMModel)
	}
	if cfg.BifrostBase != "http://localhost:8080/v1" {
		t.Fatalf("expected BifrostBase from yaml, got %q", cfg.BifrostBase)
	}
	if cfg.BifrostAPIKey != "yaml-key" {
		t.Fatalf("expected BifrostAPIKey from yaml, got %q", cfg.BifrostAPIKey)
	}
	if cfg.BifrostModel != "yaml-model" {
		t.Fatalf("expected BifrostModel from yaml, got %q", cfg.BifrostModel)
	}
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "mode: service\nworkspace_dir: ./tenant-workspace\nllm_provider: anthropic\nllm_auth_mode: api-key\nllm_oauth_provider: claude-code\nllm_base_url: http://localhost:8000\nllm_api_key: llm-yaml-key\nllm_model: llm-yaml-model\nbifrost_base_url: http://localhost:8080/v1\nbifrost_api_key: yaml-key\nbifrost_model: yaml-model\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("TARSD_MODE", "standalone")
	t.Setenv("TARSD_WORKSPACE_DIR", "./env-workspace")
	t.Setenv("BIFROST_BASE_URL", "http://localhost:9090/v1")
	t.Setenv("BIFROST_API_KEY", "env-key")
	t.Setenv("BIFROST_MODEL", "env-model")
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_AUTH_MODE", "oauth")
	t.Setenv("LLM_OAUTH_PROVIDER", "claude-code")
	t.Setenv("LLM_BASE_URL", "http://localhost:7000/v1")
	t.Setenv("LLM_API_KEY", "llm-env-key")
	t.Setenv("LLM_MODEL", "llm-env-model")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Mode != "standalone" {
		t.Fatalf("expected mode standalone from env, got %q", cfg.Mode)
	}
	if cfg.WorkspaceDir != "./env-workspace" {
		t.Fatalf("expected WorkspaceDir ./env-workspace from env, got %q", cfg.WorkspaceDir)
	}
	if cfg.LLMProvider != "openai" {
		t.Fatalf("expected LLMProvider from env, got %q", cfg.LLMProvider)
	}
	if cfg.LLMAuthMode != "oauth" {
		t.Fatalf("expected LLMAuthMode from env, got %q", cfg.LLMAuthMode)
	}
	if cfg.LLMOAuthProvider != "claude-code" {
		t.Fatalf("expected LLMOAuthProvider from env, got %q", cfg.LLMOAuthProvider)
	}
	if cfg.LLMBaseURL != "http://localhost:7000/v1" {
		t.Fatalf("expected LLMBaseURL from env, got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMAPIKey != "llm-env-key" {
		t.Fatalf("expected LLMAPIKey from env, got %q", cfg.LLMAPIKey)
	}
	if cfg.LLMModel != "llm-env-model" {
		t.Fatalf("expected LLMModel from env, got %q", cfg.LLMModel)
	}
	if cfg.BifrostBase != "http://localhost:9090/v1" {
		t.Fatalf("expected BifrostBase from env, got %q", cfg.BifrostBase)
	}
	if cfg.BifrostAPIKey != "env-key" {
		t.Fatalf("expected BifrostAPIKey from env, got %q", cfg.BifrostAPIKey)
	}
	if cfg.BifrostModel != "env-model" {
		t.Fatalf("expected BifrostModel from env, got %q", cfg.BifrostModel)
	}
}

func TestLoad_LLMProviderDefaults(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "openai-key")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.LLMBaseURL != "https://api.openai.com/v1" {
		t.Fatalf("expected openai default base url, got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMModel != "gpt-4o-mini" {
		t.Fatalf("expected openai default model, got %q", cfg.LLMModel)
	}
	if cfg.LLMAPIKey != "openai-key" {
		t.Fatalf("expected OPENAI_API_KEY fallback, got %q", cfg.LLMAPIKey)
	}
}

func TestLoad_GeminiProviderDefaults(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "gemini")
	t.Setenv("GEMINI_API_KEY", "gemini-key")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.LLMProvider != "gemini" {
		t.Fatalf("expected gemini provider, got %q", cfg.LLMProvider)
	}
	if cfg.LLMBaseURL != "https://generativelanguage.googleapis.com/v1beta/openai" {
		t.Fatalf("expected gemini base url, got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMModel != "gemini-2.5-flash" {
		t.Fatalf("expected gemini default model, got %q", cfg.LLMModel)
	}
	if cfg.LLMAPIKey != "gemini-key" {
		t.Fatalf("expected GEMINI_API_KEY fallback, got %q", cfg.LLMAPIKey)
	}
}

func TestLoad_GeminiNativeProviderDefaults(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "gemini-native")
	t.Setenv("GEMINI_API_KEY", "gemini-key")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.LLMProvider != "gemini-native" {
		t.Fatalf("expected gemini-native provider, got %q", cfg.LLMProvider)
	}
	if cfg.LLMBaseURL != "https://generativelanguage.googleapis.com/v1beta" {
		t.Fatalf("expected gemini-native base url, got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMModel != "gemini-2.5-flash" {
		t.Fatalf("expected gemini-native default model, got %q", cfg.LLMModel)
	}
	if cfg.LLMAPIKey != "gemini-key" {
		t.Fatalf("expected GEMINI_API_KEY fallback, got %q", cfg.LLMAPIKey)
	}
}

func TestLoad_GeminiOAuthDefaultsOAuthProvider(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "gemini")
	t.Setenv("LLM_AUTH_MODE", "oauth")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.LLMOAuthProvider != "google-antigravity" {
		t.Fatalf("expected gemini oauth provider default google-antigravity, got %q", cfg.LLMOAuthProvider)
	}
}

func TestLoad_GeminiNativeOAuthDefaultsOAuthProvider(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "gemini-native")
	t.Setenv("LLM_AUTH_MODE", "oauth")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.LLMOAuthProvider != "google-antigravity" {
		t.Fatalf("expected gemini-native oauth provider default google-antigravity, got %q", cfg.LLMOAuthProvider)
	}
}

func TestLoad_InvalidPathReturnsError(t *testing.T) {
	_, err := Load("./does-not-exist.yaml")
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestResolveTarsdConfigPath_ExplicitAndEnv(t *testing.T) {
	t.Setenv("TARSD_CONFIG", "/tmp/should-not-win.yaml")
	if got := ResolveTarsdConfigPath("./custom.yaml"); got != "./custom.yaml" {
		t.Fatalf("expected explicit path to win, got %q", got)
	}

	t.Setenv("TARSD_CONFIG", "/tmp/from-env.yaml")
	if got := ResolveTarsdConfigPath(""); got != "/tmp/from-env.yaml" {
		t.Fatalf("expected env path, got %q", got)
	}
}

func TestResolveTarsdConfigPath_DefaultCandidate(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "standalone.yaml")
	if err := os.WriteFile(configPath, []byte("mode: standalone\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}
	defer func() { _ = os.Chdir(wd) }()

	if got := ResolveTarsdConfigPath(""); got != DefaultTarsdConfigFilename {
		t.Fatalf("expected default candidate %q, got %q", DefaultTarsdConfigFilename, got)
	}
}

func TestLoad_InvalidFormatReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("mode standalone"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
}

func TestLoad_YAMLExpandsEnvVars(t *testing.T) {
	t.Setenv("TEST_SECRET_KEY", "expanded-value")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "llm_api_key: ${TEST_SECRET_KEY}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.LLMAPIKey != "expanded-value" {
		t.Fatalf("expected expanded value, got %q", cfg.LLMAPIKey)
	}
}

func TestLoad_AgentMaxIterationsFromEnv(t *testing.T) {
	t.Setenv("AGENT_MAX_ITERATIONS", "3")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.AgentMaxIterations != 3 {
		t.Fatalf("expected AgentMaxIterations=3, got %d", cfg.AgentMaxIterations)
	}
}

func TestLoad_APIWorkspaceAllowlistFromEnv(t *testing.T) {
	t.Setenv("API_USER_WORKSPACE_IDS_JSON", `["ws-user-a","ws-user-b"]`)
	t.Setenv("API_ADMIN_WORKSPACE_IDS_JSON", `["ws-admin"]`)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(cfg.APIUserWorkspaceIDs) != 2 {
		t.Fatalf("expected 2 user workspace ids, got %+v", cfg.APIUserWorkspaceIDs)
	}
	if cfg.APIUserWorkspaceIDs[0] != "ws-user-a" || cfg.APIUserWorkspaceIDs[1] != "ws-user-b" {
		t.Fatalf("unexpected user workspace ids: %+v", cfg.APIUserWorkspaceIDs)
	}
	if len(cfg.APIAdminWorkspaceIDs) != 1 || cfg.APIAdminWorkspaceIDs[0] != "ws-admin" {
		t.Fatalf("unexpected admin workspace ids: %+v", cfg.APIAdminWorkspaceIDs)
	}
}

func TestLoad_HeartbeatAndCronEnvOptions(t *testing.T) {
	t.Setenv("HEARTBEAT_ACTIVE_HOURS", "09:00-18:00")
	t.Setenv("HEARTBEAT_TIMEZONE", "Asia/Seoul")
	t.Setenv("CRON_RUN_HISTORY_LIMIT", "77")
	t.Setenv("NOTIFY_COMMAND", "echo notify")
	t.Setenv("NOTIFY_WHEN_NO_CLIENTS", "false")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.HeartbeatActiveHours != "09:00-18:00" {
		t.Fatalf("expected heartbeat active hours, got %q", cfg.HeartbeatActiveHours)
	}
	if cfg.HeartbeatTimezone != "Asia/Seoul" {
		t.Fatalf("expected heartbeat timezone, got %q", cfg.HeartbeatTimezone)
	}
	if cfg.CronRunHistoryLimit != 77 {
		t.Fatalf("expected cron run history limit 77, got %d", cfg.CronRunHistoryLimit)
	}
	if cfg.NotifyCommand != "echo notify" {
		t.Fatalf("expected notify command from env, got %q", cfg.NotifyCommand)
	}
	if cfg.NotifyWhenNoClients {
		t.Fatalf("expected NotifyWhenNoClients=false from env")
	}
}

func TestLoad_MCPServersFromEnv(t *testing.T) {
	t.Setenv("MCP_SERVERS_JSON", `[{"name":"filesystem","command":"npx","args":["-y","@modelcontextprotocol/server-filesystem","/tmp"],"env":{"NODE_ENV":"production"}}]`)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.MCPServers) != 1 {
		t.Fatalf("expected 1 mcp server, got %d", len(cfg.MCPServers))
	}
	srv := cfg.MCPServers[0]
	if srv.Name != "filesystem" || srv.Command != "npx" {
		t.Fatalf("unexpected mcp server: %+v", srv)
	}
	if len(srv.Args) != 3 {
		t.Fatalf("unexpected mcp args: %+v", srv.Args)
	}
	if srv.Env["NODE_ENV"] != "production" {
		t.Fatalf("unexpected mcp env: %+v", srv.Env)
	}
}

func TestLoad_OptionalToolDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.ToolsWebSearchEnabled {
		t.Fatalf("expected web search disabled by default")
	}
	if cfg.ToolsWebFetchEnabled {
		t.Fatalf("expected web fetch disabled by default")
	}
	if cfg.ToolsApplyPatchEnabled {
		t.Fatalf("expected apply_patch disabled by default")
	}
}

func TestLoad_OptionalToolsFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"tools_web_search_enabled: true",
		"tools_web_fetch_enabled: true",
		"tools_web_search_api_key: yaml-search-key",
		"tools_apply_patch_enabled: true",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.ToolsWebSearchEnabled || !cfg.ToolsWebFetchEnabled {
		t.Fatalf("expected web tools enabled from yaml, got search=%v fetch=%v", cfg.ToolsWebSearchEnabled, cfg.ToolsWebFetchEnabled)
	}
	if cfg.ToolsWebSearchAPIKey != "yaml-search-key" {
		t.Fatalf("unexpected web search api key: %q", cfg.ToolsWebSearchAPIKey)
	}
	if !cfg.ToolsApplyPatchEnabled {
		t.Fatalf("expected apply_patch enabled from yaml")
	}
}

func TestLoad_OptionalToolsFromEnv(t *testing.T) {
	t.Setenv("TOOLS_WEB_SEARCH_ENABLED", "true")
	t.Setenv("TOOLS_WEB_FETCH_ENABLED", "true")
	t.Setenv("TOOLS_WEB_SEARCH_API_KEY", "env-search-key")
	t.Setenv("TOOLS_APPLY_PATCH_ENABLED", "true")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.ToolsWebSearchEnabled || !cfg.ToolsWebFetchEnabled || !cfg.ToolsApplyPatchEnabled {
		t.Fatalf("expected optional tools enabled from env")
	}
	if cfg.ToolsWebSearchAPIKey != "env-search-key" {
		t.Fatalf("unexpected env web search api key: %q", cfg.ToolsWebSearchAPIKey)
	}
}

func TestLoad_ExpandedToolAndGatewayOptionsFromEnv(t *testing.T) {
	t.Setenv("GATEWAY_ENABLED", "true")
	t.Setenv("CHANNELS_LOCAL_ENABLED", "true")
	t.Setenv("CHANNELS_WEBHOOK_ENABLED", "true")
	t.Setenv("CHANNELS_TELEGRAM_ENABLED", "true")
	t.Setenv("TOOLS_MESSAGE_ENABLED", "true")
	t.Setenv("TOOLS_BROWSER_ENABLED", "true")
	t.Setenv("TOOLS_NODES_ENABLED", "true")
	t.Setenv("TOOLS_GATEWAY_ENABLED", "true")
	t.Setenv("TOOLS_WEB_SEARCH_PROVIDER", "perplexity")
	t.Setenv("TOOLS_WEB_SEARCH_PERPLEXITY_API_KEY", "px-key")
	t.Setenv("TOOLS_WEB_SEARCH_CACHE_TTL_SECONDS", "120")
	t.Setenv("TOOLS_WEB_FETCH_ALLOW_PRIVATE_HOSTS", "true")
	t.Setenv("TOOLS_WEB_FETCH_PRIVATE_HOST_ALLOWLIST_JSON", `["127.0.0.1","localhost"]`)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.GatewayEnabled || !cfg.ChannelsLocalEnabled || !cfg.ChannelsWebhookEnabled || !cfg.ChannelsTelegramEnabled {
		t.Fatalf("expected gateway/channel options enabled from env")
	}
	if !cfg.ToolsMessageEnabled || !cfg.ToolsBrowserEnabled || !cfg.ToolsNodesEnabled || !cfg.ToolsGatewayEnabled {
		t.Fatalf("expected tool options enabled from env")
	}
	if cfg.ToolsWebSearchProvider != "perplexity" {
		t.Fatalf("expected perplexity provider, got %q", cfg.ToolsWebSearchProvider)
	}
	if cfg.ToolsWebSearchPerplexityAPIKey != "px-key" {
		t.Fatalf("expected perplexity api key, got %q", cfg.ToolsWebSearchPerplexityAPIKey)
	}
	if cfg.ToolsWebSearchCacheTTLSeconds != 120 {
		t.Fatalf("expected cache ttl 120, got %d", cfg.ToolsWebSearchCacheTTLSeconds)
	}
	if !cfg.ToolsWebFetchAllowPrivateHosts {
		t.Fatalf("expected allow private hosts true")
	}
	if len(cfg.ToolsWebFetchPrivateHostAllowlist) != 2 {
		t.Fatalf("unexpected allowlist: %+v", cfg.ToolsWebFetchPrivateHostAllowlist)
	}
}

func TestLoad_GatewayAgentsFromEnv(t *testing.T) {
	t.Setenv("GATEWAY_DEFAULT_AGENT", "worker")
	t.Setenv("GATEWAY_AGENTS_JSON", `[{"name":"worker","description":"external worker","command":"sh","args":["-c","cat"],"env":{"WORKER_MODE":"on"},"enabled":true}]`)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.GatewayDefaultAgent != "worker" {
		t.Fatalf("expected gateway default agent worker, got %q", cfg.GatewayDefaultAgent)
	}
	if len(cfg.GatewayAgents) != 1 {
		t.Fatalf("expected 1 gateway agent, got %d", len(cfg.GatewayAgents))
	}
	agent := cfg.GatewayAgents[0]
	if agent.Name != "worker" {
		t.Fatalf("unexpected gateway agent name: %q", agent.Name)
	}
	if agent.Command != "sh" {
		t.Fatalf("unexpected gateway agent command: %q", agent.Command)
	}
	if len(agent.Args) != 2 || agent.Args[1] != "cat" {
		t.Fatalf("unexpected gateway agent args: %+v", agent.Args)
	}
	if agent.Env["WORKER_MODE"] != "on" {
		t.Fatalf("unexpected gateway agent env: %+v", agent.Env)
	}
	if !agent.Enabled {
		t.Fatalf("expected gateway agent enabled=true")
	}
}

func TestLoad_GatewayAgentsFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"gateway_enabled: true",
		"gateway_default_agent: worker",
		`gateway_agents_json: [{"name":"worker","command":"sh","args":["-c","cat"],"enabled":true}]`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.GatewayEnabled {
		t.Fatalf("expected gateway enabled from yaml")
	}
	if cfg.GatewayDefaultAgent != "worker" {
		t.Fatalf("expected gateway default agent worker, got %q", cfg.GatewayDefaultAgent)
	}
	if len(cfg.GatewayAgents) != 1 || cfg.GatewayAgents[0].Name != "worker" {
		t.Fatalf("unexpected gateway agents: %+v", cfg.GatewayAgents)
	}
}

func TestLoad_GatewayAgentsWatchDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.GatewayAgentsWatch {
		t.Fatalf("expected gateway agents watch enabled by default")
	}
	if cfg.GatewayAgentsWatchDebounceMS <= 0 {
		t.Fatalf("expected positive gateway agents watch debounce, got %d", cfg.GatewayAgentsWatchDebounceMS)
	}
}

func TestLoad_GatewayAgentsWatchFromYAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"gateway_agents_watch: true",
		"gateway_agents_watch_debounce_ms: 450",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("GATEWAY_AGENTS_WATCH_DEBOUNCE_MS", "120")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.GatewayAgentsWatch {
		t.Fatalf("expected gateway agents watch enabled from yaml")
	}
	if cfg.GatewayAgentsWatchDebounceMS != 120 {
		t.Fatalf("expected env debounce override 120, got %d", cfg.GatewayAgentsWatchDebounceMS)
	}
}

func TestLoad_GatewayPersistenceDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.GatewayPersistenceEnabled {
		t.Fatalf("expected gateway persistence enabled by default")
	}
	if !cfg.GatewayRunsPersistenceEnabled {
		t.Fatalf("expected gateway runs persistence enabled by default")
	}
	if !cfg.GatewayChannelsPersistenceEnabled {
		t.Fatalf("expected gateway channels persistence enabled by default")
	}
	if !cfg.GatewayRestoreOnStartup {
		t.Fatalf("expected gateway restore on startup enabled by default")
	}
	if cfg.GatewayRunsMaxRecords != 2000 {
		t.Fatalf("expected gateway runs max records 2000, got %d", cfg.GatewayRunsMaxRecords)
	}
	if cfg.GatewayChannelsMaxMessagesPerChannel != 500 {
		t.Fatalf("expected gateway channel max messages 500, got %d", cfg.GatewayChannelsMaxMessagesPerChannel)
	}
	expectedDir := filepath.Join(cfg.WorkspaceDir, "_shared", "gateway")
	if cfg.GatewayPersistenceDir != expectedDir {
		t.Fatalf("expected gateway persistence dir %q, got %q", expectedDir, cfg.GatewayPersistenceDir)
	}
}

func TestLoad_GatewayPersistenceFromYAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"workspace_dir: ./tenant-workspace",
		"gateway_persistence_enabled: true",
		"gateway_runs_persistence_enabled: true",
		"gateway_channels_persistence_enabled: true",
		"gateway_runs_max_records: 1234",
		"gateway_channels_max_messages_per_channel: 234",
		"gateway_persistence_dir: /tmp/yaml-gateway",
		"gateway_restore_on_startup: true",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("GATEWAY_PERSISTENCE_ENABLED", "false")
	t.Setenv("GATEWAY_RUNS_PERSISTENCE_ENABLED", "false")
	t.Setenv("GATEWAY_CHANNELS_PERSISTENCE_ENABLED", "false")
	t.Setenv("GATEWAY_RUNS_MAX_RECORDS", "345")
	t.Setenv("GATEWAY_CHANNELS_MAX_MESSAGES_PER_CHANNEL", "67")
	t.Setenv("GATEWAY_PERSISTENCE_DIR", "/tmp/env-gateway")
	t.Setenv("GATEWAY_RESTORE_ON_STARTUP", "false")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.GatewayPersistenceEnabled {
		t.Fatalf("expected gateway persistence disabled from env")
	}
	if cfg.GatewayRunsPersistenceEnabled {
		t.Fatalf("expected gateway runs persistence disabled from env")
	}
	if cfg.GatewayChannelsPersistenceEnabled {
		t.Fatalf("expected gateway channels persistence disabled from env")
	}
	if cfg.GatewayRunsMaxRecords != 345 {
		t.Fatalf("expected gateway runs max records 345, got %d", cfg.GatewayRunsMaxRecords)
	}
	if cfg.GatewayChannelsMaxMessagesPerChannel != 67 {
		t.Fatalf("expected gateway channels max messages 67, got %d", cfg.GatewayChannelsMaxMessagesPerChannel)
	}
	if cfg.GatewayPersistenceDir != "/tmp/env-gateway" {
		t.Fatalf("expected gateway persistence dir /tmp/env-gateway, got %q", cfg.GatewayPersistenceDir)
	}
	if cfg.GatewayRestoreOnStartup {
		t.Fatalf("expected gateway restore on startup false from env")
	}
}

func TestLoad_GatewayPersistenceInvalidIntFallback(t *testing.T) {
	t.Setenv("GATEWAY_RUNS_MAX_RECORDS", "not-a-number")
	t.Setenv("GATEWAY_CHANNELS_MAX_MESSAGES_PER_CHANNEL", "-1")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.GatewayRunsMaxRecords != 2000 {
		t.Fatalf("expected gateway runs max records fallback 2000, got %d", cfg.GatewayRunsMaxRecords)
	}
	if cfg.GatewayChannelsMaxMessagesPerChannel != 500 {
		t.Fatalf("expected gateway channels max messages fallback 500, got %d", cfg.GatewayChannelsMaxMessagesPerChannel)
	}
}

func TestLoad_GatewayReportDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.GatewayReportSummaryEnabled {
		t.Fatalf("expected gateway summary report enabled by default")
	}
	if cfg.GatewayArchiveEnabled {
		t.Fatalf("expected gateway archive disabled by default")
	}
	if cfg.GatewayArchiveRetentionDays != 30 {
		t.Fatalf("expected gateway archive retention days 30, got %d", cfg.GatewayArchiveRetentionDays)
	}
	if cfg.GatewayArchiveMaxFileBytes != 10485760 {
		t.Fatalf("expected gateway archive max file bytes 10485760, got %d", cfg.GatewayArchiveMaxFileBytes)
	}
	expectedDir := filepath.Join(cfg.WorkspaceDir, "_shared", "gateway", "archive")
	if cfg.GatewayArchiveDir != expectedDir {
		t.Fatalf("expected gateway archive dir %q, got %q", expectedDir, cfg.GatewayArchiveDir)
	}
}

func TestLoad_GatewayReportFromYAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"workspace_dir: ./tenant-workspace",
		"gateway_report_summary_enabled: true",
		"gateway_archive_enabled: true",
		"gateway_archive_dir: /tmp/yaml-gateway-archive",
		"gateway_archive_retention_days: 9",
		"gateway_archive_max_file_bytes: 2048",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("GATEWAY_REPORT_SUMMARY_ENABLED", "false")
	t.Setenv("GATEWAY_ARCHIVE_ENABLED", "true")
	t.Setenv("GATEWAY_ARCHIVE_DIR", "/tmp/env-gateway-archive")
	t.Setenv("GATEWAY_ARCHIVE_RETENTION_DAYS", "12")
	t.Setenv("GATEWAY_ARCHIVE_MAX_FILE_BYTES", "4096")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.GatewayReportSummaryEnabled {
		t.Fatalf("expected gateway summary report disabled from env")
	}
	if !cfg.GatewayArchiveEnabled {
		t.Fatalf("expected gateway archive enabled from env")
	}
	if cfg.GatewayArchiveDir != "/tmp/env-gateway-archive" {
		t.Fatalf("expected env archive dir, got %q", cfg.GatewayArchiveDir)
	}
	if cfg.GatewayArchiveRetentionDays != 12 {
		t.Fatalf("expected env archive retention 12, got %d", cfg.GatewayArchiveRetentionDays)
	}
	if cfg.GatewayArchiveMaxFileBytes != 4096 {
		t.Fatalf("expected env archive max file bytes 4096, got %d", cfg.GatewayArchiveMaxFileBytes)
	}
}

func TestLoad_DeprecatedToolPolicyKeysAreIgnored(t *testing.T) {
	t.Setenv("TOOLS_PROFILE", "minimal")
	t.Setenv("TOOLS_ALLOW", "session_status,memory_search")
	t.Setenv("TOOLS_DENY", "memory_get")
	t.Setenv("TOOLS_BY_PROVIDER_JSON", `{"openai":{"allow":["group:fs"]}}`)
	t.Setenv("TOOL_SELECTOR_MODE", "off")
	t.Setenv("TOOL_SELECTOR_MAX_TOOLS", "5")
	t.Setenv("TOOL_SELECTOR_AUTO_EXPAND", "true")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"tools_profile: coding",
		"tools_allow: read,write",
		"tools_deny: exec",
		`tools_by_provider_json: {"anthropic":{"profile":"minimal"}}`,
		"tool_selector_mode: heuristic",
		"tool_selector_max_tools: 7",
		"tool_selector_auto_expand: true",
		"tools_web_search_enabled: true",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.ToolsWebSearchEnabled {
		t.Fatalf("expected non-deprecated key to still be loaded")
	}
}

func TestLoad_ExtensionsDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.SkillsEnabled || !cfg.PluginsEnabled {
		t.Fatalf("expected skills/plugins enabled by default")
	}
	if !cfg.SkillsWatch || !cfg.PluginsWatch {
		t.Fatalf("expected skills/plugins watch enabled by default")
	}
	if cfg.SkillsWatchDebounceMS <= 0 || cfg.PluginsWatchDebounceMS <= 0 {
		t.Fatalf("expected positive debounce defaults, got skills=%d plugins=%d", cfg.SkillsWatchDebounceMS, cfg.PluginsWatchDebounceMS)
	}
}

func TestLoad_ExtensionsFromYAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"skills_enabled: true",
		"skills_watch: true",
		"skills_watch_debounce_ms: 55",
		`skills_extra_dirs_json: ["./team-skills"]`,
		"skills_bundled_dir: ./bundled-skills",
		"plugins_enabled: true",
		"plugins_watch: true",
		"plugins_watch_debounce_ms: 66",
		`plugins_extra_dirs_json: ["./team-plugins"]`,
		"plugins_bundled_dir: ./bundled-plugins",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("SKILLS_WATCH_DEBOUNCE_MS", "77")
	t.Setenv("PLUGINS_WATCH_DEBOUNCE_MS", "88")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.SkillsWatchDebounceMS != 77 {
		t.Fatalf("expected env override for skills debounce, got %d", cfg.SkillsWatchDebounceMS)
	}
	if cfg.PluginsWatchDebounceMS != 88 {
		t.Fatalf("expected env override for plugins debounce, got %d", cfg.PluginsWatchDebounceMS)
	}
	if cfg.SkillsBundledDir != "./bundled-skills" {
		t.Fatalf("unexpected skills bundled dir: %q", cfg.SkillsBundledDir)
	}
	if cfg.PluginsBundledDir != "./bundled-plugins" {
		t.Fatalf("unexpected plugins bundled dir: %q", cfg.PluginsBundledDir)
	}
	if len(cfg.SkillsExtraDirs) != 1 || cfg.SkillsExtraDirs[0] != "./team-skills" {
		t.Fatalf("unexpected skills extra dirs: %+v", cfg.SkillsExtraDirs)
	}
	if len(cfg.PluginsExtraDirs) != 1 || cfg.PluginsExtraDirs[0] != "./team-plugins" {
		t.Fatalf("unexpected plugins extra dirs: %+v", cfg.PluginsExtraDirs)
	}
}

func TestLoad_APIAuthDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.APIAuthMode != "external-required" {
		t.Fatalf("expected api auth mode external-required, got %q", cfg.APIAuthMode)
	}
	if cfg.APIWorkspaceHeader != "Tars-Workspace-Id" {
		t.Fatalf("expected api workspace header Tars-Workspace-Id, got %q", cfg.APIWorkspaceHeader)
	}
}

func TestLoad_APIAuthYAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"api_auth_mode: required",
		"api_auth_token: yaml-token",
		"api_user_token: yaml-user-token",
		"api_admin_token: yaml-admin-token",
		"api_workspace_header: Tenant-Workspace-Id",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("API_AUTH_MODE", "off")
	t.Setenv("API_AUTH_TOKEN", "env-token")
	t.Setenv("API_USER_TOKEN", "env-user-token")
	t.Setenv("API_ADMIN_TOKEN", "env-admin-token")
	t.Setenv("API_WORKSPACE_HEADER", "Tars-Workspace-Id")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.APIAuthMode != "off" {
		t.Fatalf("expected env override api auth mode off, got %q", cfg.APIAuthMode)
	}
	if cfg.APIAuthToken != "env-token" {
		t.Fatalf("expected env override api auth token, got %q", cfg.APIAuthToken)
	}
	if cfg.APIUserToken != "env-user-token" {
		t.Fatalf("expected env override api user token, got %q", cfg.APIUserToken)
	}
	if cfg.APIAdminToken != "env-admin-token" {
		t.Fatalf("expected env override api admin token, got %q", cfg.APIAdminToken)
	}
	if cfg.APIWorkspaceHeader != "Tars-Workspace-Id" {
		t.Fatalf("expected env override workspace header, got %q", cfg.APIWorkspaceHeader)
	}
}

func TestLoad_APIAuthYAMLInlineComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		`api_auth_token: "legacy-token" # legacy`,
		`api_user_token: "user-token" # user token`,
		`api_admin_token: "admin-token" # admin token`,
		`api_workspace_header: Tars-Workspace-Id # workspace header`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.APIAuthToken != "legacy-token" {
		t.Fatalf("expected legacy token without inline comment, got %q", cfg.APIAuthToken)
	}
	if cfg.APIUserToken != "user-token" {
		t.Fatalf("expected user token without inline comment, got %q", cfg.APIUserToken)
	}
	if cfg.APIAdminToken != "admin-token" {
		t.Fatalf("expected admin token without inline comment, got %q", cfg.APIAdminToken)
	}
	if cfg.APIWorkspaceHeader != "Tars-Workspace-Id" {
		t.Fatalf("expected workspace header without inline comment, got %q", cfg.APIWorkspaceHeader)
	}
}
