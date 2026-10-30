package localembed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
)

const testModel = "kakaocorp/kanana-2-1.3b-embedding"

// TestMain lets the test binary stand in for the Python server: Start runs
// m.python, which tests point at os.Args[0] with LOCALEMBED_HELPER set.
func TestMain(m *testing.M) {
	switch os.Getenv("LOCALEMBED_HELPER") {
	case "serve":
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "model": os.Getenv("TARS_EMBED_MODEL")})
		})
		addr := net.JoinHostPort(os.Getenv("TARS_EMBED_HOST"), os.Getenv("TARS_EMBED_PORT"))
		_ = http.ListenAndServe(addr, mux)
		os.Exit(0)
	case "crash":
		fmt.Println("boom")
		os.Exit(3)
	}
	os.Exit(m.Run())
}

func freeBaseURL(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return "http://" + addr + "/v1"
}

func newTestManager(t *testing.T, baseURL string, run Runner) *Manager {
	t.Helper()
	m, err := New(Options{Dir: t.TempDir(), Model: testModel, BaseURL: baseURL, Runner: run})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	m.python = os.Args[0]
	return m
}

func markInstalled(t *testing.T, m *Manager) {
	t.Helper()
	if err := os.MkdirAll(m.venvDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.markerPath(), []byte(requirementsKey()), 0o644); err != nil {
		t.Fatal(err)
	}
	writeWeights(t, m)
}

