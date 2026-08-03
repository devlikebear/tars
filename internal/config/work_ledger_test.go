package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkLedgerConfigSupportsNonDestructiveRollback(t *testing.T) {
	t.Run("default enabled", func(t *testing.T) {
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("load defaults: %v", err)
		}
		if !cfg.WorkLedger.Enabled {
			t.Fatal("work ledger should be enabled by default")
		}
	})

	t.Run("yaml disables default", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte("work_ledger:\n  enabled: false\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load yaml: %v", err)
		}
		if cfg.WorkLedger.Enabled {
			t.Fatal("work ledger should be disabled by yaml")
		}
	})

	t.Run("environment disables default", func(t *testing.T) {
		t.Setenv("TARS_WORK_LEDGER_ENABLED", "false")
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("load environment: %v", err)
		}
		if cfg.WorkLedger.Enabled {
			t.Fatal("work ledger should be disabled by environment")
		}
	})

	t.Run("settings patch preserves false", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte("work_ledger:\n  enabled: true\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if err := PatchYAML(path, map[string]any{"work_ledger_enabled": false}); err != nil {
			t.Fatalf("patch config: %v", err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read patched config: %v", err)
		}
		if !strings.Contains(string(raw), "enabled: false") {
			t.Fatalf("patched config did not preserve false:\n%s", raw)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load patched config: %v", err)
		}
		if cfg.WorkLedger.Enabled {
			t.Fatal("work ledger should remain disabled after settings patch")
		}
		if got := ConfigToMap(cfg)["work_ledger_enabled"]; got != false {
			t.Fatalf("config map work_ledger_enabled = %#v, want false", got)
		}
	})
}

func TestConfigSchemaIncludesWorkLedgerRollbackSwitch(t *testing.T) {
	for _, field := range Schema() {
		if field.Key != "work_ledger_enabled" {
			continue
		}
		if field.Section != "Work Ledger" || field.Path != "work_ledger.enabled" || field.Type != "bool" {
			t.Fatalf("unexpected work ledger schema: %+v", field)
		}
		if !field.RequiresRestart {
			t.Fatal("work ledger rollback switch should require restart")
		}
		return
	}
	t.Fatal("schema missing work_ledger_enabled")
}

func TestWorkSchedulerConfigSupportsIndependentRollbackAndRuntimeTuning(t *testing.T) {
	t.Run("safe defaults", func(t *testing.T) {
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("load defaults: %v", err)
		}
		if cfg.WorkLedger.SchedulerEnabled || cfg.WorkLedger.SchedulerMaxWorkers != 4 || cfg.WorkLedger.SchedulerLeaseSeconds != 60 || cfg.WorkLedger.SchedulerHeartbeatSeconds != 20 || cfg.WorkLedger.SchedulerPollMilliseconds != 250 || cfg.WorkLedger.SchedulerExecutionEnvironment != "local" || strings.TrimSpace(cfg.WorkLedger.SchedulerExecutionDataDir) == "" || len(cfg.WorkLedger.SchedulerArtifactPaths) != 0 || cfg.WorkLedger.SchedulerExternalHarnessConfigPath != "" {
			t.Fatalf("work scheduler defaults = %+v", cfg.WorkLedger)
		}
	})

	t.Run("yaml enables scheduler and tunes runtime", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		raw := "work_ledger:\n  enabled: true\n  scheduler:\n    enabled: true\n    max_workers: 2\n    lease_seconds: 90\n    heartbeat_seconds: 30\n    poll_milliseconds: 500\n    execution_environment: managed-worktree\n    execution_data_dir: /tmp/tars-execution\n    artifact_paths: [reports, '*.patch']\n    external_harness:\n      config_path: /tmp/tars-claude-code.json\n"
		if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
			t.Fatalf("write scheduler config: %v", err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load scheduler config: %v", err)
		}
		if !cfg.WorkLedger.Enabled || !cfg.WorkLedger.SchedulerEnabled || cfg.WorkLedger.SchedulerMaxWorkers != 2 || cfg.WorkLedger.SchedulerLeaseSeconds != 90 || cfg.WorkLedger.SchedulerHeartbeatSeconds != 30 || cfg.WorkLedger.SchedulerPollMilliseconds != 500 || cfg.WorkLedger.SchedulerExecutionEnvironment != "managed-worktree" || cfg.WorkLedger.SchedulerExecutionDataDir != "/tmp/tars-execution" || len(cfg.WorkLedger.SchedulerArtifactPaths) != 2 || cfg.WorkLedger.SchedulerExternalHarnessConfigPath != "/tmp/tars-claude-code.json" {
			t.Fatalf("loaded work scheduler config = %+v", cfg.WorkLedger)
		}
	})

	t.Run("environment enables scheduler", func(t *testing.T) {
		t.Setenv("TARS_WORK_SCHEDULER_ENABLED", "true")
		t.Setenv("TARS_WORK_SCHEDULER_EXECUTION_ENVIRONMENT", "managed-worktree")
		t.Setenv("TARS_WORK_SCHEDULER_EXECUTION_DATA_DIR", "/tmp/tars-env-execution")
		t.Setenv("TARS_WORK_SCHEDULER_ARTIFACT_PATHS_JSON", `["dist","reports/*.json"]`)
		t.Setenv("TARS_WORK_SCHEDULER_EXTERNAL_HARNESS_CONFIG_PATH", "/tmp/tars-env-claude-code.json")
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("load scheduler environment: %v", err)
		}
		if !cfg.WorkLedger.SchedulerEnabled {
			t.Fatal("work scheduler should be enabled by environment")
		}
		if cfg.WorkLedger.SchedulerExecutionEnvironment != "managed-worktree" || cfg.WorkLedger.SchedulerExecutionDataDir != "/tmp/tars-env-execution" || len(cfg.WorkLedger.SchedulerArtifactPaths) != 2 || cfg.WorkLedger.SchedulerExternalHarnessConfigPath != "/tmp/tars-env-claude-code.json" {
			t.Fatalf("work scheduler execution environment = %+v", cfg.WorkLedger)
		}
	})
}

