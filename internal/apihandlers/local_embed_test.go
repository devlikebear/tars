package apihandlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/localembed"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/rs/zerolog"
)

type embedFixture struct {
	configPath  string
	workspace   string
	handler     http.Handler
	release     chan struct{}
	failInstall error
	baseURL     string
}

func newEmbedFixture(t *testing.T, running config.Config, semantic memory.SemanticConfig, yaml string) *embedFixture {
	t.Helper()
	dir := t.TempDir()
	workspace := filepath.Join(dir, "ws")
	if running.WorkspaceDir == "" {
		running.WorkspaceDir = workspace
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("workspace_dir: "+workspace+"\n"+yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	uv := filepath.Join(dir, "uv")
	if err := os.WriteFile(uv, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	f := &embedFixture{configPath: path, workspace: workspace, release: make(chan struct{})}
	close(f.release)
	f.handler = NewLocalEmbedHandler(LocalEmbedOptions{
		ConfigPath: path,
		Running:    running,
		Semantic:   semantic,
		Logger:     zerolog.Nop(),
		NewManager: func(opts localembed.Options) (*localembed.Manager, error) {
			opts.UVPath = uv
			opts.BaseURL = "http://127.0.0.1:1/v1"
			if f.baseURL != "" {
				opts.BaseURL = f.baseURL
			}
			opts.Runner = func(ctx context.Context, name string, args, _ []string, out io.Writer) error {
				select {
				case <-f.release:
				case <-ctx.Done():
					return errors.New("signal: killed")
				}
				if f.failInstall != nil {
					return f.failInstall
				}
				_, _ = io.WriteString(out, "step "+args[0]+"\rprogress\n")
				if args[0] == "venv" {
					python := filepath.Join(opts.Dir, "venv", "bin", "python")
					if runtime.GOOS == "windows" {
						python = filepath.Join(opts.Dir, "venv", "Scripts", "python.exe")
					}
					if err := os.MkdirAll(filepath.Dir(python), 0o755); err != nil {
						return err
					}
					return os.WriteFile(python, nil, 0o755)
				}
				if filepath.Ext(name) == ".exe" || filepath.Base(name) == "python" {
					snap := filepath.Join(opts.Dir, "hf", "hub", "models--"+strings.ReplaceAll(opts.Model, "/", "--"), "snapshots", "x")
					if err := os.MkdirAll(snap, 0o755); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(snap, "model.safetensors"), []byte("w"), 0o644)
				}
				return nil
			}
			return localembed.New(opts)
		},
	})
	return f
}

func (f *embedFixture) do(t *testing.T, method, path string) (*httptest.ResponseRecorder, localEmbedState) {
	t.Helper()
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	var state localEmbedState
	if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		_ = json.Unmarshal(rec.Body.Bytes(), &state)
	}
	return rec, state
}

func (f *embedFixture) waitJob(t *testing.T) localEmbedJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, state := f.do(t, http.MethodGet, "/v1/admin/local-embed")
		if state.Job != nil && !state.Job.Running {
			return *state.Job
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return localEmbedJob{}
}

func TestLocalEmbedAPI_EnableInstallRemoveDisable(t *testing.T) {
	f := newEmbedFixture(t, config.Config{}, memory.SemanticConfig{}, "")

	rec, state := f.do(t, http.MethodGet, "/v1/admin/local-embed")
	if rec.Code != http.StatusOK || state.Enabled || state.RestartRequired || state.Status.ModelInstalled {
		t.Fatalf("unexpected initial state %d %+v", rec.Code, state)
	}

	rec, state = f.do(t, http.MethodPost, "/v1/admin/local-embed/install")
	if rec.Code != http.StatusAccepted || state.Job == nil || state.Job.Action != "install" {
		t.Fatalf("unexpected install response %d %s", rec.Code, rec.Body)
	}
	job := f.waitJob(t)
	if job.Error != "" || !strings.Contains(strings.Join(job.Log, "|"), "progress") {
		t.Fatalf("unexpected finished job %+v", job)
	}
	_, state = f.do(t, http.MethodGet, "/v1/admin/local-embed")
	if !state.Status.ModelInstalled || !state.Status.EnvReady {
		t.Fatalf("expected installed, got %+v", state)
	}

	rec, state = f.do(t, http.MethodPost, "/v1/admin/local-embed/enable")
	if rec.Code != http.StatusOK || !state.Enabled || !state.RestartRequired {
		t.Fatalf("expected enabled pending restart, got %d %+v", rec.Code, state)
	}
	cfg, err := config.Load(f.configPath)
	if err != nil || !localembed.Enabled(cfg) {
		t.Fatalf("expected config enabled, got %v %+v", err, cfg.MemoryConfig)
	}

	rec, state = f.do(t, http.MethodPost, "/v1/admin/local-embed/remove")
	if rec.Code != http.StatusOK || state.Status.ModelInstalled || !state.Status.EnvReady {
		t.Fatalf("expected model removed and env kept, got %d %+v", rec.Code, state)
	}
	rec, state = f.do(t, http.MethodPost, "/v1/admin/local-embed/remove-all")
	if rec.Code != http.StatusOK || state.Status.EnvReady {
		t.Fatalf("expected everything removed, got %d %+v", rec.Code, state)
	}
	rec, state = f.do(t, http.MethodPost, "/v1/admin/local-embed/disable")
	if rec.Code != http.StatusOK || state.Enabled || state.RestartRequired {
		t.Fatalf("expected disabled, got %d %+v", rec.Code, state)
	}
}

func TestLocalEmbedAPI_BusyAndCancel(t *testing.T) {
	f := newEmbedFixture(t, config.Config{}, memory.SemanticConfig{}, "")
	f.release = make(chan struct{}) // block the first step

	if rec, _ := f.do(t, http.MethodPost, "/v1/admin/local-embed/install"); rec.Code != http.StatusAccepted {
		t.Fatalf("install: %d", rec.Code)
	}
	if rec, _ := f.do(t, http.MethodPost, "/v1/admin/local-embed/install"); rec.Code != http.StatusConflict {
		t.Fatalf("expected busy, got %d", rec.Code)
	}
	if rec, _ := f.do(t, http.MethodPost, "/v1/admin/local-embed/remove"); rec.Code != http.StatusConflict {
		t.Fatalf("expected busy remove, got %d", rec.Code)
	}
	if rec, _ := f.do(t, http.MethodPost, "/v1/admin/local-embed/cancel"); rec.Code != http.StatusNoContent {
		t.Fatalf("cancel: %d", rec.Code)
	}
	if job := f.waitJob(t); job.Error != "cancelled" {
		t.Fatalf("expected cancelled job, got %+v", job)
	}
}

func TestLocalEmbedAPI_InstallFailureIsReported(t *testing.T) {
	f := newEmbedFixture(t, config.Config{}, memory.SemanticConfig{}, "")
	f.failInstall = errors.New("network down")
	f.do(t, http.MethodPost, "/v1/admin/local-embed/install")
	if job := f.waitJob(t); !strings.Contains(job.Error, "network down") {
		t.Fatalf("expected failure in job, got %+v", job)
	}
}

func TestLocalEmbedAPI_ReindexNeedsARunningLocalServer(t *testing.T) {
	f := newEmbedFixture(t, config.Config{}, memory.SemanticConfig{}, "")
	rec, _ := f.do(t, http.MethodPost, "/v1/admin/local-embed/reindex")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "restart") {
		t.Fatalf("expected restart conflict, got %d %s", rec.Code, rec.Body)
	}

	var running config.Config
	running.MemorySemanticEnabled = true
	running.MemoryEmbedProvider = "local"
	running.MemoryEmbedModel = config.DefaultLocalEmbedModel
	running.MemoryEmbedBaseURL = config.DefaultLocalEmbedBaseURL
	running.WorkspaceDir = t.TempDir()
	semantic := memory.SemanticConfig{
		Enabled: true, EmbedProvider: "local", EmbedBaseURL: "http://127.0.0.1:1/v1",
		EmbedModel: config.DefaultLocalEmbedModel, EmbedDimensions: 1024,
	}
	if err := memory.EnsureWorkspace(running.WorkspaceDir); err != nil {
		t.Fatal(err)
	}
	if err := memory.AppendExperience(running.WorkspaceDir, memory.Experience{Timestamp: time.Now(), Summary: "x"}); err != nil {
		t.Fatal(err)
	}
	f = newEmbedFixture(t, running, semantic, "memory_semantic_enabled: true\nmemory_embed_provider: local\n")
	_, state := f.do(t, http.MethodGet, "/v1/admin/local-embed")
	if !state.Enabled || state.RestartRequired || state.ReindexPending != 1 {
		t.Fatalf("expected one pending, got %+v", state)
	}
	rec, _ = f.do(t, http.MethodPost, "/v1/admin/local-embed/reindex")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "not running") {
		t.Fatalf("expected not-running conflict, got %d %s", rec.Code, rec.Body)
	}
}