func writeWeights(t *testing.T, m *Manager) {
	t.Helper()
	snap := filepath.Join(m.modelDir(), "snapshots", "abc")
	if err := os.MkdirAll(snap, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap, "model.safetensors"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNew_RejectsUnsafeOptions(t *testing.T) {
	for name, opts := range map[string]Options{
		"no dir":      {Model: testModel, BaseURL: "http://127.0.0.1:43181/v1"},
		"bad model":   {Dir: "x", Model: "../etc", BaseURL: "http://127.0.0.1:43181/v1"},
		"no org":      {Dir: "x", Model: "kanana", BaseURL: "http://127.0.0.1:43181/v1"},
		"remote host": {Dir: "x", Model: testModel, BaseURL: "http://10.0.0.5:43181/v1"},
		"https":       {Dir: "x", Model: testModel, BaseURL: "https://127.0.0.1:43181/v1"},
		"no port":     {Dir: "x", Model: testModel, BaseURL: "http://127.0.0.1/v1"},
		"bad port":    {Dir: "x", Model: testModel, BaseURL: "http://127.0.0.1:99999/v1"},
		"unparseable": {Dir: "x", Model: testModel, BaseURL: "::"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(opts); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	if _, err := New(Options{Dir: "x", Model: testModel, BaseURL: "http://localhost:43181/v1"}); err != nil {
		t.Fatalf("expected localhost to be accepted, got %v", err)
	}
}

func TestStatus_ReportsInstalledParts(t *testing.T) {
	m := newTestManager(t, freeBaseURL(t), nil)
	st := m.Status(context.Background())
	if st.EnvReady || st.ModelInstalled || st.Running || st.ModelBytes != 0 {
		t.Fatalf("expected empty status, got %+v", st)
	}
	markInstalled(t, m)
	st = m.Status(context.Background())
	if !st.EnvReady || !st.ModelInstalled || st.ModelBytes != 5 || st.Model != testModel {
		t.Fatalf("expected installed status, got %+v", st)
	}
}

func TestStatus_StaleRequirementsMeanEnvNotReady(t *testing.T) {
	m := newTestManager(t, freeBaseURL(t), nil)
	markInstalled(t, m)
	if err := os.WriteFile(m.markerPath(), []byte("3.11 torch==1.0"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m.Status(context.Background()).EnvReady {
		t.Fatal("expected a changed requirement list to need a reinstall")
	}
}

type call struct {
	name string
	args []string
	env  []string
}

func TestInstall_BuildsEnvWritesAssetsAndDownloads(t *testing.T) {
	var calls []call
	var m *Manager
	run := func(_ context.Context, name string, args, env []string, out io.Writer) error {
		calls = append(calls, call{name, args, env})
		if name == m.pythonPath() {
			writeWeights(t, m)
		}
		_, _ = io.WriteString(out, "ok\n")
		return nil
	}
	m = newTestManager(t, freeBaseURL(t), run)
	uv := filepath.Join(t.TempDir(), "uv")
	if err := os.WriteFile(uv, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	m.uvPath = uv

	var out strings.Builder
	if err := m.Install(context.Background(), &out); err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(calls) != 3 || calls[0].name != uv || calls[0].args[0] != "venv" || calls[1].args[0] != "pip" || calls[2].name != m.pythonPath() {
		t.Fatalf("unexpected install steps %+v", calls)
	}
	if !strings.Contains(strings.Join(calls[1].args, " "), "sentence-transformers==") {
		t.Fatalf("expected pinned requirements, got %v", calls[1].args)
	}
	if !strings.Contains(strings.Join(calls[2].env, " "), "HF_HOME="+m.hfHome()) || calls[2].args[len(calls[2].args)-1] != testModel {
		t.Fatalf("expected download into the private HF_HOME, got %+v", calls[2])
	}
	for _, name := range []string{"server.py", "download.py"} {
		if !isFile(filepath.Join(m.appDir(), name)) {
			t.Fatalf("expected %s to be written", name)
		}
	}
	if !m.envReady() || !strings.Contains(out.String(), "downloading") {
		t.Fatalf("expected env marker and progress output, got %q", out.String())
	}

	leftover := filepath.Join(m.modelDir(), "blobs", "abc.incomplete")
	if err := os.MkdirAll(filepath.Dir(leftover), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(leftover, []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A second install skips the environment and only re-runs the download.
	calls = nil
	if err := m.Install(context.Background(), nil); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if len(calls) != 1 || calls[0].name != m.pythonPath() {
		t.Fatalf("expected only the download step, got %+v", calls)
	}
	if isFile(leftover) {
		t.Fatal("expected the interrupted download's leftover removed")
	}
}

func TestInstall_Errors(t *testing.T) {
	m := newTestManager(t, freeBaseURL(t), func(context.Context, string, []string, []string, io.Writer) error { return nil })
	m.uvPath = filepath.Join(t.TempDir(), "missing-uv")
	if err := m.Install(context.Background(), nil); !errors.Is(err, ErrUVMissing) {
		t.Fatalf("expected ErrUVMissing, got %v", err)
	}

	markInstalled(t, m)
	if err := os.RemoveAll(m.modelDir()); err != nil {
		t.Fatal(err)
	}
	if err := m.Install(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "no weights") {
		t.Fatalf("expected missing-weights error, got %v", err)
	}

	m.run = func(context.Context, string, []string, []string, io.Writer) error { return errors.New("network down") }
	if err := m.Install(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("expected download error, got %v", err)
	}
}

func TestStart_RequiresInstall(t *testing.T) {
	m := newTestManager(t, freeBaseURL(t), nil)
	if err := m.Start(context.Background()); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("expected ErrNotInstalled, got %v", err)
	}
}

func TestStart_RunsServerUntilHealthyThenStop(t *testing.T) {
	t.Setenv("LOCALEMBED_HELPER", "serve")
	m := newTestManager(t, freeBaseURL(t), nil)
	markInstalled(t, m)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !m.Status(ctx).Running {
		t.Fatal("expected running status after start")
	}
	if !isFile(m.pidPath()) {
		t.Fatal("expected pid file")
	}
	// Starting again adopts the running server.
	if err := m.Start(ctx); err != nil {
		t.Fatalf("second start: %v", err)
	}

	m.Stop()
	deadline := time.Now().Add(10 * time.Second)
	for m.Status(ctx).Running {
		if time.Now().After(deadline) {
			t.Fatal("expected server to stop")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if isFile(m.pidPath()) {
		t.Fatal("expected pid file removed")
	}
}

func TestStart_ReportsServerThatExits(t *testing.T) {
	t.Setenv("LOCALEMBED_HELPER", "crash")
	m := newTestManager(t, freeBaseURL(t), nil)
	markInstalled(t, m)
	err := m.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exited during start") {
		t.Fatalf("expected exit error, got %v", err)
	}
	if raw, _ := os.ReadFile(m.LogPath()); !strings.Contains(string(raw), "boom") {
		t.Fatalf("expected server output in log, got %q", raw)
	}
}

func TestStart_RefusesForeignServerOnPort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "model": "someone/else"})
	}))
	defer srv.Close()
	m := newTestManager(t, srv.URL+"/v1", nil)
	markInstalled(t, m)
	err := m.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "someone/else") {
		t.Fatalf("expected foreign-server error, got %v", err)
	}
	if st := m.Status(context.Background()); st.Running || st.RunningModel != "someone/else" {
		t.Fatalf("expected foreign model reported but not running, got %+v", st)
	}
}

func TestStart_CancelledContextStops(t *testing.T) {
	t.Setenv("LOCALEMBED_HELPER", "crash")
	m := newTestManager(t, freeBaseURL(t), nil)
	markInstalled(t, m)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Either the cancellation or the exit wins; both must return an error.
	if err := m.Start(ctx); err == nil {
		t.Fatal("expected error")
	}
}

func TestRemoveModelAndUninstall(t *testing.T) {
	m := newTestManager(t, freeBaseURL(t), nil)
	markInstalled(t, m)
	if err := m.RemoveModel(); err != nil {
		t.Fatalf("remove model: %v", err)
	}
	st := m.Status(context.Background())
	if st.ModelInstalled || !st.EnvReady || st.ModelBytes != 0 {
		t.Fatalf("expected model gone and environment kept, got %+v", st)
	}
	if _, err := os.Stat(m.hfHome()); !os.IsNotExist(err) {
		t.Fatalf("expected the private HF_HOME (shared blobs included) removed, got %v", err)
	}
	if err := m.Uninstall(); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(m.dir); !os.IsNotExist(err) {
		t.Fatalf("expected dir removed, got %v", err)
	}
}

func TestStopRecorded_IgnoresGarbagePidFile(t *testing.T) {
	m := newTestManager(t, freeBaseURL(t), nil)
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.pidPath(), []byte("not-a-pid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m.stopRecorded() {
		t.Fatal("expected garbage pid to be ignored")
	}
}

func TestDirForAndPythonPath(t *testing.T) {
	if got := DirFor(filepath.Join("ws")); got != filepath.Join("ws", "_shared", "local-embed") {
		t.Fatalf("unexpected dir %q", got)
	}
	m, err := New(Options{Dir: "d", Model: testModel, BaseURL: "http://127.0.0.1:43181/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(m.pythonPath(), filepath.Join("d", "venv")) {
		t.Fatalf("unexpected python path %q", m.pythonPath())
	}
}

func TestConfigHelpers(t *testing.T) {
	var cfg config.Config
	cfg.WorkspaceDir = "ws"
	cfg.MemoryEmbedProvider = "gemini"
	cfg.MemoryEmbedModel = "gemini-embedding-2-preview"
	cfg.MemorySemanticEnabled = true
	opts := OptionsForConfig(cfg)
	if opts.Model != config.DefaultLocalEmbedModel || opts.BaseURL != config.DefaultLocalEmbedBaseURL || opts.Dir != DirFor("ws") {
		t.Fatalf("expected local defaults for another provider, got %+v", opts)
	}
	if Enabled(cfg) || IsSelected(cfg) {
		t.Fatal("expected gemini not to count as local")
	}
	updates := EnableUpdates(cfg)
	if updates["memory_embed_provider"] != "local" || updates["memory_embed_dimensions"] != config.DefaultLocalEmbedDimensions {
		t.Fatalf("expected a full switch, got %v", updates)
	}

	cfg.MemoryEmbedProvider = " Local "
	cfg.MemoryEmbedModel = "org/custom"
	cfg.MemoryEmbedBaseURL = "http://127.0.0.1:9/v1"
	if opts := OptionsForConfig(cfg); opts.Model != "org/custom" || opts.BaseURL != "http://127.0.0.1:9/v1" {
		t.Fatalf("expected configured model, got %+v", opts)
	}
	if !Enabled(cfg) {
		t.Fatal("expected local to be enabled")
	}
	if updates := EnableUpdates(cfg); len(updates) != 2 {
		t.Fatalf("expected a chosen local model to be kept, got %v", updates)
	}
	if DisableUpdates()["memory_semantic_enabled"] != false {
		t.Fatal("expected disable to turn semantic memory off")
	}
}

func TestExecRunner_StreamsOutputAndReportsFailure(t *testing.T) {
	t.Setenv("LOCALEMBED_HELPER", "crash")
	var out strings.Builder
	err := ExecRunner(context.Background(), os.Args[0], nil, []string{"X=1"}, &out)
	if err == nil || !strings.Contains(out.String(), "boom") {
		t.Fatalf("expected failure with output, got %v %q", err, out.String())
	}
}

func TestFindUV_ConfiguredPath(t *testing.T) {
	m := newTestManager(t, freeBaseURL(t), nil)
	uv := filepath.Join(t.TempDir(), "uv")
	if err := os.WriteFile(uv, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	m.uvPath = uv
	if m.FindUV() != uv {
		t.Fatalf("expected configured uv, got %q", m.FindUV())
	}
	m.uvPath = ""
	t.Setenv("PATH", filepath.Dir(uv))
	if runtime.GOOS != "windows" && m.FindUV() != uv {
		t.Fatalf("expected uv from PATH, got %q", m.FindUV())
	}
}
