package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConfigInputFields_ApplyFromEnvUsesConfiguredAliases(t *testing.T) {
	t.Setenv("SESSION_DEFAULT_ID", " legacy-session ")
	t.Setenv("TARS_SESSION_DEFAULT_ID", "new-session")

	var cfg Config
	applyConfigInputFieldsFromEnv(&cfg, configInputFields)

	if cfg.SessionDefaultID != "legacy-session" {
		t.Fatalf("expected first configured env alias to win, got %q", cfg.SessionDefaultID)
	}
}

func TestConfigInputFieldByYAMLKey_AppliesNormalizationAndMergeRules(t *testing.T) {
	sessionField, ok := configInputFieldByYAMLKey("session_telegram_scope")
	if !ok {
		t.Fatal("expected session_telegram_scope field metadata")
	}

	var cfg Config
	sessionField.apply(&cfg, " Per-User ")
	if cfg.SessionTelegramScope != "per-user" {
		t.Fatalf("expected normalized session scope, got %q", cfg.SessionTelegramScope)
	}

	dst := Config{RuntimeConfig: RuntimeConfig{SessionTelegramScope: "main"}}
	sessionField.merge(&dst, Config{})
	if dst.SessionTelegramScope != "main" {
		t.Fatalf("expected empty merge source to preserve destination, got %q", dst.SessionTelegramScope)
	}

	sessionField.merge(&dst, Config{RuntimeConfig: RuntimeConfig{SessionTelegramScope: "per-user"}})
	if dst.SessionTelegramScope != "per-user" {
		t.Fatalf("expected non-empty merge source to override destination, got %q", dst.SessionTelegramScope)
	}

	boolField, ok := configInputFieldByYAMLKey("assistant_enabled")
	if !ok {
		t.Fatal("expected assistant_enabled field metadata")
	}

	dst = Config{AssistantConfig: AssistantConfig{AssistantEnabled: true}}
	boolField.merge(&dst, Config{})
	if !dst.AssistantEnabled {
		t.Fatal("expected false merge source to preserve destination for bool fields")
	}

	var boolCfg Config
	boolField.apply(&boolCfg, "true")
	if !boolCfg.AssistantEnabled {
		t.Fatal("expected bool parser to set assistant_enabled from input field")
	}

	priceField, ok := configInputFieldByYAMLKey("usage_price_overrides_json")
	if !ok {
		t.Fatal("expected usage_price_overrides_json field metadata")
	}

	var priceCfg Config
	priceField.apply(&priceCfg, `{"gpt-4o":{"input_per_1m_usd":1.5,"output_per_1m_usd":2.5}}`)
	if got := priceCfg.UsagePriceOverrides["gpt-4o"].InputPer1MUSD; got != 1.5 {
		t.Fatalf("expected usage price override to parse, got %v", got)
	}

	srcPrices := map[string]UsagePrice{
		"gpt-4o": {InputPer1MUSD: 1.5, OutputPer1MUSD: 2.5},
	}
	var merged Config
	priceField.merge(&merged, Config{UsageConfig: UsageConfig{UsagePriceOverrides: srcPrices}})
	if !reflect.DeepEqual(merged.UsagePriceOverrides, srcPrices) {
		t.Fatalf("expected price overrides to copy on merge, got %#v", merged.UsagePriceOverrides)
	}

	srcPrices["gpt-4o"] = UsagePrice{InputPer1MUSD: 9.9, OutputPer1MUSD: 9.9}
	if got := merged.UsagePriceOverrides["gpt-4o"].InputPer1MUSD; got != 1.5 {
		t.Fatalf("expected merged map to be cloned, got %v", got)
	}
}

