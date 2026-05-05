package main

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

// initMockState records calls into the swappable orchestrator hooks
// so tests can assert what the orchestrator dispatched without
// actually starting a server or opening a browser.
type initMockState struct {
	startCalls    []initStartParams
	startResult   initStartResult
	startErr      error
	healthBaseURL string
	healthErr     error
	browserURL    string
	browserErr    error
}

func swapInitHooks(t *testing.T, state *initMockState) {
	t.Helper()
	prevStart := initServerStarter
	prevHealth := initHealthProber
	prevBrowser := initBrowserOpener
	prevGOOS := initRuntimeGOOS

	initServerStarter = func(_ context.Context, params initStartParams, _, _ io.Writer) (initStartResult, error) {
		state.startCalls = append(state.startCalls, params)
		return state.startResult, state.startErr
	}
	initHealthProber = func(_ context.Context, baseURL string) error {
		state.healthBaseURL = baseURL
		return state.healthErr
	}
	initBrowserOpener = func(_ context.Context, target string) error {
		state.browserURL = target
		return state.browserErr
	}
	t.Cleanup(func() {
		initServerStarter = prevStart
		initHealthProber = prevHealth
		initBrowserOpener = prevBrowser
		initRuntimeGOOS = prevGOOS
	})
}

func TestInit_OrchestratesEndToEnd(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	state := &initMockState{startResult: initStartResult{mode: "detached", pid: 1234, logPath: "/tmp/tars.log"}}
	swapInitHooks(t, state)
	initRuntimeGOOS = "linux" // forces detached spawn path

	workspaceDir := filepath.Join(t.TempDir(), "ws")
	var stdout, stderr strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, &stderr)
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--port", "43180"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}

	if len(state.startCalls) != 1 {
		t.Fatalf("expected 1 start call, got %d", len(state.startCalls))
	}
	call := state.startCalls[0]
	if call.apiAddr != "127.0.0.1:43180" {
		t.Fatalf("expected apiAddr=127.0.0.1:43180, got %q", call.apiAddr)
	}
	if call.useService {
		t.Fatalf("expected useService=false on linux, got true")
	}
	if call.configPath != config.FixedConfigPath() {
		t.Fatalf("expected configPath=%q, got %q", config.FixedConfigPath(), call.configPath)
	}

	if state.healthBaseURL != "http://127.0.0.1:43180" {
		t.Fatalf("expected healthz base http://127.0.0.1:43180, got %q", state.healthBaseURL)
	}
	if state.browserURL != "http://127.0.0.1:43180/console/" {
		t.Fatalf("expected browser url with /console/, got %q", state.browserURL)
	}

	out := stdout.String()
	if !strings.Contains(out, "server is up") {
		t.Fatalf("expected 'server is up' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "complete onboarding at http://127.0.0.1:43180/console/") {
		t.Fatalf("expected console hint, got:\n%s", out)
	}
}

func TestInit_PicksAlternatePortWhenDefaultBusy(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	state := &initMockState{startResult: initStartResult{mode: "detached"}}
	swapInitHooks(t, state)
	initRuntimeGOOS = "linux"

	// Hold the default port open so the picker has to fall back.
	listener, err := net.Listen("tcp", "127.0.0.1:43180")
	if err != nil {
		// On CI the port may already be in use; skip rather than fail since
		// in that case the picker also has to fall back, which is fine —
		// but we cannot make assertions about the chosen port without
		// binding it ourselves first.
		t.Skipf("cannot bind 127.0.0.1:43180 for fallback test: %v", err)
	}
	defer func() { _ = listener.Close() }()

	workspaceDir := filepath.Join(t.TempDir(), "ws")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}

	if len(state.startCalls) != 1 {
		t.Fatalf("expected 1 start call, got %d", len(state.startCalls))
	}
	got := state.startCalls[0].apiAddr
	if got == "127.0.0.1:43180" {
		t.Fatalf("expected fallback port, got %q", got)
	}
	if !strings.HasPrefix(got, "127.0.0.1:") {
		t.Fatalf("expected loopback addr, got %q", got)
	}
	port, _ := strconv.Atoi(strings.TrimPrefix(got, "127.0.0.1:"))
	if port < 43181 || port > 43199 {
		t.Fatalf("expected port in [43181..43199], got %d", port)
	}

	if !strings.Contains(stdout.String(), "TARS_SERVER_URL=http://"+got) {
		t.Fatalf("expected non-default port hint in output, got:\n%s", stdout.String())
	}
}

