package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/buildinfo"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/launchagent"
)

func writeServicePlist(t *testing.T, binary string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "io.tars.server.plist")
	content := launchagent.BuildPlist(launchagent.Config{
		Label:            launchagent.DefaultServerLabel,
		DefaultLabel:     launchagent.DefaultServerLabel,
		ProgramArguments: []string{binary, "serve", "--config", "/x/config.yaml"},
	})
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func overrideServiceBinaryVersion(t *testing.T, versions map[string]string) {
	t.Helper()
	original := serviceBinaryVersion
	t.Cleanup(func() { serviceBinaryVersion = original })
	serviceBinaryVersion = func(path string) (string, error) {
		if v, ok := versions[path]; ok {
			return v, nil
		}
		return "", errors.New("unknown binary")
	}
}

func TestInspectServiceBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("LaunchAgents are macOS only; symlinks need privileges on Windows")
	}
	restore := overrideServiceTestHooks(t)
	defer restore()
	originalVersion := buildinfo.Version
	t.Cleanup(func() { buildinfo.Version = originalVersion })
	buildinfo.Version = "0.42.2"

	dir := t.TempDir()
	current := writeExecutable(t, dir, "tars-new")
	old := writeExecutable(t, dir, "tars-old")
	serviceExecutablePath = func() (string, error) { return current, nil }
	overrideServiceBinaryVersion(t, map[string]string{old: "0.37.1"})

	// No plist: nothing installed, nothing to say.
	state, err := inspectServiceBinary(filepath.Join(dir, "missing.plist"))
	if err != nil || state.Installed || state.Warning() != "" {
		t.Fatalf("no plist = %+v, %v", state, err)
	}

	// The plist runs this tars, through a symlink as Homebrew links it.
	link := filepath.Join(dir, "bin-tars")
	if err := os.Symlink(current, link); err != nil {
		t.Fatal(err)
	}
	state, err = inspectServiceBinary(writeServicePlist(t, link))
	if err != nil || !state.Installed || !state.SameBinary || state.Warning() != "" {
		t.Fatalf("same binary = %+v, %v", state, err)
	}

	// The plist runs an older tars elsewhere: the reported case.
	state, err = inspectServiceBinary(writeServicePlist(t, old))
	if err != nil {
		t.Fatal(err)
	}
	warning := state.Warning()
	for _, want := range []string{old, "0.37.1", current, "0.42.2", "tars service install"} {
		if !strings.Contains(warning, want) {
			t.Fatalf("warning %q misses %q", warning, want)
		}
	}

	// Another copy of the same version is fine.
	overrideServiceBinaryVersion(t, map[string]string{old: "0.42.2"})
	state, _ = inspectServiceBinary(writeServicePlist(t, old))
	if state.Warning() != "" {
		t.Fatalf("same version elsewhere warned: %q", state.Warning())
	}

	// The plist's binary is gone.
	gone := filepath.Join(dir, "deleted-tars")
	state, _ = inspectServiceBinary(writeServicePlist(t, gone))
	if !state.Missing || !strings.Contains(state.Warning(), "no longer exists") {
		t.Fatalf("missing binary = %+v, warning %q", state, state.Warning())
	}

	// A plist this tars did not write is not compared.
	foreign := filepath.Join(dir, "foreign.plist")
	if err := os.WriteFile(foreign, []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	state, err = inspectServiceBinary(foreign)
	if err != nil || state.Installed {
		t.Fatalf("foreign plist = %+v, %v", state, err)
	}
}

func TestServiceStartWarnsWhenThePlistRunsAnotherTars(t *testing.T) {
	restore := overrideServiceTestHooks(t)
	defer restore()
	serviceRuntimeGOOS = "darwin"
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	current := writeExecutable(t, dir, "tars-new")
	old := writeExecutable(t, dir, "tars-old")
	serviceExecutablePath = func() (string, error) { return current, nil }
	overrideServiceBinaryVersion(t, map[string]string{old: "0.37.1"})
	plistPath := writeServicePlist(t, old)
	serviceLaunchctlRun = func(context.Context, ...string) (string, error) { return "", nil }

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"service", "start", "--plist-path", plistPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("service start: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "warning: the service runs "+old+" 0.37.1") || !strings.Contains(out, "service started") {
		t.Fatalf("output:\n%s", out)
	}
}

