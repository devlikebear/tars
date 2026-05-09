package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

func TestRootCommand_ServiceInstallWritesLaunchAgentPlist(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	clearDoctorEnv(t)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")

	workspaceDir := filepath.Join(t.TempDir(), "service-workspace")
	runInitForTest(t, workspaceDir)

	restore := overrideServiceTestHooks(t)
	serviceRuntimeGOOS = "darwin"
	serviceExecutablePath = func() (string, error) { return "/usr/local/bin/tars", nil }

	plistPath := filepath.Join(t.TempDir(), "io.tars.server.plist")
	stdoutLog := filepath.Join(t.TempDir(), "tars-server.out.log")
	stderrLog := filepath.Join(t.TempDir(), "tars-server.err.log")

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{
		"service", "install",
		"--plist-path", plistPath,
		"--stdout-log", stdoutLog,
		"--stderr-log", stderrLog,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("service install: %v", err)
	}

	data, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	configPath := config.FixedConfigPath()
	plist := string(data)
	for _, token := range []string{
		"io.tars.server",
		"/usr/local/bin/tars",
		"serve",
		configPath,
		stdoutLog,
		stderrLog,
	} {
		if !strings.Contains(plist, token) {
			t.Fatalf("expected plist to contain %q, got:\n%s", token, plist)
		}
	}
	if !strings.Contains(stdout.String(), "service installed") {
		t.Fatalf("expected install output, got:\n%s", stdout.String())
	}

	restore()
}

func TestRootCommand_ServiceStartBootstrapsAndKickstartsLaunchAgent(t *testing.T) {
	restore := overrideServiceTestHooks(t)
	serviceRuntimeGOOS = "darwin"

	plistPath := filepath.Join(t.TempDir(), "io.tars.server.plist")
	if err := os.WriteFile(plistPath, []byte("<plist/>"), 0o644); err != nil {
		t.Fatalf("write plist: %v", err)
	}

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	writeBrokenFixedConfig(t)

	var calls [][]string
	serviceLaunchctlRun = func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, append([]string{}, args...))
		return "", nil
	}

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{
		"service", "start",
		"--label", "io.tars.server",
		"--domain", "gui/501",
		"--plist-path", plistPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("service start: %v", err)
	}

	expected := [][]string{
		{"bootout", "gui/501", plistPath},
		{"bootstrap", "gui/501", plistPath},
		{"kickstart", "-k", "gui/501/io.tars.server"},
	}
	if len(calls) != len(expected) {
		t.Fatalf("expected %d launchctl calls, got %d: %#v", len(expected), len(calls), calls)
	}
	for i := range expected {
		if strings.Join(calls[i], " ") != strings.Join(expected[i], " ") {
			t.Fatalf("unexpected launchctl call %d: got %#v want %#v", i, calls[i], expected[i])
		}
	}
	if !strings.Contains(stdout.String(), "service started") {
		t.Fatalf("expected start output, got:\n%s", stdout.String())
	}

	restore()
}

func TestRootCommand_ServiceStopBootsOutLaunchAgent(t *testing.T) {
	restore := overrideServiceTestHooks(t)
	serviceRuntimeGOOS = "darwin"

	plistPath := filepath.Join(t.TempDir(), "io.tars.server.plist")
	if err := os.WriteFile(plistPath, []byte("<plist/>"), 0o644); err != nil {
		t.Fatalf("write plist: %v", err)
	}

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	writeBrokenFixedConfig(t)

	var calls [][]string
	serviceLaunchctlRun = func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, append([]string{}, args...))
		return "", nil
	}

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{
		"service", "stop",
		"--label", "io.tars.server",
		"--domain", "gui/501",
		"--plist-path", plistPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("service stop: %v", err)
	}

	if len(calls) != 1 || strings.Join(calls[0], " ") != strings.Join([]string{"bootout", "gui/501", plistPath}, " ") {
		t.Fatalf("unexpected launchctl calls: %#v", calls)
	}
	if !strings.Contains(stdout.String(), "service stopped") {
		t.Fatalf("expected stop output, got:\n%s", stdout.String())
	}

	restore()
}

func TestRunLaunchctlUsesSystemLaunchctl(t *testing.T) {
	_, err := runLaunchctl(context.Background(), "__tars_test_invalid__")
	if err == nil {
		t.Fatal("expected invalid launchctl invocation to fail")
	}
}

