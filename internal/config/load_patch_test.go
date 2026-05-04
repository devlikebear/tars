package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchYAML_WritesPreferredHierarchy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte(strings.TrimSpace(`
mode: standalone
workspace_dir: ./old-workspace
pulse_timezone: UTC
agentruntime_persistence_dir: /tmp/flat-agentruntime
tools_web_search_provider: brave
`))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	updates := map[string]any{
		"workspace_dir":                "./new-workspace",
		"pulse_timezone":               "Asia/Seoul",
		"agentruntime_persistence_dir": "/tmp/nested-agentruntime",
		"tools_web_search_provider":    "perplexity",
		"llm_default_tier":             "heavy",
		"llm_role_defaults":            map[string]any{"chat_main": "standard", "pulse_decider": "light"},
	}
	if err := PatchYAML(path, updates); err != nil {
		t.Fatalf("patch yaml: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patched config: %v", err)
	}
	text := string(content)
	if strings.Contains(text, "\nworkspace_dir:") {
		t.Fatalf("expected workspace_dir to be rewritten under runtime, got:\n%s", text)
	}
	if strings.Contains(text, "\npulse_timezone:") {
		t.Fatalf("expected pulse_timezone to be rewritten under automation.pulse, got:\n%s", text)
	}
	if strings.Contains(text, "\ngateway_persistence_dir:") || strings.Contains(text, "\nagentruntime_persistence_dir:") {
		t.Fatalf("expected agentruntime_persistence_dir to be rewritten under agentruntime.persistence, got:\n%s", text)
	}
	if strings.Contains(text, "\ntools_web_search_provider:") {
		t.Fatalf("expected tools_web_search_provider to be rewritten under tools.web_search, got:\n%s", text)
	}
	for _, expected := range []string{"runtime:", "workspace_dir: ./new-workspace", "automation:", "timezone: Asia/Seoul", "agentruntime:", "dir: /tmp/nested-agentruntime", "provider: perplexity", "default_tier: heavy"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected patched config to contain %q, got:\n%s", expected, text)
		}
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load patched config: %v", err)
	}
	if cfg.WorkspaceDir != "./new-workspace" || cfg.PulseTimezone != "Asia/Seoul" || cfg.AgentRuntimePersistenceDir != "/tmp/nested-agentruntime" || cfg.ToolsWebSearchProvider != "perplexity" {
		t.Fatalf("patched config values not loaded correctly: %+v", cfg)
	}
	if cfg.LLMDefaultTier != "heavy" || cfg.LLMRoleDefaults["pulse_decider"] != "light" {
		t.Fatalf("patched llm hierarchy not loaded correctly: default=%q roles=%+v", cfg.LLMDefaultTier, cfg.LLMRoleDefaults)
	}
}

func TestSchema_UsesPreferredHierarchicalPaths(t *testing.T) {
	fields := Schema()
	byKey := map[string]FieldMeta{}
	for _, field := range fields {
		byKey[field.Key] = field
	}
	checks := map[string]string{
		"workspace_dir":                "runtime.workspace_dir",
		"llm_providers":                "llm.providers",
		"agent_max_iterations":         "automation.agent.max_iterations",
		"pulse_timezone":               "automation.pulse.timezone",
		"tools_web_search_provider":    "tools.web_search.provider",
		"agentruntime_persistence_dir": "agentruntime.persistence.dir",
		"telegram_bot_token":           "channels.telegram.bot_token",
		"skills_extra_dirs_json":       "extensions.skills.extra_dirs",
	}
	for key, want := range checks {
		field, ok := byKey[key]
		if !ok {
			t.Fatalf("schema missing key %q", key)
		}
		if field.Path != want {
			t.Fatalf("field %q path=%q want %q", key, field.Path, want)
		}
	}
	consensusField, ok := byKey["agentruntime_consensus_enabled"]
	if !ok {
		t.Fatal("schema missing agentruntime_consensus_enabled")
	}
	if !strings.Contains(consensusField.Description, "Advanced opt-in") || !strings.Contains(consensusField.Description, "subagents_run") {
		t.Fatalf("expected consensus schema to document advanced opt-in lifecycle, got %q", consensusField.Description)
	}
}