func TestConfigSchemaIncludesDurableSchedulerSettings(t *testing.T) {
	wantPaths := map[string]string{
		"work_scheduler_enabled":                      "work_ledger.scheduler.enabled",
		"work_scheduler_max_workers":                  "work_ledger.scheduler.max_workers",
		"work_scheduler_lease_seconds":                "work_ledger.scheduler.lease_seconds",
		"work_scheduler_heartbeat_seconds":            "work_ledger.scheduler.heartbeat_seconds",
		"work_scheduler_poll_milliseconds":            "work_ledger.scheduler.poll_milliseconds",
		"work_scheduler_execution_environment":        "work_ledger.scheduler.execution_environment",
		"work_scheduler_execution_data_dir":           "work_ledger.scheduler.execution_data_dir",
		"work_scheduler_artifact_paths_json":          "work_ledger.scheduler.artifact_paths",
		"work_scheduler_external_harness_config_path": "work_ledger.scheduler.external_harness.config_path",
	}
	for _, field := range Schema() {
		path, wanted := wantPaths[field.Key]
		if !wanted {
			continue
		}
		if field.Section != "Work Scheduler" || field.Path != path || !field.RequiresRestart {
			t.Fatalf("unexpected work scheduler schema for %s: %+v", field.Key, field)
		}
		delete(wantPaths, field.Key)
	}
	if len(wantPaths) != 0 {
		t.Fatalf("schema missing work scheduler settings: %+v", wantPaths)
	}
}