// The desktop app's Start server on a machine where `tars init` never ran:
// no config, no plist. It used to fail with "service plist not found".
func TestServiceStartInstallIfMissingSetsUpAFreshMachine(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	clearDoctorEnv(t)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	restore := overrideServiceTestHooks(t)
	defer restore()
	serviceRuntimeGOOS = "darwin"
	serviceUserHomeDir = func() (string, error) { return fakeHome, nil }
	current := writeExecutable(t, t.TempDir(), "tars")
	serviceExecutablePath = func() (string, error) { return current, nil }
	var calls []string
	serviceLaunchctlRun = func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		return "", nil
	}

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"service", "start", "--install-if-missing", "--api-addr", "127.0.0.1:43181"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("service start --install-if-missing: %v\n%s", err, stdout.String())
	}

	cfg, err := config.Load(config.FixedConfigPath())
	if err != nil {
		t.Fatalf("starter config: %v", err)
	}
	if cfg.APIAuthMode != "off" || !config.NeedsSetup(cfg) {
		t.Fatalf("starter config auth=%q needsSetup=%v", cfg.APIAuthMode, config.NeedsSetup(cfg))
	}
	plistPath := filepath.Join(fakeHome, "Library", "LaunchAgents", "io.tars.server.plist")
	data, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("plist: %v", err)
	}
	args, _ := launchagent.ProgramArgumentsFromPlist(data)
	if strings.Join(args, " ") != current+" serve --config "+config.FixedConfigPath()+" --api-addr 127.0.0.1:43181" {
		t.Fatalf("plist args = %v", args)
	}
	if len(calls) == 0 || !strings.HasPrefix(calls[len(calls)-1], "kickstart -k") {
		t.Fatalf("launchctl calls = %v", calls)
	}

	// Run again: everything exists, so it only starts. The config the
	// wizard has since filled in is never replaced.
	if err := os.WriteFile(config.FixedConfigPath(), []byte("api:\n  auth_mode: required\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	cmd = newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"service", "start", "--install-if-missing"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("second start: %v", err)
	}
	if strings.Contains(stdout.String(), "service installed") {
		t.Fatalf("second start reinstalled:\n%s", stdout.String())
	}
	if data, _ := os.ReadFile(config.FixedConfigPath()); !strings.Contains(string(data), "required") {
		t.Fatalf("config replaced:\n%s", data)
	}
}