func TestLoad_ExampleConfigHierarchicalSchema(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-example")
	t.Setenv("GEMINI_API_KEY", "gemini-example")

	path := filepath.Join("..", "..", "config", "tars.config.example.yaml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load example config: %v", err)
	}
	if cfg.WorkspaceDir != "./workspace" {
		t.Fatalf("unexpected workspace dir: %q", cfg.WorkspaceDir)
	}
	if cfg.LLMProviders["default"].APIKey != "sk-ant-example" {
		t.Fatalf("expected anthropic key expansion in example config, got %+v", cfg.LLMProviders["default"])
	}
	if cfg.MemoryEmbedAPIKey != "gemini-example" {
		t.Fatalf("expected memory embed key expansion, got %q", cfg.MemoryEmbedAPIKey)
	}
	if cfg.AgentRuntimePersistenceDir != "./workspace/_shared/agentruntime" {
		t.Fatalf("unexpected agent runtime persistence dir: %q", cfg.AgentRuntimePersistenceDir)
	}
	if cfg.ChannelsTelegramDMPolicy != "pairing" {
		t.Fatalf("unexpected telegram dm policy: %q", cfg.ChannelsTelegramDMPolicy)
	}
	if len(cfg.MCPCommandAllowlist) != 1 || cfg.MCPCommandAllowlist[0] != "npx" {
		t.Fatalf("unexpected mcp command allowlist: %+v", cfg.MCPCommandAllowlist)
	}
}

func TestLoad_AgentRuntimeHardCutIgnoresLegacyAgentRuntimeConfigKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte(strings.TrimSpace(`
gateway_enabled: true
gateway:
  default_agent: legacy-worker
  persistence:
    dir: /tmp/legacy-gateway
`))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.AgentRuntimeEnabled {
		t.Fatalf("legacy gateway_enabled should not enable agent runtime")
	}
	if cfg.AgentRuntimeDefaultAgent == "legacy-worker" {
		t.Fatalf("legacy gateway.default_agent should not configure agent runtime")
	}
	if strings.Contains(cfg.AgentRuntimePersistenceDir, "legacy-gateway") || strings.Contains(cfg.AgentRuntimePersistenceDir, "_shared/gateway") {
		t.Fatalf("legacy gateway persistence paths should not be used, got %q", cfg.AgentRuntimePersistenceDir)
	}
}

func TestPatchYAML_PreservesAPIKeyWhenOmitted(t *testing.T) {
	// The editor sends the full provider alias set as the authoritative
	// state, but omits api_key for entries the user opted to keep on
	// disk. The patch must preserve those credentials per-field while
	// honoring the alias-level set the editor sent.
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte(strings.TrimSpace(`
llm:
  providers:
    kimi:
      kind: kimi
      auth_mode: api-key
      base_url: https://api.moonshot.ai/v1
      api_key: real-kimi-key
    anthropic:
      kind: anthropic
      auth_mode: api-key
      base_url: https://api.anthropic.com
      api_key: real-ant-key
  tiers:
    heavy:
      provider: anthropic
      model: claude-opus-4-5
    standard:
      provider: kimi
      model: kimi-k2.6
    light:
      provider: kimi
      model: kimi-k2.6
`))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	updates := map[string]any{
		"llm_providers": map[string]any{
			"kimi": map[string]any{
				"kind":      "kimi",
				"auth_mode": "api-key",
				"base_url":  "https://api.moonshot.ai/v2",
				// api_key intentionally omitted — keep existing
			},
			"anthropic": map[string]any{
				"kind":      "anthropic",
				"auth_mode": "api-key",
				"base_url":  "https://api.anthropic.com",
				// api_key intentionally omitted — keep existing
			},
		},
	}
	if err := PatchYAML(path, updates); err != nil {
		t.Fatalf("patch yaml: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load patched config: %v", err)
	}
	if cfg.LLMProviders["kimi"].APIKey != "real-kimi-key" {
		t.Fatalf("kimi api_key lost after patch: got %q want %q", cfg.LLMProviders["kimi"].APIKey, "real-kimi-key")
	}
	if cfg.LLMProviders["kimi"].BaseURL != "https://api.moonshot.ai/v2" {
		t.Fatalf("kimi base_url not updated: got %q", cfg.LLMProviders["kimi"].BaseURL)
	}
	if cfg.LLMProviders["anthropic"].APIKey != "real-ant-key" {
		t.Fatalf("anthropic api_key lost: got %q", cfg.LLMProviders["anthropic"].APIKey)
	}
	if cfg.LLMTiers["heavy"].Provider != "anthropic" {
		t.Fatalf("heavy tier provider changed unexpectedly: %q", cfg.LLMTiers["heavy"].Provider)
	}
}

