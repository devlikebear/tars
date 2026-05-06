package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llmdefaults"
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

	agentField, ok := configInputFieldByYAMLKey("agentruntime_agents_json")
	if !ok {
		t.Fatal("expected agentruntime_agents_json field metadata")
	}

	var agentCfg Config
	agentField.apply(&agentCfg, `[{"name":"ops","command":"run-agent","args":["--fast"],"env":{"MODE":"prod"}}]`)
	if len(agentCfg.AgentRuntimeAgents) != 1 || agentCfg.AgentRuntimeAgents[0].Name != "ops" || agentCfg.AgentRuntimeAgents[0].Command != "run-agent" {
		t.Fatalf("expected agent runtime agents to parse, got %#v", agentCfg.AgentRuntimeAgents)
	}

	srcAgents := []AgentRuntimeAgent{{Name: "ops", Command: "run-agent"}}
	var mergedAgents Config
	agentField.merge(&mergedAgents, Config{AgentRuntimeConfig: AgentRuntimeConfig{AgentRuntimeAgents: srcAgents}})
	srcAgents[0].Name = "mutated"
	if len(mergedAgents.AgentRuntimeAgents) != 1 || mergedAgents.AgentRuntimeAgents[0].Name != "ops" {
		t.Fatalf("expected merged agent runtime agents slice to be copied, got %#v", mergedAgents.AgentRuntimeAgents)
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

	if cfg.WorkspaceDir != DefaultWorkspaceDir() {
		t.Fatalf("expected WorkspaceDir %q, got %q", DefaultWorkspaceDir(), cfg.WorkspaceDir)
	}
	// Default Config (no YAML, no env) has an empty provider pool — users
	// must define llm_providers / llm_tiers in their config. Only the
	// default tier gets a fallback value.
	if len(cfg.LLMProviders) != 0 {
		t.Fatalf("expected empty LLMProviders by default, got %d entries", len(cfg.LLMProviders))
	}
	if len(cfg.LLMTiers) != 0 {
		t.Fatalf("expected empty LLMTiers by default, got %d entries", len(cfg.LLMTiers))
	}
	if cfg.LLMDefaultTier != "standard" {
		t.Fatalf("expected LLMDefaultTier standard, got %q", cfg.LLMDefaultTier)
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
	if cfg.RemoteAccessTailscaleServeEnabled {
		t.Fatalf("expected remote access disabled by default")
	}
	if cfg.RemoteAccessTailscaleServeHTTPSPort != 443 {
		t.Fatalf("expected remote access https port 443, got %d", cfg.RemoteAccessTailscaleServeHTTPSPort)
	}
	if !cfg.NotifyWhenNoClients {
		t.Fatalf("expected NotifyWhenNoClients=true by default")
	}
}

func TestLoad_YAMLOverridesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
workspace_dir: ./tenant-workspace
remote_access:
  tailscale_serve:
    enabled: true
    https_port: 9443
llm_providers:
  primary:
    kind: openai
    auth_mode: api-key
    api_key: llm-yaml-key
    base_url: http://localhost:8888/v1
llm_tiers:
  heavy:
    provider: primary
    model: llm-yaml-model
  standard:
    provider: primary
    model: llm-yaml-model
  light:
    provider: primary
    model: llm-yaml-model
llm_default_tier: standard
usage:
  limits:
    daily_tokens: 200000
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.WorkspaceDir != "./tenant-workspace" {
		t.Fatalf("expected WorkspaceDir ./tenant-workspace, got %q", cfg.WorkspaceDir)
	}
	if !cfg.RemoteAccessTailscaleServeEnabled {
		t.Fatalf("expected remote access tailscale serve enabled")
	}
	if cfg.RemoteAccessTailscaleServeHTTPSPort != 9443 {
		t.Fatalf("expected remote access tailscale serve https port 9443, got %d", cfg.RemoteAccessTailscaleServeHTTPSPort)
	}
	primary, ok := cfg.LLMProviders["primary"]
	if !ok {
		t.Fatalf("expected primary provider from yaml, got %+v", cfg.LLMProviders)
	}
	if primary.Kind != "openai" {
		t.Fatalf("expected primary Kind openai, got %q", primary.Kind)
	}
	if primary.APIKey != "llm-yaml-key" {
		t.Fatalf("expected primary APIKey from yaml, got %q", primary.APIKey)
	}
	if primary.BaseURL != "http://localhost:8888/v1" {
		t.Fatalf("expected primary BaseURL from yaml, got %q", primary.BaseURL)
	}
	if cfg.LLMDefaultTier != "standard" {
		t.Fatalf("expected llm_default_tier standard, got %q", cfg.LLMDefaultTier)
	}
	if cfg.UsageDailyTokenBudget != 200000 {
		t.Fatalf("expected usage daily token budget 200000, got %d", cfg.UsageDailyTokenBudget)
	}
	if cfg.LLMTiers["heavy"].Model != "llm-yaml-model" {
		t.Fatalf("expected heavy Model from yaml, got %q", cfg.LLMTiers["heavy"].Model)
	}
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
mode: service
workspace_dir: ./tenant-workspace
llm_providers:
  yaml_prov:
    kind: anthropic
    auth_mode: api-key
    api_key: yaml-key
    base_url: http://localhost:8000
llm_tiers:
  heavy:
    provider: yaml_prov
    model: yaml-model
  standard:
    provider: yaml_prov
    model: yaml-model
  light:
    provider: yaml_prov
    model: yaml-model
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("TARS_WORKSPACE_DIR", "./env-workspace")
	t.Setenv("TARS_LLM_PROVIDERS_JSON", `{"env_prov":{"kind":"openai","auth_mode":"oauth","base_url":"http://localhost:7000/v1","api_key":"env-key"}}`)
	t.Setenv("TARS_LLM_TIERS_JSON", `{"heavy":{"provider":"env_prov","model":"env-model","reasoning_effort":"veryhigh","thinking_budget":4096,"service_tier":"priority"},"standard":{"provider":"env_prov","model":"env-model"},"light":{"provider":"env_prov","model":"env-model"}}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.WorkspaceDir != "./env-workspace" {
		t.Fatalf("expected WorkspaceDir ./env-workspace from env, got %q", cfg.WorkspaceDir)
	}
	// Env override replaces the entire pool.
	if _, ok := cfg.LLMProviders["yaml_prov"]; ok {
		t.Fatal("expected env override to replace yaml pool")
	}
	envProv, ok := cfg.LLMProviders["env_prov"]
	if !ok {
		t.Fatalf("expected env_prov from env, got %+v", cfg.LLMProviders)
	}
	if envProv.Kind != "openai" || envProv.AuthMode != "oauth" {
		t.Fatalf("env_prov fields mismatch: %+v", envProv)
	}
	// Env tier binding knobs are normalized by applyLLMPoolDefaults.
	heavy := cfg.LLMTiers["heavy"]
	if heavy.Provider != "env_prov" || heavy.Model != "env-model" {
		t.Fatalf("heavy tier from env: %+v", heavy)
	}
	if heavy.ReasoningEffort != "high" {
		t.Fatalf("expected ReasoningEffort normalized (veryhigh→high), got %q", heavy.ReasoningEffort)
	}
	if heavy.ThinkingBudget != 4096 {
		t.Fatalf("expected ThinkingBudget 4096, got %d", heavy.ThinkingBudget)
	}
	if heavy.ServiceTier != "priority" {
		t.Fatalf("expected ServiceTier priority, got %q", heavy.ServiceTier)
	}
}

func TestLoad_LLMTierKnobsFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"llm_providers:",
		"  p: {kind: gemini-native}",
		"llm_tiers:",
		"  heavy: {provider: p, model: gemini-2.5-pro, reasoning_effort: minimal, thinking_budget: 2048, service_tier: flex}",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	heavy := cfg.LLMTiers["heavy"]
	if heavy.ReasoningEffort != "minimal" {
		t.Fatalf("expected reasoning effort minimal, got %q", heavy.ReasoningEffort)
	}
	if heavy.ThinkingBudget != 2048 {
		t.Fatalf("expected thinking budget 2048, got %d", heavy.ThinkingBudget)
	}
	if heavy.ServiceTier != "flex" {
		t.Fatalf("expected service tier flex, got %q", heavy.ServiceTier)
	}
}

func TestLoad_LLMPoolKindDefaults_OpenAI(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-key")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "llm_providers:\n  p: {kind: openai}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := cfg.LLMProviders["p"]
	if p.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("expected openai default base url, got %q", p.BaseURL)
	}
	if p.APIKey != "openai-key" {
		t.Errorf("expected OPENAI_API_KEY fallback, got %q", p.APIKey)
	}
	if p.AuthMode != "api-key" {
		t.Errorf("expected default AuthMode api-key, got %q", p.AuthMode)
	}
}

func TestLoad_LLMPoolKindDefaults_Gemini(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "gemini-key")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "llm_providers:\n  p: {kind: gemini}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := cfg.LLMProviders["p"]
	if p.BaseURL != "https://generativelanguage.googleapis.com/v1beta/openai" {
		t.Errorf("expected gemini base url, got %q", p.BaseURL)
	}
	if p.APIKey != "gemini-key" {
		t.Errorf("expected GEMINI_API_KEY fallback, got %q", p.APIKey)
	}
}

func TestLoad_LLMPoolKindDefaults_Kimi(t *testing.T) {
	t.Setenv("KIMI_API_KEY", "kimi-key")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "llm_providers:\n  p: {kind: kimi}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := cfg.LLMProviders["p"]
	if p.BaseURL != llmdefaults.KimiBaseURL {
		t.Errorf("expected kimi default base url, got %q", p.BaseURL)
	}
	if p.APIKey != "kimi-key" {
		t.Errorf("expected KIMI_API_KEY fallback, got %q", p.APIKey)
	}
	if p.AuthMode != "api-key" {
		t.Errorf("expected default AuthMode api-key, got %q", p.AuthMode)
	}
}

func TestLoad_LLMPoolKindDefaults_GeminiNative(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "gemini-key")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "llm_providers:\n  p: {kind: gemini-native}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := cfg.LLMProviders["p"]
	defaults, ok := llmdefaults.ForKind("gemini-native")
	if !ok {
		t.Fatal("expected shared gemini-native provider defaults")
	}
	if p.BaseURL != defaults.BaseURL {
		t.Errorf("expected gemini-native base url, got %q", p.BaseURL)
	}
	if p.APIKey != "gemini-key" {
		t.Errorf("expected GEMINI_API_KEY fallback, got %q", p.APIKey)
	}
}

func TestLoad_LLMPoolKindDefaults_Anthropic(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-key")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "llm_providers:\n  p: {kind: anthropic}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := cfg.LLMProviders["p"]
	if p.BaseURL != "https://api.anthropic.com" {
		t.Errorf("expected anthropic base url, got %q", p.BaseURL)
	}
	if p.APIKey != "anthropic-key" {
		t.Errorf("expected ANTHROPIC_API_KEY fallback, got %q", p.APIKey)
	}
}

func TestLoad_LLMPoolKindDefaults_OpenAICodex(t *testing.T) {
	// Ensure codex oauth env vars are empty so we test the "api-key with
	// no key → promote to oauth" fallback path.
	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", "")
	t.Setenv("TARS_OPENAI_CODEX_OAUTH_TOKEN", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "llm_providers:\n  p: {kind: openai-codex, auth_mode: api-key}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := cfg.LLMProviders["p"]
	if p.BaseURL != "https://chatgpt.com/backend-api" {
		t.Errorf("expected openai-codex base url, got %q", p.BaseURL)
	}
	if p.AuthMode != "oauth" {
		t.Errorf("expected api-key→oauth promotion when no key present, got %q", p.AuthMode)
	}
}

func TestResolveLLMTier_GeminiOAuthDerivesProvider(t *testing.T) {
	cfg := &Config{
		LLMConfig: LLMConfig{
			LLMProviders: map[string]LLMProviderSettings{
				"p": {Kind: "gemini", AuthMode: "oauth"},
			},
			LLMTiers: map[string]LLMTierBinding{
				"standard": {Provider: "p", Model: "gemini-2.5-flash"},
			},
		},
	}
	resolved, err := ResolveLLMTier(cfg, "standard")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.OAuthProvider != "google-antigravity" {
		t.Fatalf("expected derived OAuthProvider google-antigravity, got %q", resolved.OAuthProvider)
	}
}

func TestResolveLLMTier_APIKeyModeOmitsOAuthProvider(t *testing.T) {
	cfg := &Config{
		LLMConfig: LLMConfig{
			LLMProviders: map[string]LLMProviderSettings{
				"p": {Kind: "anthropic", AuthMode: "api-key", APIKey: "x"},
			},
			LLMTiers: map[string]LLMTierBinding{
				"standard": {Provider: "p", Model: "claude-sonnet-4-6"},
			},
		},
	}
	resolved, err := ResolveLLMTier(cfg, "standard")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.OAuthProvider != "" {
		t.Fatalf("expected empty OAuthProvider for api-key mode, got %q", resolved.OAuthProvider)
	}
}

func TestLoad_LLMDefaultTierFallback(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.LLMDefaultTier != "standard" {
		t.Fatalf("expected default llm_default_tier=standard, got %q", cfg.LLMDefaultTier)
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
	configPath := filepath.Join(configDir, "default.yaml")
	if err := os.WriteFile(configPath, []byte("workspace_dir: ./resolved-default\n"), 0o644); err != nil {
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

func TestResolveConfigPath_FixedPathFallback(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	// No config/default.yaml in CWD, create fixed config.
	emptyDir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(emptyDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(wd) }()

	fixedPath := FixedConfigPath()
	if err := os.MkdirAll(filepath.Dir(fixedPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(fixedPath, []byte("workspace_dir: ./resolved-fixed\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := ResolveConfigPath("")
	if got != fixedPath {
		t.Fatalf("expected fixed path %q, got %q", fixedPath, got)
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
	// Uses api_auth_token as a stand-in for testing ${VAR} expansion on
	// top-level scalar fields. Nested fields (llm_providers etc.) have
	// their own expansion path tested in llm_providers_field_test.go.
	t.Setenv("TEST_SECRET_TOKEN", "expanded-value")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "api_auth_token: ${TEST_SECRET_TOKEN}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.APIAuthToken != "expanded-value" {
		t.Fatalf("expected expanded value, got %q", cfg.APIAuthToken)
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

func TestLoad_PulseAndCronEnvOptions(t *testing.T) {
	t.Setenv("PULSE_ACTIVE_HOURS", "09:00-18:00")
	t.Setenv("PULSE_TIMEZONE", "Asia/Seoul")
	t.Setenv("CRON_RUN_HISTORY_LIMIT", "77")
	t.Setenv("NOTIFY_COMMAND", "echo notify")
	t.Setenv("NOTIFY_WHEN_NO_CLIENTS", "false")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.PulseActiveHours != "09:00-18:00" {
		t.Fatalf("expected pulse active hours, got %q", cfg.PulseActiveHours)
	}
	if cfg.PulseTimezone != "Asia/Seoul" {
		t.Fatalf("expected pulse timezone, got %q", cfg.PulseTimezone)
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

func TestLoad_ExpandedToolAndAgentRuntimeOptionsFromEnv(t *testing.T) {
	t.Setenv("AGENTRUNTIME_ENABLED", "true")
	t.Setenv("CHANNELS_LOCAL_ENABLED", "true")
	t.Setenv("CHANNELS_WEBHOOK_ENABLED", "true")
	t.Setenv("CHANNELS_TELEGRAM_ENABLED", "true")
	t.Setenv("TOOLS_MESSAGE_ENABLED", "true")
	t.Setenv("TOOLS_BROWSER_ENABLED", "true")
	t.Setenv("TOOLS_NODES_ENABLED", "true")
	t.Setenv("TOOLS_AGENTRUNTIME_ENABLED", "true")
	t.Setenv("TOOLS_WEB_SEARCH_PROVIDER", "perplexity")
	t.Setenv("TOOLS_WEB_SEARCH_PERPLEXITY_API_KEY", "px-key")
	t.Setenv("TOOLS_WEB_SEARCH_CACHE_TTL_SECONDS", "120")
	t.Setenv("TOOLS_WEB_FETCH_ALLOW_PRIVATE_HOSTS", "true")
	t.Setenv("TOOLS_WEB_FETCH_PRIVATE_HOST_ALLOWLIST_JSON", `["127.0.0.1","localhost"]`)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.AgentRuntimeEnabled || !cfg.ChannelsLocalEnabled || !cfg.ChannelsWebhookEnabled || !cfg.ChannelsTelegramEnabled {
		t.Fatalf("expected agentruntime/channel options enabled from env")
	}
	if !cfg.ToolsMessageEnabled || !cfg.ToolsAgentRuntimeEnabled {
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

func TestLoad_AgentRuntimeAgentsFromEnv(t *testing.T) {
	t.Setenv("AGENTRUNTIME_DEFAULT_AGENT", "worker")
	t.Setenv("AGENTRUNTIME_AGENTS_JSON", `[{"name":"worker","description":"external worker","command":"sh","args":["-c","cat"],"env":{"WORKER_MODE":"on"},"enabled":true}]`)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.AgentRuntimeDefaultAgent != "worker" {
		t.Fatalf("expected agent runtime default agent worker, got %q", cfg.AgentRuntimeDefaultAgent)
	}
	if len(cfg.AgentRuntimeAgents) != 1 {
		t.Fatalf("expected 1 agent runtime agent, got %d", len(cfg.AgentRuntimeAgents))
	}
	agent := cfg.AgentRuntimeAgents[0]
	if agent.Name != "worker" {
		t.Fatalf("unexpected agent runtime agent name: %q", agent.Name)
	}
	if agent.Command != "sh" {
		t.Fatalf("unexpected agent runtime agent command: %q", agent.Command)
	}
	if len(agent.Args) != 2 || agent.Args[1] != "cat" {
		t.Fatalf("unexpected agent runtime agent args: %+v", agent.Args)
	}
	if agent.Env["WORKER_MODE"] != "on" {
		t.Fatalf("unexpected agent runtime agent env: %+v", agent.Env)
	}
	if !agent.Enabled {
		t.Fatalf("expected agent runtime agent enabled=true")
	}
}

func TestLoad_AgentRuntimeAgentsFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"agentruntime_enabled: true",
		"agentruntime_default_agent: worker",
		`agentruntime_agents_json: [{"name":"worker","command":"sh","args":["-c","cat"],"enabled":true}]`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.AgentRuntimeEnabled {
		t.Fatalf("expected agent runtime enabled from yaml")
	}
	if cfg.AgentRuntimeDefaultAgent != "worker" {
		t.Fatalf("expected agent runtime default agent worker, got %q", cfg.AgentRuntimeDefaultAgent)
	}
	if len(cfg.AgentRuntimeAgents) != 1 || cfg.AgentRuntimeAgents[0].Name != "worker" {
		t.Fatalf("unexpected agent runtime agents: %+v", cfg.AgentRuntimeAgents)
	}
}

func TestLoad_AgentRuntimeAgentsWatchDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.AgentRuntimeAgentsWatch {
		t.Fatalf("expected agent runtime agents watch enabled by default")
	}
	if cfg.AgentRuntimeAgentsWatchDebounceMS <= 0 {
		t.Fatalf("expected positive agent runtime agents watch debounce, got %d", cfg.AgentRuntimeAgentsWatchDebounceMS)
	}
}

func TestLoad_AgentRuntimeAgentsWatchFromYAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"agentruntime_agents_watch: true",
		"agentruntime_agents_watch_debounce_ms: 450",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("AGENTRUNTIME_AGENTS_WATCH_DEBOUNCE_MS", "120")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.AgentRuntimeAgentsWatch {
		t.Fatalf("expected agent runtime agents watch enabled from yaml")
	}
	if cfg.AgentRuntimeAgentsWatchDebounceMS != 120 {
		t.Fatalf("expected env debounce override 120, got %d", cfg.AgentRuntimeAgentsWatchDebounceMS)
	}
}

func TestLoad_AgentRuntimePersistenceDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.AgentRuntimePersistenceEnabled {
		t.Fatalf("expected agent runtime persistence enabled by default")
	}
	if !cfg.AgentRuntimeRunsPersistenceEnabled {
		t.Fatalf("expected agent runtime runs persistence enabled by default")
	}
	if !cfg.AgentRuntimeChannelsPersistenceEnabled {
		t.Fatalf("expected agent runtime channels persistence enabled by default")
	}
	if !cfg.AgentRuntimeRestoreOnStartup {
		t.Fatalf("expected agent runtime restore on startup enabled by default")
	}
	if cfg.AgentRuntimeRunsMaxRecords != 2000 {
		t.Fatalf("expected agent runtime runs max records 2000, got %d", cfg.AgentRuntimeRunsMaxRecords)
	}
	if cfg.AgentRuntimeChannelsMaxMessagesPerChannel != 500 {
		t.Fatalf("expected agent runtime channel max messages 500, got %d", cfg.AgentRuntimeChannelsMaxMessagesPerChannel)
	}
	if cfg.AgentRuntimeSubagentsMaxThreads != 4 {
		t.Fatalf("expected agent runtime subagent max threads 4, got %d", cfg.AgentRuntimeSubagentsMaxThreads)
	}
	if cfg.AgentRuntimeSubagentsMaxDepth != 1 {
		t.Fatalf("expected agent runtime subagent max depth 1, got %d", cfg.AgentRuntimeSubagentsMaxDepth)
	}
	expectedDir := filepath.Join(cfg.WorkspaceDir, "_shared", "agentruntime")
	if cfg.AgentRuntimePersistenceDir != expectedDir {
		t.Fatalf("expected agent runtime persistence dir %q, got %q", expectedDir, cfg.AgentRuntimePersistenceDir)
	}
}

func TestLoad_AgentRuntimePersistenceFromYAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"workspace_dir: ./tenant-workspace",
		"agentruntime_persistence_enabled: true",
		"agentruntime_runs_persistence_enabled: true",
		"agentruntime_channels_persistence_enabled: true",
		"agentruntime_runs_max_records: 1234",
		"agentruntime_channels_max_messages_per_channel: 234",
		"agentruntime_subagents_max_threads: 6",
		"agentruntime_subagents_max_depth: 2",
		"agentruntime_persistence_dir: /tmp/yaml-agentruntime",
		"agentruntime_restore_on_startup: true",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("AGENTRUNTIME_PERSISTENCE_ENABLED", "false")
	t.Setenv("AGENTRUNTIME_RUNS_PERSISTENCE_ENABLED", "false")
	t.Setenv("AGENTRUNTIME_CHANNELS_PERSISTENCE_ENABLED", "false")
	t.Setenv("AGENTRUNTIME_RUNS_MAX_RECORDS", "345")
	t.Setenv("AGENTRUNTIME_CHANNELS_MAX_MESSAGES_PER_CHANNEL", "67")
	t.Setenv("AGENTRUNTIME_SUBAGENTS_MAX_THREADS", "3")
	t.Setenv("AGENTRUNTIME_SUBAGENTS_MAX_DEPTH", "4")
	t.Setenv("AGENTRUNTIME_PERSISTENCE_DIR", "/tmp/env-agentruntime")
	t.Setenv("AGENTRUNTIME_RESTORE_ON_STARTUP", "false")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.AgentRuntimePersistenceEnabled {
		t.Fatalf("expected agent runtime persistence disabled from env")
	}
	if cfg.AgentRuntimeRunsPersistenceEnabled {
		t.Fatalf("expected agent runtime runs persistence disabled from env")
	}
	if cfg.AgentRuntimeChannelsPersistenceEnabled {
		t.Fatalf("expected agent runtime channels persistence disabled from env")
	}
	if cfg.AgentRuntimeRunsMaxRecords != 345 {
		t.Fatalf("expected agent runtime runs max records 345, got %d", cfg.AgentRuntimeRunsMaxRecords)
	}
	if cfg.AgentRuntimeChannelsMaxMessagesPerChannel != 67 {
		t.Fatalf("expected agent runtime channels max messages 67, got %d", cfg.AgentRuntimeChannelsMaxMessagesPerChannel)
	}
	if cfg.AgentRuntimeSubagentsMaxThreads != 3 {
		t.Fatalf("expected agent runtime subagent max threads 3, got %d", cfg.AgentRuntimeSubagentsMaxThreads)
	}
	if cfg.AgentRuntimeSubagentsMaxDepth != 4 {
		t.Fatalf("expected agent runtime subagent max depth 4, got %d", cfg.AgentRuntimeSubagentsMaxDepth)
	}
	if cfg.AgentRuntimePersistenceDir != "/tmp/env-agentruntime" {
		t.Fatalf("expected agent runtime persistence dir /tmp/env-agentruntime, got %q", cfg.AgentRuntimePersistenceDir)
	}
	if cfg.AgentRuntimeRestoreOnStartup {
		t.Fatalf("expected agent runtime restore on startup false from env")
	}
}

func TestLoad_AgentRuntimePersistenceInvalidIntFallback(t *testing.T) {
	t.Setenv("AGENTRUNTIME_RUNS_MAX_RECORDS", "not-a-number")
	t.Setenv("AGENTRUNTIME_CHANNELS_MAX_MESSAGES_PER_CHANNEL", "-1")
	t.Setenv("AGENTRUNTIME_SUBAGENTS_MAX_THREADS", "0")
	t.Setenv("AGENTRUNTIME_SUBAGENTS_MAX_DEPTH", "-1")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.AgentRuntimeRunsMaxRecords != 2000 {
		t.Fatalf("expected agent runtime runs max records fallback 2000, got %d", cfg.AgentRuntimeRunsMaxRecords)
	}
	if cfg.AgentRuntimeChannelsMaxMessagesPerChannel != 500 {
		t.Fatalf("expected agent runtime channels max messages fallback 500, got %d", cfg.AgentRuntimeChannelsMaxMessagesPerChannel)
	}
	if cfg.AgentRuntimeSubagentsMaxThreads != 4 {
		t.Fatalf("expected agent runtime subagent max threads fallback 4, got %d", cfg.AgentRuntimeSubagentsMaxThreads)
	}
	if cfg.AgentRuntimeSubagentsMaxDepth != 1 {
		t.Fatalf("expected agent runtime subagent max depth fallback 1, got %d", cfg.AgentRuntimeSubagentsMaxDepth)
	}
}

func TestLoad_AgentRuntimeReportDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.AgentRuntimeReportSummaryEnabled {
		t.Fatalf("expected agent runtime summary report enabled by default")
	}
	if cfg.AgentRuntimeArchiveEnabled {
		t.Fatalf("expected agent runtime archive disabled by default")
	}
	if cfg.AgentRuntimeArchiveRetentionDays != 30 {
		t.Fatalf("expected agent runtime archive retention days 30, got %d", cfg.AgentRuntimeArchiveRetentionDays)
	}
	if cfg.AgentRuntimeArchiveMaxFileBytes != 10485760 {
		t.Fatalf("expected agent runtime archive max file bytes 10485760, got %d", cfg.AgentRuntimeArchiveMaxFileBytes)
	}
	expectedDir := filepath.Join(cfg.WorkspaceDir, "_shared", "agentruntime", "archive")
	if cfg.AgentRuntimeArchiveDir != expectedDir {
		t.Fatalf("expected agent runtime archive dir %q, got %q", expectedDir, cfg.AgentRuntimeArchiveDir)
	}
}

func TestLoad_AgentRuntimeReportFromYAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := strings.Join([]string{
		"workspace_dir: ./tenant-workspace",
		"agentruntime_report_summary_enabled: true",
		"agentruntime_archive_enabled: true",
		"agentruntime_archive_dir: /tmp/yaml-agentruntime-archive",
		"agentruntime_archive_retention_days: 9",
		"agentruntime_archive_max_file_bytes: 2048",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("AGENTRUNTIME_REPORT_SUMMARY_ENABLED", "false")
	t.Setenv("AGENTRUNTIME_ARCHIVE_ENABLED", "true")
	t.Setenv("AGENTRUNTIME_ARCHIVE_DIR", "/tmp/env-agentruntime-archive")
	t.Setenv("AGENTRUNTIME_ARCHIVE_RETENTION_DAYS", "12")
	t.Setenv("AGENTRUNTIME_ARCHIVE_MAX_FILE_BYTES", "4096")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.AgentRuntimeReportSummaryEnabled {
		t.Fatalf("expected agent runtime summary report disabled from env")
	}
	if !cfg.AgentRuntimeArchiveEnabled {
		t.Fatalf("expected agent runtime archive enabled from env")
	}
	if cfg.AgentRuntimeArchiveDir != "/tmp/env-agentruntime-archive" {
		t.Fatalf("expected env archive dir, got %q", cfg.AgentRuntimeArchiveDir)
	}
	if cfg.AgentRuntimeArchiveRetentionDays != 12 {
		t.Fatalf("expected env archive retention 12, got %d", cfg.AgentRuntimeArchiveRetentionDays)
	}
	if cfg.AgentRuntimeArchiveMaxFileBytes != 4096 {
		t.Fatalf("expected env archive max file bytes 4096, got %d", cfg.AgentRuntimeArchiveMaxFileBytes)
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

	if defaults.WorkspaceDir != DefaultWorkspaceDir() {
		t.Fatalf("expected workspace default %q, got %q", DefaultWorkspaceDir(), defaults.WorkspaceDir)
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
	if cfg.AgentRuntimeRunsMaxRecords != 2000 {
		t.Fatalf("expected agent runtime runs max records default 2000, got %d", cfg.AgentRuntimeRunsMaxRecords)
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