func TestInit_NoServerSkipsOrchestration(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	state := &initMockState{}
	swapInitHooks(t, state)

	workspaceDir := filepath.Join(t.TempDir(), "ws")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--no-server", "--no-browser"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}

	if len(state.startCalls) != 0 {
		t.Fatalf("expected no start calls with --no-server, got %d", len(state.startCalls))
	}
	if state.healthBaseURL != "" {
		t.Fatalf("expected no health probe, got url=%q", state.healthBaseURL)
	}
	if state.browserURL != "" {
		t.Fatalf("expected no browser open, got url=%q", state.browserURL)
	}
}

func TestInit_NoBrowserSkipsOnlyBrowser(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	state := &initMockState{startResult: initStartResult{mode: "detached"}}
	swapInitHooks(t, state)
	initRuntimeGOOS = "linux"

	workspaceDir := filepath.Join(t.TempDir(), "ws")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--no-browser", "--port", "43180"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}

	if len(state.startCalls) != 1 {
		t.Fatalf("expected start to be called, got %d", len(state.startCalls))
	}
	if state.healthBaseURL == "" {
		t.Fatalf("expected health probe to run")
	}
	if state.browserURL != "" {
		t.Fatalf("expected no browser with --no-browser, got %q", state.browserURL)
	}
}

func TestInit_ForceOverwritesExistingConfig(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	configPath := config.FixedConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("sentinel"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	state := &initMockState{startResult: initStartResult{mode: "detached"}}
	swapInitHooks(t, state)
	initRuntimeGOOS = "linux"

	workspaceDir := filepath.Join(t.TempDir(), "ws")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--force", "--no-server", "--no-browser", "--port", "43180"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --force: %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(data) == "sentinel" {
		t.Fatalf("expected sentinel config to be overwritten")
	}
	if !strings.Contains(string(data), "workspace_dir:") {
		t.Fatalf("expected fresh skeleton, got:\n%s", string(data))
	}
}

func TestInit_HealthProbeFailureSurfacesLogPath(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	state := &initMockState{
		startResult: initStartResult{mode: "detached", logPath: "/tmp/diag.log"},
		healthErr:   context.DeadlineExceeded,
	}
	swapInitHooks(t, state)
	initRuntimeGOOS = "linux"

	workspaceDir := filepath.Join(t.TempDir(), "ws")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--port", "43180"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected init to fail when healthz never succeeds")
	}
	if !strings.Contains(err.Error(), "/tmp/diag.log") {
		t.Fatalf("expected log path in error, got %v", err)
	}
}

func TestInit_AcceptsExplicitAPIAddr(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	state := &initMockState{startResult: initStartResult{mode: "detached"}}
	swapInitHooks(t, state)
	initRuntimeGOOS = "linux"

	workspaceDir := filepath.Join(t.TempDir(), "ws")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--api-addr", "127.0.0.1:55555"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if len(state.startCalls) != 1 || state.startCalls[0].apiAddr != "127.0.0.1:55555" {
		t.Fatalf("expected explicit api-addr to flow through, got %+v", state.startCalls)
	}
}

