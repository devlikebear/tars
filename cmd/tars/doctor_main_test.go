package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

func TestRootCommand_DoctorFailsForMissingStarterState(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	clearDoctorEnv(t)

	workspaceDir := filepath.Join(t.TempDir(), "doctor-workspace")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"doctor", "--workspace-dir", workspaceDir})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected doctor to fail when starter config and workspace are missing")
	}

	out := stdout.String()
	if !strings.Contains(out, "doctor: FAIL") {
		t.Fatalf("expected FAIL summary, got:\n%s", out)
	}
	if !strings.Contains(out, "config file") {
		t.Fatalf("expected config file check in output, got:\n%s", out)
	}
	if !strings.Contains(out, "--fix") {
		t.Fatalf("expected fix guidance in output, got:\n%s", out)
	}
}

func TestRootCommand_DoctorFixCreatesStarterWorkspaceButStillRequiresBYOK(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	clearDoctorEnv(t)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	workspaceDir := filepath.Join(t.TempDir(), "doctor-workspace")
	workspaceAbs, err := filepath.Abs(workspaceDir)
	if err != nil {
		t.Fatalf("workspace abs path: %v", err)
	}

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"doctor", "--workspace-dir", workspaceDir, "--fix"})

	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected doctor --fix to keep failing until BYOK is configured")
	}
	if !strings.Contains(err.Error(), "failing checks") {
		t.Fatalf("expected failing checks error, got %v", err)
	}

	configPath := config.FixedConfigPath()
	assertPathExists(t, configPath)
	assertPathExists(t, filepath.Join(workspaceAbs, "memory"))
	assertPathExists(t, filepath.Join(workspaceAbs, "MEMORY.md"))
	assertPathExists(t, filepath.Join(workspaceAbs, "plugins", "project-swarm", "tars.plugin.json"))
	assertPathExists(t, filepath.Join(workspaceAbs, "plugins", "ops-service", "tars.plugin.json"))

	out := stdout.String()
	if !strings.Contains(out, "[fixed] config file") {
		t.Fatalf("expected fixed config check in output, got:\n%s", out)
	}
	if !strings.Contains(out, "OPENAI_API_KEY") {
		t.Fatalf("expected OPENAI_API_KEY guidance in output, got:\n%s", out)
	}
}

func TestRootCommand_DoctorPassesWhenStarterWorkspaceAndBYOKPresent(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	clearDoctorEnv(t)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	workspaceDir := filepath.Join(t.TempDir(), "doctor-workspace")
	var initStdout strings.Builder
	initCmd := newRootCommand(strings.NewReader(""), &initStdout, io.Discard)
	initCmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
	}

	var doctorStdout strings.Builder
	doctorCmd := newRootCommand(strings.NewReader(""), &doctorStdout, io.Discard)
	doctorCmd.SetArgs([]string{"doctor", "--workspace-dir", workspaceDir})
	if err := doctorCmd.Execute(); err != nil {
		t.Fatalf("doctor command: %v", err)
	}

	out := doctorStdout.String()
	if !strings.Contains(out, "doctor: PASS") {
		t.Fatalf("expected PASS summary, got:\n%s", out)
	}
	if strings.Contains(out, "[fail]") {
		t.Fatalf("expected no failing checks, got:\n%s", out)
	}
}

func TestRootCommand_DoctorFixRestoresBundledWorkspacePlugin(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	clearDoctorEnv(t)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	workspaceDir := filepath.Join(t.TempDir(), "doctor-workspace")
	var initStdout strings.Builder
	initCmd := newRootCommand(strings.NewReader(""), &initStdout, io.Discard)
	initCmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
	}

	workspaceAbs, err := filepath.Abs(workspaceDir)
	if err != nil {
		t.Fatalf("workspace abs path: %v", err)
	}
	manifestPath := filepath.Join(workspaceAbs, "plugins", "project-swarm", "tars.plugin.json")
	if err := os.Remove(manifestPath); err != nil {
		t.Fatalf("remove plugin manifest: %v", err)
	}

	var doctorStdout strings.Builder
	doctorCmd := newRootCommand(strings.NewReader(""), &doctorStdout, io.Discard)
	doctorCmd.SetArgs([]string{"doctor", "--workspace-dir", workspaceDir, "--fix"})
	if err := doctorCmd.Execute(); err != nil {
		t.Fatalf("doctor --fix command: %v", err)
	}

	assertPathExists(t, manifestPath)
	assertPathExists(t, filepath.Join(workspaceAbs, "plugins", "ops-service", "tars.plugin.json"))
}

func TestRootCommand_DoctorWarnsWhenGatewayDisabledForProjectWorkflow(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	clearDoctorEnv(t)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	workspaceDir := filepath.Join(t.TempDir(), "doctor-workspace")
	var initStdout strings.Builder
	initCmd := newRootCommand(strings.NewReader(""), &initStdout, io.Discard)
	initCmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
	}

	configPath := config.FixedConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	configText := strings.Replace(string(data), "gateway_enabled: true", "gateway_enabled: false", 1)
	if err := os.WriteFile(configPath, []byte(configText), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var doctorStdout strings.Builder
	doctorCmd := newRootCommand(strings.NewReader(""), &doctorStdout, io.Discard)
	doctorCmd.SetArgs([]string{"doctor", "--workspace-dir", workspaceDir})
	if err := doctorCmd.Execute(); err != nil {
		t.Fatalf("doctor command: %v", err)
	}

	out := doctorStdout.String()
	if !strings.Contains(out, "[warn] project workflow gateway") {
		t.Fatalf("expected gateway warning, got:\n%s", out)
	}
	if !strings.Contains(out, "gateway_enabled: true") {
		t.Fatalf("expected gateway enable guidance, got:\n%s", out)
	}
}

