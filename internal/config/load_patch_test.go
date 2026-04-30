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