func TestConfigInputFieldByYAMLKey_CoversStructuredFields(t *testing.T) {
	profileField, ok := configInputFieldByYAMLKey("browser_default_profile")
	if !ok {
		t.Fatal("expected browser_default_profile field metadata")
	}

	var profileCfg Config
	profileField.apply(&profileCfg, " Work ")
	if profileCfg.BrowserDefaultProfile != "work" {
		t.Fatalf("expected browser profile to normalize to lower-trimmed value, got %q", profileCfg.BrowserDefaultProfile)
	}

	allowlistField, ok := configInputFieldByYAMLKey("tools_web_fetch_private_host_allowlist_json")
	if !ok {
		t.Fatal("expected tools_web_fetch_private_host_allowlist_json field metadata")
	}

	var allowlistCfg Config
	allowlistField.apply(&allowlistCfg, `[" localhost ", "10.0.0.5"]`)
	if !reflect.DeepEqual(allowlistCfg.ToolsWebFetchPrivateHostAllowlist, []string{"localhost", "10.0.0.5"}) {
		t.Fatalf("expected private host allowlist to parse, got %#v", allowlistCfg.ToolsWebFetchPrivateHostAllowlist)
	}

	srcAllowlist := []string{"localhost", "10.0.0.5"}
	var mergedAllowlist Config
	allowlistField.merge(&mergedAllowlist, Config{ToolConfig: ToolConfig{ToolsWebFetchPrivateHostAllowlist: srcAllowlist}})
	srcAllowlist[0] = "mutated"
	if !reflect.DeepEqual(mergedAllowlist.ToolsWebFetchPrivateHostAllowlist, []string{"localhost", "10.0.0.5"}) {
		t.Fatalf("expected merged allowlist to be cloned, got %#v", mergedAllowlist.ToolsWebFetchPrivateHostAllowlist)
	}

	agentField, ok := configInputFieldByYAMLKey("gateway_agents_json")
	if !ok {
		t.Fatal("expected gateway_agents_json field metadata")
	}

	var agentCfg Config
	agentField.apply(&agentCfg, `[{"name":"ops","command":"run-agent","args":["--fast"],"env":{"MODE":"prod"}}]`)
	if len(agentCfg.GatewayAgents) != 1 || agentCfg.GatewayAgents[0].Name != "ops" || agentCfg.GatewayAgents[0].Command != "run-agent" {
		t.Fatalf("expected gateway agents to parse, got %#v", agentCfg.GatewayAgents)
	}

	srcAgents := []GatewayAgent{{Name: "ops", Command: "run-agent"}}
	var mergedAgents Config
	agentField.merge(&mergedAgents, Config{GatewayConfig: GatewayConfig{GatewayAgents: srcAgents}})
	srcAgents[0].Name = "mutated"
	if len(mergedAgents.GatewayAgents) != 1 || mergedAgents.GatewayAgents[0].Name != "ops" {
		t.Fatalf("expected merged gateway agents slice to be copied, got %#v", mergedAgents.GatewayAgents)
	}

	mcpField, ok := configInputFieldByYAMLKey("mcp_servers_json")
	if !ok {
		t.Fatal("expected mcp_servers_json field metadata")
	}

	var mcpCfg Config
	mcpField.apply(&mcpCfg, `[{"name":"fs","command":"npx","args":["-y","mcp"]}]`)
	if len(mcpCfg.MCPServers) != 1 || mcpCfg.MCPServers[0].Name != "fs" || mcpCfg.MCPServers[0].Command != "npx" {
		t.Fatalf("expected mcp servers to parse, got %#v", mcpCfg.MCPServers)
	}

	memoryProviderField, ok := configInputFieldByYAMLKey("memory_embed_provider")
	if !ok {
		t.Fatal("expected memory_embed_provider field metadata")
	}

	var memoryCfg Config
	memoryProviderField.apply(&memoryCfg, " Gemini ")
	if memoryCfg.MemoryEmbedProvider != "gemini" {
		t.Fatalf("expected memory embed provider to normalize, got %q", memoryCfg.MemoryEmbedProvider)
	}

	memoryDimensionsField, ok := configInputFieldByYAMLKey("memory_embed_dimensions")
	if !ok {
		t.Fatal("expected memory_embed_dimensions field metadata")
	}
	memoryDimensionsField.apply(&memoryCfg, "1024")
	if memoryCfg.MemoryEmbedDimensions != 1024 {
		t.Fatalf("expected memory embed dimensions to parse, got %d", memoryCfg.MemoryEmbedDimensions)
	}
}

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
	if cfg.MemoryEmbedProvider != "gemini" {
		t.Fatalf("expected MemoryEmbedProvider gemini, got %q", cfg.MemoryEmbedProvider)
	}
	if cfg.MemoryEmbedBaseURL != "https://generativelanguage.googleapis.com/v1beta" {
		t.Fatalf("expected MemoryEmbedBaseURL gemini native endpoint, got %q", cfg.MemoryEmbedBaseURL)
	}
	if cfg.MemoryEmbedModel != "gemini-embedding-2-preview" {
		t.Fatalf("expected MemoryEmbedModel gemini-embedding-2-preview, got %q", cfg.MemoryEmbedModel)
	}
	if cfg.MemoryEmbedDimensions != 768 {
		t.Fatalf("expected MemoryEmbedDimensions 768, got %d", cfg.MemoryEmbedDimensions)
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

	t.Setenv("TARS_MODE", "standalone")
	t.Setenv("TARS_WORKSPACE_DIR", "./env-workspace")
	t.Setenv("BIFROST_BASE_URL", "http://localhost:9090/v1")
	t.Setenv("BIFROST_API_KEY", "env-key")
	t.Setenv("BIFROST_MODEL", "env-model")
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_AUTH_MODE", "oauth")
	t.Setenv("LLM_OAUTH_PROVIDER", "claude-code")
	t.Setenv("LLM_BASE_URL", "http://localhost:7000/v1")
	t.Setenv("LLM_API_KEY", "llm-env-key")
	t.Setenv("LLM_MODEL", "llm-env-model")
	t.Setenv("LLM_REASONING_EFFORT", "veryhigh")
	t.Setenv("LLM_THINKING_BUDGET", "4096")
	t.Setenv("LLM_SERVICE_TIER", "priority")

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
	if cfg.LLMReasoningEffort != "high" {
		t.Fatalf("expected LLMReasoningEffort normalized from env, got %q", cfg.LLMReasoningEffort)
	}
	if cfg.LLMThinkingBudget != 4096 {
		t.Fatalf("expected LLMThinkingBudget from env, got %d", cfg.LLMThinkingBudget)
	}
	if cfg.LLMServiceTier != "priority" {
		t.Fatalf("expected LLMServiceTier from env, got %q", cfg.LLMServiceTier)
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

func TestLoad_LLMReasoningOptionsFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"llm_provider: gemini-native",
		"llm_reasoning_effort: minimal",
		"llm_thinking_budget: 2048",
		"llm_service_tier: flex",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.LLMReasoningEffort != "minimal" {
		t.Fatalf("expected reasoning effort minimal, got %q", cfg.LLMReasoningEffort)
	}
	if cfg.LLMThinkingBudget != 2048 {
		t.Fatalf("expected thinking budget 2048, got %d", cfg.LLMThinkingBudget)
	}
	if cfg.LLMServiceTier != "flex" {
		t.Fatalf("expected service tier flex, got %q", cfg.LLMServiceTier)
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

func TestLoad_OpenAICodexProviderDefaults(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openai-codex")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.LLMProvider != "openai-codex" {
		t.Fatalf("expected openai-codex provider, got %q", cfg.LLMProvider)
	}
	if cfg.LLMBaseURL != "https://chatgpt.com/backend-api" {
		t.Fatalf("expected openai-codex base url, got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMModel != "gpt-5.3-codex" {
		t.Fatalf("expected openai-codex default model gpt-5.3-codex, got %q", cfg.LLMModel)
	}
	if cfg.LLMAuthMode != "oauth" {
		t.Fatalf("expected openai-codex default auth mode oauth, got %q", cfg.LLMAuthMode)
	}
	if cfg.LLMOAuthProvider != "openai-codex" {
		t.Fatalf("expected openai-codex default oauth provider openai-codex, got %q", cfg.LLMOAuthProvider)
	}
}

func TestLoad_ClaudeCodeCLIProviderDefaults(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "claude-code-cli")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.LLMProvider != "claude-code-cli" {
		t.Fatalf("expected claude-code-cli provider, got %q", cfg.LLMProvider)
	}
	if cfg.LLMModel != "sonnet" {
		t.Fatalf("expected claude-code-cli default model sonnet, got %q", cfg.LLMModel)
	}
	if cfg.LLMAuthMode != "cli" {
		t.Fatalf("expected claude-code-cli default auth mode cli, got %q", cfg.LLMAuthMode)
	}
	if cfg.LLMOAuthProvider != "" {
		t.Fatalf("expected claude-code-cli oauth provider to stay empty, got %q", cfg.LLMOAuthProvider)
	}
}

func TestLoad_InvalidPathReturnsError(t *testing.T) {
	_, err := Load("./does-not-exist.yaml")
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestConfig_TelegramToken_FromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("telegram_bot_token: yaml-bot-token\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.TelegramBotToken != "yaml-bot-token" {
		t.Fatalf("expected telegram token from yaml, got %q", cfg.TelegramBotToken)
	}
}

func TestConfig_TelegramToken_FromEnv(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "env-telegram-token")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.TelegramBotToken != "env-telegram-token" {
		t.Fatalf("expected telegram token from env, got %q", cfg.TelegramBotToken)
	}
}

func TestConfig_TelegramDMPolicy_FromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("channels_telegram_dm_policy: open\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ChannelsTelegramDMPolicy != "open" {
		t.Fatalf("expected telegram dm policy from yaml, got %q", cfg.ChannelsTelegramDMPolicy)
	}
}

