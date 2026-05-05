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