func TestInit_MigrationFlowsIntoOrchestrator(t *testing.T) {
	// Regression: a user who upgrades from a layout that put config in
	// workspace/config/ used to hit only the migration branch — the
	// orchestrator (server start, healthz, browser) was short-circuited.
	// Also asserts the workspace scaffold is SKIPPED when migrating —
	// the migrated config already references a populated workspace,
	// and re-scaffolding at the default path would either duplicate
	// the workspace or fail when the bundled plugins dir is unavailable
	// (e.g. dev binaries built outside the source tree). Pointing
	// TARS_PLUGINS_BUNDLED_DIR at a non-existent path makes that
	// failure observable.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", filepath.Join(t.TempDir(), "no-such-bundled-plugins"))

	// Stage a legacy config in CWD (the migration probe scans relative
	// candidates from the current working directory).
	legacyDir := t.TempDir()
	legacyConfigDir := filepath.Join(legacyDir, "workspace", "config")
	if err := os.MkdirAll(legacyConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	legacyConfig := filepath.Join(legacyConfigDir, "tars.config.yaml")
	if err := os.WriteFile(legacyConfig, []byte("workspace_dir: ./workspace\n"), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	wd, _ := os.Getwd()
	_ = os.Chdir(legacyDir)
	defer func() { _ = os.Chdir(wd) }()

	state := &initMockState{startResult: initStartResult{mode: "detached", logPath: "/tmp/x.log"}}
	swapInitHooks(t, state)
	initRuntimeGOOS = "linux"

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--migrate", "--port", "43180"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init after migration: %v", err)
	}

	if !strings.Contains(stdout.String(), "migrated legacy config") {
		t.Fatalf("expected migration banner, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "starting server with migrated config") {
		t.Fatalf("expected migrated-config server start banner, got:\n%s", stdout.String())
	}
	if len(state.startCalls) != 1 {
		t.Fatalf("expected orchestrator to start the server after migration, got %d calls", len(state.startCalls))
	}
	if state.startCalls[0].apiAddr != "127.0.0.1:43180" {
		t.Fatalf("expected api addr to flow through, got %q", state.startCalls[0].apiAddr)
	}
	// The starter must receive the migrated config's workspace_dir
	// (resolved from the legacy `./workspace`), not the default
	// ~/.tars/workspace passed via --workspace-dir. macOS uses
	// /private/var <-> /var symlinks under tempdirs so both paths get
	// resolved through EvalSymlinks before comparing.
	expectedWS, _ := filepath.EvalSymlinks(filepath.Join(legacyDir, "workspace"))
	gotWS, _ := filepath.EvalSymlinks(state.startCalls[0].workspaceDir)
	if expectedWS != gotWS {
		t.Fatalf("expected starter to receive migrated workspace_dir %q, got %q", expectedWS, state.startCalls[0].workspaceDir)
	}
	if state.healthBaseURL == "" {
		t.Fatalf("expected health probe to run after migration")
	}
	if state.browserURL == "" {
		t.Fatalf("expected browser to open after migration")
	}

	// The migrated payload must survive — the skeleton must not have
	// overwritten it.
	data, err := os.ReadFile(config.FixedConfigPath())
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	if !strings.Contains(string(data), "workspace_dir:") {
		t.Fatalf("expected migrated workspace_dir, got:\n%s", string(data))
	}
	if strings.Contains(string(data), "TARS skeleton config generated by") {
		t.Fatalf("migration must not overwrite with skeleton, got:\n%s", string(data))
	}
}

func TestInit_RejectsUnknownPositionalArgs(t *testing.T) {
	// Typos like `tars init relay` (not a real subcommand) used to
	// silently re-run init because cobra accepts arbitrary positional
	// args by default. With cobra.NoArgs they must error loudly.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	state := &initMockState{}
	swapInitHooks(t, state)

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "definitely-not-a-real-subcommand", "--no-server", "--no-browser"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown positional arg")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected 'unknown command' error, got: %v", err)
	}
	if len(state.startCalls) != 0 {
		t.Fatalf("expected no orchestration when args are rejected, got %d start calls", len(state.startCalls))
	}
}

func TestInit_FreshInstallTolerantOfMissingBundledPlugins(t *testing.T) {
	// Regression: dev binaries built outside a release tree (e.g.
	// `make build` in a fresh clone with no checked-in plugins/) used
	// to fail `tars init` with "bundled plugins dir not found". The
	// bundled plugins seed the workspace's plugins/ dir but the
	// system boots fine without them; treat the missing dir as a
	// soft condition for fresh installs too.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", filepath.Join(t.TempDir(), "no-such-bundled"))

	state := &initMockState{startResult: initStartResult{mode: "detached"}}
	swapInitHooks(t, state)
	initRuntimeGOOS = "linux"

	workspaceDir := filepath.Join(t.TempDir(), "ws")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--port", "43180", "--no-browser"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if len(state.startCalls) != 1 {
		t.Fatalf("expected server start despite missing bundled plugins, got %d calls", len(state.startCalls))
	}
	if _, err := os.Stat(config.FixedConfigPath()); err != nil {
		t.Fatalf("expected fresh skeleton written, got err=%v", err)
	}
}