func TestPatchYAML_RenamesProviderAlias(t *testing.T) {
	// Renaming an alias in the editor sends the new alias and drops
	// the old one from the patch payload. The on-disk provider map
	// must mirror the editor's authoritative state — the old alias is
	// removed, not retained alongside the new one.
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte(strings.TrimSpace(`
llm:
  providers:
    kimi:
      kind: kimi
      auth_mode: api-key
      base_url: https://api.moonshot.ai/v1
      api_key: real-kimi-key
`))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	updates := map[string]any{
		"llm_providers": map[string]any{
			"moonshot": map[string]any{
				"kind":      "kimi",
				"auth_mode": "api-key",
				"base_url":  "https://api.moonshot.ai/v1",
				"api_key":   "fresh-key",
			},
		},
	}
	if err := PatchYAML(path, updates); err != nil {
		t.Fatalf("patch yaml: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load patched config: %v", err)
	}
	if _, exists := cfg.LLMProviders["kimi"]; exists {
		t.Fatalf("renamed provider 'kimi' still present after rename to 'moonshot'")
	}
	if cfg.LLMProviders["moonshot"].APIKey != "fresh-key" {
		t.Fatalf("renamed provider api_key not stored: %+v", cfg.LLMProviders["moonshot"])
	}
}

func TestPatchYAML_RemovesAliasMissingFromPatch(t *testing.T) {
	// Removing a provider in the editor sends a payload without that
	// alias. The on-disk map must drop it.
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte(strings.TrimSpace(`
llm:
  providers:
    kimi:
      kind: kimi
      api_key: kimi-key
    anthropic:
      kind: anthropic
      api_key: ant-key
`))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	updates := map[string]any{
		"llm_providers": map[string]any{
			"kimi": map[string]any{
				"kind":    "kimi",
				"api_key": "kimi-key",
			},
		},
	}
	if err := PatchYAML(path, updates); err != nil {
		t.Fatalf("patch yaml: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load patched config: %v", err)
	}
	if _, exists := cfg.LLMProviders["anthropic"]; exists {
		t.Fatalf("anthropic should have been removed by alias-replace patch")
	}
	if cfg.LLMProviders["kimi"].APIKey != "kimi-key" {
		t.Fatalf("kimi entry corrupted: %+v", cfg.LLMProviders["kimi"])
	}
}

func TestPatchYAML_CreatesParentDirectory(t *testing.T) {
	root := t.TempDir()
	// Use a path whose parent does NOT exist yet — simulates the
	// wizard's first save against ~/.tars/config/ on a brand-new
	// install.
	path := filepath.Join(root, "fresh", "nested", "config.yaml")
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("expected parent dir to be missing before PatchYAML; stat err=%v", err)
	}

	updates := map[string]any{
		"workspace_dir": filepath.Join(root, "ws"),
	}
	if err := PatchYAML(path, updates); err != nil {
		t.Fatalf("patch yaml on missing parent dir: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file to be created at %s, got err=%v", path, err)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("expected parent dir to exist after PatchYAML, got err=%v", err)
	}
}