func TestRootCommand_ServiceStatusReportsInstalledButNotLoaded(t *testing.T) {
	restore := overrideServiceTestHooks(t)
	serviceRuntimeGOOS = "darwin"

	plistPath := filepath.Join(t.TempDir(), "io.tars.server.plist")
	if err := os.WriteFile(plistPath, []byte("<plist/>"), 0o644); err != nil {
		t.Fatalf("write plist: %v", err)
	}

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	writeBrokenFixedConfig(t)

	serviceLaunchctlRun = func(_ context.Context, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "print" {
			return "Could not find service", errors.New("exit status 113")
		}
		return "", nil
	}

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{
		"service", "status",
		"--label", "io.tars.server",
		"--domain", "gui/501",
		"--plist-path", plistPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("service status: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "installed: yes") {
		t.Fatalf("expected installed=yes, got:\n%s", out)
	}
	if !strings.Contains(out, "loaded: no") {
		t.Fatalf("expected loaded=no, got:\n%s", out)
	}

	restore()
}

func TestRootCommand_ServiceInstall_AllowNeedsSetupBypassesLLMDoctor(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	clearDoctorEnv(t)

	// Run init WITHOUT appending the LLM block — config stays in
	// setup-only state. Without --allow-needs-setup the install must
	// fail; with it, install must succeed and bake --api-addr.
	bundledPluginsDir := writeBundledPluginSource(t)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", bundledPluginsDir)
	workspaceDir := filepath.Join(t.TempDir(), "ws")
	var initOut strings.Builder
	initCmd := newRootCommand(strings.NewReader(""), &initOut, io.Discard)
	initCmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--no-server", "--no-browser"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}

	restore := overrideServiceTestHooks(t)
	defer restore()
	serviceRuntimeGOOS = "darwin"
	serviceExecutablePath = func() (string, error) { return "/usr/local/bin/tars", nil }

	plistPath := filepath.Join(t.TempDir(), "io.tars.server.plist")

	// First attempt — no allow flag, should fail at LLM check.
	failCmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	failCmd.SetArgs([]string{
		"service", "install",
		"--plist-path", plistPath,
	})
	if err := failCmd.Execute(); err == nil {
		t.Fatal("expected service install to fail without --allow-needs-setup when config is wizard-pending")
	}

	// Second attempt — with --allow-needs-setup and --api-addr, must succeed.
	var ok strings.Builder
	okCmd := newRootCommand(strings.NewReader(""), &ok, io.Discard)
	okCmd.SetArgs([]string{
		"service", "install",
		"--plist-path", plistPath,
		"--allow-needs-setup",
		"--api-addr", "127.0.0.1:43185",
	})
	if err := okCmd.Execute(); err != nil {
		t.Fatalf("service install --allow-needs-setup: %v", err)
	}
	data, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	plist := string(data)
	for _, token := range []string{"--api-addr", "127.0.0.1:43185"} {
		if !strings.Contains(plist, token) {
			t.Fatalf("expected plist to bake %q, got:\n%s", token, plist)
		}
	}
}

func TestRootCommand_ServiceInstallWritesLaunchdIdentityEnvironment(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	clearDoctorEnv(t)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")

	workspaceDir := filepath.Join(t.TempDir(), "service-workspace")
	runInitForTest(t, workspaceDir)

	restore := overrideServiceTestHooks(t)
	serviceRuntimeGOOS = "darwin"
	serviceExecutablePath = func() (string, error) { return "/usr/local/bin/tars", nil }

	plistPath := filepath.Join(t.TempDir(), "io.custom.tars.plist")

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{
		"service", "install",
		"--label", "io.custom.tars",
		"--domain", "gui/777",
		"--plist-path", plistPath,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("service install: %v", err)
	}

	data, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	plist := string(data)
	for _, token := range []string{
		"io.custom.tars",
		"TARS_LAUNCHD_LABEL",
		"TARS_LAUNCHD_DOMAIN",
		"gui/777",
	} {
		if !strings.Contains(plist, token) {
			t.Fatalf("expected plist to contain %q, got:\n%s", token, plist)
		}
	}

	restore()
}

func runInitForTest(t *testing.T, workspaceDir string) {
	t.Helper()
	bundledPluginsDir := writeBundledPluginSource(t)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", bundledPluginsDir)

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	// --no-server keeps init's orchestrator from trying to spawn an
	// actual server; service tests that need a running server stub
	// the launchd hooks themselves.
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--no-server", "--no-browser"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
	}
	// init writes a wizard-driven skeleton (no LLM section). Service
	// install tests below need a config doctor would consider healthy,
	// so append the minimal LLM block users would normally save through
	// the wizard.
	appendTestLLMConfig(t)
}

// appendTestLLMConfig writes a complete llm_providers + llm_tiers block
// to the fixed config so doctor's LLM checks pass during service tests.
// Mirrors what the onboarding wizard PATCHes after the user submits.
func appendTestLLMConfig(t *testing.T) {
	t.Helper()
	llmYAML := `
llm_providers:
  default:
    kind: openai
    auth_mode: api-key
    base_url: https://api.openai.com/v1
    api_key: ${OPENAI_API_KEY}
llm_tiers:
  heavy:
    provider: default
    model: gpt-4o-mini
  standard:
    provider: default
    model: gpt-4o-mini
  light:
    provider: default
    model: gpt-4o-mini
llm_default_tier: standard
`
	cfgPath := config.FixedConfigPath()
	f, err := os.OpenFile(cfgPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open config for append: %v", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(llmYAML); err != nil {
		t.Fatalf("append llm yaml: %v", err)
	}
}

func writeBrokenFixedConfig(t *testing.T) {
	t.Helper()
	fixedCfg := config.FixedConfigPath()
	if err := os.MkdirAll(filepath.Dir(fixedCfg), 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(fixedCfg, []byte("runtime:\n  workspace_dir: [broken\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func overrideServiceTestHooks(t *testing.T) func() {
	t.Helper()
	originalGOOS := serviceRuntimeGOOS
	originalExecutable := serviceExecutablePath
	originalUserHome := serviceUserHomeDir
	originalGetuid := serviceGetuid
	originalLaunchctl := serviceLaunchctlRun
	return func() {
		serviceRuntimeGOOS = originalGOOS
		serviceExecutablePath = originalExecutable
		serviceUserHomeDir = originalUserHome
		serviceGetuid = originalGetuid
		serviceLaunchctlRun = originalLaunchctl
	}
}
