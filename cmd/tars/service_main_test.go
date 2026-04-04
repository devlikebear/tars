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
	fixedCfg := config.FixedConfigPath()
	if err := os.MkdirAll(filepath.Dir(fixedCfg), 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(fixedCfg, []byte("mode: standalone\nworkspace_dir: /tmp/ws\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

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
	fixedCfg := config.FixedConfigPath()
	if err := os.MkdirAll(filepath.Dir(fixedCfg), 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(fixedCfg, []byte("mode: standalone\nworkspace_dir: /tmp/ws\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

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

func TestRootCommand_ServiceStatusReportsInstalledButNotLoaded(t *testing.T) {
	restore := overrideServiceTestHooks(t)
	serviceRuntimeGOOS = "darwin"

	plistPath := filepath.Join(t.TempDir(), "io.tars.server.plist")
	if err := os.WriteFile(plistPath, []byte("<plist/>"), 0o644); err != nil {
		t.Fatalf("write plist: %v", err)
	}

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	fixedCfg := config.FixedConfigPath()
	if err := os.MkdirAll(filepath.Dir(fixedCfg), 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(fixedCfg, []byte("mode: standalone\nworkspace_dir: /tmp/ws\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

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

func runInitForTest(t *testing.T, workspaceDir string) {
	t.Helper()
	bundledPluginsDir := writeBundledPluginSource(t)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", bundledPluginsDir)

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
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
