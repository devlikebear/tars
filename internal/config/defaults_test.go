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

func TestLoad_ToolSelectorDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.ToolsProfile != "full" {
		t.Fatalf("expected tools profile full, got %q", cfg.ToolsProfile)
	}
	if cfg.ToolSelectorMode != "heuristic" {
		t.Fatalf("expected selector mode heuristic, got %q", cfg.ToolSelectorMode)
	}
	if cfg.ToolSelectorMaxTools != 16 {
		t.Fatalf("expected selector max tools 16, got %d", cfg.ToolSelectorMaxTools)
	}
	if cfg.ToolSelectorAutoExpand {
		t.Fatalf("expected selector auto expand disabled by default")
	}
}

func TestLoad_ToolPolicyAndOptionalToolsFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"tools_profile: coding",
		"tools_allow: read,write,group:runtime",
		"tools_deny: exec",
		`tools_by_provider_json: {"anthropic":{"profile":"minimal","allow":["memory_search"]}}`,
		"tool_selector_mode: heuristic",
		"tool_selector_max_tools: 7",
		"tool_selector_auto_expand: true",
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
	if cfg.ToolsProfile != "coding" {
		t.Fatalf("expected tools profile coding, got %q", cfg.ToolsProfile)
	}
	if len(cfg.ToolsAllow) != 3 || cfg.ToolsAllow[0] != "read" {
		t.Fatalf("unexpected tools allow: %+v", cfg.ToolsAllow)
	}
	if len(cfg.ToolsDeny) != 1 || cfg.ToolsDeny[0] != "exec" {
		t.Fatalf("unexpected tools deny: %+v", cfg.ToolsDeny)
	}
	if cfg.ToolSelectorMaxTools != 7 {
		t.Fatalf("expected selector max tools 7, got %d", cfg.ToolSelectorMaxTools)
	}
	if !cfg.ToolSelectorAutoExpand {
		t.Fatalf("expected selector auto expand=true from yaml")
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
	if cfg.ToolsByProvider["anthropic"].Profile != "minimal" {
		t.Fatalf("unexpected tools by provider: %+v", cfg.ToolsByProvider)
	}
}

func TestLoad_ToolPolicyAndOptionalToolsFromEnv(t *testing.T) {
	t.Setenv("TOOLS_PROFILE", "minimal")
	t.Setenv("TOOLS_ALLOW", "session_status,memory_search")
	t.Setenv("TOOLS_DENY", "memory_get")
	t.Setenv("TOOLS_BY_PROVIDER_JSON", `{"openai":{"allow":["group:fs"]}}`)
	t.Setenv("TOOL_SELECTOR_MODE", "off")
	t.Setenv("TOOL_SELECTOR_MAX_TOOLS", "5")
	t.Setenv("TOOL_SELECTOR_AUTO_EXPAND", "true")
	t.Setenv("TOOLS_WEB_SEARCH_ENABLED", "true")
	t.Setenv("TOOLS_WEB_FETCH_ENABLED", "true")
	t.Setenv("TOOLS_WEB_SEARCH_API_KEY", "env-search-key")
	t.Setenv("TOOLS_APPLY_PATCH_ENABLED", "true")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.ToolsProfile != "minimal" {
		t.Fatalf("expected tools profile minimal, got %q", cfg.ToolsProfile)
	}
	if cfg.ToolSelectorMode != "off" {
		t.Fatalf("expected selector mode off, got %q", cfg.ToolSelectorMode)
	}
	if cfg.ToolSelectorMaxTools != 5 {
		t.Fatalf("expected selector max tools 5, got %d", cfg.ToolSelectorMaxTools)
	}
	if !cfg.ToolSelectorAutoExpand {
		t.Fatalf("expected auto expand true from env")
	}
	if !cfg.ToolsWebSearchEnabled || !cfg.ToolsWebFetchEnabled || !cfg.ToolsApplyPatchEnabled {
		t.Fatalf("expected optional tools enabled from env")
	}
	if cfg.ToolsWebSearchAPIKey != "env-search-key" {
		t.Fatalf("unexpected env web search api key: %q", cfg.ToolsWebSearchAPIKey)
	}
	if cfg.ToolsByProvider["openai"].Allow[0] != "group:fs" {
		t.Fatalf("unexpected tools by provider from env: %+v", cfg.ToolsByProvider)
	}
}