func TestInit_DefaultDoesNotMigrateButHints(t *testing.T) {
	// Migration is opt-in via --migrate. Without the flag, init must
	// (a) NOT slurp the legacy config, (b) write a fresh wizard
	// skeleton, and (c) print a hint pointing at --migrate so the
	// user can discover the explicit path.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	legacyDir := t.TempDir()
	legacyConfigDir := filepath.Join(legacyDir, "workspace", "config")
	if err := os.MkdirAll(legacyConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	legacyConfig := filepath.Join(legacyConfigDir, "tars.config.yaml")
	if err := os.WriteFile(legacyConfig, []byte("mode: standalone\nworkspace_dir: ./workspace\n"), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	wd, _ := os.Getwd()
	_ = os.Chdir(legacyDir)
	defer func() { _ = os.Chdir(wd) }()

	state := &initMockState{startResult: initStartResult{mode: "detached"}}
	swapInitHooks(t, state)
	initRuntimeGOOS = "linux"

	workspaceDir := filepath.Join(t.TempDir(), "ws")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--port", "43180", "--no-browser"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "note: detected possible legacy config") {
		t.Fatalf("expected legacy-discovery hint, got:\n%s", out)
	}
	if !strings.Contains(out, "tars init --migrate") {
		t.Fatalf("expected --migrate hint to mention the flag, got:\n%s", out)
	}
	if strings.Contains(out, "migrated legacy config") {
		t.Fatalf("expected NO migration without --migrate, got:\n%s", out)
	}

	data, err := os.ReadFile(config.FixedConfigPath())
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	if !strings.Contains(string(data), "TARS skeleton config generated by") {
		t.Fatalf("expected fresh skeleton without --migrate, got:\n%s", string(data))
	}
	// Legacy file untouched.
	if _, err := os.Stat(legacyConfig); err != nil {
		t.Fatalf("expected legacy config preserved, got err=%v", err)
	}
}

func TestInit_MigrateWithoutLegacyErrors(t *testing.T) {
	// --migrate must error when there is no legacy config to import,
	// rather than silently falling back to the wizard skeleton.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	emptyDir := t.TempDir()
	wd, _ := os.Getwd()
	_ = os.Chdir(emptyDir)
	defer func() { _ = os.Chdir(wd) }()

	workspaceDir := filepath.Join(t.TempDir(), "ws")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--migrate", "--workspace-dir", workspaceDir, "--no-server", "--no-browser"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected --migrate to fail when no legacy config is present")
	}
	if !strings.Contains(err.Error(), "no legacy config found") {
		t.Fatalf("expected 'no legacy config found' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "workspace/config/tars.config.yaml") {
		t.Fatalf("expected error to mention scanned candidates, got: %v", err)
	}
}

func TestInit_ForceAndMigrateAreMutuallyExclusive(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	workspaceDir := filepath.Join(t.TempDir(), "ws")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir, "--force", "--migrate", "--no-server", "--no-browser"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --force and --migrate are combined")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected 'mutually exclusive' in error, got: %v", err)
	}
}

func TestOnboardCommand_RunsInitOrchestrator(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("TARS_PLUGINS_BUNDLED_DIR", writeBundledPluginSource(t))

	state := &initMockState{startResult: initStartResult{mode: "detached"}}
	swapInitHooks(t, state)
	initRuntimeGOOS = "linux"

	workspaceDir := filepath.Join(t.TempDir(), "ws")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"onboard", "--workspace-dir", workspaceDir, "--port", "43180", "--no-browser"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("onboard: %v", err)
	}
	if len(state.startCalls) != 1 {
		t.Fatalf("expected onboard to invoke init starter, got %d calls", len(state.startCalls))
	}
}