func TestDoctorWarnsWhenTheServiceRunsAnotherTars(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	restore := overrideServiceTestHooks(t)
	defer restore()
	serviceRuntimeGOOS = "darwin"
	serviceUserHomeDir = func() (string, error) { return fakeHome, nil }

	dir := t.TempDir()
	current := writeExecutable(t, dir, "tars-new")
	old := writeExecutable(t, dir, "tars-old")
	serviceExecutablePath = func() (string, error) { return current, nil }
	overrideServiceBinaryVersion(t, map[string]string{old: "0.37.1"})
	plistDir := filepath.Join(fakeHome, "Library", "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(writeServicePlist(t, old))
	if err := os.WriteFile(filepath.Join(plistDir, "io.tars.server.plist"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	report := doctorReport{}
	checkDoctorServiceBinary(&report)
	if len(report.checks) != 1 || report.checks[0].status != "warn" || !strings.Contains(report.checks[0].detail, "0.37.1") {
		t.Fatalf("report = %+v", report)
	}
	if report.failureCount() != 0 {
		t.Fatal("a stale service binary is a warning, not a failure")
	}
}

func TestServiceBinaryVersionReadsTarsVersionOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script stand-ins need a POSIX shell")
	}
	dir := t.TempDir()
	good := filepath.Join(dir, "tars")
	if err := os.WriteFile(good, []byte("#!/bin/sh\necho 'tars 0.37.1 (9e7d0e8, 2026-09-09T00:26:07Z)'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if v, err := serviceBinaryVersion(good); err != nil || v != "0.37.1" {
		t.Fatalf("version = %q, %v", v, err)
	}
	other := filepath.Join(dir, "other")
	if err := os.WriteFile(other, []byte("#!/bin/sh\necho 'something else'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := serviceBinaryVersion(other); err == nil {
		t.Fatal("output that is not tars --version must be an error")
	}
	failing := filepath.Join(dir, "failing")
	if err := os.WriteFile(failing, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := serviceBinaryVersion(failing); err == nil {
		t.Fatal("a failing binary must be an error")
	}
}

// The plist names a tars that was uninstalled: launchd would fail to start
// it, so --install-if-missing reinstalls the plist for this tars and keeps
// the existing config.
func TestServiceStartInstallIfMissingReplacesAPlistWhoseBinaryIsGone(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	clearDoctorEnv(t)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))
	runInitForTest(t, filepath.Join(t.TempDir(), "ws"))
	configBefore, err := os.ReadFile(config.FixedConfigPath())
	if err != nil {
		t.Fatal(err)
	}

	restore := overrideServiceTestHooks(t)
	defer restore()
	serviceRuntimeGOOS = "darwin"
	serviceUserHomeDir = func() (string, error) { return fakeHome, nil }
	current := writeExecutable(t, t.TempDir(), "tars")
	serviceExecutablePath = func() (string, error) { return current, nil }
	serviceLaunchctlRun = func(context.Context, ...string) (string, error) { return "", nil }

	plistDir := filepath.Join(fakeHome, "Library", "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		t.Fatal(err)
	}
	plistPath := filepath.Join(plistDir, "io.tars.server.plist")
	gone := filepath.Join(t.TempDir(), "uninstalled-tars")
	data, _ := os.ReadFile(writeServicePlist(t, gone))
	if err := os.WriteFile(plistPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"service", "start", "--install-if-missing"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("start: %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "no longer exists; reinstalling") {
		t.Fatalf("output:\n%s", stdout.String())
	}
	data, _ = os.ReadFile(plistPath)
	if args, _ := launchagent.ProgramArgumentsFromPlist(data); len(args) == 0 || args[0] != current {
		t.Fatalf("plist args = %v, want %s first", args, current)
	}
	if after, _ := os.ReadFile(config.FixedConfigPath()); string(after) != string(configBefore) {
		t.Fatal("an existing config must be kept")
	}
}

func TestDoctorReportsTheServiceBinaryWhenItIsThisTars(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	restore := overrideServiceTestHooks(t)
	defer restore()
	serviceRuntimeGOOS = "darwin"
	serviceUserHomeDir = func() (string, error) { return fakeHome, nil }
	current := writeExecutable(t, t.TempDir(), "tars")
	serviceExecutablePath = func() (string, error) { return current, nil }

	report := doctorReport{}
	checkDoctorServiceBinary(&report)
	if len(report.checks) != 0 {
		t.Fatalf("no plist must add nothing: %+v", report.checks)
	}

	plistDir := filepath.Join(fakeHome, "Library", "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(writeServicePlist(t, current))
	if err := os.WriteFile(filepath.Join(plistDir, "io.tars.server.plist"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	checkDoctorServiceBinary(&report)
	if len(report.checks) != 1 || report.checks[0].status != "ok" || report.checks[0].detail != current {
		t.Fatalf("report = %+v", report.checks)
	}

	serviceRuntimeGOOS = "linux"
	report = doctorReport{}
	checkDoctorServiceBinary(&report)
	if len(report.checks) != 0 {
		t.Fatal("LaunchAgents are macOS only")
	}
}
