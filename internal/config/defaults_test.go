package config

import (
	"os"
	"path/filepath"
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
