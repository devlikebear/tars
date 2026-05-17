package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	assertPathExists(t, filepath.Join(workspaceAbs, "memory", "raw"))
	assertPathNotExists(t, filepath.Join(workspaceAbs, "memory", "wiki"))
	assertPathExists(t, filepath.Join(workspaceAbs, "MEMORY.md"))
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
	initCmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--no-server", "--no-browser"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
	}
	// init writes a wizard-driven skeleton; mirror what the wizard
	// PATCHes so doctor sees a complete LLM configuration.
	appendTestLLMConfig(t)

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
	initCmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--no-server", "--no-browser"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
	}
	appendTestLLMConfig(t)

	workspaceAbs, err := filepath.Abs(workspaceDir)
	if err != nil {
		t.Fatalf("workspace abs path: %v", err)
	}
	manifestPath := filepath.Join(workspaceAbs, "plugins", "ops-service", "tars.plugin.json")
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
	initCmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--no-server", "--no-browser"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
	}

	// Rewrite the starter config to use claude-code-cli as the pool kind
	// so that the doctor's runtime check looks for the local claude
	// binary. The full content is rewritten rather than text-patched to
	// avoid brittleness on future template changes.
	configPath := config.FixedConfigPath()
	claudeConfig := `mode: standalone
workspace_dir: ` + workspaceDir + `
api_auth_mode: off
api_allow_insecure_local_auth: true
llm_providers:
  default:
    kind: claude-code-cli
    auth_mode: cli
llm_tiers:
  heavy:
    provider: default
    model: sonnet
  standard:
    provider: default
    model: sonnet
  light:
    provider: default
    model: sonnet
llm_default_tier: standard
agentruntime_enabled: true
`
	if err := os.WriteFile(configPath, []byte(claudeConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var doctorStdout strings.Builder
	doctorCmd := newRootCommand(strings.NewReader(""), &doctorStdout, io.Discard)
	doctorCmd.SetArgs([]string{"doctor", "--workspace-dir", workspaceDir})
	if err := doctorCmd.Execute(); err == nil {
		t.Fatal("expected doctor to fail when claude cli is missing")
	}

	out := doctorStdout.String()
	if !strings.Contains(out, "claude") {
		t.Fatalf("expected claude cli guidance, got:\n%s", out)
	}
}

func TestRootCommand_DoctorFailsWhenAgentRuntimeDefaultAgentUsesMissingWorkspaceCommand(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	clearDoctorEnv(t)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	workspaceDir := filepath.Join(t.TempDir(), "doctor-workspace")
	var initStdout strings.Builder
	initCmd := newRootCommand(strings.NewReader(""), &initStdout, io.Discard)
	initCmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--no-server", "--no-browser"})
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
		`llm_providers: { default: { kind: openai, auth_mode: api-key, base_url: "https://api.openai.com/v1", api_key: "${OPENAI_API_KEY}" } }`,
		`llm_tiers: { heavy: { provider: default, model: gpt-4o-mini }, standard: { provider: default, model: gpt-4o-mini }, light: { provider: default, model: gpt-4o-mini } }`,
		"agentruntime_enabled: true",
		"agentruntime_default_agent: worker",
		`agentruntime_agents_json: [{"name":"worker","command":"python3","args":["./worker_agent.py"],"working_dir":".","enabled":true}]`,
	}, "\n")) + "\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var doctorStdout strings.Builder
	doctorCmd := newRootCommand(strings.NewReader(""), &doctorStdout, io.Discard)
	doctorCmd.SetArgs([]string{"doctor", "--workspace-dir", workspaceDir})
	err = doctorCmd.Execute()
	if err == nil {
		t.Fatal("expected doctor to fail when agent runtime default agent points to a missing workspace command")
	}

	out := doctorStdout.String()
	if !strings.Contains(out, "agent runtime agents") {
		t.Fatalf("expected agent runtime agents failure, got:\n%s", out)
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
	initCmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--no-server", "--no-browser"})
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
		"CLAUDE_API_KEY",
	} {
		t.Setenv(key, "")
	}
}