func TestRootCommand_DoctorFailsWhenClaudeCodeCLIIsMissing(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	clearDoctorEnv(t)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))
	t.Setenv("CLAUDE_CODE_CLI_PATH", filepath.Join(t.TempDir(), "missing-claude"))

	workspaceDir := filepath.Join(t.TempDir(), "doctor-workspace")
	var initStdout strings.Builder
	initCmd := newRootCommand(strings.NewReader(""), &initStdout, io.Discard)
	initCmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
	}

	configPath := config.FixedConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	configText := strings.Replace(string(data), "llm_provider: openai", "llm_provider: claude-code-cli", 1)
	configText = strings.Replace(configText, "llm_auth_mode: api-key", "llm_auth_mode: cli", 1)
	configText = strings.Replace(configText, "llm_base_url: https://api.openai.com/v1", "llm_base_url: \"\"", 1)
	configText = strings.Replace(configText, "llm_api_key: ${OPENAI_API_KEY}", "llm_api_key: \"\"", 1)
	if err := os.WriteFile(configPath, []byte(configText), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var doctorStdout strings.Builder
	doctorCmd := newRootCommand(strings.NewReader(""), &doctorStdout, io.Discard)
	doctorCmd.SetArgs([]string{"doctor", "--workspace-dir", workspaceDir})
	err = doctorCmd.Execute()
	if err == nil {
		t.Fatal("expected doctor to fail when claude cli is missing")
	}

	out := doctorStdout.String()
	if !strings.Contains(out, "claude") {
		t.Fatalf("expected claude cli guidance, got:\n%s", out)
	}
}

func TestRootCommand_DoctorFailsWhenGatewayDefaultAgentUsesMissingWorkspaceCommand(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	clearDoctorEnv(t)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	workspaceDir := filepath.Join(t.TempDir(), "doctor-workspace")
	var initStdout strings.Builder
	initCmd := newRootCommand(strings.NewReader(""), &initStdout, io.Discard)
	initCmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
	}

	workspaceAbs, err := filepath.Abs(workspaceDir)
	if err != nil {
		t.Fatalf("workspace abs path: %v", err)
	}
	configPath := config.FixedConfigPath()
	content := strings.TrimSpace(strings.Join([]string{
		"mode: standalone",
		"workspace_dir: " + workspaceAbs,
		"api_auth_mode: off",
		"api_allow_insecure_local_auth: true",
		"llm_provider: openai",
		"llm_auth_mode: api-key",
		"llm_base_url: https://api.openai.com/v1",
		"llm_model: gpt-4o-mini",
		"llm_api_key: ${OPENAI_API_KEY}",
		"gateway_enabled: true",
		"gateway_default_agent: worker",
		`gateway_agents_json: [{"name":"worker","command":"python3","args":["./worker_agent.py"],"working_dir":".","enabled":true}]`,
	}, "\n")) + "\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var doctorStdout strings.Builder
	doctorCmd := newRootCommand(strings.NewReader(""), &doctorStdout, io.Discard)
	doctorCmd.SetArgs([]string{"doctor", "--workspace-dir", workspaceDir})
	err = doctorCmd.Execute()
	if err == nil {
		t.Fatal("expected doctor to fail when gateway default agent points to a missing workspace command")
	}

	out := doctorStdout.String()
	if !strings.Contains(out, "gateway agents") {
		t.Fatalf("expected gateway agents failure, got:\n%s", out)
	}
	if !strings.Contains(out, "worker_agent.py") {
		t.Fatalf("expected missing worker agent detail, got:\n%s", out)
	}
}

func TestRootCommand_DoctorFailsWhenSemanticMemoryProviderIsUnsupported(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	clearDoctorEnv(t)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	workspaceDir := filepath.Join(t.TempDir(), "doctor-workspace")
	var initStdout strings.Builder
	initCmd := newRootCommand(strings.NewReader(""), &initStdout, io.Discard)
	initCmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
	}

	configPath := config.FixedConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	configText := strings.TrimSpace(string(data)) + "\n" + strings.Join([]string{
		"memory_semantic_enabled: true",
		"memory_embed_provider: openai",
		"memory_embed_base_url: https://api.openai.com/v1",
		"memory_embed_api_key: test-embed-key",
		"memory_embed_model: text-embedding-3-small",
		"memory_embed_dimensions: 1536",
	}, "\n") + "\n"
	if err := os.WriteFile(configPath, []byte(configText), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var doctorStdout strings.Builder
	doctorCmd := newRootCommand(strings.NewReader(""), &doctorStdout, io.Discard)
	doctorCmd.SetArgs([]string{"doctor", "--workspace-dir", workspaceDir})
	err = doctorCmd.Execute()
	if err == nil {
		t.Fatal("expected doctor to fail for unsupported semantic memory provider")
	}

	out := doctorStdout.String()
	if !strings.Contains(out, "[fail] semantic memory") {
		t.Fatalf("expected semantic memory failure, got:\n%s", out)
	}
	if !strings.Contains(out, "supported providers: gemini") {
		t.Fatalf("expected supported provider guidance, got:\n%s", out)
	}
}

func clearDoctorEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
		"GEMINI_API_KEY",
		"OPENAI_CODEX_OAUTH_TOKEN",
		"TARS_OPENAI_CODEX_OAUTH_TOKEN",
		"LLM_API_KEY",
		"TARS_LLM_API_KEY",
		"CLAUDE_CODE_CLI_PATH",
		"TARS_PLUGINS_BUNDLED_DIR",
		"TARS_WORKSPACE_DIR",
		"TARS_CONFIG",
		"TARS_CONFIG_PATH",
	} {
		t.Setenv(key, "")
	}
}