func TestRemoteWorkerAndA2ASchedulerConfigIsOptInAndSourceAware(t *testing.T) {
	t.Run("safe defaults", func(t *testing.T) {
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("load defaults: %v", err)
		}
		if cfg.WorkLedger.SchedulerRemoteWorkersEnabled || cfg.WorkLedger.SchedulerRemoteWorkersGatewayConfigPath != "" || cfg.WorkLedger.SchedulerA2AEnabled ||
			cfg.WorkLedger.SchedulerA2ADiscoveryURL != "" || cfg.WorkLedger.SchedulerA2ABearerToken != "" ||
			len(cfg.WorkLedger.SchedulerA2AAllowedHosts) != 0 || cfg.WorkLedger.SchedulerA2AAllowPrivateHosts ||
			cfg.WorkLedger.SchedulerA2AAllowInsecureLoopback || cfg.WorkLedger.SchedulerA2APollMilliseconds != 2000 ||
			cfg.WorkLedger.SchedulerA2AMaxPollSeconds != 1800 {
			t.Fatalf("unsafe remote scheduler defaults: %+v", cfg.WorkLedger)
		}
	})

	t.Run("yaml", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		raw := `work_ledger:
  scheduler:
    remote_workers:
      enabled: true
      gateway_config_path: /tmp/tars-remote-gateway.json
    a2a:
      enabled: true
      discovery_url: https://agent.example.test
      bearer_token: gateway-only
      allowed_hosts: [agent.example.test, api.example.test]
      allow_private_hosts: true
      allow_insecure_loopback: false
      poll_milliseconds: 125
      max_poll_seconds: 75
`
		if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load yaml config: %v", err)
		}
		if !cfg.WorkLedger.SchedulerRemoteWorkersEnabled || cfg.WorkLedger.SchedulerRemoteWorkersGatewayConfigPath != "/tmp/tars-remote-gateway.json" || !cfg.WorkLedger.SchedulerA2AEnabled ||
			cfg.WorkLedger.SchedulerA2ADiscoveryURL != "https://agent.example.test" ||
			cfg.WorkLedger.SchedulerA2ABearerToken != "gateway-only" || len(cfg.WorkLedger.SchedulerA2AAllowedHosts) != 2 ||
			!cfg.WorkLedger.SchedulerA2AAllowPrivateHosts || cfg.WorkLedger.SchedulerA2AAllowInsecureLoopback ||
			cfg.WorkLedger.SchedulerA2APollMilliseconds != 125 || cfg.WorkLedger.SchedulerA2AMaxPollSeconds != 75 {
			t.Fatalf("loaded remote scheduler config: %+v", cfg.WorkLedger)
		}
	})

	t.Run("environment", func(t *testing.T) {
		t.Setenv("TARS_WORK_SCHEDULER_REMOTE_WORKERS_ENABLED", "true")
		t.Setenv("TARS_WORK_SCHEDULER_REMOTE_WORKERS_GATEWAY_CONFIG_PATH", "/tmp/tars-env-remote-gateway.json")
		t.Setenv("TARS_WORK_SCHEDULER_A2A_ENABLED", "true")
		t.Setenv("TARS_WORK_SCHEDULER_A2A_DISCOVERY_URL", "https://env-agent.example.test")
		t.Setenv("TARS_WORK_SCHEDULER_A2A_BEARER_TOKEN", "env-secret")
		t.Setenv("TARS_WORK_SCHEDULER_A2A_ALLOWED_HOSTS_JSON", `["env-agent.example.test"]`)
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("load environment config: %v", err)
		}
		if !cfg.WorkLedger.SchedulerRemoteWorkersEnabled || cfg.WorkLedger.SchedulerRemoteWorkersGatewayConfigPath != "/tmp/tars-env-remote-gateway.json" || !cfg.WorkLedger.SchedulerA2AEnabled ||
			cfg.WorkLedger.SchedulerA2ADiscoveryURL != "https://env-agent.example.test" ||
			cfg.WorkLedger.SchedulerA2ABearerToken != "env-secret" || len(cfg.WorkLedger.SchedulerA2AAllowedHosts) != 1 {
			t.Fatalf("loaded remote scheduler environment: %+v", cfg.WorkLedger)
		}
	})
}

func TestConfigSchemaMarksA2ATokenSensitive(t *testing.T) {
	wantPaths := map[string]string{
		"work_scheduler_remote_workers_enabled":             "work_ledger.scheduler.remote_workers.enabled",
		"work_scheduler_remote_workers_gateway_config_path": "work_ledger.scheduler.remote_workers.gateway_config_path",
		"work_scheduler_a2a_enabled":                        "work_ledger.scheduler.a2a.enabled",
		"work_scheduler_a2a_discovery_url":                  "work_ledger.scheduler.a2a.discovery_url",
		"work_scheduler_a2a_bearer_token":                   "work_ledger.scheduler.a2a.bearer_token",
		"work_scheduler_a2a_allowed_hosts_json":             "work_ledger.scheduler.a2a.allowed_hosts",
		"work_scheduler_a2a_allow_private_hosts":            "work_ledger.scheduler.a2a.allow_private_hosts",
		"work_scheduler_a2a_allow_insecure_loopback":        "work_ledger.scheduler.a2a.allow_insecure_loopback",
		"work_scheduler_a2a_poll_milliseconds":              "work_ledger.scheduler.a2a.poll_milliseconds",
		"work_scheduler_a2a_max_poll_seconds":               "work_ledger.scheduler.a2a.max_poll_seconds",
	}
	for _, field := range Schema() {
		path, ok := wantPaths[field.Key]
		if !ok {
			continue
		}
		if field.Section != "Work Scheduler" || field.Path != path || !field.RequiresRestart {
			t.Fatalf("unexpected remote scheduler schema for %s: %+v", field.Key, field)
		}
		if field.Key == "work_scheduler_a2a_bearer_token" && !field.Sensitive {
			t.Fatal("A2A bearer token must be marked sensitive")
		}
		delete(wantPaths, field.Key)
	}
	if len(wantPaths) != 0 {
		t.Fatalf("schema missing remote scheduler settings: %+v", wantPaths)
	}
}