func TestConfig_TelegramPollingEnabled_FromEnv(t *testing.T) {
	t.Setenv("CHANNELS_TELEGRAM_POLLING_ENABLED", "false")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ChannelsTelegramPollingEnabled {
		t.Fatalf("expected telegram polling disabled from env")
	}
}

func TestConfig_SessionScope_DefaultMain(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.SessionTelegramScope != "main" {
		t.Fatalf("expected session_telegram_scope=main by default, got %q", cfg.SessionTelegramScope)
	}
	if cfg.SessionDefaultID != "" {
		t.Fatalf("expected empty session_default_id by default, got %q", cfg.SessionDefaultID)
	}
}

func TestConfig_SessionScope_FromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"session_default_id: sess-main",
		"session_telegram_scope: per-user",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.SessionDefaultID != "sess-main" {
		t.Fatalf("expected session_default_id from yaml, got %q", cfg.SessionDefaultID)
	}
	if cfg.SessionTelegramScope != "per-user" {
		t.Fatalf("expected session_telegram_scope from yaml, got %q", cfg.SessionTelegramScope)
	}
}

func TestConfig_SessionScope_InvalidFallsBackToMain(t *testing.T) {
	t.Setenv("SESSION_TELEGRAM_SCOPE", "invalid")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.SessionTelegramScope != "main" {
		t.Fatalf("expected invalid scope to fallback to main, got %q", cfg.SessionTelegramScope)
	}
}