// TestCheckDoctorLLMRuntime_ClaudeCodeCLI exercises the
// checkDoctorLLMRuntime function directly across the three branches the
// auth/cutover code introduces. We can't easily monkey-patch
// claudeCodeAgentSDKCutoverDate (it's a package-level var) so the
// before-cutover path is verified via the current date (test runs before
// 2026-06-15). The skipped runtime case (no claude-code-cli provider in
// config) is also covered to lock the early-return.
func TestCheckDoctorLLMRuntime_ClaudeCodeCLI(t *testing.T) {
	// 1) No claude-code-cli provider configured → check skipped entirely,
	// report stays empty.
	t.Run("skipped when no claude-code-cli provider", func(t *testing.T) {
		clearDoctorEnv(t)
		var r doctorReport
		checkDoctorLLMRuntime(&r, config.Config{
			LLMConfig: config.LLMConfig{
				LLMProviders: map[string]config.LLMProviderSettings{
					"default": {Kind: "anthropic"},
				},
			},
		})
		for _, c := range r.checks {
			if c.name == "llm runtime" {
				t.Fatalf("expected no llm runtime check, got %+v", c)
			}
		}
	})

	// 2) claude-code-cli configured + binary missing → fail + install hint.
	t.Run("fail when binary missing", func(t *testing.T) {
		clearDoctorEnv(t)
		// Point CLAUDE_CODE_CLI_PATH at a non-existent file so FindClaudeCodeCLIPath errors out.
		t.Setenv("CLAUDE_CODE_CLI_PATH", filepath.Join(t.TempDir(), "no-such-claude"))
		var r doctorReport
		checkDoctorLLMRuntime(&r, config.Config{
			LLMConfig: config.LLMConfig{
				LLMProviders: map[string]config.LLMProviderSettings{
					"default": {Kind: "claude-code-cli"},
				},
			},
		})
		found := false
		for _, c := range r.checks {
			if c.name == "llm runtime" && c.status == "fail" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected llm runtime FAIL, got checks=%+v", r.checks)
		}
		if len(r.hints) == 0 {
			t.Fatal("expected install hint")
		}
	})

	// 3) claude-code-cli configured + binary present + subscription mode +
	// pre-cutover → ok + auth=subscription in detail + cutover hint.
	t.Run("ok with subscription cutover hint", func(t *testing.T) {
		if time.Now().UTC().After(claudeCodeAgentSDKCutoverDate) {
			t.Skip("post-cutover; hint suppressed by design")
		}
		clearDoctorEnv(t)
		dir := t.TempDir()
		fakeClaude := filepath.Join(dir, "claude")
		if err := os.WriteFile(fakeClaude, []byte("#!/bin/sh\necho stub\n"), 0o755); err != nil {
			t.Fatalf("write fake claude: %v", err)
		}
		t.Setenv("CLAUDE_CODE_CLI_PATH", fakeClaude)
		var r doctorReport
		checkDoctorLLMRuntime(&r, config.Config{
			LLMConfig: config.LLMConfig{
				LLMProviders: map[string]config.LLMProviderSettings{
					"default": {Kind: "claude-code-cli"},
				},
			},
		})
		var runtimeCheck *doctorCheck
		for i := range r.checks {
			if r.checks[i].name == "llm runtime" {
				runtimeCheck = &r.checks[i]
			}
		}
		if runtimeCheck == nil || runtimeCheck.status != "ok" {
			t.Fatalf("expected llm runtime ok, got %+v", r.checks)
		}
		if !strings.Contains(runtimeCheck.detail, "auth=subscription") {
			t.Fatalf("expected auth=subscription in detail, got %q", runtimeCheck.detail)
		}
		hintFound := false
		for _, h := range r.hints {
			if strings.Contains(h, "2026-06-15") {
				hintFound = true
			}
		}
		if !hintFound {
			t.Fatalf("expected 2026-06-15 cutover hint, got hints=%v", r.hints)
		}
	})

	// 4) claude-code-cli + binary present + api_key mode → ok + auth=api_key
	// in detail, NO cutover hint (api_key path is unaffected by the change).
	t.Run("api_key mode suppresses cutover hint", func(t *testing.T) {
		clearDoctorEnv(t)
		t.Setenv("ANTHROPIC_API_KEY", "sk-xxx")
		dir := t.TempDir()
		fakeClaude := filepath.Join(dir, "claude")
		if err := os.WriteFile(fakeClaude, []byte("#!/bin/sh\necho stub\n"), 0o755); err != nil {
			t.Fatalf("write fake claude: %v", err)
		}
		t.Setenv("CLAUDE_CODE_CLI_PATH", fakeClaude)
		var r doctorReport
		checkDoctorLLMRuntime(&r, config.Config{
			LLMConfig: config.LLMConfig{
				LLMProviders: map[string]config.LLMProviderSettings{
					"default": {Kind: "claude-code-cli"},
				},
			},
		})
		for _, c := range r.checks {
			if c.name == "llm runtime" && c.status == "ok" {
				if !strings.Contains(c.detail, "auth=api_key") {
					t.Fatalf("expected auth=api_key in detail, got %q", c.detail)
				}
			}
		}
		for _, h := range r.hints {
			if strings.Contains(h, "2026-06-15") {
				t.Fatalf("api_key mode should not produce cutover hint, got %q", h)
			}
		}
	})
}

