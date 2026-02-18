package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCased_RequiresTargetCommand(t *testing.T) {
	_, err := LoadCased("")
	if err == nil {
		t.Fatal("expected error when target_command is missing")
	}
	if !strings.Contains(err.Error(), "target_command") {
		t.Fatalf("expected target_command validation error, got %v", err)
	}
}

func TestLoadCased_DefaultsAndEnvOverride(t *testing.T) {
	t.Setenv("CASED_TARGET_COMMAND", "go")
	t.Setenv("CASED_TARGET_ARGS_JSON", `["run","./cmd/tarsd"]`)
	t.Setenv("CASED_PROBE_INTERVAL_MS", "1500")
	t.Setenv("CASED_AUTOSTART", "false")

	cfg, err := LoadCased("")
	if err != nil {
		t.Fatalf("load cased config: %v", err)
	}
	if cfg.APIAddr != "127.0.0.1:43181" {
		t.Fatalf("expected default api_addr, got %q", cfg.APIAddr)
	}
	if cfg.TargetCommand != "go" {
		t.Fatalf("expected env target_command, got %q", cfg.TargetCommand)
	}
	if len(cfg.TargetArgs) != 2 || cfg.TargetArgs[0] != "run" || cfg.TargetArgs[1] != "./cmd/tarsd" {
		t.Fatalf("unexpected target_args: %+v", cfg.TargetArgs)
	}
	if cfg.ProbeURL != "http://127.0.0.1:43180/v1/healthz" {
		t.Fatalf("expected default probe_url, got %q", cfg.ProbeURL)
	}
	if cfg.ProbeIntervalMS != 1500 {
		t.Fatalf("expected probe_interval_ms=1500, got %d", cfg.ProbeIntervalMS)
	}
	if cfg.Autostart {
		t.Fatalf("expected autostart=false from env")
	}
}

func TestLoadCased_YAMLAndEnvPrecedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cased.yaml")
	content := strings.Join([]string{
		"api_addr: 127.0.0.1:50000",
		"target_command: ./bin/tarsd",
		"target_args_json: '[\"--serve-api\",\"--api-addr\",\"127.0.0.1:43180\"]'",
		"target_working_dir: ./workspace",
		"target_env_json: '{\"FOO\":\"BAR\"}'",
		"probe_url: http://127.0.0.1:43180/v1/healthz",
		"probe_fail_threshold: 5",
		"restart_max_attempts: 4",
		"event_buffer_size: 512",
		"autostart: true",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("CASED_RESTART_MAX_ATTEMPTS", "3")
	t.Setenv("CASED_EVENT_BUFFER_SIZE", "200")

	cfg, err := LoadCased(path)
	if err != nil {
		t.Fatalf("load cased config: %v", err)
	}
	if cfg.APIAddr != "127.0.0.1:50000" {
		t.Fatalf("expected yaml api_addr, got %q", cfg.APIAddr)
	}
	if cfg.TargetCommand != "./bin/tarsd" {
		t.Fatalf("expected yaml target_command, got %q", cfg.TargetCommand)
	}
	if len(cfg.TargetArgs) != 3 {
		t.Fatalf("expected 3 args, got %+v", cfg.TargetArgs)
	}
	if cfg.TargetWorkingDir != "./workspace" {
		t.Fatalf("expected target_working_dir from yaml, got %q", cfg.TargetWorkingDir)
	}
	if cfg.TargetEnv["FOO"] != "BAR" {
		t.Fatalf("expected target_env_json parsed, got %+v", cfg.TargetEnv)
	}
	if cfg.RestartMaxAttempts != 3 {
		t.Fatalf("expected env override restart_max_attempts=3, got %d", cfg.RestartMaxAttempts)
	}
	if cfg.EventBufferSize != 200 {
		t.Fatalf("expected env override event_buffer_size=200, got %d", cfg.EventBufferSize)
	}
}

func TestResolveCasedConfigPath(t *testing.T) {
	t.Setenv("CASED_CONFIG", "/tmp/from-env.yaml")
	if got := ResolveCasedConfigPath("./custom.yaml"); got != "./custom.yaml" {
		t.Fatalf("expected explicit config path, got %q", got)
	}
	if got := ResolveCasedConfigPath(""); got != "/tmp/from-env.yaml" {
		t.Fatalf("expected env config path, got %q", got)
	}

	t.Setenv("CASED_CONFIG", "")
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	defaultPath := filepath.Join(configDir, "cased.yaml")
	if err := os.WriteFile(defaultPath, []byte("target_command: ./bin/tarsd\n"), 0o644); err != nil {
		t.Fatalf("write default config: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}
	defer func() { _ = os.Chdir(wd) }()

	if got := ResolveCasedConfigPath(""); got != DefaultCasedConfigFilename {
		t.Fatalf("expected default cased config filename, got %q", got)
	}
}

func TestLoadCased_APIAuthDefaults(t *testing.T) {
	t.Setenv("CASED_TARGET_COMMAND", "go")
	cfg, err := LoadCased("")
	if err != nil {
		t.Fatalf("load cased config: %v", err)
	}
	if cfg.APIAuthMode != "external-required" {
		t.Fatalf("expected api auth mode external-required, got %q", cfg.APIAuthMode)
	}
}

func TestLoadCased_APIAuthYAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cased.yaml")
	content := strings.Join([]string{
		"target_command: ./bin/tarsd",
		"api_auth_mode: required",
		"api_auth_token: yaml-cased-token",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CASED_API_AUTH_MODE", "off")
	t.Setenv("CASED_API_AUTH_TOKEN", "env-cased-token")

	cfg, err := LoadCased(path)
	if err != nil {
		t.Fatalf("load cased config: %v", err)
	}
	if cfg.APIAuthMode != "off" {
		t.Fatalf("expected env override api auth mode off, got %q", cfg.APIAuthMode)
	}
	if cfg.APIAuthToken != "env-cased-token" {
		t.Fatalf("expected env override api auth token, got %q", cfg.APIAuthToken)
	}
}