func TestResolveConfigPath_ExplicitAndEnv(t *testing.T) {
	t.Setenv("TARS_CONFIG", "/tmp/should-not-win.yaml")
	if got := ResolveConfigPath("./custom.yaml"); got != "./custom.yaml" {
		t.Fatalf("expected explicit path to win, got %q", got)
	}

	t.Setenv("TARS_CONFIG", "/tmp/from-env.yaml")
	if got := ResolveConfigPath(""); got != "/tmp/from-env.yaml" {
		t.Fatalf("expected env path, got %q", got)
	}
}

func TestResolveConfigPath_DefaultCandidate(t *testing.T) {
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

	if got := ResolveConfigPath(""); got != DefaultConfigFilename {
		t.Fatalf("expected default candidate %q, got %q", DefaultConfigFilename, got)
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
	if cfg.GatewaySubagentsMaxThreads != 4 {
		t.Fatalf("expected gateway subagent max threads 4, got %d", cfg.GatewaySubagentsMaxThreads)
	}
	if cfg.GatewaySubagentsMaxDepth != 1 {
		t.Fatalf("expected gateway subagent max depth 1, got %d", cfg.GatewaySubagentsMaxDepth)
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
		"gateway_subagents_max_threads: 6",
		"gateway_subagents_max_depth: 2",
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
	t.Setenv("GATEWAY_SUBAGENTS_MAX_THREADS", "3")
	t.Setenv("GATEWAY_SUBAGENTS_MAX_DEPTH", "4")
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
	if cfg.GatewaySubagentsMaxThreads != 3 {
		t.Fatalf("expected gateway subagent max threads 3, got %d", cfg.GatewaySubagentsMaxThreads)
	}
	if cfg.GatewaySubagentsMaxDepth != 4 {
		t.Fatalf("expected gateway subagent max depth 4, got %d", cfg.GatewaySubagentsMaxDepth)
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
	t.Setenv("GATEWAY_SUBAGENTS_MAX_THREADS", "0")
	t.Setenv("GATEWAY_SUBAGENTS_MAX_DEPTH", "-1")

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
	if cfg.GatewaySubagentsMaxThreads != 4 {
		t.Fatalf("expected gateway subagent max threads fallback 4, got %d", cfg.GatewaySubagentsMaxThreads)
	}
	if cfg.GatewaySubagentsMaxDepth != 1 {
		t.Fatalf("expected gateway subagent max depth fallback 1, got %d", cfg.GatewaySubagentsMaxDepth)
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

func TestLoad_VaultAndBrowserRuntimeDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.VaultEnabled {
		t.Fatalf("expected vault disabled by default")
	}
	if cfg.VaultAddr != "http://127.0.0.1:8200" {
		t.Fatalf("expected vault addr default, got %q", cfg.VaultAddr)
	}
	if cfg.VaultAuthMode != "token" {
		t.Fatalf("expected vault auth mode token, got %q", cfg.VaultAuthMode)
	}
	if cfg.VaultKVMount != "secret" {
		t.Fatalf("expected vault kv mount secret, got %q", cfg.VaultKVMount)
	}
	if cfg.VaultKVVersion != 2 {
		t.Fatalf("expected vault kv version 2, got %d", cfg.VaultKVVersion)
	}
	if cfg.VaultTimeoutMS != 1500 {
		t.Fatalf("expected vault timeout 1500ms, got %d", cfg.VaultTimeoutMS)
	}
	if cfg.BrowserRuntimeEnabled != true {
		t.Fatalf("expected browser runtime enabled by default")
	}
	if cfg.BrowserDefaultProfile != "managed" {
		t.Fatalf("expected browser default profile managed, got %q", cfg.BrowserDefaultProfile)
	}
	if cfg.BrowserSiteFlowsDir != filepath.Join(cfg.WorkspaceDir, "automation", "sites") {
		t.Fatalf("unexpected browser site flows dir: %q", cfg.BrowserSiteFlowsDir)
	}
	if cfg.BrowserManagedUserDataDir != filepath.Join(cfg.WorkspaceDir, "_shared", "browser", "managed") {
		t.Fatalf("unexpected browser managed user data dir: %q", cfg.BrowserManagedUserDataDir)
	}
}

func TestLoad_VaultAndBrowserRuntimeFromYAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"workspace_dir: ./tenant-workspace",
		"vault_enabled: true",
		"vault_addr: https://vault.local:8200",
		"vault_auth_mode: approle",
		"vault_token: yaml-vault-token",
		"vault_namespace: team-a",
		"vault_timeout_ms: 2400",
		"vault_kv_mount: kv",
		"vault_kv_version: 1",
		"vault_approle_mount: auth-approle",
		"vault_approle_role_id: role-yaml",
		"vault_approle_secret_id: secret-yaml",
		`vault_secret_path_allowlist_json: ["ops/","sites/"]`,
		"browser_runtime_enabled: true",
		"browser_default_profile: chrome",
		"browser_managed_headless: true",
		"browser_managed_executable_path: /Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"browser_managed_user_data_dir: /tmp/yaml-browser-profile",
		"browser_site_flows_dir: /tmp/yaml-site-flows",
		`browser_auto_login_site_allowlist_json: ["intranet","grafana"]`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("VAULT_ENABLED", "false")
	t.Setenv("VAULT_TIMEOUT_MS", "3200")
	t.Setenv("VAULT_TOKEN", "env-vault-token")
	t.Setenv("BROWSER_DEFAULT_PROFILE", "managed")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.VaultEnabled {
		t.Fatalf("expected env override vault disabled")
	}
	if cfg.VaultToken != "env-vault-token" {
		t.Fatalf("expected env vault token, got %q", cfg.VaultToken)
	}
	if cfg.VaultTimeoutMS != 3200 {
		t.Fatalf("expected env vault timeout 3200, got %d", cfg.VaultTimeoutMS)
	}
	if cfg.VaultAuthMode != "approle" {
		t.Fatalf("expected yaml vault auth mode approle, got %q", cfg.VaultAuthMode)
	}
	if cfg.BrowserDefaultProfile != "managed" {
		t.Fatalf("expected env browser default profile managed, got %q", cfg.BrowserDefaultProfile)
	}
	if len(cfg.BrowserAutoLoginSiteAllowlist) != 2 {
		t.Fatalf("unexpected browser auto login allowlist: %+v", cfg.BrowserAutoLoginSiteAllowlist)
	}
	if len(cfg.VaultSecretPathAllowlist) != 2 {
		t.Fatalf("unexpected vault secret path allowlist: %+v", cfg.VaultSecretPathAllowlist)
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
	if cfg.APIAuthMode != "required" {
		t.Fatalf("expected api auth mode required, got %q", cfg.APIAuthMode)
	}
}

func TestLoad_APIAuthModeInvalidFallsBackToRequired(t *testing.T) {
	t.Setenv("API_AUTH_MODE", "invalid-mode")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.APIAuthMode != "required" {
		t.Fatalf("expected invalid api_auth_mode fallback to required, got %q", cfg.APIAuthMode)
	}
}

func TestLoad_DashboardAuthDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.DashboardAuthMode != "inherit" {
		t.Fatalf("expected dashboard auth mode inherit, got %q", cfg.DashboardAuthMode)
	}
}

func TestLoad_DashboardAuthYAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"dashboard_auth_mode: off",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("DASHBOARD_AUTH_MODE", "inherit")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.DashboardAuthMode != "inherit" {
		t.Fatalf("expected env override dashboard auth mode inherit, got %q", cfg.DashboardAuthMode)
	}
}

func TestLoad_SecurityHardeningDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.APIAllowInsecureLocalAuth {
		t.Fatalf("expected api_allow_insecure_local_auth=false by default")
	}
	if cfg.APIMaxInflightChat != 2 {
		t.Fatalf("expected api_max_inflight_chat default 2, got %d", cfg.APIMaxInflightChat)
	}
	if cfg.APIMaxInflightAgentRuns != 4 {
		t.Fatalf("expected api_max_inflight_agent_runs default 4, got %d", cfg.APIMaxInflightAgentRuns)
	}
	if cfg.ToolsAllowHighRiskUser {
		t.Fatalf("expected tools_allow_high_risk_user=false by default")
	}
	if cfg.PluginsAllowMCPServers {
		t.Fatalf("expected plugins_allow_mcp_servers=false by default")
	}
	if len(cfg.MCPCommandAllowlist) != 0 {
		t.Fatalf("expected empty mcp command allowlist by default, got %+v", cfg.MCPCommandAllowlist)
	}
}

func TestLoad_SecurityHardeningFromYAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"api_allow_insecure_local_auth: true",
		"api_max_inflight_chat: 7",
		"api_max_inflight_agent_runs: 9",
		"tools_allow_high_risk_user: true",
		"plugins_allow_mcp_servers: true",
		`mcp_command_allowlist_json: ["npx","node"]`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("API_MAX_INFLIGHT_CHAT", "11")
	t.Setenv("API_MAX_INFLIGHT_AGENT_RUNS", "13")
	t.Setenv("MCP_COMMAND_ALLOWLIST_JSON", `["uvx"]`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.APIAllowInsecureLocalAuth {
		t.Fatalf("expected yaml api_allow_insecure_local_auth=true")
	}
	if cfg.APIMaxInflightChat != 11 {
		t.Fatalf("expected env override api_max_inflight_chat=11, got %d", cfg.APIMaxInflightChat)
	}
	if cfg.APIMaxInflightAgentRuns != 13 {
		t.Fatalf("expected env override api_max_inflight_agent_runs=13, got %d", cfg.APIMaxInflightAgentRuns)
	}
	if !cfg.ToolsAllowHighRiskUser {
		t.Fatalf("expected yaml tools_allow_high_risk_user=true")
	}
	if !cfg.PluginsAllowMCPServers {
		t.Fatalf("expected yaml plugins_allow_mcp_servers=true")
	}
	if len(cfg.MCPCommandAllowlist) != 1 || cfg.MCPCommandAllowlist[0] != "uvx" {
		t.Fatalf("expected env override mcp command allowlist, got %+v", cfg.MCPCommandAllowlist)
	}
}

func TestLoad_SecurityHardeningInflightLimitFallback(t *testing.T) {
	t.Setenv("API_MAX_INFLIGHT_CHAT", "0")
	t.Setenv("API_MAX_INFLIGHT_AGENT_RUNS", "-3")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.APIMaxInflightChat != 2 {
		t.Fatalf("expected fallback api_max_inflight_chat=2, got %d", cfg.APIMaxInflightChat)
	}
	if cfg.APIMaxInflightAgentRuns != 4 {
		t.Fatalf("expected fallback api_max_inflight_agent_runs=4, got %d", cfg.APIMaxInflightAgentRuns)
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
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("API_AUTH_MODE", "off")
	t.Setenv("API_AUTH_TOKEN", "env-token")
	t.Setenv("API_USER_TOKEN", "env-user-token")
	t.Setenv("API_ADMIN_TOKEN", "env-admin-token")

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
}

func TestLoad_APIAuthYAMLInlineComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		`api_auth_token: "legacy-token" # legacy`,
		`api_user_token: "user-token" # user token`,
		`api_admin_token: "admin-token" # admin token`,
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
}

func TestDefaultConfigValues_SharedBaseline(t *testing.T) {
	defaults := defaultConfigValues()

	if defaults.Mode != "standalone" {
		t.Fatalf("expected mode standalone, got %q", defaults.Mode)
	}
	if defaults.WorkspaceDir != "./workspace" {
		t.Fatalf("expected workspace default ./workspace, got %q", defaults.WorkspaceDir)
	}
	if defaults.APIAuthMode != "required" {
		t.Fatalf("expected api auth mode required, got %q", defaults.APIAuthMode)
	}
	if defaults.APIMaxInflightChat != 2 {
		t.Fatalf("expected inflight chat default 2, got %d", defaults.APIMaxInflightChat)
	}
	if defaults.MemoryEmbedModel != "gemini-embedding-2-preview" {
		t.Fatalf("expected memory embedding model default, got %q", defaults.MemoryEmbedModel)
	}
}

func TestApplyDefaults_UsesSharedDefaults(t *testing.T) {
	cfg := Config{
		RuntimeConfig: RuntimeConfig{WorkspaceDir: "./workspace"},
	}

	applyDefaults(&cfg)

	if cfg.ScheduleTimezone != "Asia/Seoul" {
		t.Fatalf("expected schedule timezone default Asia/Seoul, got %q", cfg.ScheduleTimezone)
	}
	if cfg.AssistantHotkey != "Ctrl+Option+Space" {
		t.Fatalf("expected assistant hotkey default, got %q", cfg.AssistantHotkey)
	}
	if cfg.ToolsWebSearchPerplexityBaseURL != "https://api.perplexity.ai/chat/completions" {
		t.Fatalf("expected perplexity base url default, got %q", cfg.ToolsWebSearchPerplexityBaseURL)
	}
	if cfg.MemoryEmbedDimensions != 768 {
		t.Fatalf("expected memory embed dimensions default 768, got %d", cfg.MemoryEmbedDimensions)
	}
	if cfg.GatewayRunsMaxRecords != 2000 {
		t.Fatalf("expected gateway runs max records default 2000, got %d", cfg.GatewayRunsMaxRecords)
	}
}

func TestDefaultAndApplyDefaults_StayAlignedForCoreValues(t *testing.T) {
	cfg := Config{
		RuntimeConfig: RuntimeConfig{WorkspaceDir: "./workspace"},
	}

	applyDefaults(&cfg)
	defaults := defaultConfigValues()

	if cfg.APIAuthMode != defaults.APIAuthMode {
		t.Fatalf("expected api auth mode alignment, got cfg=%q defaults=%q", cfg.APIAuthMode, defaults.APIAuthMode)
	}
	if cfg.APIMaxInflightAgentRuns != defaults.APIMaxInflightAgentRuns {
		t.Fatalf("expected inflight agent runs alignment, got cfg=%d defaults=%d", cfg.APIMaxInflightAgentRuns, defaults.APIMaxInflightAgentRuns)
	}
	if cfg.CronRunHistoryLimit != defaults.CronRunHistoryLimit {
		t.Fatalf("expected cron history alignment, got cfg=%d defaults=%d", cfg.CronRunHistoryLimit, defaults.CronRunHistoryLimit)
	}
	if cfg.MemoryEmbedBaseURL != defaults.MemoryEmbedBaseURL {
		t.Fatalf("expected memory embed base URL alignment, got cfg=%q defaults=%q", cfg.MemoryEmbedBaseURL, defaults.MemoryEmbedBaseURL)
	}
}
