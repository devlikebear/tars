package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadYAML_HierarchicalSections(t *testing.T) {
	t.Setenv("TARS_TEST_PROVIDER_KEY", "sk-test-provider")

	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte(`
runtime:
  workspace_dir: ./workspace
  session:
    default_id: main

log:
  level: debug
  file: ./logs/test.log

api:
  auth_mode: required
  max_inflight:
    chat: 3
    agent_runs: 4

dashboard:
  auth_mode: off

llm:
  providers:
    codex:
      kind: openai-codex
      auth_mode: oauth
      oauth_provider: openai-codex
      base_url: https://chatgpt.com/backend-api
      service_tier: priority
    anthropic:
      kind: anthropic
      auth_mode: api-key
      api_key: ${TARS_TEST_PROVIDER_KEY}
  tiers:
    heavy:
      provider: codex
      model: gpt-5.4
      reasoning_effort: high
    standard:
      provider: anthropic
      model: claude-sonnet-4-6
      reasoning_effort: medium
    light:
      provider: codex
      model: gpt-5.4-mini
      reasoning_effort: minimal
  default_tier: standard
  role_defaults:
    chat_main: standard
    pulse_decider: light

automation:
  agent:
    max_iterations: 12
  pulse:
    active_hours: 00:00-24:00
    timezone: Asia/Seoul
    allowed_autofixes: [compress_old_logs, cleanup_stale_tmp]
  reflection:
    sleep_window: 02:00-05:00
    timezone: Asia/Seoul
  cron:
    run_history_limit: 210
  notify:
    command: /tmp/notify.sh
    when_no_clients: true

tools:
  default_set: safe
  allow_high_risk_user: true
  web_search:
    enabled: true
    provider: perplexity
    api_key: search-key
    cache_ttl_seconds: 77
    perplexity:
      api_key: pplx-key
      model: sonar-pro
      base_url: https://perplexity.example
  web_fetch:
    enabled: true
    allow_private_hosts: true
    private_host_allowlist: [localhost, 127.0.0.1]
  apply_patch:
    enabled: true
  browser:
    enabled: true
  agentruntime:
    enabled: true

browser:
  runtime:
    enabled: true
  default_profile: managed
  managed:
    headless: true
    executable_path: /Applications/Test.app/Contents/MacOS/Test
    user_data_dir: /tmp/browser
  site_flows_dir: /tmp/site-flows
  auto_login:
    site_allowlist: [intranet, grafana]

channels:
  local:
    enabled: true
  webhook:
    enabled: true
  telegram:
    enabled: true
    dm_policy: pairing
    polling:
      enabled: true
    bot_token: yaml-bot-token

extensions:
  skills:
    enabled: true
    extra_dirs: [./team-skills]
  plugins:
    enabled: true
    extra_dirs: [./team-plugins]
  mcp:
    command_allowlist: [npx, uvx]
    servers:
      - name: filesystem
        command: npx
        args: ["-y", "@modelcontextprotocol/server-filesystem", "."]

agentruntime:
  enabled: true
  default_agent: worker
  agents:
    list:
      - name: worker
        command: python3
        args: ["./worker.py"]
        enabled: true
    watch: true
    watch_debounce_ms: 33
  task_override:
    enabled: true
    allowed_aliases: [codex]
    allowed_models: [gpt-5.4]
  persistence:
    enabled: true
    dir: /tmp/agentruntime
  runs:
    persistence_enabled: true
    max_records: 123
  channels:
    persistence_enabled: true
    max_messages_per_channel: 45
  subagents:
    max_threads: 6
    max_depth: 2
  consensus:
    enabled: true
    max_fanout: 4
    budget_tokens: 8000
    budget_usd: 2.5
    timeout_seconds: 90
    allowed_aliases: [codex, anthropic]
    concurrent_runs: 2
  restore_on_startup: true
  report:
    summary_enabled: true
  archive:
    enabled: true
    dir: /tmp/archive
    retention_days: 7
    max_file_bytes: 4096
`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadYAML(path)
	if err != nil {
		t.Fatalf("loadYAML: %v", err)
	}

	if cfg.WorkspaceDir != "./workspace" || cfg.SessionDefaultID != "main" {
		t.Fatalf("runtime fields not parsed: workspace=%q session=%q", cfg.WorkspaceDir, cfg.SessionDefaultID)
	}
	if cfg.LogLevel != "debug" || cfg.LogFile != "./logs/test.log" {
		t.Fatalf("log fields not parsed: level=%q file=%q", cfg.LogLevel, cfg.LogFile)
	}
	if cfg.APIAuthMode != "required" || cfg.APIMaxInflightChat != 3 || cfg.APIMaxInflightAgentRuns != 4 || cfg.DashboardAuthMode != "off" {
		t.Fatalf("api/dashboard fields not parsed: auth=%q chat=%d runs=%d dashboard=%q", cfg.APIAuthMode, cfg.APIMaxInflightChat, cfg.APIMaxInflightAgentRuns, cfg.DashboardAuthMode)
	}
	if cfg.LLMProviders["anthropic"].APIKey != "sk-test-provider" {
		t.Fatalf("nested llm provider env expansion failed: %+v", cfg.LLMProviders["anthropic"])
	}
	if cfg.LLMTiers["standard"].Provider != "anthropic" || cfg.LLMDefaultTier != "standard" || cfg.LLMRoleDefaults["pulse_decider"] != "light" {
		t.Fatalf("llm hierarchy not parsed: tiers=%+v default=%q roles=%+v", cfg.LLMTiers, cfg.LLMDefaultTier, cfg.LLMRoleDefaults)
	}
	if cfg.AgentMaxIterations != 12 || cfg.CronRunHistoryLimit != 210 || cfg.NotifyCommand != "/tmp/notify.sh" || !cfg.NotifyWhenNoClients {
		t.Fatalf("automation fields not parsed: iterations=%d cron=%d notify=%q no_clients=%t", cfg.AgentMaxIterations, cfg.CronRunHistoryLimit, cfg.NotifyCommand, cfg.NotifyWhenNoClients)
	}
	if cfg.PulseActiveHours != "00:00-24:00" || cfg.PulseTimezone != "Asia/Seoul" || !reflect.DeepEqual(cfg.PulseAllowedAutofixes, []string{"compress_old_logs", "cleanup_stale_tmp"}) {
		t.Fatalf("pulse fields not parsed: hours=%q timezone=%q allowlist=%+v", cfg.PulseActiveHours, cfg.PulseTimezone, cfg.PulseAllowedAutofixes)
	}
	if cfg.ReflectionSleepWindow != "02:00-05:00" || cfg.ReflectionTimezone != "Asia/Seoul" {
		t.Fatalf("reflection fields not parsed: window=%q timezone=%q", cfg.ReflectionSleepWindow, cfg.ReflectionTimezone)
	}
	if !cfg.ToolsWebSearchEnabled || cfg.ToolsWebSearchProvider != "perplexity" || cfg.ToolsWebSearchPerplexityAPIKey != "pplx-key" {
		t.Fatalf("tool hierarchy not parsed: enabled=%t provider=%q pplx=%q", cfg.ToolsWebSearchEnabled, cfg.ToolsWebSearchProvider, cfg.ToolsWebSearchPerplexityAPIKey)
	}
	if !reflect.DeepEqual(cfg.ToolsWebFetchPrivateHostAllowlist, []string{"localhost", "127.0.0.1"}) || !cfg.ToolsApplyPatchEnabled || !cfg.ToolsAgentRuntimeEnabled {
		t.Fatalf("tool leaf collections not parsed: hosts=%+v patch=%t agentruntime=%t", cfg.ToolsWebFetchPrivateHostAllowlist, cfg.ToolsApplyPatchEnabled, cfg.ToolsAgentRuntimeEnabled)
	}
	if !cfg.ChannelsTelegramEnabled || cfg.TelegramBotToken != "yaml-bot-token" || !cfg.ChannelsTelegramPollingEnabled {
		t.Fatalf("channel hierarchy not parsed: telegram=%t token=%q polling=%t", cfg.ChannelsTelegramEnabled, cfg.TelegramBotToken, cfg.ChannelsTelegramPollingEnabled)
	}
	if !reflect.DeepEqual(cfg.SkillsExtraDirs, []string{"./team-skills"}) || !reflect.DeepEqual(cfg.PluginsExtraDirs, []string{"./team-plugins"}) {
		t.Fatalf("extension hierarchy not parsed: skills=%+v plugins=%+v", cfg.SkillsExtraDirs, cfg.PluginsExtraDirs)
	}
	if len(cfg.MCPServers) != 1 || cfg.MCPServers[0].Name != "filesystem" {
		t.Fatalf("mcp hierarchy not parsed: %+v", cfg.MCPServers)
	}
	if !cfg.AgentRuntimeEnabled || cfg.AgentRuntimeDefaultAgent != "worker" || len(cfg.AgentRuntimeAgents) != 1 || cfg.AgentRuntimeAgents[0].Name != "worker" {
		t.Fatalf("agent runtime agents not parsed: enabled=%t default=%q agents=%+v", cfg.AgentRuntimeEnabled, cfg.AgentRuntimeDefaultAgent, cfg.AgentRuntimeAgents)
	}
	if !cfg.AgentRuntimeTaskOverride.Enabled || !reflect.DeepEqual(cfg.AgentRuntimeTaskOverride.AllowedAliases, []string{"codex"}) {
		t.Fatalf("agent runtime task override not parsed: %+v", cfg.AgentRuntimeTaskOverride)
	}
	if cfg.AgentRuntimePersistenceDir != "/tmp/agentruntime" || cfg.AgentRuntimeRunsMaxRecords != 123 || cfg.AgentRuntimeChannelsMaxMessagesPerChannel != 45 {
		t.Fatalf("agent runtime persistence hierarchy not parsed: dir=%q runs=%d channels=%d", cfg.AgentRuntimePersistenceDir, cfg.AgentRuntimeRunsMaxRecords, cfg.AgentRuntimeChannelsMaxMessagesPerChannel)
	}
	if cfg.AgentRuntimeSubagentsMaxThreads != 6 || cfg.AgentRuntimeSubagentsMaxDepth != 2 || !cfg.AgentRuntimeConsensusEnabled || !reflect.DeepEqual(cfg.AgentRuntimeConsensusAllowedAliases, []string{"codex", "anthropic"}) {
		t.Fatalf("agent runtime nested sections not parsed: threads=%d depth=%d consensus=%t aliases=%+v", cfg.AgentRuntimeSubagentsMaxThreads, cfg.AgentRuntimeSubagentsMaxDepth, cfg.AgentRuntimeConsensusEnabled, cfg.AgentRuntimeConsensusAllowedAliases)
	}
	if !cfg.AgentRuntimeArchiveEnabled || cfg.AgentRuntimeArchiveDir != "/tmp/archive" || cfg.AgentRuntimeArchiveMaxFileBytes != 4096 {
		t.Fatalf("agent runtime archive hierarchy not parsed: enabled=%t dir=%q bytes=%d", cfg.AgentRuntimeArchiveEnabled, cfg.AgentRuntimeArchiveDir, cfg.AgentRuntimeArchiveMaxFileBytes)
	}
}

func TestLoad_HierarchicalYAMLRespectsFlatAndEnvPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte(`
workspace_dir: ./flat-workspace
tools_web_search_provider: brave
agentruntime_persistence_dir: /tmp/flat-agentruntime

runtime:
  workspace_dir: ./nested-workspace

tools:
  web_search:
    provider: perplexity

agentruntime:
  persistence:
    dir: /tmp/nested-agentruntime
`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("AGENTRUNTIME_PERSISTENCE_DIR", "/tmp/env-agentruntime")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.WorkspaceDir != "./flat-workspace" {
		t.Fatalf("expected flat workspace_dir to win over nested alias, got %q", cfg.WorkspaceDir)
	}
	if cfg.ToolsWebSearchProvider != "brave" {
		t.Fatalf("expected flat tools_web_search_provider to win over nested alias, got %q", cfg.ToolsWebSearchProvider)
	}
	if cfg.AgentRuntimePersistenceDir != "/tmp/env-agentruntime" {
		t.Fatalf("expected env override to win over file config, got %q", cfg.AgentRuntimePersistenceDir)
	}
}