func TestLocalEmbedAPI_RoutesAndErrors(t *testing.T) {
	f := newEmbedFixture(t, config.Config{}, memory.SemanticConfig{}, "")
	if rec, _ := f.do(t, http.MethodPost, "/v1/admin/local-embed/explode"); rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if rec, _ := f.do(t, http.MethodGet, "/v1/admin/local-embed/install"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if rec, _ := f.do(t, http.MethodPost, "/v1/admin/local-embed"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(bad, []byte("memory_embed_provider: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewLocalEmbedHandler(LocalEmbedOptions{ConfigPath: bad, Logger: zerolog.Nop()})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/local-embed", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for an unreadable config, got %d", rec.Code)
	}
	nonLoopback := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(nonLoopback, []byte("memory_embed_provider: local\nmemory_embed_base_url: http://10.0.0.1:43181/v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h = NewLocalEmbedHandler(LocalEmbedOptions{ConfigPath: nonLoopback, Logger: zerolog.Nop()})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/local-embed", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a non-loopback base_url, got %d", rec.Code)
	}
}

func TestJobWriter_KeepsLastLinesAndJoinsPartials(t *testing.T) {
	h := &localEmbedHandler{}
	job := &localEmbedJob{}
	w := &jobWriter{h: h, job: job}
	_, _ = w.Write([]byte("par"))
	_, _ = w.Write([]byte("tial\n"))
	if len(job.Log) != 1 || job.Log[0] != "partial" {
		t.Fatalf("expected joined line, got %v", job.Log)
	}
	for i := 0; i < localEmbedLogLines+5; i++ {
		_, _ = w.Write([]byte(strings.Repeat("x", 300) + "\n"))
	}
	if len(job.Log) != localEmbedLogLines || len(job.Log[0]) != 200 {
		t.Fatalf("expected a capped log of capped lines, got %d lines of %d", len(job.Log), len(job.Log[0]))
	}
}

// fakeSidecar answers like the Python server, for the model the tests use.
func fakeSidecar(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "model": config.DefaultLocalEmbedModel})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"embedding": []float64{1, 0}}}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLocalEmbedAPI_ReindexJobReportsProgress(t *testing.T) {
	srv := fakeSidecar(t)
	var running config.Config
	running.MemorySemanticEnabled = true
	running.MemoryEmbedProvider = "local"
	running.MemoryEmbedModel = config.DefaultLocalEmbedModel
	running.MemoryEmbedBaseURL = srv.URL + "/v1"
	running.WorkspaceDir = t.TempDir()
	if err := memory.EnsureWorkspace(running.WorkspaceDir); err != nil {
		t.Fatal(err)
	}
	for _, summary := range []string{"one", "two"} {
		if err := memory.AppendExperience(running.WorkspaceDir, memory.Experience{Timestamp: time.Now(), Summary: summary}); err != nil {
			t.Fatal(err)
		}
	}
	semantic := memory.SemanticConfig{
		Enabled: true, EmbedProvider: "local", EmbedBaseURL: srv.URL + "/v1",
		EmbedModel: config.DefaultLocalEmbedModel, EmbedDimensions: 2,
	}
	f := newEmbedFixture(t, running, semantic, "memory_semantic_enabled: true\nmemory_embed_provider: local\n")
	f.baseURL = srv.URL + "/v1"

	if rec, state := f.do(t, http.MethodPost, "/v1/admin/local-embed/reindex"); rec.Code != http.StatusAccepted || !state.Status.Running {
		t.Fatalf("expected the reindex to start, got %d %s", rec.Code, rec.Body)
	}
	job := f.waitJob(t)
	if job.Error != "" || job.Done != 2 || job.Total != 2 {
		t.Fatalf("unexpected reindex job %+v", job)
	}
	if _, state := f.do(t, http.MethodGet, "/v1/admin/local-embed"); state.ReindexPending != 0 {
		t.Fatalf("expected nothing pending, got %+v", state)
	}

	// Installing while on brings the sidecar up as part of the job.
	if rec, _ := f.do(t, http.MethodPost, "/v1/admin/local-embed/install"); rec.Code != http.StatusAccepted {
		t.Fatalf("install: %d", rec.Code)
	}
	if job := f.waitJob(t); job.Action != "install" || job.Error != "" {
		t.Fatalf("unexpected install job %+v", job)
	}
}