func TestCheckDoctorEmbodiment(t *testing.T) {
	var disabled doctorReport
	checkDoctorEmbodiment(&disabled, config.Config{})
	if len(disabled.checks) != 1 {
		t.Fatalf("checks = %+v", disabled.checks)
	}
	if disabled.checks[0].name != "embodiment" || disabled.checks[0].detail != "disabled" {
		t.Fatalf("disabled embodiment check = %+v", disabled.checks[0])
	}

	var enabled doctorReport
	checkDoctorEmbodiment(&enabled, config.Config{
		Embodiment: config.EmbodimentConfig{
			Enabled: true,
			Providers: []config.EmbodimentProviderConfig{{
				Name:    "host",
				Enabled: true,
			}, {
				Name:    "disabled",
				Enabled: false,
			}, {
				Name:    " ",
				Enabled: true,
			}, {
				Name:    "host",
				Enabled: true,
			}},
		},
	})
	if len(enabled.checks) != 1 {
		t.Fatalf("checks = %+v", enabled.checks)
	}
	if enabled.checks[0].detail != "enabled (providers: host)" {
		t.Fatalf("enabled embodiment check = %+v", enabled.checks[0])
	}
}

// TestDetectClaudeCodeAuthMode verifies the env-var-based inference:
// - both keys empty → "subscription"
// - ANTHROPIC_API_KEY set → "api_key" with that env name in the detail
// - CLAUDE_API_KEY set → "api_key" with that env name in the detail
// - ANTHROPIC_API_KEY beats CLAUDE_API_KEY when both present (declaration order)
func TestDetectClaudeCodeAuthMode(t *testing.T) {
	t.Run("subscription when both unset", func(t *testing.T) {
		clearDoctorEnv(t)
		mode, detail := detectClaudeCodeAuthMode()
		if mode != "subscription" || detail != "" {
			t.Fatalf("expected subscription/empty detail, got %q/%q", mode, detail)
		}
	})
	t.Run("api_key when ANTHROPIC_API_KEY set", func(t *testing.T) {
		clearDoctorEnv(t)
		t.Setenv("ANTHROPIC_API_KEY", "sk-xxx")
		mode, detail := detectClaudeCodeAuthMode()
		if mode != "api_key" || !strings.Contains(detail, "ANTHROPIC_API_KEY") {
			t.Fatalf("expected api_key with ANTHROPIC_API_KEY detail, got %q/%q", mode, detail)
		}
	})
	t.Run("api_key when CLAUDE_API_KEY set", func(t *testing.T) {
		clearDoctorEnv(t)
		t.Setenv("CLAUDE_API_KEY", "sk-yyy")
		mode, detail := detectClaudeCodeAuthMode()
		if mode != "api_key" || !strings.Contains(detail, "CLAUDE_API_KEY") {
			t.Fatalf("expected api_key with CLAUDE_API_KEY detail, got %q/%q", mode, detail)
		}
	})
	t.Run("subscription when api key is whitespace-only", func(t *testing.T) {
		clearDoctorEnv(t)
		t.Setenv("ANTHROPIC_API_KEY", "   ")
		mode, _ := detectClaudeCodeAuthMode()
		if mode != "subscription" {
			t.Fatalf("whitespace-only api key should not flip mode, got %q", mode)
		}
	})
}
